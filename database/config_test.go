package database

import (
	"fmt"
	"testing"
)

// TestConfig_ApplyDefaults_MaxOpenConns tests default for MaxOpenConns
func TestConfig_ApplyDefaults_MaxOpenConns(t *testing.T) {
	cfg := Config{}
	cfg.ApplyDefaults()

	if cfg.MaxOpenConns != 25 {
		t.Errorf("MaxOpenConns = %d, want 25", cfg.MaxOpenConns)
	}
}

// TestConfig_ApplyDefaults_MaxIdleConns tests default for MaxIdleConns
func TestConfig_ApplyDefaults_MaxIdleConns(t *testing.T) {
	cfg := Config{}
	cfg.ApplyDefaults()

	if cfg.MaxIdleConns != 5 {
		t.Errorf("MaxIdleConns = %d, want 5", cfg.MaxIdleConns)
	}
}

// TestConfig_ApplyDefaults_ConnMaxLifetime tests default for ConnMaxLifetime
func TestConfig_ApplyDefaults_ConnMaxLifetime(t *testing.T) {
	cfg := Config{}
	cfg.ApplyDefaults()

	if cfg.ConnMaxLifetime != "1h" {
		t.Errorf("ConnMaxLifetime = %q, want %q", cfg.ConnMaxLifetime, "1h")
	}
}

// TestConfig_ApplyDefaults_ConnMaxIdleTime tests default for ConnMaxIdleTime
func TestConfig_ApplyDefaults_ConnMaxIdleTime(t *testing.T) {
	cfg := Config{}
	cfg.ApplyDefaults()

	if cfg.ConnMaxIdleTime != "5m" {
		t.Errorf("ConnMaxIdleTime = %q, want %q", cfg.ConnMaxIdleTime, "5m")
	}
}

// TestConfig_ApplyDefaults_MaxRetries tests default for MaxRetries
func TestConfig_ApplyDefaults_MaxRetries(t *testing.T) {
	cfg := Config{}
	cfg.ApplyDefaults()

	if cfg.MaxRetries != 5 {
		t.Errorf("MaxRetries = %d, want 5", cfg.MaxRetries)
	}
}

// TestConfig_ApplyDefaults_SlowQueryThreshold tests default for SlowQueryThreshold
func TestConfig_ApplyDefaults_SlowQueryThreshold(t *testing.T) {
	cfg := Config{}
	cfg.ApplyDefaults()

	if cfg.SlowQueryThreshold != "200ms" {
		t.Errorf("SlowQueryThreshold = %q, want %q", cfg.SlowQueryThreshold, "200ms")
	}
}

// TestConfig_ApplyDefaults_LogLevel tests default for LogLevel
func TestConfig_ApplyDefaults_LogLevel(t *testing.T) {
	cfg := Config{}
	cfg.ApplyDefaults()

	if cfg.LogLevel != "warn" {
		t.Errorf("LogLevel = %q, want %q", cfg.LogLevel, "warn")
	}
}

// TestConfig_ApplyDefaults_PreservesExistingValues tests that non-zero values are preserved
func TestConfig_ApplyDefaults_PreservesExistingValues(t *testing.T) {
	cfg := Config{
		MaxOpenConns:       50,
		MaxIdleConns:       10,
		ConnMaxLifetime:    "2h",
		ConnMaxIdleTime:    "10m",
		MaxRetries:         10,
		SlowQueryThreshold: "500ms",
		LogLevel:           "info",
	}
	cfg.ApplyDefaults()

	if cfg.MaxOpenConns != 50 {
		t.Errorf("MaxOpenConns = %d, want 50", cfg.MaxOpenConns)
	}
	if cfg.MaxIdleConns != 10 {
		t.Errorf("MaxIdleConns = %d, want 10", cfg.MaxIdleConns)
	}
	if cfg.ConnMaxLifetime != "2h" {
		t.Errorf("ConnMaxLifetime = %q, want %q", cfg.ConnMaxLifetime, "2h")
	}
	if cfg.ConnMaxIdleTime != "10m" {
		t.Errorf("ConnMaxIdleTime = %q, want %q", cfg.ConnMaxIdleTime, "10m")
	}
	if cfg.MaxRetries != 10 {
		t.Errorf("MaxRetries = %d, want 10", cfg.MaxRetries)
	}
	if cfg.SlowQueryThreshold != "500ms" {
		t.Errorf("SlowQueryThreshold = %q, want %q", cfg.SlowQueryThreshold, "500ms")
	}
	if cfg.LogLevel != "info" {
		t.Errorf("LogLevel = %q, want %q", cfg.LogLevel, "info")
	}
}

