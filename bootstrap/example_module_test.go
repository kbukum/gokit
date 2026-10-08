package bootstrap_test

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"testing"
	"time"

	"github.com/kbukum/gokit/bootstrap"
	"github.com/kbukum/gokit/bootstrap/testutil"
	"github.com/kbukum/gokit/config"
	"github.com/kbukum/gokit/logging"
)

// Greeter is the port the greeter module provides.
type Greeter interface {
	Greet(ctx context.Context, name string) (string, error)
}

// GreeterPort is declared once, next to its interface, and referenced everywhere by this variable.
var GreeterPort = bootstrap.NewPort[Greeter]("greeter.service")

// greeterModule provides Greeter in-process and serves it to other services on the "internal" listener.
type greeterModule struct{}

type localGreeter struct{}

func (localGreeter) Greet(_ context.Context, name string) (string, error) {
	return "hello, " + name, nil
}

func (greeterModule) Spec() bootstrap.ModuleSpec {
	return bootstrap.ModuleSpec{Name: "greeter", Provides: []bootstrap.PortRef{GreeterPort.Ref()}, Listeners: []string{"internal"}}
}

func (greeterModule) Register(_ context.Context, mc *bootstrap.ModuleContext) error {
	g := localGreeter{}
	if err := mc.Handle("internal", "GET /greet", greeterHandler(g)); err != nil {
		return err
	}
	return bootstrap.Provide(mc, GreeterPort, Greeter(g))
}

// greeterHandler serves Greeter over HTTP for remote callers.
func greeterHandler(g Greeter) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		msg, err := g.Greet(r.Context(), r.URL.Query().Get("name"))
		if err != nil {
			http.Error(w, "greet failed", http.StatusInternalServerError)
			return
		}
		_, _ = io.WriteString(w, msg)
	})
}

// welcomeModule needs Greeter and does not know whether it runs in the same process.
type welcomeModule struct{}

func (welcomeModule) Spec() bootstrap.ModuleSpec {
	return bootstrap.ModuleSpec{Name: "welcome", Needs: []bootstrap.PortRef{GreeterPort.Ref()}, Listeners: []string{"public"}}
}

func (welcomeModule) Register(_ context.Context, mc *bootstrap.ModuleContext) error {
	g, err := bootstrap.Need(mc, GreeterPort)
	if err != nil {
		return err
	}
	return mc.Handle("public", "GET /welcome", http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		msg, err := g.Greet(r.Context(), r.URL.Query().Get("name"))
		if err != nil {
			http.Error(w, "greeter unavailable", http.StatusBadGateway)
			return
		}
		_, _ = io.WriteString(w, msg+"!")
	}))
}

// greeterClientModule is the greeter module's remote stand-in. The greeter package exports it next to greeterModule; a command that runs the greeter elsewhere uses it instead, and modules that need Greeter do not change. A client that owns connections would also register its component here, so it starts before the modules that need it.
type greeterClientModule struct {
	base string
	http *http.Client
}

func (greeterClientModule) Spec() bootstrap.ModuleSpec {
	return bootstrap.ModuleSpec{Name: "greeter-client", Provides: []bootstrap.PortRef{GreeterPort.Ref()}}
}

func (m greeterClientModule) Register(_ context.Context, mc *bootstrap.ModuleContext) error {
	return bootstrap.Provide(mc, GreeterPort, Greeter(greeterClient(m)))
}

// greeterClient implements Greeter over HTTP.
type greeterClient struct {
	base string
	http *http.Client
}

func (c greeterClient) Greet(ctx context.Context, name string) (string, error) {
	return get(ctx, c.http, c.base+"/greet?name="+url.QueryEscape(name))
}

// Example_modules runs the greeter and welcome modules in one app, then splits them across two apps, with the greeter's client module standing in for it. Neither module changes.
func Example_modules() {
	ctx := context.Background()
	hc := &http.Client{Timeout: 2 * time.Second}

	// One service: both modules in one process.
	one, err := run(ctx, "all-in-one", func(app *bootstrap.App[*exampleConfig], l listeners) error {
		return errors.Join(app.Use(greeterModule{}, welcomeModule{}), l.listen(app, "public", "internal"))
	})
	if err != nil {
		fmt.Println(err)
		return
	}
	fmt.Println(welcome(ctx, hc, one, "ada"))
	fmt.Println(one.app.Shutdown(ctx))

	// Two services: the greeter module moves out; the welcome service uses its client module.
	greeterSvc, err := run(ctx, "greeter-svc", func(app *bootstrap.App[*exampleConfig], l listeners) error {
		return errors.Join(app.Use(greeterModule{}), l.listen(app, "internal"))
	})
	if err != nil {
		fmt.Println(err)
		return
	}
	defer func() { _ = greeterSvc.app.Shutdown(ctx) }()
	client := greeterClientModule{base: greeterSvc.listeners["internal"].URL(), http: hc}
	welcomeSvc, err := run(ctx, "welcome-svc", func(app *bootstrap.App[*exampleConfig], l listeners) error {
		return errors.Join(app.Use(welcomeModule{}, client), l.listen(app, "public"))
	})
	if err != nil {
		fmt.Println(err)
		return
	}
	fmt.Println(welcome(ctx, hc, welcomeSvc, "grace"))
	fmt.Println(welcomeSvc.app.Shutdown(ctx))

	// Output:
	// hello, ada!
	// <nil>
	// hello, grace!
	// <nil>
}

