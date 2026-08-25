module github.com/kbukum/gokit/dataset

go 1.26.0

toolchain go1.26.6

require (
	github.com/kbukum/gokit v0.3.0-alpha.1
	github.com/kbukum/gokit/schema v0.3.0-alpha.1
)

require (
	github.com/bahlo/generic-list-go v0.2.0 // indirect
	github.com/buger/jsonparser v1.1.2 // indirect
	github.com/fsnotify/fsnotify v1.10.1 // indirect
	github.com/invopop/jsonschema v0.14.0 // indirect
	github.com/klauspost/cpuid/v2 v2.3.0 // indirect
	github.com/pb33f/ordered-map/v2 v2.3.1 // indirect
	github.com/zeebo/blake3 v0.2.4 // indirect
	go.yaml.in/yaml/v4 v4.0.0-rc.6 // indirect
	golang.org/x/sys v0.47.0 // indirect
)

replace (
	github.com/kbukum/gokit => ../
	github.com/kbukum/gokit/schema => ../schema
)
