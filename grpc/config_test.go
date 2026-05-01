package grpc

import (
	"crypto/tls"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/kbukum/gokit/security"
)

// ---------------------------------------------------------------------------
// ApplyDefaults
// ---------------------------------------------------------------------------

func TestApplyDefaults_AllZeroValues(t *testing.T) {
	t.Parallel()
	cfg := Config{}
	cfg.ApplyDefaults()

	assert.Equal(t, "localhost:50051", cfg.Addr, "default addr")
	assert.Equal(t, 4*1024*1024, cfg.MaxRecvMsgSize, "default max recv")
	assert.Equal(t, 4*1024*1024, cfg.MaxSendMsgSize, "default max send")
	assert.Equal(t, 30*time.Second, cfg.Keepalive.Time, "default keepalive time")
	assert.Equal(t, 10*time.Second, cfg.Keepalive.Timeout, "default keepalive timeout")
	assert.Equal(t, 30*time.Second, cfg.CallTimeout, "default call timeout")
}

func TestApplyDefaults_PreservesExistingValues(t *testing.T) {
	t.Parallel()
	cfg := Config{
		Addr:           "custom:9090",
		MaxRecvMsgSize: 8 * 1024 * 1024,
		MaxSendMsgSize: 8 * 1024 * 1024,
		Keepalive: KeepaliveConfig{
			Time:    60 * time.Second,
			Timeout: 20 * time.Second,
		},
		CallTimeout: 5 * time.Second,
	}
	cfg.ApplyDefaults()

	assert.Equal(t, "custom:9090", cfg.Addr)
	assert.Equal(t, 8*1024*1024, cfg.MaxRecvMsgSize)
	assert.Equal(t, 8*1024*1024, cfg.MaxSendMsgSize)
	assert.Equal(t, 60*time.Second, cfg.Keepalive.Time)
	assert.Equal(t, 20*time.Second, cfg.Keepalive.Timeout)
	assert.Equal(t, 5*time.Second, cfg.CallTimeout)
}

func TestApplyDefaults_PartiallySet(t *testing.T) {
	t.Parallel()
	cfg := Config{
		Addr:        "myhost:443",
		CallTimeout: 15 * time.Second,
	}
	cfg.ApplyDefaults()

	assert.Equal(t, "myhost:443", cfg.Addr, "explicit addr preserved")
	assert.Equal(t, 4*1024*1024, cfg.MaxRecvMsgSize, "default max recv")
	assert.Equal(t, 4*1024*1024, cfg.MaxSendMsgSize, "default max send")
	assert.Equal(t, 30*time.Second, cfg.Keepalive.Time, "default keepalive time")
	assert.Equal(t, 10*time.Second, cfg.Keepalive.Timeout, "default keepalive timeout")
	assert.Equal(t, 15*time.Second, cfg.CallTimeout, "explicit call timeout preserved")
}

func TestApplyDefaults_KeepalivePermitWithoutStream(t *testing.T) {
	t.Parallel()
	cfg := Config{Keepalive: KeepaliveConfig{PermitWithoutStream: true}}
	cfg.ApplyDefaults()

	assert.True(t, cfg.Keepalive.PermitWithoutStream, "explicit PermitWithoutStream preserved")
	assert.Equal(t, 30*time.Second, cfg.Keepalive.Time, "default keepalive time still applied")
}

func TestApplyDefaults_Idempotent(t *testing.T) {
	t.Parallel()
	cfg := Config{}
	cfg.ApplyDefaults()
	snapshot := cfg
	cfg.ApplyDefaults()

	assert.Equal(t, snapshot, cfg, "second ApplyDefaults must be a no-op")
}

// ---------------------------------------------------------------------------
// Validate
// ---------------------------------------------------------------------------

func TestValidate_ValidConfig(t *testing.T) {
	t.Parallel()
	cfg := Config{Addr: "host:50051", MaxRecvMsgSize: 1024, MaxSendMsgSize: 1024}
	require.NoError(t, cfg.Validate())
}

func TestValidate_AfterApplyDefaults(t *testing.T) {
	t.Parallel()
	cfg := Config{}
	cfg.ApplyDefaults()
	require.NoError(t, cfg.Validate(), "defaults should produce a valid config")
}

func TestValidate_EmptyAddr(t *testing.T) {
	t.Parallel()
	cfg := Config{Addr: "", MaxRecvMsgSize: 1024, MaxSendMsgSize: 1024}
	err := cfg.Validate()
	require.Error(t, err)
	assert.Contains(t, err.Error(), "addr must not be empty")
}

func TestValidate_NegativeMaxRecvMsgSize(t *testing.T) {
	t.Parallel()
	cfg := Config{Addr: "host:50051", MaxRecvMsgSize: -1, MaxSendMsgSize: 1024}
	err := cfg.Validate()
	require.Error(t, err)
	assert.Contains(t, err.Error(), "max_recv_msg_size must be positive")
}

