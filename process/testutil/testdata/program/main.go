package main

import (
	"fmt"
	"runtime/debug"
)

func main() {
	info, ok := debug.ReadBuildInfo()
	if !ok {
		return
	}
	for _, setting := range info.Settings {
		fmt.Printf("%s=%s\n", setting.Key, setting.Value)
	}
}
