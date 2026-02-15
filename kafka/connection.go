package kafka

import (
	"crypto/tls"
	"crypto/x509"
	"fmt"
	"os"

	"github.com/segmentio/kafka-go"
	"github.com/segmentio/kafka-go/sasl"
	"github.com/segmentio/kafka-go/sasl/plain"
	"github.com/segmentio/kafka-go/sasl/scram"
)

// CreateTransport builds a kafka.Transport with optional TLS/SASL for producers.
func CreateTransport(cfg *Config) (*kafka.Transport, error) {
	transport := &kafka.Transport{
		IdleTimeout: ParseDuration(cfg.IdleTimeout),
		MetadataTTL: ParseDuration(cfg.MetadataTTL),
	}

	if cfg.EnableTLS {
		tc, err := buildTLSConfig(cfg)
		if err != nil {
			return nil, fmt.Errorf("TLS config: %w", err)
		}
		transport.TLS = tc
	}

	if cfg.EnableSASL {
		m, err := buildSASLMechanism(cfg)
		if err != nil {
			return nil, fmt.Errorf("SASL config: %w", err)
		}
		transport.SASL = m
	}

	return transport, nil
}

// CreateDialer builds a kafka.Dialer with optional TLS/SASL for consumers.
func CreateDialer(cfg *Config) (*kafka.Dialer, error) {
	dialer := &kafka.Dialer{
		Timeout:   ParseDuration(cfg.DialTimeout),
		DualStack: true,
	}

	if cfg.EnableTLS {
		tc, err := buildTLSConfig(cfg)
		if err != nil {
			return nil, fmt.Errorf("TLS config: %w", err)
		}
		dialer.TLS = tc
	}

	if cfg.EnableSASL {
		m, err := buildSASLMechanism(cfg)
		if err != nil {
			return nil, fmt.Errorf("SASL config: %w", err)
		}
		dialer.SASLMechanism = m
	}

	return dialer, nil
}

func buildTLSConfig(cfg *Config) (*tls.Config, error) {
	tc := &tls.Config{
		InsecureSkipVerify: cfg.TLSSkipVerify,
		MinVersion:         tls.VersionTLS12,
	}

	if cfg.TLSCAFile != "" {
		caCert, err := os.ReadFile(cfg.TLSCAFile)
		if err != nil {
			return nil, fmt.Errorf("read CA file: %w", err)
		}
		pool := x509.NewCertPool()
		if !pool.AppendCertsFromPEM(caCert) {
			return nil, fmt.Errorf("parse CA certificate")
		}
		tc.RootCAs = pool
	}

	if cfg.TLSCertFile != "" && cfg.TLSKeyFile != "" {
		cert, err := tls.LoadX509KeyPair(cfg.TLSCertFile, cfg.TLSKeyFile)
		if err != nil {
			return nil, fmt.Errorf("load client cert: %w", err)
		}
		tc.Certificates = []tls.Certificate{cert}
	}

	return tc, nil
}

func buildSASLMechanism(cfg *Config) (sasl.Mechanism, error) {
	switch cfg.SASLMechanism {
	case "PLAIN":
		return plain.Mechanism{
			Username: cfg.Username,
			Password: cfg.Password,
		}, nil
	case "SCRAM-SHA-256":
		return scram.Mechanism(scram.SHA256, cfg.Username, cfg.Password)
	case "SCRAM-SHA-512":
		return scram.Mechanism(scram.SHA512, cfg.Username, cfg.Password)
	default:
		return nil, fmt.Errorf("unsupported SASL mechanism: %s", cfg.SASLMechanism)
	}
}

// ResolveCompression maps a compression name to a kafka.Compression codec.
func ResolveCompression(name string) kafka.Compression {
	switch name {
	case "gzip":
		return kafka.Gzip
	case "lz4":
		return kafka.Lz4
	case "zstd":
		return kafka.Zstd
	case "snappy":
		return kafka.Snappy
	case "none":
		return 0
	default:
		return kafka.Snappy
	}
}
