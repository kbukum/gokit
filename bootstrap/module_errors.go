package bootstrap

import (
	"fmt"
	"strings"
)

// ModuleProblemKind classifies one module wiring problem.
type ModuleProblemKind string

const (
	// ProblemInvalid is an invalid declaration: a nil or unnamed module, a duplicate module name, a nil, unnamed or non-interface port, a port listed twice in one spec, an empty or repeated listener in a module spec, or a nil, unnamed, duplicate or non-ingress listener.
	ProblemInvalid ModuleProblemKind = "invalid"
	// ProblemMissingPort is a needed port that no module provides.
	ProblemMissingPort ModuleProblemKind = "missing_port"
	// ProblemDuplicatePort is a port that more than one module provides, such as a module and its client module in the same App.
	ProblemDuplicatePort ModuleProblemKind = "duplicate_port"
	// ProblemMissingListener is a listener named in module specs that the command did not declare with RegisterListener.
	ProblemMissingListener ModuleProblemKind = "missing_listener"
	// ProblemCycle is a dependency cycle between modules.
	ProblemCycle ModuleProblemKind = "dependency_cycle"
	// ProblemNotProvided is a declared provided port that Register did not provide.
	ProblemNotProvided ModuleProblemKind = "port_not_provided"
)

// ModuleProblem is one wiring problem. Port or Listener names the port or listener when the problem concerns one; Modules lists the modules involved (for a cycle, the path from a module back to itself); Detail adds context.
type ModuleProblem struct {
	Kind     ModuleProblemKind
	Port     PortRef
	Listener string
	Modules  []string
	Detail   string
}

func (p ModuleProblem) String() string {
	var b strings.Builder
	b.WriteString(strings.ReplaceAll(string(p.Kind), "_", " "))
	if p.Port.Name() != "" {
		fmt.Fprintf(&b, " %q", p.Port.Name())
	}
	if p.Listener != "" {
		fmt.Fprintf(&b, " %q", p.Listener)
	}
	if len(p.Modules) > 0 {
		sep := ", "
		if p.Kind == ProblemCycle {
			sep = " → "
		}
		fmt.Fprintf(&b, " [%s]", strings.Join(p.Modules, sep))
	}
	if p.Detail != "" {
		b.WriteString(": " + p.Detail)
	}
	return b.String()
}

// ModuleError reports module wiring problems. Startup (and [App.CheckModules]) checks the declared module set before any module registers and reports every problem it finds at once. The one check that needs Register to run, [ProblemNotProvided], is reported for the module that failed it, after it registered. Startup returns the error as the cause of a [StartupError] in [PhaseModules].
type ModuleError struct {
	Problems []ModuleProblem
}

func (e *ModuleError) Error() string {
	lines := make([]string, len(e.Problems))
	for i, p := range e.Problems {
		lines[i] = p.String()
	}
	return fmt.Sprintf("bootstrap: %d module wiring problem(s): %s", len(e.Problems), strings.Join(lines, "; "))
}
