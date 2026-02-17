module github.com/kbukum/gokit/auth

go 1.26.0

require (
	github.com/golang-jwt/jwt/v5 v5.3.1
	golang.org/x/crypto v0.48.0
)

require golang.org/x/sys v0.41.0 // indirect

replace github.com/kbukum/gokit => ../
