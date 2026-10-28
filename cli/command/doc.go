// Package command runs a tree of subcommands on the standard [flag] package, following the command-line guidelines at https://clig.dev.
//
// A [Program] selects a [Command] from the arguments, parses global and command flags in any order, and runs it. It prints help with examples for "-h", "--help" and "help <command>", suggests the closest command or flag after a typo, and prints bash, zsh and fish completion scripts that ask the program itself for candidates. Usage errors are typed invalid-input errors, so [ExitCode] maps them to [github.com/kbukum/gokit/cli/render.ExitUsage]; an error with its own ExitCode method keeps its code.
//
// The package holds no global state: every Execute builds fresh flag sets from the tree, so a Program is safe to run again and to test with in-memory writers.
package command
