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
	"regexp"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/credentials"
	awss3 "github.com/aws/aws-sdk-go-v2/service/s3"
	"github.com/distribution/reference"
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

var imageID = regexp.MustCompile(`^sha256:[0-9a-f]{64}$`)

type options struct {
	image string
}

// Option configures StartS3.
type Option func(*options) error

// WithImage runs an alternate SeaweedFS-compatible image instead of S3Image, such as an artifact built from source. The reference must be immutable: a local image ID (sha256:<hex>) or a name pinned by digest. A missing image fails provisioning; the fixture never falls back to another image. The image must provide /usr/bin/weed.
func WithImage(ref string) Option {
	return func(o *options) error {
		if !imageID.MatchString(ref) {
			parsed, err := reference.Parse(ref)
			if err != nil {
				return fmt.Errorf("S3 fixture image must be an image ID or digest-pinned reference: %w", err)
			}
			_, named := parsed.(reference.Named)
			if _, digested := parsed.(reference.Digested); !named || !digested {
				return errors.New("S3 fixture image must be an image ID or digest-pinned reference")
			}
		}
		o.image = ref
		return nil
	}
}

// S3Fixture owns a private authenticated single-node S3 server and its persistent test data.
type S3Fixture struct {
	mu        sync.Mutex
	container testcontainers.Container
	config    storages3.Config
	setup     s3Credential
}

const (
	restartTimeout          = 60 * time.Second
	gracefulStopTimeout     = 20 * time.Second
	readinessTimeout        = 20 * time.Second
	readinessAttemptTimeout = 5 * time.Second
	readinessRetryDelay     = 250 * time.Millisecond
)

type s3Credential struct {
	AccessKey string `json:"accessKey"`
	SecretKey string `json:"secretKey"`
}

type s3Identity struct {
	Name        string         `json:"name"`
	Credentials []s3Credential `json:"credentials"`
	Actions     []string       `json:"actions"`
}

// StartS3 requires Docker and an owned temporary directory. It runs S3Image unless WithImage selects another artifact. Provisioning is bounded at three minutes and returns once the bucket accepts writes. Config exposes mounted credential paths; callers must never retain their contents outside the fixture. A non-nil fixture transfers cleanup ownership even on error; register Close before checking the error.
func StartS3(ctx context.Context, dir string, opts ...Option) (*S3Fixture, error) {
	if util.IsNil(ctx) || dir == "" {
		return nil, errors.New("S3 fixture: context and an owned temporary directory are required")
	}
	o := options{image: S3Image}
	for _, opt := range opts {
		if opt == nil {
			return nil, errors.New("S3 fixture: nil option")
		}
		if err := opt(&o); err != nil {
			return nil, err
		}
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
		Image: o.image, Entrypoint: []string{"/usr/bin/weed"}, ExposedPorts: []string{"8333/tcp"},
		Cmd: []string{
			// "server" starts only the requested components; "mini" always adds an admin server and maintenance worker.
			"server", "-dir=/data", "-ip=127.0.0.1", "-ip.bind=0.0.0.0", "-filer", "-s3",
			"-s3.port.iceberg=0", "-s3.port.lance=0", "-debug=false",
			"-s3.iam=false", "-s3.autoCreateBucket=false", "-s3.config=/run/s3.json", "-volume.max=4", "-master.volumeSizeLimitMB=64",
			// A single master needs no 10s election wait, and the volume pre-stop delay would outlast the stop grace.
			"-master.electionTimeout=1s", "-volume.preStopSeconds=0",
		},
		// The default growth of seven volumes per collection exceeds the four bounded volume slots.
		Env: map[string]string{"WEED_MASTER_VOLUME_GROWTH_COPY_1": "1"},
		// An HTTP response proves the server itself is serving. A port wait would pass on Docker's proxy alone in images without a shell, because testcontainers then skips its in-container listener check.
		WaitingFor: wait.ForHTTP("/").WithPort("8333/tcp").WithStatusCodeMatcher(func(int) bool { return true }).
			WithStartupTimeout(30 * time.Second),
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
		AccessKeyFile: accessFile, SecretKeyFile: secretFile, ForcePathStyle: true, AllowInsecureDev: true,
	}
	f.setup = setup
	client, closeClient := f.setupClient(endpoint)
	defer closeClient()
	provision, finish := context.WithTimeout(ctx, 10*time.Second)
	defer finish()
	if _, err := client.CreateBucket(provision, &awss3.CreateBucketInput{Bucket: aws.String(bucket)}); err != nil {
		return f.rollback(ctx, fmt.Errorf("S3 bucket provisioning: %w", err))
	}
	if err := f.awaitWritable(ctx); err != nil {
		return f.rollback(ctx, err)
	}
	return f, nil
}

