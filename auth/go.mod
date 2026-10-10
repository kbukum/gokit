module github.com/kbukum/gokit/auth

go 1.27.1

require (
	github.com/golang-jwt/jwt/v5 v5.3.1
	github.com/kbukum/gokit v0.3.0-alpha.1
	go.uber.org/goleak v1.3.0
	golang.org/x/crypto v0.57.0
)

require (
	github.com/fsnotify/fsnotify v1.10.1 // indirect
	github.com/klauspost/cpuid/v2 v2.4.0 // indirect
	github.com/zeebo/blake3 v0.2.4 // indirect
	golang.org/x/sys v0.48.0 // indirect
)

replace github.com/kbukum/gokit => ../