func TestValidate_ZeroMaxRecvMsgSize(t *testing.T) {
	t.Parallel()
	cfg := Config{Addr: "host:50051", MaxRecvMsgSize: 0, MaxSendMsgSize: 1024}
	err := cfg.Validate()
	require.Error(t, err)
	assert.Contains(t, err.Error(), "max_recv_msg_size must be positive")
}

func TestValidate_NegativeMaxSendMsgSize(t *testing.T) {
	t.Parallel()
	cfg := Config{Addr: "host:50051", MaxRecvMsgSize: 1024, MaxSendMsgSize: -1}
	err := cfg.Validate()
	require.Error(t, err)
	assert.Contains(t, err.Error(), "max_send_msg_size must be positive")
}

func TestValidate_ZeroMaxSendMsgSize(t *testing.T) {
	t.Parallel()
	cfg := Config{Addr: "host:50051", MaxRecvMsgSize: 1024, MaxSendMsgSize: 0}
	err := cfg.Validate()
	require.Error(t, err)
	assert.Contains(t, err.Error(), "max_send_msg_size must be positive")
}

func TestValidate_TLSCertWithoutKey(t *testing.T) {
	t.Parallel()
	cfg := Config{
		Addr:           "host:50051",
		MaxRecvMsgSize: 1024,
		MaxSendMsgSize: 1024,
		TLS:            &security.TLSConfig{CertFile: "/cert.pem"},
	}
	err := cfg.Validate()
	require.Error(t, err)
	assert.Contains(t, err.Error(), "cert_file and key_file must be provided together")
}

func TestValidate_TLSKeyWithoutCert(t *testing.T) {
	t.Parallel()
	cfg := Config{
		Addr:           "host:50051",
		MaxRecvMsgSize: 1024,
		MaxSendMsgSize: 1024,
		TLS:            &security.TLSConfig{KeyFile: "/key.pem"},
	}
	err := cfg.Validate()
	require.Error(t, err)
	assert.Contains(t, err.Error(), "cert_file and key_file must be provided together")
}

func TestValidate_NilTLS(t *testing.T) {
	t.Parallel()
	cfg := Config{Addr: "host:50051", MaxRecvMsgSize: 1024, MaxSendMsgSize: 1024, TLS: nil}
	require.NoError(t, cfg.Validate())
}

func TestValidate_ValidTLSSkipVerify(t *testing.T) {
	t.Parallel()
	cfg := Config{
		Addr:           "host:50051",
		MaxRecvMsgSize: 1024,
		MaxSendMsgSize: 1024,
		TLS:            &security.TLSConfig{SkipVerify: true},
	}
	require.NoError(t, cfg.Validate())
}

func TestValidate_RejectsLegacyTLSFloor(t *testing.T) {
	t.Parallel()
	cfg := Config{
		Addr:           "host:50051",
		MaxRecvMsgSize: 1024,
		MaxSendMsgSize: 1024,
		TLS:            &security.TLSConfig{MinVersion: tls.VersionTLS11},
	}
	err := cfg.Validate()
	require.Error(t, err)
	assert.Contains(t, err.Error(), "TLS 1.2 or newer")
}

// ---------------------------------------------------------------------------
// Address
// ---------------------------------------------------------------------------

func TestAddress_ReturnsAddr(t *testing.T) {
	t.Parallel()
	cfg := Config{Addr: "example.com:443"}
	assert.Equal(t, "example.com:443", cfg.Address())
}

func TestAddress_EmptyAddr(t *testing.T) {
	t.Parallel()
	cfg := Config{}
	assert.Empty(t, cfg.Address())
}

// ---------------------------------------------------------------------------
// Config with all fields set
// ---------------------------------------------------------------------------

func TestConfig_AllFieldsSet(t *testing.T) {
	t.Parallel()
	cfg := Config{
		Name:           "my-service",
		Addr:           "prod.example.com:443",
		MaxRecvMsgSize: 16 * 1024 * 1024,
		MaxSendMsgSize: 8 * 1024 * 1024,
		Keepalive: KeepaliveConfig{
			Time:                60 * time.Second,
			Timeout:             20 * time.Second,
			PermitWithoutStream: true,
		},
		TLS:         &security.TLSConfig{SkipVerify: true},
		Enabled:     true,
		CallTimeout: 10 * time.Second,
	}

	require.NoError(t, cfg.Validate())
	assert.Equal(t, "prod.example.com:443", cfg.Address())
	assert.Equal(t, "my-service", cfg.Name)
	assert.True(t, cfg.Enabled)
	assert.True(t, cfg.Keepalive.PermitWithoutStream)
}