// Example_modulesMissingPort shows the wiring error when a needed port has no provider. CheckModules reports it without starting anything; Startup reports the same error.
func Example_modulesMissingPort() {
	app, err := newExampleApp("welcome-svc")
	if err != nil {
		fmt.Println(err)
		return
	}
	l := listeners{}
	if err := errors.Join(app.Use(welcomeModule{}), l.listen(app, "public")); err != nil {
		fmt.Println(err)
		return
	}
	var modErr *bootstrap.ModuleError
	if errors.As(app.CheckModules(), &modErr) {
		for _, p := range modErr.Problems {
			fmt.Println(p)
		}
	}
	// Output:
	// missing port "greeter.service" [welcome]: needed but not provided
}

// TestWelcomeModule tests the welcome module alone, with a test double for the Greeter it needs, through its public route.
func TestWelcomeModule(t *testing.T) {
	t.Parallel()
	app := testutil.NewApp(t)
	public := testutil.RegisterListener(t, app, "public")
	if err := app.Use(welcomeModule{}, bootstrap.ValueModule("greeter-double", GreeterPort, Greeter(localGreeter{}))); err != nil {
		t.Fatal(err)
	}
	testutil.Start(t, app)
	got, err := get(t.Context(), http.DefaultClient, public.URL()+"/welcome?name=ada")
	if err != nil || got != "hello, ada!" {
		t.Fatalf("GET /welcome = %q, %v", got, err)
	}
}

// TestGreeterPortContract holds the in-process greeter and its HTTP client to the same behavior, which is what lets the greeter move between services.
func TestGreeterPortContract(t *testing.T) {
	t.Parallel()
	testutil.AssertRemoteShape(t, GreeterPort)
	testutil.Contract(t, GreeterPort, func(t *testing.T, g Greeter) {
		got, err := g.Greet(t.Context(), "ada lovelace")
		if err != nil || got != "hello, ada lovelace" {
			t.Fatalf("Greet = %q, %v", got, err)
		}
	},
		testutil.Implementation[Greeter]{Name: "local", New: func(*testing.T) Greeter { return localGreeter{} }},
		testutil.Implementation[Greeter]{Name: "http", New: func(t *testing.T) Greeter {
			// The client module is tested through the App, as a split service runs it.
			app := testutil.NewApp(t)
			internal := testutil.RegisterListener(t, app, "internal")
			if err := app.Use(greeterModule{}); err != nil {
				t.Fatal(err)
			}
			testutil.Start(t, app)
			return greeterClient{base: internal.URL(), http: http.DefaultClient}
		}},
	)
}

type exampleConfig struct {
	config.ServiceConfig
}

type service struct {
	app       *bootstrap.App[*exampleConfig]
	listeners listeners
}

type listeners map[string]*testutil.Listener

// listen declares a loopback HTTP listener for each name.
func (l listeners) listen(app *bootstrap.App[*exampleConfig], names ...string) error {
	errs := make([]error, 0, len(names))
	for _, name := range names {
		l[name] = testutil.NewListener(name)
		errs = append(errs, app.RegisterListener(name, l[name]))
	}
	return errors.Join(errs...)
}

// newExampleApp builds a quiet app, as a command would with its own config.
func newExampleApp(name string) (*bootstrap.App[*exampleConfig], error) {
	logCfg := &logging.Config{Level: "error", Format: "json"}
	logCfg.ApplyDefaults()
	logger, err := logging.New(logCfg, name, logging.WithWriter(io.Discard))
	if err != nil {
		return nil, err
	}
	cfg := &exampleConfig{ServiceConfig: config.ServiceConfig{Name: name, Version: "1.0.0", Environment: "development"}}
	app, err := bootstrap.NewApp(cfg, bootstrap.WithLogger(logger), bootstrap.WithGracefulTimeout(5*time.Second))
	if err != nil {
		return nil, err
	}
	app.Summary.SetWriter(io.Discard)
	return app, nil
}

// run builds an app, lets compose wire it, and starts it.
func run(ctx context.Context, name string, compose func(*bootstrap.App[*exampleConfig], listeners) error) (service, error) {
	app, err := newExampleApp(name)
	if err != nil {
		return service{}, err
	}
	svc := service{app: app, listeners: listeners{}}
	if err := compose(app, svc.listeners); err != nil {
		return service{}, err
	}
	return svc, app.Startup(ctx)
}

func welcome(ctx context.Context, hc *http.Client, svc service, name string) string {
	body, err := get(ctx, hc, svc.listeners["public"].URL()+"/welcome?name="+url.QueryEscape(name))
	if err != nil {
		return err.Error()
	}
	return body
}

func get(ctx context.Context, hc *http.Client, target string) (string, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, target, http.NoBody)
	if err != nil {
		return "", err
	}
	resp, err := hc.Do(req)
	if err != nil {
		return "", err
	}
	defer func() { _ = resp.Body.Close() }()
	body, err := io.ReadAll(io.LimitReader(resp.Body, 1<<10))
	if err != nil {
		return "", err
	}
	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("GET %s: %s", target, resp.Status)
	}
	return string(body), nil
}
