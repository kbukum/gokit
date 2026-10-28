package command

import (
	"context"
	stderrors "errors"
	"flag"
	"fmt"
	"io"
	"slices"
	"strings"

	"github.com/kbukum/gokit/cli/render"
	"github.com/kbukum/gokit/errors"
)

// Program runs a command tree over a list of arguments.
type Program struct {
	// Root is the program itself; its Name is the program name and its Flags are global.
	Root *Command
	// Version is printed by --version when it is set.
	Version string
	// Stdout and Stderr receive the program's output. Help goes to Stdout when asked for and to Stderr with a usage error.
	Stdout, Stderr io.Writer
}

// Execute selects the command named by args, parses its flags and runs it.
//
// Global flags may come before or after command words, and flags and positional arguments may be mixed; "--" ends the flags. "-h", "--help" and "help <command>" print help and return nil. An unknown command or flag returns a usage error that suggests the closest match. "completion bash|zsh|fish" prints a completion script.
func (p Program) Execute(ctx context.Context, args []string) error {
	if p.Root == nil || p.Stdout == nil || p.Stderr == nil {
		return errors.InvalidInput("program", "Program needs a root command and both output writers")
	}
	if err := validate(p.Root, nil); err != nil {
		return err
	}
	path := []*Command{p.Root}
	rest := slices.Clone(args)
	for {
		cmd := path[len(path)-1]
		if cmd.Run != nil && len(cmd.Commands) == 0 {
			break
		}
		flags, err := p.flagSet(path, false)
		if err != nil {
			return err
		}
		// Leading global flags are parsed only to find the next word; they are parsed again with the command's own flags.
		if err := p.parse(flags, path, rest, false); err != nil {
			if stderrors.Is(err, flag.ErrHelp) {
				return p.help(path, p.Stdout)
			}
			return err
		}
		consumed := len(rest) - flags.NArg()
		if flags.NArg() == 0 || consumed > 0 && rest[consumed-1] == "--" {
			break
		}
		word := flags.Arg(0)
		if len(path) == 1 {
			switch word {
			case "help":
				return p.helpFor(flags.Args()[1:])
			case "completion":
				return p.completion(flags.Args()[1:])
			case completeCommand:
				return p.complete(flags.Args()[1:])
			}
		}
		sub := cmd.find(word)
		if sub == nil {
			if cmd.Run != nil {
				break
			}
			return p.unknown(path, word)
		}
		path = append(path, sub)
		rest = append(slices.Clip(rest[:consumed]), flags.Args()[1:]...)
	}
	return p.run(ctx, path, rest)
}

func (p Program) run(ctx context.Context, path []*Command, args []string) error {
	cmd := path[len(path)-1]
	flags, err := p.flagSet(path, true)
	if err != nil {
		return err
	}
	if err := p.parse(flags, path, args, true); err != nil {
		if stderrors.Is(err, flag.ErrHelp) {
			return p.help(path, p.Stdout)
		}
		return err
	}
	if p.Version != "" && isSet(flags, "version") {
		_, err := fmt.Fprintf(p.Stdout, "%s %s\n", p.Root.Name, p.Version)
		return err
	}
	if cmd.Run == nil {
		if flags.NArg() > 0 {
			return p.unknown(path, flags.Arg(0))
		}
		if err := p.help(path, p.Stderr); err != nil {
			return err
		}
		return UsageError(names(path), strings.Join(names(path), " ")+" needs a command")
	}
	return cmd.Run(ctx, &Invocation{Path: names(path), Args: flags.Args(), Flags: flags, Stdout: p.Stdout, Stderr: p.Stderr})
}

