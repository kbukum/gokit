package command_test

import (
	"bytes"
	"context"
	"flag"
	"strings"
	"testing"

	"github.com/kbukum/gokit/cli/command"
	"github.com/kbukum/gokit/cli/render"
	"github.com/kbukum/gokit/errors"
)

type recorded struct {
	path []string
	args []string
	json bool
	out  string
}

// tree builds a small program: a global --json flag, a "gate" group with "run" and "list", and a leaf "verify".
func tree(got *recorded) *command.Command {
	var jsonOut bool
	var outFile string
	record := func(_ context.Context, inv *command.Invocation) error {
		*got = recorded{path: inv.Path, args: inv.Args, json: jsonOut, out: outFile}
		return nil
	}
	return &command.Command{
		Name:  "tool",
		Flags: func(fs *flag.FlagSet) { fs.BoolVar(&jsonOut, "json", false, "Print JSON") },
		Commands: []*command.Command{
			{
				Name: "gate", Summary: "Work with gates",
				Commands: []*command.Command{
					{
						Name: "run", Summary: "Run a gate", Args: "<gate>", Run: record,
						Flags:    func(fs *flag.FlagSet) { fs.StringVar(&outFile, "out", "", "Write the `file`") },
						Examples: []command.Example{{Description: "Run the main gate", Command: "tool gate run main"}},
					},
					{Name: "list", Aliases: []string{"ls"}, Summary: "List gates", Run: record},
				},
			},
			{Name: "verify", Summary: "Verify a file", Run: record},
			{Name: "secret", Hidden: true, Run: record},
		},
	}
}

func execute(t *testing.T, args ...string) (got recorded, stdout, stderr string, err error) {
	t.Helper()
	var out, errOut bytes.Buffer
	err = command.Program{Root: tree(&got), Version: "1.2.3", Stdout: &out, Stderr: &errOut}.Execute(context.Background(), args)
	return got, out.String(), errOut.String(), err
}

func TestExecuteDispatchesWithFlagsInAnyOrder(t *testing.T) {
	cases := map[string][]string{
		"global first":     {"--json", "gate", "run", "--out", "f", "main", "extra"},
		"global last":      {"gate", "run", "main", "--out=f", "extra", "--json"},
		"global between":   {"gate", "--json", "run", "main", "--out", "f", "extra"},
		"dash ends flags":  {"gate", "run", "--json", "--out", "f", "--", "main", "extra"},
		"positional first": {"gate", "run", "main", "extra", "--json", "--out", "f"},
	}
	for name, args := range cases {
		t.Run(name, func(t *testing.T) {
			got, _, _, err := execute(t, args...)
			if err != nil {
				t.Fatalf("Execute: %v", err)
			}
			if strings.Join(got.path, " ") != "tool gate run" || strings.Join(got.args, ",") != "main,extra" || !got.json || got.out != "f" {
				t.Fatalf("got %+v", got)
			}
		})
	}
}

func TestExecuteKeepsArgumentsAfterDashDash(t *testing.T) {
	got, _, _, err := execute(t, "gate", "run", "--", "--json", "-x")
	if err != nil {
		t.Fatalf("Execute: %v", err)
	}
	if got.json || strings.Join(got.args, ",") != "--json,-x" {
		t.Fatalf("got %+v", got)
	}
}

func TestExecuteSelectsAlias(t *testing.T) {
	got, _, _, err := execute(t, "gate", "ls")
	if err != nil || strings.Join(got.path, " ") != "tool gate list" {
		t.Fatalf("got %+v, %v", got, err)
	}
}

func TestExecuteSuggestsClosestCommandAndFlag(t *testing.T) {
	cases := map[string]struct {
		args []string
		want string
	}{
		"swapped letters": {[]string{"gaet", "run"}, `did you mean gate?`},
		"prefix":          {[]string{"ver"}, `did you mean verify?`},
		"nested":          {[]string{"gate", "rnu"}, `unknown command "rnu" for "tool gate"; did you mean run?`},
		"flag":            {[]string{"gate", "run", "--jsn"}, "unknown flag --jsn; did you mean --json?"},
		"no match":        {[]string{"zzzzzz"}, `unknown command "zzzzzz" for "tool"`},
	}
	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			_, _, _, err := execute(t, tc.args...)
			if err == nil || !strings.Contains(err.Error(), tc.want) {
				t.Fatalf("error = %v, want %q", err, tc.want)
			}
			if command.ExitCode(err) != render.ExitUsage {
				t.Fatalf("exit = %d, want usage", command.ExitCode(err))
			}
		})
	}
}

func TestExecuteNeverSuggestsHiddenCommands(t *testing.T) {
	_, _, _, err := execute(t, "secrte")
	if err == nil || strings.Contains(err.Error(), "secret") {
		t.Fatalf("error = %v", err)
	}
}

