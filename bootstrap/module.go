package bootstrap

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"slices"
	"sync"

	"github.com/kbukum/gokit/component"
	"github.com/kbukum/gokit/logging"
	"github.com/kbukum/gokit/util"
)

// Module is one named capability that an App composes with others. Spec declares what the module provides and needs; Register wires it through a [ModuleContext]. A module does not know where its needs come from: the command satisfies them with modules in the same App, with client modules that reach another service, or with a [ValueModule] in tests.
//
// Register runs once, during the App's modules phase (after configure hooks), after every needed port is available. It must not block or perform network I/O: it builds handlers, use cases and components and returns. Components it adds do their I/O in Start.
//
// IMPROVE-RSKIT: rskit has no module layer yet. The same concept (named modules with typed provided and needed ports, dependency ordering and one wiring error) should land in rskit's bootstrap as a trait so services in both kits compose the same way.
type Module interface {
	Spec() ModuleSpec
	Register(ctx context.Context, mc *ModuleContext) error
}

// ModuleSpec declares a module's name and the ports it provides and needs.
type ModuleSpec struct {
	// Name identifies the module in errors and the startup summary. It must be unique within an App.
	Name string
	// Provides lists the ports Register must provide with [Provide].
	Provides []PortRef
	// Needs lists the ports Register may read with [Need]. Each must be provided by exactly one other module.
	Needs []PortRef
	// Listeners names the listeners Register mounts routes on with [ModuleContext.Handle]. Each must be declared with [App.RegisterListener].
	Listeners []string
}

// Listener is an HTTP listener that modules mount routes on. It is a lifecycle component that stops accepting requests on Quiesce and drains in-flight requests as [component.DrainIngress], so on shutdown it finishes requests before module workers drain. gokit's *server.Component satisfies it. Handle uses [http.ServeMux] patterns, with the unqualified "/" reserved for fallback dispatch. Handle and Fallback reject nil and typed-nil handlers; Handle also rejects invalid or conflicting patterns. The App reports these errors as [ErrRouteConflict]. Fallback replaces the handler for requests no route matches, such as a single-page app; the App calls it at most once, after every module has registered. Register routes and fallbacks before starting the listener.
//
// Listeners are HTTP only; Connect services mount as HTTP handlers. A listener with another mount model, such as a *grpc.Server, is wired by the command outside the module layer.
type Listener interface {
	component.Component
	component.Quiescer
	component.Drainer
	Handle(pattern string, handler http.Handler) error
	Fallback(handler http.Handler) error
}

// Errors returned by [ModuleContext] methods, [Provide] and [Need].
var (
	// ErrPortNotDeclared reports a Provide or Need for a port missing from the module's spec.
	ErrPortNotDeclared = errors.New("bootstrap: port not declared by module")
	// ErrPortAlreadyProvided reports a second Provide for the same port.
	ErrPortAlreadyProvided = errors.New("bootstrap: port already provided")
	// ErrNilPortValue reports a nil value given to Provide.
	ErrNilPortValue = errors.New("bootstrap: nil port value")
	// ErrListenerNotDeclared reports a route for a listener missing from the module's spec.
	ErrListenerNotDeclared = errors.New("bootstrap: listener not declared by module")
	// ErrRouteConflict reports an empty, invalid or already mounted route, or a second fallback on a listener.
	ErrRouteConflict = errors.New("bootstrap: route conflict")
	// ErrModuleContextClosed reports use of a ModuleContext after its Register returned.
	ErrModuleContextClosed = errors.New("bootstrap: module context used after Register returned")
)

// ModuleContext is what a module's Register uses to wire itself into the App. It reaches only the module's declared ports, the App's listeners and its component registry; it never exposes the DI container. It is valid only until Register returns.
type ModuleContext struct {
	spec   ModuleSpec
	logger *logging.Logger
	wiring *moduleWiring
	info   *ModuleInfo

	mu       sync.Mutex
	closed   bool
	provided map[PortRef]bool
}

// Name returns the module name.
func (mc *ModuleContext) Name() string { return mc.spec.Name }

// Logger returns the App logger tagged with the module name.
func (mc *ModuleContext) Logger() *logging.Logger { return mc.logger }

// RegisterComponent registers a lifecycle component. Module components start after components registered before the modules phase (including in configure hooks), in module dependency order, and before listeners; they stop in reverse within their shutdown phase. A component reports its own phase by implementing ShutdownPhase(); otherwise it is a resource.
func (mc *ModuleContext) RegisterComponent(c component.Component) error {
	return mc.addComponent(c, func(r *component.Registry) error { return r.Register(c) })
}

// RegisterComponentInPhase registers a lifecycle component in the given shutdown phase.
func (mc *ModuleContext) RegisterComponentInPhase(c component.Component, phase component.ShutdownPhase) error {
	return mc.addComponent(c, func(r *component.Registry) error { return r.RegisterInPhase(c, phase) })
}

