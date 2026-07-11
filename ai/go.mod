module github.com/kbukum/gokit/ai

go 1.26.0

toolchain go1.26.3

require (
	github.com/kbukum/gokit v0.2.0
	github.com/kbukum/gokit/schema v0.2.0
)

require (
	github.com/bahlo/generic-list-go v0.2.0 // indirect
	github.com/buger/jsonparser v1.1.2 // indirect
	github.com/invopop/jsonschema v0.14.0 // indirect
	github.com/pb33f/ordered-map/v2 v2.3.1 // indirect
	go.yaml.in/yaml/v4 v4.0.0-rc.6 // indirect
	golang.org/x/sys v0.46.0 // indirect
	google.golang.org/grpc v1.82.0 // indirect
)

replace (
	github.com/kbukum/gokit => ../
	github.com/kbukum/gokit/schema => ../schema
)