// TestConfig_Validate_DisabledSkipsValidation tests that disabled config doesn't validate
func TestConfig_Validate_DisabledSkipsValidation(t *testing.T) {
	cfg := Config{
		Enabled: false,
		// All fields empty - would fail if validation ran
	}

	err := cfg.Validate()
	if err != nil {
		t.Errorf("Validate() should skip when Enabled=false, got error: %v", err)
	}
}

// TestConfig_Validate_Success tests successful validation
func TestConfig_Validate_Success(t *testing.T) {
	cfg := Config{
		Enabled:            true,
		DSN:                ":memory:",
		MaxOpenConns:       25,
		MaxIdleConns:       5,
		ConnMaxLifetime:    "1h",
		ConnMaxIdleTime:    "5m",
		MaxRetries:         5,
		SlowQueryThreshold: "200ms",
		LogLevel:           "warn",
	}

	err := cfg.Validate()
	if err != nil {
		t.Errorf("Validate() failed: %v", err)
	}
}

// TestConfig_Validate_MissingDSN tests validation fails with missing DSN
func TestConfig_Validate_MissingDSN(t *testing.T) {
	cfg := Config{
		Enabled:            true,
		DSN:                "",
		MaxOpenConns:       25,
		MaxIdleConns:       5,
		ConnMaxLifetime:    "1h",
		ConnMaxIdleTime:    "5m",
		MaxRetries:         5,
		SlowQueryThreshold: "200ms",
	}
	cfg.ApplyDefaults()

	err := cfg.Validate()
	if err == nil {
		t.Error("Validate() should fail with empty DSN")
	}

	expectedMsg := "database DSN is required"
	if err.Error() != expectedMsg {
		t.Errorf("Error message = %q, want %q", err.Error(), expectedMsg)
	}
}

// TestConfig_Validate_MaxOpenConnsZero tests validation fails with MaxOpenConns <= 0
func TestConfig_Validate_MaxOpenConnsZero(t *testing.T) {
	cfg := Config{
		Enabled:            true,
		DSN:                ":memory:",
		MaxOpenConns:       0,
		MaxIdleConns:       5,
		ConnMaxLifetime:    "1h",
		MaxRetries:         5,
		SlowQueryThreshold: "200ms",
	}

	err := cfg.Validate()
	if err == nil {
		t.Error("Validate() should fail with MaxOpenConns = 0")
	}

	expectedMsg := "max_open_conns must be > 0"
	if err.Error() != expectedMsg {
		t.Errorf("Error message = %q, want %q", err.Error(), expectedMsg)
	}
}

// TestConfig_Validate_MaxOpenConnsNegative tests validation fails with negative MaxOpenConns
func TestConfig_Validate_MaxOpenConnsNegative(t *testing.T) {
	cfg := Config{
		Enabled:            true,
		DSN:                ":memory:",
		MaxOpenConns:       -1,
		MaxIdleConns:       5,
		ConnMaxLifetime:    "1h",
		MaxRetries:         5,
		SlowQueryThreshold: "200ms",
	}

	err := cfg.Validate()
	if err == nil {
		t.Error("Validate() should fail with negative MaxOpenConns")
	}

	expectedMsg := "max_open_conns must be > 0"
	if err.Error() != expectedMsg {
		t.Errorf("Error message = %q, want %q", err.Error(), expectedMsg)
	}
}

// TestConfig_Validate_MaxIdleConnsZero tests validation fails with MaxIdleConns <= 0
func TestConfig_Validate_MaxIdleConnsZero(t *testing.T) {
	cfg := Config{
		Enabled:            true,
		DSN:                ":memory:",
		MaxOpenConns:       25,
		MaxIdleConns:       0,
		ConnMaxLifetime:    "1h",
		MaxRetries:         5,
		SlowQueryThreshold: "200ms",
	}

	err := cfg.Validate()
	if err == nil {
		t.Error("Validate() should fail with MaxIdleConns = 0")
	}

	expectedMsg := "max_idle_conns must be > 0"
	if err.Error() != expectedMsg {
		t.Errorf("Error message = %q, want %q", err.Error(), expectedMsg)
	}
}

