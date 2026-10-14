package testutil

import (
	"context"
	"crypto/rand"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/netip"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/credentials"
	awss3 "github.com/aws/aws-sdk-go-v2/service/s3"
	"github.com/moby/moby/api/types/container"
	"github.com/moby/moby/api/types/network"
	dockerclient "github.com/moby/moby/client"
	"github.com/testcontainers/testcontainers-go"
	"github.com/testcontainers/testcontainers-go/wait"

	storages3 "github.com/kbukum/gokit/storage/s3"
	"github.com/kbukum/gokit/util"
)

// S3Image pins the multi-platform SeaweedFS community server used by real storage proofs.
const S3Image = "docker.io/chrislusf/seaweedfs:4.48@sha256:4e61d15fd35994cb1e43e1e553dff106794841fd9a99ade2fc8c8bfce4d7872d"

// S3Fixture owns a private authenticated single-node S3 server and its persistent test data.
type S3Fixture struct {
	mu        sync.Mutex
	container testcontainers.Container
	config    storages3.Config
}

type s3Credential struct {
	AccessKey string `json:"accessKey"`
	SecretKey string `json:"secretKey"`
}

type s3Identity struct {
	Name        string         `json:"name"`
	Credentials []s3Credential `json:"credentials"`
	Actions     []string       `json:"actions"`
}

// StartS3 requires Docker and an owned temporary directory. Provisioning is bounded at three minutes.
// Config exposes mounted credential paths; callers must never retain their contents outside the fixture.
// A non-nil fixture transfers cleanup ownership even on error; register Close before checking the error.
func StartS3(ctx context.Context, dir string) (*S3Fixture, error) {
	if util.IsNil(ctx) || dir == "" {
		return nil, errors.New("S3 fixture: context and an owned temporary directory are required")
	}
	ctx, cancel := context.WithTimeout(ctx, 3*time.Minute)
	defer cancel()
	dir, err := filepath.Abs(dir)
	if err != nil {
		return nil, err
	}
	data := filepath.Join(dir, "data")
	if mkdirErr := os.Mkdir(data, 0o700); mkdirErr != nil {
		return nil, mkdirErr
	}
	uid := os.Getuid()
	if uid == 0 {
		uid = 1000
		if chownErr := os.Chown(data, uid, uid); chownErr != nil {
			return nil, chownErr
		}
	}
	setup := s3Credential{AccessKey: rand.Text(), SecretKey: rand.Text()}
	runtime := s3Credential{AccessKey: rand.Text(), SecretKey: rand.Text()}
	bucket := "fixture-" + strings.ToLower(rand.Text())
	auth := struct {
		Identities []s3Identity `json:"identities"`
	}{Identities: []s3Identity{
		{Name: "setup", Credentials: []s3Credential{setup}, Actions: []string{"Admin", "Read", "Write", "List"}},
		{Name: "runtime", Credentials: []s3Credential{runtime}, Actions: []string{"Read:" + bucket, "Write:" + bucket, "List:" + bucket}},
	}}
	encoded, err := json.Marshal(auth)
	if err != nil {
		return nil, err
	}
	authFile := filepath.Join(dir, "s3.json")
	if writeErr := os.WriteFile(authFile, encoded, 0o400); writeErr != nil {
		return nil, writeErr
	}
	if os.Getuid() == 0 {
		if chownErr := os.Chown(authFile, uid, uid); chownErr != nil {
			return nil, chownErr
		}
	}
	accessFile, secretFile := filepath.Join(dir, "access"), filepath.Join(dir, "secret")
	for path, value := range map[string]string{accessFile: runtime.AccessKey, secretFile: runtime.SecretKey} {
		if writeErr := os.WriteFile(path, []byte(value), 0o600); writeErr != nil {
			return nil, writeErr
		}
	}
	f := &S3Fixture{}
	port, err := network.ParsePort("8333/tcp")
	if err != nil {
		return nil, err
	}
	request := testcontainers.ContainerRequest{
		Image: S3Image, Entrypoint: []string{"/usr/bin/weed"}, ExposedPorts: []string{"8333/tcp"},
		Cmd: []string{
			"mini", "-dir=/data", "-ip=127.0.0.1", "-ip.bind=0.0.0.0", "-webdav=false", "-admin.ui=false",
			"-s3.port.iceberg=0", "-s3.port.lance=0", "-debug=false",
			"-s3.iam=false", "-s3.autoCreateBucket=false", "-s3.config=/run/s3.json", "-volume.max=4", "-master.volumeSizeLimitMB=64",
		},
		WaitingFor: wait.ForListeningPort("8333/tcp").WithStartupTimeout(30 * time.Second),
		ConfigModifier: func(c *container.Config) {
			c.User, c.WorkingDir = strconv.Itoa(uid)+":"+strconv.Itoa(uid), "/data"
		},
		HostConfigModifier: func(c *container.HostConfig) {
			c.ReadonlyRootfs, c.CapDrop, c.SecurityOpt = true, []string{"ALL"}, []string{"no-new-privileges"}
			c.Binds = []string{data + ":/data:rw", authFile + ":/run/s3.json:ro"}
			c.Tmpfs = map[string]string{"/tmp": "rw,noexec,nosuid,size=33554432,uid=" + strconv.Itoa(uid)}
			c.Memory, c.NanoCPUs = 512<<20, 1_000_000_000
			c.PortBindings = network.PortMap{port: {{HostIP: netip.AddrFrom4([4]byte{127, 0, 0, 1}), HostPort: "0"}}}
		},
	}
	f.container, err = testcontainers.GenericContainer(ctx, testcontainers.GenericContainerRequest{ContainerRequest: request, Started: true})
	if err != nil {
		return f.rollback(ctx, fmt.Errorf("S3 fixture provisioning: %w", err))
	}
	endpoint, err := f.container.PortEndpoint(ctx, "8333/tcp", "http")
	if err != nil {
		return f.rollback(ctx, err)
	}
	f.config = storages3.Config{
		Bucket: bucket, Region: storages3.DefaultRegion, Endpoint: endpoint,
		AccessKeyFile: accessFile, SecretKeyFile: secretFile, ForcePathStyle: true,
	}
	transport := &http.Transport{MaxConnsPerHost: 2, IdleConnTimeout: 10 * time.Second}
	httpClient := &http.Client{Transport: transport, Timeout: 10 * time.Second, CheckRedirect: func(*http.Request, []*http.Request) error {
		return errors.New("S3 fixture redirects are refused")
	}}
	defer httpClient.CloseIdleConnections()
	client := awss3.NewFromConfig(aws.Config{
		Region: storages3.DefaultRegion, Credentials: credentials.NewStaticCredentialsProvider(setup.AccessKey, setup.SecretKey, ""),
		RetryMaxAttempts: 1, HTTPClient: httpClient,
	}, func(o *awss3.Options) {
		o.BaseEndpoint, o.UsePathStyle = aws.String(endpoint), true
	})
	provision, finish := context.WithTimeout(ctx, 10*time.Second)
	defer finish()
	if _, err := client.CreateBucket(provision, &awss3.CreateBucketInput{Bucket: aws.String(bucket)}); err != nil {
		return f.rollback(ctx, fmt.Errorf("S3 bucket provisioning: %w", err))
	}
	return f, nil
}

