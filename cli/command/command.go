package command

import (
	"context"
	"flag"
	"io"
	"regexp"
	"slices"
	"strings"

	"github.com/kbukum/gokit/errors"
)

// Example is one documented use of a command.
type Example struct {
	// Description says what the example does, in a short sentence.
	Description string
	// Command is the full command line, starting with the program name.
	Command string
}

// Command is one node of a command tree. A command with subcommands and no Run is a group: running it alone prints its help and fails with a usage error.
type Command struct {
	// Name is the word that selects the command: lowercase letters, digits and hyphens.
	Name string
	// Aliases are other words that select the command.
	Aliases []string
	// Summary is a one-line description shown in lists.
	Summary string
	// Description is shown under the usage line of the command's help.
	Description string
	// Args describes the positional arguments in the usage line, such as "<run> [file]".
	Args string
	// Examples are shown at the end of the command's help.
	Examples []Example
	// Flags declares the command's flags. On the root command they are global: every command accepts them, before or after its name.
	Flags func(*flag.FlagSet)
	// Run runs the command. It is nil on a group.
	Run func(context.Context, *Invocation) error
	// Commands are the subcommands, listed in this order.
	Commands []*Command
	// Hidden leaves the command out of help and completions.
	Hidden bool
}

// Invocation is what a command receives when it runs.
type Invocation struct {
	// Path is the command words from the program name to this command.
	Path []string
	// Args are the positional arguments left after the flags.
	Args []string
	// Flags holds the global flags and the command's own flags, already parsed.
	Flags *flag.FlagSet
	// Stdout receives results; Stderr receives progress and diagnostics.
	Stdout, Stderr io.Writer
}

// Set reports whether the named flag was given on the command line.
func (inv *Invocation) Set(name string) bool {
	return isSet(inv.Flags, name)
}

func isSet(flags *flag.FlagSet, name string) bool {
	set := false
	flags.Visit(func(f *flag.Flag) { set = set || f.Name == name })
	return set
}

// ReasonUsage marks an error in how the program was called: an unknown command or flag, or missing or extra arguments.
const ReasonUsage = "CLI_USAGE"

// UsageError returns an invalid-input error for the command at path. Commands use it for argument errors so they exit like parse errors.
func UsageError(path []string, message string) *errors.AppError {
	return errors.InvalidInput(strings.Join(path, " "), message).WithReason(ReasonUsage)
}

var namePattern = regexp.MustCompile(`^[a-z][a-z0-9-]{0,31}$`)

// reserved are the root words the program provides itself.
var reserved = []string{"help", "completion", completeCommand}

// validate checks the tree before it runs: names are well formed and unique among siblings, and every command runs or has subcommands.
func validate(c *Command, path []string) error {
	path = append(slices.Clip(path), c.Name)
	where := strings.Join(path, " ")
	if !namePattern.MatchString(c.Name) {
		return errors.InvalidInput("command", "Command name "+where+" must be lowercase letters, digits and hyphens")
	}
	if c.Run == nil && len(c.Commands) == 0 {
		return errors.InvalidInput("command", "Command "+where+" has neither Run nor subcommands")
	}
	seen := map[string]bool{}
	if len(path) == 1 {
		for _, word := range reserved {
			seen[word] = true
		}
	}
	for _, sub := range c.Commands {
		for _, word := range append([]string{sub.Name}, sub.Aliases...) {
			if !namePattern.MatchString(word) || seen[word] {
				return errors.InvalidInput("command", "Command word "+word+" under "+where+" is invalid or repeated")
			}
			seen[word] = true
		}
		if err := validate(sub, path); err != nil {
			return err
		}
	}
	return nil
}

// find returns the subcommand selected by word, hidden or not.
func (c *Command) find(word string) *Command {
	for _, sub := range c.Commands {
		if sub.Name == word || slices.Contains(sub.Aliases, word) {
			return sub
		}
	}
	return nil
}

// visible lists the subcommands shown in help and completions.
func (c *Command) visible() []*Command {
	out := make([]*Command, 0, len(c.Commands))
	for _, sub := range c.Commands {
		if !sub.Hidden {
			out = append(out, sub)
		}
	}
	return out
}