// TestConfig_Validate_MaxIdleConnsNegative tests validation fails with negative MaxIdleConns
func TestConfig_Validate_MaxIdleConnsNegative(t *testing.T) {
	cfg := Config{
		Enabled:            true,
		DSN:                ":memory:",
		MaxOpenConns:       25,
		MaxIdleConns:       -5,
		ConnMaxLifetime:    "1h",
		MaxRetries:         5,
		SlowQueryThreshold: "200ms",
	}

	err := cfg.Validate()
	if err == nil {
		t.Error("Validate() should fail with negative MaxIdleConns")
	}

	expectedMsg := "max_idle_conns must be > 0"
	if err.Error() != expectedMsg {
		t.Errorf("Error message = %q, want %q", err.Error(), expectedMsg)
	}
}

// TestConfig_Validate_MaxIdleGreaterThanMaxOpen tests validation fails when idle > open
func TestConfig_Validate_MaxIdleGreaterThanMaxOpen(t *testing.T) {
	cfg := Config{
		Enabled:            true,
		DSN:                ":memory:",
		MaxOpenConns:       10,
		MaxIdleConns:       20,
		ConnMaxLifetime:    "1h",
		MaxRetries:         5,
		SlowQueryThreshold: "200ms",
	}

	err := cfg.Validate()
	if err == nil {
		t.Error("Validate() should fail when MaxIdleConns > MaxOpenConns")
	}

	expectedMsg := fmt.Sprintf("max_idle_conns (%d) must be <= max_open_conns (%d)", 20, 10)
	if err.Error() != expectedMsg {
		t.Errorf("Error message = %q, want %q", err.Error(), expectedMsg)
	}
}

// TestConfig_Validate_InvalidConnMaxLifetime tests validation fails with invalid ConnMaxLifetime
func TestConfig_Validate_InvalidConnMaxLifetime(t *testing.T) {
	cfg := Config{
		Enabled:            true,
		DSN:                ":memory:",
		MaxOpenConns:       25,
		MaxIdleConns:       5,
		ConnMaxLifetime:    "invalid-duration",
		MaxRetries:         5,
		SlowQueryThreshold: "200ms",
	}

	err := cfg.Validate()
	if err == nil {
		t.Error("Validate() should fail with invalid ConnMaxLifetime")
	}

	if err.Error() == "" {
		t.Error("Error message should not be empty")
	}
}

// TestConfig_Validate_InvalidConnMaxIdleTime tests validation fails with invalid ConnMaxIdleTime
func TestConfig_Validate_InvalidConnMaxIdleTime(t *testing.T) {
	cfg := Config{
		Enabled:            true,
		DSN:                ":memory:",
		MaxOpenConns:       25,
		MaxIdleConns:       5,
		ConnMaxLifetime:    "1h",
		ConnMaxIdleTime:    "not-a-duration",
		MaxRetries:         5,
		SlowQueryThreshold: "200ms",
	}

	err := cfg.Validate()
	if err == nil {
		t.Error("Validate() should fail with invalid ConnMaxIdleTime")
	}

	if err.Error() == "" {
		t.Error("Error message should not be empty")
	}
}

// TestConfig_Validate_EmptyConnMaxIdleTimeAllowed tests that empty ConnMaxIdleTime is allowed
func TestConfig_Validate_EmptyConnMaxIdleTimeAllowed(t *testing.T) {
	cfg := Config{
		Enabled:            true,
		DSN:                ":memory:",
		MaxOpenConns:       25,
		MaxIdleConns:       5,
		ConnMaxLifetime:    "1h",
		ConnMaxIdleTime:    "",
		MaxRetries:         5,
		SlowQueryThreshold: "200ms",
	}

	err := cfg.Validate()
	if err != nil {
		t.Errorf("Validate() should allow empty ConnMaxIdleTime, got error: %v", err)
	}
}

