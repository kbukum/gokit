package command

import (
	"flag"
	"fmt"
	"io"
	"strings"
)

// completeCommand is the hidden root word the completion scripts call to list candidates for the words typed so far.
const completeCommand = "__complete"

// completion prints the completion script for one shell. The script asks the program itself for candidates, so it never goes stale.
func (p Program) completion(args []string) error {
	path := []string{p.Root.Name, "completion"}
	if len(args) != 1 {
		return UsageError(path, "completion needs one shell: bash, zsh or fish")
	}
	name := p.Root.Name
	var script string
	switch args[0] {
	case "bash":
		script = fmt.Sprintf(`_%[2]s() {
  local IFS=$'\n'
  COMPREPLY=($(compgen -W "$(%[1]s %[3]s "${COMP_WORDS[@]:1:COMP_CWORD-1}" 2>/dev/null)" -- "${COMP_WORDS[COMP_CWORD]}"))
}
complete -F _%[2]s %[1]s
`, name, ident(name), completeCommand)
	case "zsh":
		script = fmt.Sprintf(`#compdef %[1]s
_%[2]s() {
  local -a candidates
  candidates=(${(f)"$(%[1]s %[3]s ${words[2,CURRENT-1]} 2>/dev/null)"})
  compadd -a candidates
}
compdef _%[2]s %[1]s
`, name, ident(name), completeCommand)
	case "fish":
		script = fmt.Sprintf("complete -c %[1]s -f -a '(%[1]s %[2]s (commandline -opc)[2..-1])'\n", name, completeCommand)
	default:
		return UsageError(path, "unknown shell "+fmt.Sprintf("%q", args[0])+"; use bash, zsh or fish")
	}
	_, err := io.WriteString(p.Stdout, script)
	return err
}

// ident turns a program name into a shell function name.
func ident(name string) string {
	return strings.ReplaceAll(name, "-", "_")
}

// complete prints one candidate per line for the word after words: the subcommands of the command they name, then its flags.
func (p Program) complete(words []string) error {
	path := []*Command{p.Root}
	for _, word := range words {
		if strings.HasPrefix(word, "-") {
			continue
		}
		sub := path[len(path)-1].find(word)
		if sub == nil {
			break
		}
		path = append(path, sub)
	}
	cmd := path[len(path)-1]
	var b strings.Builder
	for _, sub := range cmd.visible() {
		b.WriteString(sub.Name + "\n")
	}
	if len(path) == 1 {
		b.WriteString("completion\nhelp\n")
	}
	own, global := p.flagGroups(path)
	for _, group := range [][]*flag.Flag{own, global} {
		for _, f := range group {
			b.WriteString("--" + f.Name + "\n")
		}
	}
	_, err := io.WriteString(p.Stdout, b.String())
	return err
}
