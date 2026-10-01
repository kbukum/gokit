module github.com/kbukum/gokit/ai

go 1.27.1

require (
	github.com/kbukum/gokit v0.3.0-alpha.1
	github.com/kbukum/gokit/schema v0.3.0-alpha.1
)

require (
	github.com/bahlo/generic-list-go v0.2.0 // indirect
	github.com/buger/jsonparser v1.6.1 // indirect
	github.com/invopop/jsonschema v0.14.0 // indirect
	github.com/pb33f/go-yaml v0.1.1 // indirect
	github.com/pb33f/ordered-map/v2 v2.3.2 // indirect
	github.com/santhosh-tekuri/jsonschema/v6 v6.0.3 // indirect
	golang.org/x/text v0.42.0 // indirect
)

replace (
	github.com/kbukum/gokit => ../
	github.com/kbukum/gokit/schema => ../schema
)