// TestConfig_Validate_InvalidSlowQueryThreshold tests validation fails with invalid SlowQueryThreshold
func TestConfig_Validate_InvalidSlowQueryThreshold(t *testing.T) {
	cfg := Config{
		Enabled:            true,
		DSN:                ":memory:",
		MaxOpenConns:       25,
		MaxIdleConns:       5,
		ConnMaxLifetime:    "1h",
		MaxRetries:         5,
		SlowQueryThreshold: "slow",
	}

	err := cfg.Validate()
	if err == nil {
		t.Error("Validate() should fail with invalid SlowQueryThreshold")
	}

	if err.Error() == "" {
		t.Error("Error message should not be empty")
	}
}

// TestConfig_Validate_MaxRetriesZero tests validation fails with MaxRetries = 0
func TestConfig_Validate_MaxRetriesZero(t *testing.T) {
	cfg := Config{
		Enabled:            true,
		DSN:                ":memory:",
		MaxOpenConns:       25,
		MaxIdleConns:       5,
		ConnMaxLifetime:    "1h",
		MaxRetries:         0,
		SlowQueryThreshold: "200ms",
	}

	err := cfg.Validate()
	if err == nil {
		t.Error("Validate() should fail with MaxRetries = 0")
	}

	expectedMsg := "max_retries must be > 0"
	if err.Error() != expectedMsg {
		t.Errorf("Error message = %q, want %q", err.Error(), expectedMsg)
	}
}

// TestConfig_Validate_MaxRetriesNegative tests validation fails with negative MaxRetries
func TestConfig_Validate_MaxRetriesNegative(t *testing.T) {
	cfg := Config{
		Enabled:            true,
		DSN:                ":memory:",
		MaxOpenConns:       25,
		MaxIdleConns:       5,
		ConnMaxLifetime:    "1h",
		MaxRetries:         -1,
		SlowQueryThreshold: "200ms",
	}

	err := cfg.Validate()
	if err == nil {
		t.Error("Validate() should fail with negative MaxRetries")
	}

	expectedMsg := "max_retries must be > 0"
	if err.Error() != expectedMsg {
		t.Errorf("Error message = %q, want %q", err.Error(), expectedMsg)
	}
}

// TestConfig_Validate_ValidDurations tests validation with valid duration formats
func TestConfig_Validate_ValidDurations(t *testing.T) {
	validDurations := []string{
		"1h",
		"30m",
		"500ms",
		"5s",
		"1h30m",
		"1h30m45s",
	}

	for _, duration := range validDurations {
		cfg := Config{
			Enabled:            true,
			DSN:                ":memory:",
			MaxOpenConns:       25,
			MaxIdleConns:       5,
			ConnMaxLifetime:    duration,
			MaxRetries:         5,
			SlowQueryThreshold: "200ms",
		}

		err := cfg.Validate()
		if err != nil {
			t.Errorf("Validate() failed for valid duration %q: %v", duration, err)
		}
	}
}

// TestConfig_Validate_ZeroMaxIdleConnsAllowedByApplyDefaults tests ApplyDefaults sets value
func TestConfig_Validate_ZeroMaxIdleConnsAllowedByApplyDefaults(t *testing.T) {
	cfg := Config{
		Enabled:      true,
		DSN:          ":memory:",
		MaxOpenConns: 0,
		MaxIdleConns: 0,
	}
	cfg.ApplyDefaults()

	// After ApplyDefaults, validation should succeed
	err := cfg.Validate()
	if err != nil {
		t.Errorf("Validate() failed after ApplyDefaults: %v", err)
	}
}

// TestConfig_ApplyDefaults_Idempotent tests that ApplyDefaults is idempotent
func TestConfig_ApplyDefaults_Idempotent(t *testing.T) {
	cfg := Config{}
	cfg.ApplyDefaults()

	first := cfg
	cfg.ApplyDefaults()
	second := cfg

	if first.MaxOpenConns != second.MaxOpenConns ||
		first.MaxIdleConns != second.MaxIdleConns ||
		first.ConnMaxLifetime != second.ConnMaxLifetime ||
		first.ConnMaxIdleTime != second.ConnMaxIdleTime ||
		first.MaxRetries != second.MaxRetries ||
		first.SlowQueryThreshold != second.SlowQueryThreshold ||
		first.LogLevel != second.LogLevel {
		t.Error("ApplyDefaults should be idempotent")
	}
}