func (f *S3Fixture) setupClient(endpoint string) (client *awss3.Client, closeIdle func()) {
	transport := &http.Transport{MaxConnsPerHost: 2, IdleConnTimeout: 10 * time.Second}
	httpClient := &http.Client{Transport: transport, Timeout: 10 * time.Second, CheckRedirect: func(*http.Request, []*http.Request) error {
		return errors.New("S3 fixture redirects are refused")
	}}
	client = awss3.NewFromConfig(aws.Config{
		Region: storages3.DefaultRegion, Credentials: credentials.NewStaticCredentialsProvider(f.setup.AccessKey, f.setup.SecretKey, ""),
		RetryMaxAttempts: 1, HTTPClient: httpClient,
	}, func(o *awss3.Options) {
		o.BaseEndpoint, o.UsePathStyle = aws.String(endpoint), true
	})
	return client, httpClient.CloseIdleConnections
}

// awaitWritable proves the bucket accepts and deletes a sentinel object. The S3 listener opens before the master elects a leader and the volume server registers, so an open port alone does not mean writes can be assigned. Callers must hold f.mu or have exclusive ownership of f.
func (f *S3Fixture) awaitWritable(ctx context.Context) error {
	ctx, cancel := context.WithTimeout(ctx, readinessTimeout)
	defer cancel()
	client, closeClient := f.setupClient(f.config.Endpoint)
	defer closeClient()
	bucket, key := aws.String(f.config.Bucket), aws.String(".fixture-ready/"+rand.Text())
	for {
		attempt, finish := context.WithTimeout(ctx, readinessAttemptTimeout)
		_, err := client.PutObject(attempt, &awss3.PutObjectInput{Bucket: bucket, Key: key, Body: strings.NewReader("ready")})
		if err == nil {
			_, err = client.DeleteObject(attempt, &awss3.DeleteObjectInput{Bucket: bucket, Key: key})
		}
		finish()
		if err == nil {
			return nil
		}
		select {
		case <-ctx.Done():
			return fmt.Errorf("S3 fixture write readiness: %w", errors.Join(err, context.Cause(ctx)))
		case <-time.After(readinessRetryDelay):
		}
	}
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

// Restart stops the same container with SIGTERM, verifies a clean exit, and starts it with its persistent metadata/data, returning once writable, within sixty seconds. The filer's graceful gRPC stop alone may take fifteen seconds.
func (f *S3Fixture) Restart(ctx context.Context) error {
	if util.IsNil(ctx) {
		return errors.New("S3 fixture restart requires a context")
	}
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.container == nil {
		return errors.New("S3 fixture is closed")
	}

	ctx, cancel := context.WithTimeout(ctx, restartTimeout)
	defer cancel()
	grace := gracefulStopTimeout
	if err := f.container.Stop(ctx, &grace); err != nil {
		return err
	}
	state, err := f.container.State(ctx)
	if err != nil {
		return err
	}
	if state.Running || state.ExitCode != 0 {
		return fmt.Errorf("S3 fixture did not stop gracefully: exit status %d", state.ExitCode)
	}
	err = f.container.Start(ctx)
	if err != nil {
		return err
	}
	endpoint, err := f.container.PortEndpoint(ctx, "8333/tcp", "http")
	if err != nil {
		return err
	}
	f.config.Endpoint = endpoint
	return f.awaitWritable(ctx)
}

// CrashRestart kills PID 1 with SIGKILL and restarts the same persistent node, returning once writable, within thirty seconds. This proves process-crash recovery, not host power-loss or multi-node durability.
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
	return f.awaitWritable(ctx)
}

// ContainerID identifies the owned container for attachment to a caller-owned test network. It is empty after release. The fixture, not the network caller, owns container teardown.
func (f *S3Fixture) ContainerID() string {
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.container == nil {
		return ""
	}
	return f.container.GetContainerID()
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
