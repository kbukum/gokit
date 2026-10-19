package testutil

import (
	"context"
	"crypto/rand"
	"errors"
	"fmt"
	"net"
	"net/netip"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/moby/moby/api/types/container"
	"github.com/moby/moby/api/types/network"
	natsgo "github.com/nats-io/nats.go"
	"github.com/nats-io/nats.go/jetstream"
	"github.com/testcontainers/testcontainers-go"
	"github.com/testcontainers/testcontainers-go/wait"

	gokitnats "github.com/kbukum/gokit/messaging/nats"
	"github.com/kbukum/gokit/util"
)

// JetStreamImage pins the multi-platform official NATS server image (scratch-based, no shell).
const JetStreamImage = "docker.io/library/nats:2.15.1@sha256:76e4b7a718f0b9e1a07442fe62b12973678a93c1ec2d494355f470dd59402490"

const (
	provisionTimeout = 2 * time.Minute
	readinessTimeout = 20 * time.Second
	closeTimeout     = 30 * time.Second
	username         = "app"
)

var userName = regexp.MustCompile(`^[a-z][a-z0-9-]{0,31}$`)

// JetStreamFixture owns a private authenticated JetStream server with in-container storage.
type JetStreamFixture struct {
	mu        sync.Mutex
	container testcontainers.Container
	config    gokitnats.Config
	password  string
	users     map[string]string
}

// Option configures [StartJetStream].
type Option func(*options) error

type options struct {
	users []user
}

type user struct {
	name, permissions string
}

// WithUser adds a user whose rights are limited by permissions, the body of a NATS server permissions block such as `publish: ["facts.>"], subscribe: ["_INBOX.>"]`. Use it to prove which rights an operation needs. The name is lower case letters, digits and dashes, starting with a letter, and must differ from the default user "app".
func WithUser(name, permissions string) Option {
	return func(o *options) error {
		if !userName.MatchString(name) || name == username {
			return fmt.Errorf("JetStream fixture: invalid user name %q", name)
		}
		if strings.TrimSpace(permissions) == "" {
			return fmt.Errorf("JetStream fixture: user %q needs permissions", name)
		}
		for _, u := range o.users {
			if u.name == name {
				return fmt.Errorf("JetStream fixture: duplicate user %q", name)
			}
		}
		o.users = append(o.users, user{name: name, permissions: permissions})
		return nil
	}
}

// StartJetStream requires Docker and an owned temporary directory for the server configuration and the client password files. Provisioning is bounded at two minutes and returns once JetStream answers an authenticated account request. A non-nil fixture transfers cleanup ownership even on error; register Close before checking the error.
func StartJetStream(ctx context.Context, dir string, opts ...Option) (*JetStreamFixture, error) {
	if util.IsNil(ctx) || dir == "" {
		return nil, errors.New("JetStream fixture: context and an owned temporary directory are required")
	}
	var o options
	for _, opt := range opts {
		if err := opt(&o); err != nil {
			return nil, err
		}
	}
	ctx, cancel := context.WithTimeout(ctx, provisionTimeout)
	defer cancel()
	dir, err := filepath.Abs(dir)
	if err != nil {
		return nil, err
	}
	uid := os.Getuid()
	if uid == 0 {
		uid = 1000
	}
	password := rand.Text()
	users := map[string]string{}
	entries := []string{fmt.Sprintf("    { user: %q, password: %q }", username, password)}
	for _, u := range o.users {
		users[u.name] = rand.Text()
		entries = append(entries, fmt.Sprintf("    { user: %q, password: %q, permissions: { %s } }", u.name, users[u.name], u.permissions))
	}
	serverConf := filepath.Join(dir, "nats.conf")
	conf := fmt.Sprintf(`listen: "0.0.0.0:4222"
max_payload: 1MB
jetstream {
  store_dir: "/data"
  max_memory_store: 64MB
  max_file_store: 256MB
}
authorization {
  users = [
%s
  ]
}
`, strings.Join(entries, "\n"))
	if writeErr := os.WriteFile(serverConf, []byte(conf), 0o400); writeErr != nil {
		return nil, writeErr
	}
	if os.Getuid() == 0 {
		if chownErr := os.Chown(serverConf, uid, uid); chownErr != nil {
			return nil, chownErr
		}
	}
	passwordFile := filepath.Join(dir, "password")
	if writeErr := os.WriteFile(passwordFile, []byte(password), 0o600); writeErr != nil {
		return nil, writeErr
	}
	for name, secret := range users {
		if writeErr := os.WriteFile(filepath.Join(dir, "password-"+name), []byte(secret), 0o600); writeErr != nil {
			return nil, writeErr
		}
	}
	port, err := network.ParsePort("4222/tcp")
	if err != nil {
		return nil, err
	}
	// A fixed host port keeps the client URL valid across Restart.
	hostPort, err := freePort(ctx)
	if err != nil {
		return nil, err
	}
	request := testcontainers.ContainerRequest{
		Image: JetStreamImage, ExposedPorts: []string{"4222/tcp"},
		Cmd: []string{"--config", "/etc/nats/nats.conf"},
		// The image has no shell, so readiness is read from the server log and then proven by an authenticated request.
		WaitingFor: wait.ForLog("Server is ready").WithStartupTimeout(30 * time.Second),
		ConfigModifier: func(c *container.Config) {
			c.User = strconv.Itoa(uid) + ":" + strconv.Itoa(uid)
		},
		HostConfigModifier: func(c *container.HostConfig) {
			c.ReadonlyRootfs, c.CapDrop, c.SecurityOpt = true, []string{"ALL"}, []string{"no-new-privileges"}
			c.Binds = []string{serverConf + ":/etc/nats/nats.conf:ro"}
			c.Tmpfs = map[string]string{"/data": "rw,noexec,nosuid,size=268435456,uid=" + strconv.Itoa(uid)}
			c.Memory, c.NanoCPUs = 256<<20, 1_000_000_000
			c.PortBindings = network.PortMap{port: {{HostIP: netip.AddrFrom4([4]byte{127, 0, 0, 1}), HostPort: hostPort}}}
		},
	}
	f := &JetStreamFixture{password: password, users: users}
	f.container, err = testcontainers.GenericContainer(ctx, testcontainers.GenericContainerRequest{ContainerRequest: request, Started: true})
	if err != nil {
		return f.rollback(ctx, fmt.Errorf("JetStream fixture provisioning: %w", err))
	}
	endpoint, err := f.container.PortEndpoint(ctx, "4222/tcp", "nats")
	if err != nil {
		return f.rollback(ctx, err)
	}
	// The fixture listens on host loopback only; TLS is out of scope for this test server.
	f.config = gokitnats.Config{URL: endpoint, Username: username, PasswordFile: passwordFile, AllowInsecureDev: true}
	if err := awaitJetStream(ctx, endpoint, password); err != nil {
		return f.rollback(ctx, err)
	}
	return f, nil
}

