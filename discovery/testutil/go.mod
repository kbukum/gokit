module github.com/kbukum/gokit/discovery/testutil

go 1.25.5

require (
	github.com/kbukum/gokit v0.1.2
	github.com/kbukum/gokit/discovery v0.1.2
	github.com/kbukum/gokit/testutil v0.1.2
)

require (
	github.com/mattn/go-colorable v0.1.14 // indirect
	github.com/mattn/go-isatty v0.0.20 // indirect
	github.com/rs/zerolog v1.34.0 // indirect
	golang.org/x/sys v0.41.0 // indirect
)

replace github.com/kbukum/gokit => ../../

replace github.com/kbukum/gokit/testutil => ../../testutil

replace github.com/kbukum/gokit/discovery => ../../discovery
