// Package main deliberately fails type checking so build diagnostics can be asserted.
package main

func main() {
	var count int = "not a number"
	_ = count
}