func TestGroupWithoutCommandPrintsHelpAndFails(t *testing.T) {
	_, stdout, stderr, err := execute(t, "gate")
	if command.ExitCode(err) != render.ExitUsage || stdout != "" || !strings.Contains(stderr, "Usage: tool gate <command>") {
		t.Fatalf("err %v stdout %q stderr %q", err, stdout, stderr)
	}
}

func TestHelpShowsUsageFlagsAndExamples(t *testing.T) {
	for _, args := range [][]string{{"gate", "run", "--help"}, {"help", "gate", "run"}, {"gate", "run", "-h"}} {
		_, stdout, _, err := execute(t, args...)
		if err != nil {
			t.Fatalf("%v: %v", args, err)
		}
		for _, want := range []string{"Usage: tool gate run [flags] <gate>", "--out file", "Write the file", "Global flags:", "--json", "# Run the main gate", "$ tool gate run main"} {
			if !strings.Contains(stdout, want) {
				t.Fatalf("%v: help lacks %q:\n%s", args, want, stdout)
			}
		}
	}
	_, stdout, _, err := execute(t, "--help")
	if err != nil || !strings.Contains(stdout, "gate ") || !strings.Contains(stdout, "completion") || strings.Contains(stdout, "secret") {
		t.Fatalf("root help: %v\n%s", err, stdout)
	}
}

func TestVersion(t *testing.T) {
	_, stdout, _, err := execute(t, "verify", "--version")
	if err != nil || stdout != "tool 1.2.3\n" {
		t.Fatalf("%q %v", stdout, err)
	}
}

func TestCompletionScriptsAndCandidates(t *testing.T) {
	for shell, want := range map[string]string{"bash": "complete -F _tool tool", "zsh": "compdef _tool tool", "fish": "complete -c tool"} {
		_, stdout, _, err := execute(t, "completion", shell)
		if err != nil || !strings.Contains(stdout, want) || !strings.Contains(stdout, "__complete") {
			t.Fatalf("%s: %v\n%s", shell, err, stdout)
		}
	}
	if _, _, _, err := execute(t, "completion", "tcsh"); command.ExitCode(err) != render.ExitUsage {
		t.Fatalf("unknown shell: %v", err)
	}
	_, stdout, _, err := execute(t, "__complete", "--json", "gate", "run")
	if err != nil || stdout != "--out\n--json\n--version\n" {
		t.Fatalf("leaf candidates: %v %q", err, stdout)
	}
	_, stdout, _, _ = execute(t, "__complete")
	if !strings.HasPrefix(stdout, "gate\nverify\ncompletion\nhelp\n") || strings.Contains(stdout, "secret") {
		t.Fatalf("root candidates: %q", stdout)
	}
}

func TestInvalidTreeIsRefused(t *testing.T) {
	for name, root := range map[string]*command.Command{
		"bad name":     {Name: "Tool", Run: func(context.Context, *command.Invocation) error { return nil }},
		"empty group":  {Name: "tool", Commands: []*command.Command{{Name: "x"}}},
		"reserved":     {Name: "tool", Commands: []*command.Command{{Name: "help", Run: func(context.Context, *command.Invocation) error { return nil }}}},
		"repeated":     {Name: "tool", Commands: []*command.Command{{Name: "a", Run: func(context.Context, *command.Invocation) error { return nil }}, {Name: "b", Aliases: []string{"a"}, Run: func(context.Context, *command.Invocation) error { return nil }}}},
		"flag clashes": {Name: "tool", Flags: func(fs *flag.FlagSet) { fs.Bool("json", false, "") }, Commands: []*command.Command{{Name: "a", Flags: func(fs *flag.FlagSet) { fs.Bool("json", false, "") }, Run: func(context.Context, *command.Invocation) error { return nil }}}},
	} {
		t.Run(name, func(t *testing.T) {
			var out bytes.Buffer
			err := command.Program{Root: root, Stdout: &out, Stderr: &out}.Execute(context.Background(), []string{"a"})
			if e, ok := errors.AsAppError(err); !ok || e.Code != errors.ErrCodeInvalidInput {
				t.Fatalf("err = %v", err)
			}
		})
	}
}

type codedError struct{}

func (codedError) Error() string             { return "regression" }
func (codedError) ExitCode() render.ExitCode { return 10 }

func TestExitCodeKeepsAnErrorsOwnCode(t *testing.T) {
	if got := command.ExitCode(codedError{}); got != 10 {
		t.Fatalf("exit = %d", got)
	}
	if got := command.ExitCode(errors.Unauthorized("revoked")); got != render.ExitPermission {
		t.Fatalf("exit = %d", got)
	}
	if got := command.ExitCode(nil); got != render.ExitSuccess {
		t.Fatalf("exit = %d", got)
	}
}
