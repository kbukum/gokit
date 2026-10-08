package bootstrap

import (
	"context"
	"fmt"
)

// Use adds modules to the App. It can be called before the lifecycle starts or from a configure hook. Startup checks the whole module set before any module registers: every needed port must be provided by exactly one module, every listener a module names must be declared with [App.RegisterListener], and modules must not depend on each other in a cycle. All problems are returned together as a [*ModuleError]. Modules then register in dependency order, keeping Use order where they are independent. Use returns [ErrLifecycleUsed] once the modules phase has begun.
func (a *App[C]) Use(modules ...Module) error {
	return a.declare(func(s *moduleSet) { s.modules = append(s.modules, modules...) })
}

// RegisterListener declares a named HTTP listener that modules mount routes on with [ModuleContext.Handle]. The name is what module specs refer to; the listener's component name is what the registry and health checks use. The App registers listener components after every module component, so a listener accepts traffic only after the module resources behind it have started, and on shutdown it stops accepting and drains in-flight requests before module workers drain. Listeners register in RegisterListener order. RegisterListener returns [ErrLifecycleUsed] once the modules phase has begun.
func (a *App[C]) RegisterListener(name string, l Listener) error {
	return a.declare(func(s *moduleSet) { s.listeners = append(s.listeners, namedListener{name: name, l: l}) })
}

// CheckModules validates the modules and listeners declared so far, including listener component names against components already registered, without registering or starting anything, and returns a [*ModuleError] listing every problem, or nil. Startup runs the same check, so use CheckModules to test a service composition without its infrastructure. Declarations a configure hook would add are not included.
func (a *App[C]) CheckModules() error {
	a.lifecycleMu.Lock()
	set := moduleSet{modules: append([]Module(nil), a.modules.modules...), listeners: append([]namedListener(nil), a.modules.listeners...)}
	a.lifecycleMu.Unlock()
	if _, err := planModules(&set, a.componentRegistered); err != nil {
		return err
	}
	return nil
}

func (a *App[C]) componentRegistered(name string) bool { return a.Components.Get(name) != nil }

// declare records a module declaration before the lifecycle starts or while startup runs configure hooks.
func (a *App[C]) declare(fn func(*moduleSet)) error {
	a.lifecycleMu.Lock()
	defer a.lifecycleMu.Unlock()
	if a.modulesSealed || (a.used && !a.starting) {
		return ErrLifecycleUsed
	}
	fn(&a.modules)
	return nil
}

// sealModules closes the module set to further declarations.
func (a *App[C]) sealModules() {
	a.lifecycleMu.Lock()
	defer a.lifecycleMu.Unlock()
	a.modulesSealed = true
}

// wireModules runs the modules phase: close Use and RegisterListener, validate the module set, register modules in dependency order, then register listener components. It runs once per lifecycle.
func (a *App[C]) wireModules(ctx context.Context) error {
	a.sealModules()
	set := &a.modules
	if set.empty() {
		return nil
	}
	plan, modErr := planModules(set, a.componentRegistered)
	if modErr != nil {
		return modErr
	}
	w := &moduleWiring{
		registry:  a.Components,
		listeners: map[string]Listener{},
		mounted:   map[string]map[string]bool{},
		fallbacks: map[string]fallback{},
		values:    map[PortRef]any{},
	}
	for _, nl := range set.listeners {
		w.listeners[nl.name] = nl.l
		w.mounted[nl.name] = map[string]bool{}
	}
	for _, pm := range plan.order {
		if ctx.Err() != nil {
			return context.Cause(ctx)
		}
		if err := a.registerModule(ctx, pm, w, plan.providers); err != nil {
			return err
		}
	}
	if err := w.installFallbacks(); err != nil {
		return fmt.Errorf("bootstrap: %w", err)
	}
	for _, nl := range set.listeners {
		if err := a.Components.Register(nl.l); err != nil {
			return fmt.Errorf("bootstrap: listener %q: %w", nl.name, err)
		}
	}
	return nil
}

func (a *App[C]) registerModule(ctx context.Context, pm plannedModule, w *moduleWiring, providers map[PortRef]string) error {
	name := pm.spec.Name
	info := ModuleInfo{Name: name}
	for _, ref := range pm.spec.Provides {
		info.Provides = append(info.Provides, ref.Name())
	}
	for _, ref := range pm.spec.Needs {
		info.Needs = append(info.Needs, ref.Name()+" ← "+providers[ref])
	}
	mc := &ModuleContext{
		spec:     pm.spec,
		logger:   a.Logger.WithComponent(name),
		wiring:   w,
		info:     &info,
		provided: map[PortRef]bool{},
	}
	err := pm.module.Register(ctx, mc)
	mc.close()
	if err != nil {
		return fmt.Errorf("bootstrap: module %q register: %w", name, err)
	}
	var problems []ModuleProblem
	for _, ref := range pm.spec.Provides {
		if !mc.provided[ref] {
			problems = append(problems, ModuleProblem{Kind: ProblemNotProvided, Port: ref, Modules: []string{name}, Detail: "declared in Provides but not provided by Register"})
		}
	}
	if len(problems) > 0 {
		return &ModuleError{Problems: problems}
	}
	a.Summary.TrackModule(info)
	return nil
}
