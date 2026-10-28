package command

import (
	"flag"
	"fmt"
	"io"
	"strings"
	"text/tabwriter"
)

// helpFor prints the help of the command named by words, as in "help gate run".
func (p Program) helpFor(words []string) error {
	path := []*Command{p.Root}
	for _, word := range words {
		sub := path[len(path)-1].find(word)
		if sub == nil {
			return p.unknown(path, word)
		}
		path = append(path, sub)
	}
	return p.help(path, p.Stdout)
}

// help writes the usage, description, subcommands, flags and examples of the command at the end of path.
func (p Program) help(path []*Command, w io.Writer) error {
	cmd := path[len(path)-1]
	var b strings.Builder
	usage := strings.Join(names(path), " ")
	if len(cmd.Commands) > 0 {
		usage += " <command>"
	}
	usage += " [flags]"
	if cmd.Args != "" {
		usage += " " + cmd.Args
	}
	fmt.Fprintf(&b, "Usage: %s\n", usage)
	if text := firstNonEmpty(cmd.Description, cmd.Summary); text != "" {
		fmt.Fprintf(&b, "\n%s\n", text)
	}
	if subs := cmd.visible(); len(subs) > 0 {
		b.WriteString("\nCommands:\n")
		tw := tabwriter.NewWriter(&b, 0, 0, 3, ' ', 0)
		for _, sub := range subs {
			_, _ = fmt.Fprintf(tw, "  %s\t%s\n", sub.Name, sub.Summary)
		}
		if len(path) == 1 {
			_, _ = fmt.Fprintf(tw, "  %s\t%s\n", "completion", "Print a shell completion script (bash, zsh or fish)")
			_, _ = fmt.Fprintf(tw, "  %s\t%s\n", "help", "Show help for a command")
		}
		_ = tw.Flush()
	}
	own, global := p.flagGroups(path)
	writeFlags(&b, "Flags", own)
	writeFlags(&b, "Global flags", global)
	if len(cmd.Examples) > 0 {
		b.WriteString("\nExamples:\n")
		for i, ex := range cmd.Examples {
			if i > 0 {
				b.WriteString("\n")
			}
			if ex.Description != "" {
				fmt.Fprintf(&b, "  # %s\n", ex.Description)
			}
			fmt.Fprintf(&b, "  $ %s\n", ex.Command)
		}
	}
	if len(cmd.Commands) > 0 {
		fmt.Fprintf(&b, "\nRun \"%s help <command>\" for more about a command.\n", p.Root.Name)
	}
	_, err := io.WriteString(w, b.String())
	return err
}

// flagGroups lists the command's own flags and the global flags separately; on the root they are one group.
func (p Program) flagGroups(path []*Command) (own, global []*flag.Flag) {
	globals := flag.NewFlagSet("", flag.ContinueOnError)
	if p.Root.Flags != nil {
		p.Root.Flags(globals)
	}
	if p.Version != "" && globals.Lookup("version") == nil {
		globals.Bool("version", false, "Print the version")
	}
	globals.VisitAll(func(f *flag.Flag) { global = append(global, f) })
	cmd := path[len(path)-1]
	if len(path) == 1 {
		return global, nil
	}
	if cmd.Flags != nil {
		set := flag.NewFlagSet("", flag.ContinueOnError)
		cmd.Flags(set)
		set.VisitAll(func(f *flag.Flag) { own = append(own, f) })
	}
	return own, global
}

func writeFlags(b *strings.Builder, title string, flags []*flag.Flag) {
	if len(flags) == 0 {
		return
	}
	fmt.Fprintf(b, "\n%s:\n", title)
	tw := tabwriter.NewWriter(b, 0, 0, 3, ' ', 0)
	for _, f := range flags {
		name, usage := flag.UnquoteUsage(f)
		left := "--" + f.Name
		if name != "" {
			left += " " + name
		}
		if def := f.DefValue; def != "" && def != "false" && def != "0" && def != "[]" {
			usage += fmt.Sprintf(" (default %s)", def)
		}
		_, _ = fmt.Fprintf(tw, "  %s\t%s\n", left, usage)
	}
	_ = tw.Flush()
}

func firstNonEmpty(values ...string) string {
	for _, v := range values {
		if v != "" {
			return v
		}
	}
	return ""
}
