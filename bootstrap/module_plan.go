package bootstrap

import (
	"cmp"
	"fmt"
	"net/http"
	"net/url"
	"reflect"
	"slices"
	"strings"
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

// planModules validates modules and listeners and orders modules so every provider registers before the modules that need it. Ties keep Use order. registered reports component names the App already holds, which listener components must not reuse.
func planModules(set *moduleSet, registered func(name string) bool) (*modulePlan, *ModuleError) {
	p := &planner{}
	modules := p.checkModules(set.modules)
	p.checkModuleListeners(modules, p.checkListeners(set.listeners, registered))

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
		}
		out = append(out, plannedModule{module: m, spec: spec})
	}
	return out
}

// checkListeners validates declared listeners and returns their names. The Listener type guarantees a listener quiesces and drains; its drain phase is a value, so it is checked here. Listener components register after modules, so their component names are checked here too, against each other and the components the App already holds, rather than failing after every module registered.
func (p *planner) checkListeners(in []namedListener, registered func(string) bool) map[string]bool {
	seen := map[string]bool{}
	components := map[string][]string{}
	var componentOrder []string
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
		case nl.l.Name() == "":
			problem.Detail = "listener component has no name"
		case registered(nl.l.Name()):
			problem.Detail = fmt.Sprintf("component name %q already registered", nl.l.Name())
		default:
			c := nl.l.Name()
			if len(components[c]) == 0 {
				componentOrder = append(componentOrder, c)
			}
			components[c] = append(components[c], nl.name)
		}
		if nl.name != "" {
			seen[nl.name] = true
		}
		if problem.Detail != "" {
			p.add(problem)
		}
	}
	for _, c := range componentOrder {
		if names := components[c]; len(names) > 1 {
			p.invalid("", fmt.Sprintf("component name %q used by listeners %s", c, strings.Join(names, ", ")))
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

// order sorts modules topologically by provider edges. If modules remain, every dependency cycle among them is reported, one representative path per strongly connected group, so a single check shows them all.
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
			for _, group := range cyclicGroups(deps, done) {
				p.add(ModuleProblem{Kind: ProblemCycle, Modules: cyclePath(modules, deps, group)})
			}
			return nil
		}
		done[next] = true
		order = append(order, modules[next])
	}
	return order
}

// cyclicGroups returns the strongly connected groups of unfinished modules that contain a cycle, ordered by their first module. Modules that only depend on a cycle are not in any group.
func cyclicGroups(deps [][]int, done []bool) [][]int {
	// Tarjan's algorithm over the unfinished modules.
	n := len(deps)
	index, low := make([]int, n), make([]int, n)
	onStack := make([]bool, n)
	for i := range index {
		index[i] = -1
	}
	var stack []int
	var groups [][]int
	counter := 0
	var visit func(v int)
	visit = func(v int) {
		index[v], low[v] = counter, counter
		counter++
		stack = append(stack, v)
		onStack[v] = true
		for _, w := range deps[v] {
			switch {
			case done[w]:
			case index[w] < 0:
				visit(w)
				low[v] = min(low[v], low[w])
			case onStack[w]:
				low[v] = min(low[v], index[w])
			}
		}
		if low[v] != index[v] {
			return
		}
		var group []int
		for {
			w := stack[len(stack)-1]
			stack = stack[:len(stack)-1]
			onStack[w] = false
			group = append(group, w)
			if w == v {
				break
			}
		}
		if len(group) > 1 || slices.Contains(deps[v], v) {
			slices.Sort(group)
			groups = append(groups, group)
		}
	}
	for v := range n {
		if !done[v] && index[v] < 0 {
			visit(v)
		}
	}
	slices.SortFunc(groups, func(a, b []int) int { return cmp.Compare(a[0], b[0]) })
	return groups
}

// cyclePath walks from the group's first module to a provider in the same group until a module repeats. Every module in a cyclic group has a provider in it, so the walk always closes a cycle.
func cyclePath(modules []plannedModule, deps [][]int, group []int) []string {
	visited := map[int]int{}
	var path []int
	for at := group[0]; ; {
		if pos, ok := visited[at]; ok {
			path = append(path[pos:], at)
			break
		}
		visited[at] = len(path)
		path = append(path, at)
		for _, j := range deps[at] {
			if slices.Contains(group, j) {
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
	fallbacks map[string]fallback
	values    map[PortRef]any
}

type fallback struct {
	module  string
	handler http.Handler
}

func (w *moduleWiring) setFallback(listener, module string, handler http.Handler) error {
	w.mu.Lock()
	defer w.mu.Unlock()
	if prev, ok := w.fallbacks[listener]; ok {
		return fmt.Errorf("%w: %s already has a fallback from module %q", ErrRouteConflict, listener, prev.module)
	}
	w.fallbacks[listener] = fallback{module: module, handler: handler}
	return nil
}

// installFallbacks gives each listener its fallback, guarded so it never answers under a prefix a module route owns.
func (w *moduleWiring) installFallbacks() (err error) {
	w.mu.Lock()
	defer w.mu.Unlock()
	for listener, fb := range w.fallbacks {
		var reserved []string
		for pattern := range w.mounted[listener] {
			if prefix := routePrefix(pattern); prefix != "" && !slices.Contains(reserved, prefix) {
				reserved = append(reserved, prefix)
			}
		}
		if err := installFallback(w.listeners[listener], listener, fb, reserved); err != nil {
			return err
		}
	}
	return nil
}

func installFallback(l Listener, listener string, fb fallback, reserved []string) error {
	err := l.Fallback(http.HandlerFunc(func(rw http.ResponseWriter, r *http.Request) {
		// http.ServeMux matches unescaped segments of the escaped path, so the guard compares the same way.
		first, _, _ := strings.Cut(strings.TrimPrefix(r.URL.EscapedPath(), "/"), "/")
		if segment, err := url.PathUnescape(first); err != nil || slices.Contains(reserved, "/"+segment) {
			http.NotFound(rw, r)
			return
		}
		fb.handler.ServeHTTP(rw, r)
	}))
	if err != nil {
		return fmt.Errorf("%w: %s fallback from module %q: %w", ErrRouteConflict, listener, fb.module, err)
	}
	return nil
}

// routePrefix returns the first literal path segment of an http.ServeMux pattern ("[METHOD ][HOST]/[PATH]"), unescaped as the mux matches it, such as "/auth" for "POST /auth/login" or "POST /%61uth/login", or "" for the root, a wildcard segment or an invalid escape.
func routePrefix(pattern string) string {
	_, rest, found := strings.Cut(pattern, " ")
	if !found {
		rest = pattern
	}
	slash := strings.IndexByte(rest, '/')
	if slash < 0 {
		return ""
	}
	segment, _, _ := strings.Cut(rest[slash+1:], "/")
	if segment == "" || strings.Contains(segment, "{") {
		return ""
	}
	unescaped, err := url.PathUnescape(segment)
	if err != nil {
		return ""
	}
	return "/" + unescaped
}

func (w *moduleWiring) mount(listener, pattern string, handler http.Handler) error {
	w.mu.Lock()
	defer w.mu.Unlock()
	// Planning matched every listener a module declares to one the App has, and Handle accepts only declared listeners.
	l := w.listeners[listener]
	if w.mounted[listener][pattern] {
		return fmt.Errorf("%w: %s %q already mounted", ErrRouteConflict, listener, pattern)
	}
	if err := l.Handle(pattern, handler); err != nil {
		return fmt.Errorf("%w: %s %q: %w", ErrRouteConflict, listener, pattern, err)
	}
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
