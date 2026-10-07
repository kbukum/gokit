package bootstrap

import (
	"cmp"
	"fmt"
	"net/http"
	"reflect"
	"slices"
	"sync"

	"github.com/kbukum/gokit/component"
	"github.com/kbukum/gokit/util"
)

type namedListener struct {
	name string
	l    Listener
}

// moduleSet is what the command declared with Use and Listen.
type moduleSet struct {
	modules   []Module
	listeners []namedListener
}

func (s *moduleSet) empty() bool { return len(s.modules) == 0 && len(s.listeners) == 0 }

type plannedModule struct {
	module Module
	spec   ModuleSpec
}

// modulePlan is a validated module set: modules in dependency order and the module providing every port.
type modulePlan struct {
	order     []plannedModule
	providers map[PortRef]string
}

// planner collects every problem in one pass so the command sees all gaps at once.
type planner struct{ problems []ModuleProblem }

func (p *planner) add(problem ModuleProblem) { p.problems = append(p.problems, problem) }

func (p *planner) invalid(owner, detail string) {
	var modules []string
	if owner != "" {
		modules = []string{owner}
	}
	p.add(ModuleProblem{Kind: ProblemInvalid, Modules: modules, Detail: detail})
}

// refs validates one spec list and returns its valid ports without repeats.
func (p *planner) refs(list []PortRef, owner, field string) []PortRef {
	var out []PortRef
	for _, ref := range list {
		switch {
		case !ref.valid():
			p.invalid(owner, field+" lists a nil or zero port; declare ports with NewPort")
		case ref.key.name == "":
			p.invalid(owner, field+" lists an unnamed "+ref.key.typ.String()+" port")
		case ref.key.typ.Kind() != reflect.Interface:
			p.add(ModuleProblem{Kind: ProblemInvalid, Port: ref, Modules: []string{owner}, Detail: "port type " + ref.key.typ.String() + " is not an interface"})
		case slices.Contains(out, ref):
			p.add(ModuleProblem{Kind: ProblemInvalid, Port: ref, Modules: []string{owner}, Detail: field + " lists the port twice"})
		default:
			out = append(out, ref)
		}
	}
	return out
}

// planModules validates modules and listeners and orders modules so every provider registers before the modules that need it. Ties keep Use order.
func planModules(set *moduleSet) (*modulePlan, *ModuleError) {
	p := &planner{}
	modules := p.checkModules(set.modules)
	p.checkModuleListeners(modules, p.checkListeners(set.listeners))

	providers := map[PortRef][]int{}
	needers := map[PortRef][]int{}
	var ports []PortRef
	note := func(ref PortRef) {
		if !slices.Contains(ports, ref) {
			ports = append(ports, ref)
		}
	}
	for i := range modules {
		m := &modules[i]
		m.spec.Provides = p.refs(m.spec.Provides, m.spec.Name, "Provides")
		m.spec.Needs = p.refs(m.spec.Needs, m.spec.Name, "Needs")
		for _, ref := range m.spec.Provides {
			providers[ref] = append(providers[ref], i)
			note(ref)
		}
		for _, ref := range m.spec.Needs {
			needers[ref] = append(needers[ref], i)
			note(ref)
		}
	}

	names := func(idx []int) []string {
		out := make([]string, len(idx))
		for i, j := range idx {
			out[i] = modules[j].spec.Name
		}
		return out
	}
	sources := map[PortRef]string{}
	for _, ref := range ports {
		provs, needs := providers[ref], needers[ref]
		switch {
		case len(provs) > 1:
			p.add(ModuleProblem{Kind: ProblemDuplicatePort, Port: ref, Modules: names(provs)})
		case len(provs) == 0:
			p.add(ModuleProblem{Kind: ProblemMissingPort, Port: ref, Modules: names(needs), Detail: "needed but not provided"})
		default:
			sources[ref] = modules[provs[0]].spec.Name
		}
	}

	order := p.order(modules, providers)
	if len(p.problems) > 0 {
		return nil, &ModuleError{Problems: p.problems}
	}
	return &modulePlan{order: order, providers: sources}, nil
}

func (p *planner) checkModules(in []Module) []plannedModule {
	var out []plannedModule
	seen := map[string]bool{}
	for i, m := range in {
		if util.IsNil(m) {
			p.invalid("", fmt.Sprintf("module %d is nil", i))
			continue
		}
		spec := m.Spec()
		switch {
		case spec.Name == "":
			p.invalid("", fmt.Sprintf("module %d has no name", i))
		case seen[spec.Name]:
			p.invalid(spec.Name, "module name used twice")
		default:
			seen[spec.Name] = true
			out = append(out, plannedModule{module: m, spec: spec})
		}
	}
	return out
}

