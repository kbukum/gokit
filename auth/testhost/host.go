package testhost

import (
	"context"
	"crypto/rand"
	"crypto/tls"
	"errors"
	"net"
	"net/http"
	"net/url"
	"os"
	"strconv"
	"sync"
	"sync/atomic"
	"time"

	"github.com/kbukum/gokit/auth"
	"github.com/kbukum/gokit/auth/apikey"
	"github.com/kbukum/gokit/auth/password"
	"github.com/kbukum/gokit/auth/session"
	sessiondb "github.com/kbukum/gokit/auth/session/database"
	"github.com/kbukum/gokit/component"
	"github.com/kbukum/gokit/database"
	"github.com/kbukum/gokit/database/migration"
	"github.com/kbukum/gokit/database/sqlite"
	apperrors "github.com/kbukum/gokit/errors"
	"github.com/kbukum/gokit/logging"
	"github.com/kbukum/gokit/security"
	"github.com/kbukum/gokit/server"
	"github.com/kbukum/gokit/sse"
	"github.com/kbukum/gokit/testutil"
	"github.com/kbukum/gokit/util"
)

// Host owns a production-adapter authentication fixture. New starts it; Close releases all resources even when its setup/request context was canceled.
type Host struct {
	config     Config
	fixture    Fixture
	log        *logging.Logger
	db         *database.DB
	store      *controlledStore
	migrations migration.Config
	manager    *session.Manager
	server     *server.Server
	bus        *sse.Bus
	clock      *util.FakeClock
	startedAt  time.Time
	apiKey     string
	assets     *os.Root
	origin     string
	cleanup    testutil.CleanupFunc
	activity   sync.RWMutex
	lifecycle  sync.Mutex
	status     atomic.Pointer[heldStatus]
}

// New starts an isolated real HTTPS host through the kit's owned test lifecycle.
func New(ctx context.Context, config Config, fixture Fixture) (*Host, error) {
	if err := config.Validate(); err != nil {
		return nil, err
	}
	if len(fixture.DigestKey) != 32 || len(fixture.CSRFKey) != 32 || len(fixture.KeyPepper) != 32 || fixture.APIKey == "" || len(fixture.Password) < 26 || len(fixture.ControlToken) < 26 {
		return nil, apperrors.InvalidInput("fixture", "Private fixture material is required")
	}
	h := &Host{config: config, fixture: fixture, log: logging.NewDefault("auth-testhost")} //nolint:contextcheck // NewDefault is console-only and never initializes an exporter.
	cleanup, err := testutil.SetupWithContext(ctx, h)
	if err != nil {
		return nil, err
	}
	h.cleanup = cleanup
	return h, nil
}

// Origin returns the actual HTTPS origin, including an allocated ephemeral port.
func (h *Host) Origin() string { return h.origin }

// APIKey returns this host's synthetic automation credential. Keep it in private runner state, never logs or retained evidence.
func (h *Host) APIKey() string { return h.apiKey }

// Name identifies the fixture component.
func (h *Host) Name() string { return "auth-https-testhost" }

// Health reports authoritative fixture availability, including schema readiness.
func (h *Host) Health(ctx context.Context) component.Health {
	h.lifecycle.Lock()
	defer h.lifecycle.Unlock()
	status := component.StatusHealthy
	if h.store == nil || h.store.check(ctx) != nil || h.migrations.Ready(ctx, sessiondb.SchemaVersion) != nil {
		status = component.StatusUnhealthy
	}
	return component.Health{Name: h.Name(), Status: status}
}

// Start acquires database, session workers and the HTTPS listener. A failed start releases partial acquisition using a fresh cleanup budget.
func (h *Host) Start(ctx context.Context) (err error) {
	h.lifecycle.Lock()
	defer h.lifecycle.Unlock()
	if h.db != nil {
		return apperrors.New(apperrors.ErrCodeConflict, "Fixture already started")
	}
	defer func() {
		if err != nil {
			cleanupCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), 10*time.Second)
			defer cancel()
			err = errors.Join(err, h.stop(cleanupCtx))
		}
	}()
	cfg := database.Config{Enabled: true, DSN: h.config.StateFile, MaxRetries: 1, LogLevel: "silent"}
	cfg.ApplyDefaults()
	h.db, err = database.NewWithContext(ctx, sqlite.Dialect(), cfg, h.log)
	if err != nil {
		return err
	}
	h.migrations = sessiondb.Migrations(h.db, sqlite.MigrateDriver())
	if migrationErr := h.migrations.Up(ctx); migrationErr != nil {
		return migrationErr
	}
	if readinessErr := h.migrations.Ready(ctx, sessiondb.SchemaVersion); readinessErr != nil {
		return readinessErr
	}
	h.startedAt = time.Now().UTC()
	h.clock = util.NewFakeClock(h.startedAt)
	sqlStore, err := sessiondb.NewStore(h.db, h.clock)
	if err != nil {
		return err
	}
	h.store = &controlledStore{Store: sqlStore}
	csrf, err := security.NewSignedCSRF(h.fixture.CSRFKey, rand.Reader)
	if err != nil {
		return err
	}
	h.manager, err = session.NewManager(session.Config{Store: h.store, Clock: h.clock, Random: rand.Reader, Pepper: string(h.fixture.DigestKey), CSRF: csrf, ReportError: func(ctx context.Context, err error) {
		h.log.WarnCtx(ctx, "Session worker failed", map[string]any{"code": string(apperrors.Normalize(err).Code)})
	}})
	if err != nil {
		return err
	}
	h.bus, err = sse.NewBus(sse.DefaultLimits())
	if err != nil {
		return err
	}
	u, err := url.Parse(h.config.Origin)
	if err != nil {
		return err
	}
	port, err := strconv.Atoi(u.Port())
	if err != nil {
		return err
	}
	serverConfig := server.Config{Host: u.Hostname(), Port: port, TLS: &security.TLSConfig{CertFile: h.config.CertFile, KeyFile: h.config.KeyFile, MinVersion: tls.VersionTLS13}}
	serverConfig.ApplyDefaults()
	serverConfig.Port, serverConfig.WriteTimeout = port, 0
	h.server = server.New(&serverConfig, h.log) //nolint:contextcheck // The supplied logger avoids the constructor's console-only nil fallback.
	if assetsErr := h.mountAssets(); assetsErr != nil {
		return assetsErr
	}
	h.server.ApplyMiddleware() //nolint:contextcheck // Recovery's console-only logger cannot acquire exporter resources.
	if startErr := h.server.Start(ctx); startErr != nil {
		return startErr
	}
	_, boundPort, err := net.SplitHostPort(h.server.ListenAddr().String())
	if err != nil {
		return err
	}
	h.origin = "https://" + net.JoinHostPort(u.Hostname(), boundPort)
	if err := h.mount(ctx); err != nil {
		return err
	}
	return nil
}