func freePort(ctx context.Context) (port string, err error) {
	var lc net.ListenConfig
	l, err := lc.Listen(ctx, "tcp", "127.0.0.1:0")
	if err != nil {
		return "", err
	}
	defer func() { err = errors.Join(err, l.Close()) }()
	return strconv.Itoa(l.Addr().(*net.TCPAddr).Port), nil
}

func awaitJetStream(ctx context.Context, endpoint, password string) error {
	ctx, cancel := context.WithTimeout(ctx, readinessTimeout)
	defer cancel()
	for {
		err := func() error {
			conn, err := natsgo.Connect(endpoint, natsgo.UserInfo(username, password), natsgo.Timeout(2*time.Second))
			if err != nil {
				return err
			}
			defer conn.Close()
			js, err := jetstream.New(conn)
			if err != nil {
				return err
			}
			_, err = js.AccountInfo(ctx)
			return err
		}()
		if err == nil {
			return nil
		}
		select {
		case <-ctx.Done():
			return fmt.Errorf("JetStream fixture readiness: %w", errors.Join(err, context.Cause(ctx)))
		case <-time.After(250 * time.Millisecond):
		}
	}
}

func (f *JetStreamFixture) rollback(ctx context.Context, cause error) (*JetStreamFixture, error) {
	if err := f.Close(ctx); err != nil {
		return f, errors.Join(cause, err)
	}
	return nil, cause
}

// Config returns client settings for the fixture's user. PasswordFile points into the caller's temporary directory.
func (f *JetStreamFixture) Config() gokitnats.Config {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.config
}

// ConfigFor returns client settings for a user added with [WithUser]. Its PasswordFile is in the caller's directory.
func (f *JetStreamFixture) ConfigFor(name string) (gokitnats.Config, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if _, ok := f.users[name]; !ok {
		return gokitnats.Config{}, fmt.Errorf("JetStream fixture: unknown user %q", name)
	}
	cfg := f.config
	cfg.Username, cfg.PasswordFile = name, filepath.Join(filepath.Dir(f.config.PasswordFile), "password-"+name)
	return cfg, nil
}

// Restart stops the server and starts it again on the same address, to prove clients reconnect. Stream data does not survive, since it lives in memory. It is bounded at two minutes and returns once JetStream answers again.
func (f *JetStreamFixture) Restart(ctx context.Context) error {
	if util.IsNil(ctx) {
		return errors.New("JetStream fixture restart requires a context")
	}
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.container == nil {
		return errors.New("JetStream fixture is closed")
	}
	ctx, cancel := context.WithTimeout(ctx, provisionTimeout)
	defer cancel()
	stop := 10 * time.Second
	if err := f.container.Stop(ctx, &stop); err != nil {
		return fmt.Errorf("JetStream fixture stop: %w", err)
	}
	if err := f.container.Start(ctx); err != nil {
		return fmt.Errorf("JetStream fixture start: %w", err)
	}
	return awaitJetStream(ctx, f.config.URL, f.password)
}

// Close releases the owned container with a fresh thirty-second budget; callers own the temporary directory.
func (f *JetStreamFixture) Close(ctx context.Context) error {
	if util.IsNil(ctx) {
		return errors.New("JetStream fixture close requires a context")
	}
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.container == nil {
		return nil
	}
	ctx, cancel := context.WithTimeout(context.WithoutCancel(ctx), closeTimeout)
	defer cancel()
	if err := f.container.Terminate(ctx); err != nil {
		return err
	}
	f.container = nil
	return nil
}
