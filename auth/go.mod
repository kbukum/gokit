module github.com/kbukum/gokit/auth

go 1.26.0

toolchain go1.26.3

require (
	github.com/golang-jwt/jwt/v5 v5.3.1
	github.com/kbukum/gokit v0.2.0
	golang.org/x/crypto v0.53.0
)

require golang.org/x/sys v0.46.0 // indirect

replace github.com/kbukum/gokit => ../