func (h *Host) mount(ctx context.Context) error {
	hasher, err := password.NewHasher(password.Config{})
	if err != nil {
		return err
	}
	hash, err := hasher.Hash(h.fixture.Password)
	if err != nil {
		return err
	}
	verify := session.LoginVerifierFunc(func(ctx context.Context, input session.LoginCredentials) (auth.Principal, error) {
		if contextErr := ctx.Err(); contextErr != nil {
			return auth.Principal{}, contextErr
		}
		if verifyErr := hasher.Verify(input.Password, hash); verifyErr != nil || input.Username != "fixture-user" {
			return auth.Principal{}, auth.Failure("LOGIN_INVALID")
		}
		return auth.Principal{Subject: "fixture-user", Kind: auth.User, Restrictions: auth.Restrictions{Mode: auth.Unrestricted}}, nil
	})
	backend, err := session.NewLocalBackend(h.manager, verify)
	if err != nil {
		return err
	}
	handlers, err := session.NewHandler(backend, session.HandlerConfig{Origin: h.origin, Errors: h.writeError, Clock: h.clock})
	if err != nil {
		return err
	}
	if mountErr := h.server.Handle("/auth/", h.track(h.fenceStatus(handlers))); mountErr != nil {
		return mountErr
	}
	keys, err := apikey.NewMemoryStore(8)
	if err != nil {
		return err
	}
	protection, err := apikey.NewHasher(apikey.HashingConfig{Pepper: string(h.fixture.KeyPepper)})
	if err != nil {
		return err
	}
	keyManager, err := apikey.NewManager(apikey.Config{Store: keys, Hasher: protection, Clock: h.clock})
	if err != nil {
		return err
	}
	prefix, _, err := apikey.SplitKey(h.fixture.APIKey)
	if err != nil {
		return err
	}
	err = keys.Create(ctx, &apikey.Key{
		ID: "fixture-key", OwnerID: "fixture-user", Name: "fixture", Kind: auth.User,
		RestrictionMode: auth.Restricted, Resources: []string{"resource-a"}, Scopes: []string{"read"},
		KeyPrefix: prefix, KeyDigest: protection.Digest(h.fixture.APIKey), CreatedAt: h.clock.Now(),
	})
	if err != nil {
		return err
	}
	h.apiKey = h.fixture.APIKey
	chain := auth.NewChain(h.manager, keyManager)
	if err := h.mountProtected(chain); err != nil {
		return err
	}
	return errors.Join(
		h.server.Handle("GET /_test/ready", http.HandlerFunc(h.ready)),
		h.server.Handle("GET /_test/state", http.HandlerFunc(h.state)),
		h.server.Handle("POST /_test/reset", http.HandlerFunc(h.reset)),
		h.server.Handle("POST /_test/scenario", http.HandlerFunc(h.scenario)),
	)
}

func (h *Host) track(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		h.activity.RLock()
		defer h.activity.RUnlock()
		next.ServeHTTP(w, r)
	})
}

// Close runs the lifecycle's fresh ten-second cleanup budget, independent of caller cancellation.
func (h *Host) Close(_ context.Context) error {
	if h.cleanup == nil {
		return nil
	}
	return h.cleanup()
}

// Stop cancels authenticated streams before draining HTTPS and closing database pools.
func (h *Host) Stop(ctx context.Context) error {
	h.lifecycle.Lock()
	defer h.lifecycle.Unlock()
	return h.stop(ctx)
}

func (h *Host) stop(ctx context.Context) error {
	var failures []error
	if held := h.status.Swap(nil); held != nil {
		held.close()
	}
	if h.manager != nil {
		failures = append(failures, h.manager.Close(ctx))
	}
	if h.bus != nil {
		h.bus.Close()
	}
	if h.server != nil {
		failures = append(failures, h.server.Stop(ctx))
	}
	if h.db != nil {
		failures = append(failures, h.db.Close())
	}
	if h.assets != nil {
		failures = append(failures, h.assets.Close())
		h.assets = nil
	}
	return errors.Join(failures...)
}
