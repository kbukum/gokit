module github.com/kbukum/gokit/redis/testutil

go 1.25.0

require (
	github.com/alicebob/miniredis/v2 v2.34.0
	github.com/kbukum/gokit v0.1.1
	github.com/kbukum/gokit/testutil v0.1.1
	github.com/redis/go-redis/v9 v9.18.0
)

require (
	github.com/alicebob/gopher-json v0.0.0-20230218143504-906a9b012302 // indirect
	github.com/cespare/xxhash/v2 v2.3.0 // indirect
	github.com/dgryski/go-rendezvous v0.0.0-20200823014737-9f7001d12a5f // indirect
	github.com/mattn/go-colorable v0.1.14 // indirect
	github.com/mattn/go-isatty v0.0.20 // indirect
	github.com/rs/zerolog v1.34.0 // indirect
	github.com/yuin/gopher-lua v1.1.1 // indirect
	go.uber.org/atomic v1.11.0 // indirect
	golang.org/x/sys v0.41.0 // indirect
)

replace github.com/kbukum/gokit => ../../

replace github.com/kbukum/gokit/testutil => ../../testutil
