module github.com/kbukum/gokit/vectorstore/qdrant

go 1.26.0

toolchain go1.26.6

require (
	github.com/google/uuid v1.6.0
	github.com/kbukum/gokit/vectorstore v0.3.0-alpha.1
)

require github.com/kbukum/gokit v0.3.0-alpha.1 // indirect

replace (
	github.com/kbukum/gokit => ../../
	github.com/kbukum/gokit/vectorstore => ../
)
