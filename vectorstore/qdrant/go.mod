module github.com/kbukum/gokit/vectorstore/qdrant

go 1.26.0

toolchain go1.26.5

require (
	github.com/google/uuid v1.6.0
	github.com/kbukum/gokit/vectorstore v0.2.0
)

require github.com/kbukum/gokit v0.2.0 // indirect

replace (
	github.com/kbukum/gokit => ../../
	github.com/kbukum/gokit/vectorstore => ../
)
