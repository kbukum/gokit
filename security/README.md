# gokit/security

TLS configuration, secure header policies,
and test helpers for secure transport across gokit modules.

## Overview

The `security` package provides:

- `TLSConfig` for TLS 1.2+ transport policy, CA bundles, and mTLS
- `HeadersConfig` for secure-by-default HTTP response headers

Configuration fields are tagged for YAML and mapstructure,
so they integrate directly with gokit's config loading.

The locked transport policy is explicit:

- minimum supported floor: TLS 1.2
- default negotiation outcome: TLS 1.3 whenever both peers support it
- explicit floors below TLS 1.2 are rejected during validation
- secure headers default-on: HSTS, CSP, X-Content-Type-Options, X-Frame-Options, Referrer-Policy,
  Permissions-Policy

The companion `tlstest` sub-package generates self-signed certificates for integration tests —
no external tools or fixtures required.

## Installation

```bash
go get github.com/kbukum/gokit
```

> `security` is part of the core module — no separate `go get` needed.

## Quick Start

```go
package main

import (
	"crypto/tls"
	"fmt"

	"github.com/kbukum/gokit/security"
)

func main() {
	cfg := security.TLSConfig{
		CAFile:     "/etc/certs/ca.pem",
		CertFile:   "/etc/certs/client.pem",
		KeyFile:    "/etc/certs/client-key.pem",
		ServerName: "api.example.com",
		MinVersion: tls.VersionTLS13,
	}

	if err := cfg.Validate(); err != nil {
		panic(err)
	}

	tlsConfig, err := cfg.Build()
	if err != nil {
		panic(err)
	}

	fmt.Println(tlsConfig != nil) // true
}
```

## API Reference

### TLSConfig

| Field | Type | Description |
|-------|------|-------------|
| `SkipVerify` | `bool` | Disable server certificate verification (not for production) |
| `CAFile` | `string` | Path to CA certificate PEM file |
| `CertFile` | `string` | Path to client certificate PEM (for mTLS) |
| `KeyFile` | `string` | Path to client private key PEM (for mTLS) |
| `ServerName` | `string` | Override server name for certificate verification (SNI) |
| `MinVersion` | `uint16` | Minimum TLS version; defaults to TLS 1.2 |

### HeadersConfig

| Field | Type | Description |
|-------|------|-------------|
| `Disabled` | `bool` | Disable response-header injection entirely |
| `HSTSMaxAge` | `time.Duration` | Strict-Transport-Security max-age |
| `DisableHSTSIncludeSubdomains` | `bool` | Omit `includeSubDomains` |
| `DisableHSTSPreload` | `bool` | Omit `preload` |
| `ContentSecurityPolicy` | `string` | Content-Security-Policy value |
| `ReferrerPolicy` | `string` | Referrer-Policy value |
| `PermissionsPolicy` | `string` | Permissions-Policy value |
| `XFrameOptions` | `string` | `DENY` or `SAMEORIGIN` |

### Methods

| Method | Description |
|--------|-------------|
| `Build() (*tls.Config, error)` | Creates a `*tls.Config`; returns `nil` if no settings are configured |
| `Validate() error` | Checks that CertFile and KeyFile are both set or both empty |
| `IsEnabled() bool` | Returns `true` if any TLS setting is configured |
| `HeaderMap() (map[string]string, error)` | Builds the response-header policy |
| `Apply(http.Header) error` | Applies headers to an HTTP response |

## Advanced Usage

### Mutual TLS (mTLS)

```go
cfg := security.TLSConfig{
	CAFile:   "/etc/certs/ca.pem",
	CertFile: "/etc/certs/client.pem",
	KeyFile:  "/etc/certs/client-key.pem",
}

tlsCfg, _ := cfg.Build()
// tlsCfg.Certificates contains the client cert
// tlsCfg.RootCAs contains the CA for server verification
```

### Embedding in Service Config

```yaml
tls:
  ca_file: /etc/certs/ca.pem
  cert_file: /etc/certs/client.pem
  key_file: /etc/certs/client-key.pem
  server_name: api.example.com
  min_version: 772  # tls.VersionTLS13
```

```go
type KafkaConfig struct {
	Brokers []string          `yaml:"brokers"`
	TLS     security.TLSConfig `yaml:"tls"`
}
```

### Testing with `tlstest`

The `tlstest` package generates ephemeral self-signed certificates for tests.

```go
import "github.com/kbukum/gokit/security/tlstest"

func TestMTLS(t *testing.T) {
	certs := tlstest.GenerateTLSCerts(t)

	// Use generated file paths
	cfg := security.TLSConfig{
		CAFile:   certs.CAFile,
		CertFile: certs.CertFile,
		KeyFile:  certs.KeyFile,
	}

	tlsCfg, err := cfg.Build()
	require.NoError(t, err)
	require.NotNil(t, tlsCfg)

	// Or use the pre-built objects directly
	_ = certs.ServerTLS  // tls.Certificate
	_ = certs.CertPool   // *x509.CertPool
}
```

`GenerateTLSCerts` creates an ECDSA P-256 CA and server certificate valid for `localhost`,
`127.0.0.1`, and `::1`. Files are written to `t.TempDir()` and cleaned up automatically.

Use `WriteInvalidPEM(t, "bad.pem")` to generate invalid certificate files for error-path testing.

## Testing

```bash
cd security
go test -race ./...
```

## Contributing

Please refer to the root [CONTRIBUTING.md](../CONTRIBUTING.md) for guidelines.
