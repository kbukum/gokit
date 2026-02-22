module github.com/kbukum/gokit/connect

go 1.25.0

require (
	connectrpc.com/connect v1.19.1
	github.com/golang-jwt/jwt/v5 v5.3.1
	github.com/kbukum/gokit v0.0.0
	github.com/kbukum/gokit/auth v0.0.0
	golang.org/x/net v0.50.0
)

require (
	github.com/mattn/go-colorable v0.1.14 // indirect
	github.com/mattn/go-isatty v0.0.20 // indirect
	github.com/rs/zerolog v1.34.0 // indirect
	golang.org/x/crypto v0.48.0 // indirect
	golang.org/x/sys v0.41.0 // indirect
	golang.org/x/text v0.34.0 // indirect
	google.golang.org/protobuf v1.36.11 // indirect
)

replace github.com/kbukum/gokit => ../

replace github.com/kbukum/gokit/auth => ../auth

replace github.com/kbukum/gokit/authz => ../authz