func (f *S3Fixture) rollback(ctx context.Context, cause error) (*S3Fixture, error) {
	if err := f.Close(ctx); err != nil {
		return f, errors.Join(cause, err)
	}
	return nil, cause
}

// Config returns a copy of the runtime identity bound to the provisioned bucket.
func (f *S3Fixture) Config() storages3.Config {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.config
}

// Restart stops and starts the same container and persistent metadata/data within thirty seconds.
func (f *S3Fixture) Restart(ctx context.Context) error {
	if util.IsNil(ctx) {
		return errors.New("S3 fixture restart requires a context")
	}
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.container == nil {
		return errors.New("S3 fixture is closed")
	}

	ctx, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()
	grace := 5 * time.Second
	if err := f.container.Stop(ctx, &grace); err != nil {
		return err
	}
	if err := f.container.Start(ctx); err != nil {
		return err
	}
	endpoint, err := f.container.PortEndpoint(ctx, "8333/tcp", "http")
	if err != nil {
		return err
	}
	f.config.Endpoint = endpoint
	return nil
}

// CrashRestart kills PID 1 with SIGKILL and restarts the same persistent node within thirty seconds.
// This proves process-crash recovery, not host power-loss or multi-node durability.
func (f *S3Fixture) CrashRestart(ctx context.Context) (err error) {
	if util.IsNil(ctx) {
		return errors.New("S3 fixture crash restart requires a context")
	}
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.container == nil {
		return errors.New("S3 fixture is closed")
	}
	ctx, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()
	docker, err := testcontainers.NewDockerClientWithOpts(ctx)
	if err != nil {
		return err
	}
	defer func() { err = errors.Join(err, docker.Close()) }()
	if _, killErr := docker.ContainerKill(ctx, f.container.GetContainerID(), dockerclient.ContainerKillOptions{Signal: "SIGKILL"}); killErr != nil {
		return killErr
	}
	state, err := f.container.State(ctx)
	if err != nil {
		return err
	}
	if state.Running || state.ExitCode != 137 {
		return errors.New("S3 fixture did not observe SIGKILL termination")
	}
	if startErr := f.container.Start(ctx); startErr != nil {
		return startErr
	}
	endpoint, err := f.container.PortEndpoint(ctx, "8333/tcp", "http")
	if err != nil {
		return err
	}
	f.config.Endpoint = endpoint
	return nil
}

// Close releases the owned container with a fresh thirty-second budget; callers own the temporary directory.
func (f *S3Fixture) Close(ctx context.Context) error {
	if util.IsNil(ctx) {
		return errors.New("S3 fixture close requires a context")
	}
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.container == nil {
		return nil
	}
	ctx, cancel := context.WithTimeout(context.WithoutCancel(ctx), 30*time.Second)
	defer cancel()
	if err := f.container.Terminate(ctx); err != nil {
		return err
	}
	f.container = nil
	return nil
}