func (mc *ModuleContext) addComponent(c component.Component, register func(*component.Registry) error) error {
	return mc.locked(func() error {
		if util.IsNil(c) {
			return fmt.Errorf("bootstrap: module %q: nil component", mc.spec.Name)
		}
		if err := register(mc.wiring.registry); err != nil {
			return fmt.Errorf("bootstrap: module %q: %w", mc.spec.Name, err)
		}
		mc.info.Components = append(mc.info.Components, c.Name())
		return nil
	})
}

// Handle mounts handler at pattern on the named listener, which must be in the module's Listeners. The command decides which listeners exist with [App.RegisterListener]; a module only names the listeners its routes belong to.
func (mc *ModuleContext) Handle(listener, pattern string, handler http.Handler) error {
	return mc.locked(func() error {
		if !slices.Contains(mc.spec.Listeners, listener) {
			return fmt.Errorf("%w: module %q mounts on %q", ErrListenerNotDeclared, mc.spec.Name, listener)
		}
		if pattern == "" || util.IsNil(handler) {
			return fmt.Errorf("%w: module %q needs a pattern and a handler", ErrRouteConflict, mc.spec.Name)
		}
		if err := mc.wiring.mount(listener, pattern, handler); err != nil {
			return fmt.Errorf("module %q: %w", mc.spec.Name, err)
		}
		mc.info.Routes = append(mc.info.Routes, listener+" "+pattern)
		return nil
	})
}

// Fallback sets the handler for requests on the named listener that no route matches, such as a single-page app. A listener has at most one fallback. The App installs it after every module has registered, and it answers 404 under the first path segment of every route modules mount on that listener (for "POST /auth/login", everything under /auth), so unknown API paths never reach it.
func (mc *ModuleContext) Fallback(listener string, handler http.Handler) error {
	return mc.locked(func() error {
		if !slices.Contains(mc.spec.Listeners, listener) {
			return fmt.Errorf("%w: module %q sets a fallback on %q", ErrListenerNotDeclared, mc.spec.Name, listener)
		}
		if util.IsNil(handler) {
			return fmt.Errorf("%w: module %q needs a fallback handler", ErrRouteConflict, mc.spec.Name)
		}
		if err := mc.wiring.setFallback(listener, mc.spec.Name, handler); err != nil {
			return fmt.Errorf("module %q: %w", mc.spec.Name, err)
		}
		mc.info.Routes = append(mc.info.Routes, listener+" fallback")
		return nil
	})
}

// locked runs fn while the context is open, so no wiring happens after Register returns.
func (mc *ModuleContext) locked(fn func() error) error {
	mc.mu.Lock()
	defer mc.mu.Unlock()
	if mc.closed {
		return fmt.Errorf("%w: %q", ErrModuleContextClosed, mc.spec.Name)
	}
	return fn()
}

func (mc *ModuleContext) close() {
	mc.mu.Lock()
	defer mc.mu.Unlock()
	mc.closed = true
}

func (mc *ModuleContext) declares(list []PortRef, ref PortRef) bool {
	return slices.Contains(list, ref)
}

// Provide makes value available as port p to modules that need it. p must be in the module's Provides, and each declared port must be provided exactly once before Register returns.
func Provide[T any](mc *ModuleContext, p *Port[T], value T) error {
	ref := p.Ref()
	return mc.locked(func() error {
		switch {
		case !ref.valid() || !mc.declares(mc.spec.Provides, ref):
			return fmt.Errorf("%w: module %q provides %s", ErrPortNotDeclared, mc.spec.Name, ref)
		case util.IsNil(value):
			return fmt.Errorf("%w: module %q provides %s", ErrNilPortValue, mc.spec.Name, ref)
		case mc.provided[ref]:
			return fmt.Errorf("%w: module %q provides %s twice", ErrPortAlreadyProvided, mc.spec.Name, ref)
		}
		mc.provided[ref] = true
		mc.wiring.provide(ref, value)
		return nil
	})
}

// Need returns the value another module provided for port p. p must be in the module's Needs.
func Need[T any](mc *ModuleContext, p *Port[T]) (T, error) {
	var v T
	ref := p.Ref()
	err := mc.locked(func() error {
		if !ref.valid() || !mc.declares(mc.spec.Needs, ref) {
			return fmt.Errorf("%w: module %q needs %s", ErrPortNotDeclared, mc.spec.Name, ref)
		}
		// Planning matched every need to one provider and ordered it first, and that provider's Provide checked the type and nil, so the value is present.
		v, _ = mc.wiring.value(ref).(T)
		return nil
	})
	return v, err
}

// ValueModule returns a module named name that provides port p with value. Use it for values the command or a test already holds, such as a test double or a shared client; a client that owns connections belongs in its own client module, which registers its component. Startup reports a nil value as [ErrNilPortValue].
func ValueModule[T any](name string, p *Port[T], value T) Module {
	return valueModule[T]{name: name, port: p, value: value}
}

type valueModule[T any] struct {
	name  string
	port  *Port[T]
	value T
}

func (m valueModule[T]) Spec() ModuleSpec {
	return ModuleSpec{Name: m.name, Provides: []PortRef{m.port.Ref()}}
}

func (m valueModule[T]) Register(_ context.Context, mc *ModuleContext) error {
	return Provide(mc, m.port, m.value)
}