// flagSet builds the flags for the command at the end of path: the root's global flags, then the command's own when leaf is true.
func (p Program) flagSet(path []*Command, leaf bool) (*flag.FlagSet, error) {
	flags := flag.NewFlagSet(strings.Join(names(path), " "), flag.ContinueOnError)
	flags.SetOutput(io.Discard)
	flags.Usage = func() {}
	if p.Root.Flags != nil {
		p.Root.Flags(flags)
	}
	if p.Version != "" && flags.Lookup("version") == nil {
		flags.Bool("version", false, "Print the version")
	}
	cmd := path[len(path)-1]
	if !leaf || len(path) == 1 || cmd.Flags == nil {
		return flags, nil
	}
	own := flag.NewFlagSet("", flag.ContinueOnError)
	cmd.Flags(own)
	var clash error
	own.VisitAll(func(f *flag.Flag) {
		if clash == nil && flags.Lookup(f.Name) != nil {
			clash = errors.InvalidInput("command", "Flag --"+f.Name+" of "+strings.Join(names(path), " ")+" repeats a global flag")
		}
	})
	if clash != nil {
		return nil, clash
	}
	cmd.Flags(flags)
	return flags, nil
}

// parse parses args into flags. With interspersed set, positional arguments may come between flags; otherwise parsing stops at the first one.
func (p Program) parse(flags *flag.FlagSet, path []*Command, args []string, interspersed bool) error {
	var positional []string
	for {
		if err := checkFlags(flags, path, args); err != nil {
			return err
		}
		if err := flags.Parse(args); err != nil {
			if stderrors.Is(err, flag.ErrHelp) {
				return err
			}
			return UsageError(names(path), err.Error())
		}
		rest := flags.Args()
		consumed := len(args) - len(rest)
		if !interspersed || len(rest) == 0 {
			break
		}
		if consumed > 0 && args[consumed-1] == "--" {
			positional = append(positional, rest...)
			break
		}
		positional = append(positional, rest[0])
		args = rest[1:]
	}
	if interspersed {
		// Parsing "--" alone leaves every flag value as it is and sets Args to the collected positional arguments.
		if err := flags.Parse(append([]string{"--"}, positional...)); err != nil {
			return UsageError(names(path), err.Error())
		}
	}
	return nil
}

// checkFlags finds the first unknown flag before parsing so the error can suggest the closest known one. It stops at the first positional argument, as the parse does.
func checkFlags(flags *flag.FlagSet, path []*Command, args []string) error {
	for i := 0; i < len(args); i++ {
		a := args[i]
		if a == "--" || a == "-" || !strings.HasPrefix(a, "-") {
			return nil
		}
		name, _, hasValue := strings.Cut(strings.TrimLeft(a, "-"), "=")
		if name == "h" || name == "help" {
			continue
		}
		if f := flags.Lookup(name); f != nil {
			if b, ok := f.Value.(interface{ IsBoolFlag() bool }); !hasValue && (!ok || !b.IsBoolFlag()) {
				i++ // the next argument is the flag's value
			}
			continue
		}
		var known []string
		flags.VisitAll(func(f *flag.Flag) { known = append(known, f.Name) })
		message := "unknown flag --" + name
		if s := suggest(name, known); len(s) > 0 {
			message += "; did you mean --" + strings.Join(s, " or --") + "?"
		}
		return UsageError(names(path), message)
	}
	return nil
}

func (p Program) unknown(path []*Command, word string) error {
	cmd := path[len(path)-1]
	var known []string
	for _, sub := range cmd.visible() {
		known = append(known, sub.Name)
		known = append(known, sub.Aliases...)
	}
	if len(path) == 1 {
		known = append(known, "help", "completion")
	}
	message := fmt.Sprintf("unknown command %q for %q", word, strings.Join(names(path), " "))
	if s := suggest(word, known); len(s) > 0 {
		message += "; did you mean " + strings.Join(s, " or ") + "?"
	}
	return UsageError(names(path), message)
}

func names(path []*Command) []string {
	out := make([]string, len(path))
	for i, c := range path {
		out[i] = c.Name
	}
	return out
}

// ExitCode maps an error from [Program.Execute] onto the gokit exit-code convention. An error that carries its own code through an ExitCode method keeps it.
func ExitCode(err error) render.ExitCode {
	var coded interface{ ExitCode() render.ExitCode }
	if stderrors.As(err, &coded) {
		return coded.ExitCode()
	}
	return render.ExitCodeForError(err)
}