// TestConfig_Validate_ComplexConnMaxIdleTime tests validation with complex ConnMaxIdleTime
func TestConfig_Validate_ComplexConnMaxIdleTime(t *testing.T) {
	cfg := Config{
		Enabled:            true,
		DSN:                ":memory:",
		MaxOpenConns:       25,
		MaxIdleConns:       5,
		ConnMaxLifetime:    "1h",
		ConnMaxIdleTime:    "2h30m45s",
		MaxRetries:         5,
		SlowQueryThreshold: "200ms",
	}

	err := cfg.Validate()
	if err != nil {
		t.Errorf("Validate() failed with complex ConnMaxIdleTime: %v", err)
	}
}

// TestConfig_Validate_VariousLogLevels tests that LogLevel doesn't affect validation
func TestConfig_Validate_VariousLogLevels(t *testing.T) {
	logLevels := []string{"silent", "error", "warn", "info", "debug"}

	for _, level := range logLevels {
		cfg := Config{
			Enabled:            true,
			DSN:                ":memory:",
			MaxOpenConns:       25,
			MaxIdleConns:       5,
			ConnMaxLifetime:    "1h",
			MaxRetries:         5,
			SlowQueryThreshold: "200ms",
			LogLevel:           level,
		}

		err := cfg.Validate()
		if err != nil {
			t.Errorf("Validate() failed for LogLevel %q: %v", level, err)
		}
	}
}

// TestConfig_Validate_AllErrorCasesCovered tests multiple error conditions
func TestConfig_Validate_AllErrorCasesCovered(t *testing.T) {
	testCases := []struct {
		name    string
		cfg     Config
		wantErr bool
	}{
		{
			name: "Valid config",
			cfg: Config{
				Enabled:            true,
				DSN:                ":memory:",
				MaxOpenConns:       25,
				MaxIdleConns:       5,
				ConnMaxLifetime:    "1h",
				MaxRetries:         5,
				SlowQueryThreshold: "200ms",
			},
			wantErr: false,
		},
		{
			name: "Missing DSN",
			cfg: Config{
				Enabled:            true,
				DSN:                "",
				MaxOpenConns:       25,
				MaxIdleConns:       5,
				ConnMaxLifetime:    "1h",
				MaxRetries:         5,
				SlowQueryThreshold: "200ms",
			},
			wantErr: true,
		},
		{
			name: "Invalid MaxOpenConns",
			cfg: Config{
				Enabled:            true,
				DSN:                ":memory:",
				MaxOpenConns:       -5,
				MaxIdleConns:       5,
				ConnMaxLifetime:    "1h",
				MaxRetries:         5,
				SlowQueryThreshold: "200ms",
			},
			wantErr: true,
		},
		{
			name: "MaxIdleConns too high",
			cfg: Config{
				Enabled:            true,
				DSN:                ":memory:",
				MaxOpenConns:       10,
				MaxIdleConns:       20,
				ConnMaxLifetime:    "1h",
				MaxRetries:         5,
				SlowQueryThreshold: "200ms",
			},
			wantErr: true,
		},
		{
			name: "Invalid ConnMaxLifetime",
			cfg: Config{
				Enabled:            true,
				DSN:                ":memory:",
				MaxOpenConns:       25,
				MaxIdleConns:       5,
				ConnMaxLifetime:    "xyz",
				MaxRetries:         5,
				SlowQueryThreshold: "200ms",
			},
			wantErr: true,
		},
		{
			name: "Invalid SlowQueryThreshold",
			cfg: Config{
				Enabled:            true,
				DSN:                ":memory:",
				MaxOpenConns:       25,
				MaxIdleConns:       5,
				ConnMaxLifetime:    "1h",
				MaxRetries:         5,
				SlowQueryThreshold: "abc",
			},
			wantErr: true,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			err := tc.cfg.Validate()
			if (err != nil) != tc.wantErr {
				t.Errorf("Validate() error = %v, wantErr %v", err, tc.wantErr)
			}
		})
	}
}
