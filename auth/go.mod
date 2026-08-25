module github.com/kbukum/gokit/auth

go 1.26.0

toolchain go1.26.6

require (
	github.com/golang-jwt/jwt/v5 v5.3.1
	github.com/kbukum/gokit v0.3.0-alpha.1
	golang.org/x/crypto v0.55.0
)

require golang.org/x/sys v0.47.0 // indirect

replace github.com/kbukum/gokit => ../