// checkListeners validates declared listeners and returns their names. The Listener type guarantees a listener quiesces and drains; its drain phase is a value, so it is checked here.
func (p *planner) checkListeners(in []namedListener) map[string]bool {
	seen := map[string]bool{}
	for _, nl := range in {
		problem := ModuleProblem{Kind: ProblemInvalid, Listener: nl.name}
		switch {
		case nl.name == "":
			problem.Detail = "listener has no name"
		case seen[nl.name]:
			problem.Detail = "listener declared twice"
		case util.IsNil(nl.l):
			problem.Detail = "listener is nil"
		case nl.l.DrainPhase() != component.DrainIngress:
			problem.Detail = "listener must drain as component.DrainIngress"
		}
		if nl.name != "" {
			seen[nl.name] = true
		}
		if problem.Detail != "" {
			p.add(problem)
		}
	}
	return seen
}

// checkModuleListeners reports listeners that module specs name but the command did not declare, one problem per listener listing every module that needs it.
func (p *planner) checkModuleListeners(modules []plannedModule, declared map[string]bool) {
	var missing []string
	needers := map[string][]string{}
	for _, m := range modules {
		var listed []string
		for _, name := range m.spec.Listeners {
			switch {
			case name == "":
				p.invalid(m.spec.Name, "module lists an unnamed listener")
			case slices.Contains(listed, name):
				p.add(ModuleProblem{Kind: ProblemInvalid, Listener: name, Modules: []string{m.spec.Name}, Detail: "listener listed twice"})
			default:
				listed = append(listed, name)
				if declared[name] {
					continue
				}
				if len(needers[name]) == 0 {
					missing = append(missing, name)
				}
				needers[name] = append(needers[name], m.spec.Name)
			}
		}
	}
	for _, name := range missing {
		p.add(ModuleProblem{Kind: ProblemMissingListener, Listener: name, Modules: needers[name], Detail: "not declared with Listen"})
	}
}

// order sorts modules topologically by provider edges and reports a cycle if one remains.
func (p *planner) order(modules []plannedModule, providers map[PortRef][]int) []plannedModule {
	deps := make([][]int, len(modules))
	for i, m := range modules {
		for _, ref := range m.spec.Needs {
			for _, j := range providers[ref] {
				if !slices.Contains(deps[i], j) {
					deps[i] = append(deps[i], j)
				}
			}
		}
		slices.SortFunc(deps[i], cmp.Compare)
	}
	done := make([]bool, len(modules))
	var order []plannedModule
	for len(order) < len(modules) {
		next := -1
		for i := range modules {
			if !done[i] && !slices.ContainsFunc(deps[i], func(j int) bool { return !done[j] }) {
				next = i
				break
			}
		}
		if next < 0 {
			p.add(ModuleProblem{Kind: ProblemCycle, Modules: cyclePath(modules, deps, done)})
			return nil
		}
		done[next] = true
		order = append(order, modules[next])
	}
	return order
}

// cyclePath walks from the first unfinished module to an unfinished provider until a module repeats. Every unfinished module has an unfinished provider, so the walk always closes a cycle.
func cyclePath(modules []plannedModule, deps [][]int, done []bool) []string {
	start := slices.Index(done, false)
	visited := map[int]int{}
	var path []int
	for at := start; ; {
		if pos, ok := visited[at]; ok {
			path = append(path[pos:], at)
			break
		}
		visited[at] = len(path)
		path = append(path, at)
		for _, j := range deps[at] {
			if !done[j] {
				at = j
				break
			}
		}
	}
	out := make([]string, len(path))
	for i, j := range path {
		out[i] = modules[j].spec.Name
	}
	return out
}

// moduleWiring is the state module contexts share during the modules phase.
type moduleWiring struct {
	mu        sync.Mutex
	registry  *component.Registry
	listeners map[string]Listener
	mounted   map[string]map[string]bool
	values    map[PortRef]any
}

func (w *moduleWiring) mount(listener, pattern string, handler http.Handler) (err error) {
	w.mu.Lock()
	defer w.mu.Unlock()
	// Planning matched every listener a module declares to one the App has, and Handle accepts only declared listeners.
	l := w.listeners[listener]
	if w.mounted[listener][pattern] {
		return fmt.Errorf("%w: %s %q already mounted", ErrRouteConflict, listener, pattern)
	}
	defer func() {
		// http.ServeMux reports invalid and conflicting patterns by panicking; startup reports them as errors.
		if r := recover(); r != nil {
			err = fmt.Errorf("%w: %s %q: %v", ErrRouteConflict, listener, pattern, r)
		}
	}()
	l.Handle(pattern, handler)
	w.mounted[listener][pattern] = true
	return nil
}

func (w *moduleWiring) provide(ref PortRef, value any) {
	w.mu.Lock()
	defer w.mu.Unlock()
	w.values[ref] = value
}

func (w *moduleWiring) value(ref PortRef) any {
	w.mu.Lock()
	defer w.mu.Unlock()
	return w.values[ref]
}
