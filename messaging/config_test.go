package messaging

import (
	"strings"
	"testing"
)

func TestConfigApplyDefaultsAndValidate(t *testing.T) {
	t.Parallel()

	cfg := Config{DLQ: DLQPolicy{Enabled: false}}
	cfg.ApplyDefaults()

	if cfg.Adapter != DefaultAdapter {
		t.Fatalf("adapter = %q, want %q", cfg.Adapter, DefaultAdapter)
	}
	if !cfg.IsEnabled() {
		t.Fatal("config should default to enabled")
	}
	if cfg.DLQ.Enabled {
		t.Fatal("DLQ enabled default = true, want explicit false preserved")
	}
	if cfg.DLQ.Suffix != DefaultDLQSuffix {
		t.Fatalf("DLQ suffix = %q, want %q", cfg.DLQ.Suffix, DefaultDLQSuffix)
	}
	if err := cfg.Validate(); err != nil {
		t.Fatalf("validate defaults: %v", err)
	}
}

func TestConfigValidateRejectsInvalidValues(t *testing.T) {
	t.Parallel()

	cases := map[string]Config{
		"adapter":        {Adapter: "bad adapter", DeliveryGuarantee: DeliveryAtLeastOnce, CommitStrategy: CommitAfterHandlerSuccess, MaxInFlight: 1, RequestTimeout: "1s", RetryBackoff: "1s", DLQ: DLQPolicy{Suffix: DefaultDLQSuffix}},
		"delivery":       {Adapter: "memory", DeliveryGuarantee: "never", CommitStrategy: CommitAfterHandlerSuccess, MaxInFlight: 1, RequestTimeout: "1s", RetryBackoff: "1s", DLQ: DLQPolicy{Suffix: DefaultDLQSuffix}},
		"commit":         {Adapter: "memory", DeliveryGuarantee: DeliveryAtLeastOnce, CommitStrategy: "later", MaxInFlight: 1, RequestTimeout: "1s", RetryBackoff: "1s", DLQ: DLQPolicy{Suffix: DefaultDLQSuffix}},
		"inflight":       {Adapter: "memory", DeliveryGuarantee: DeliveryAtLeastOnce, CommitStrategy: CommitAfterHandlerSuccess, MaxInFlight: 0, RequestTimeout: "1s", RetryBackoff: "1s", DLQ: DLQPolicy{Suffix: DefaultDLQSuffix}},
		"timeout":        {Adapter: "memory", DeliveryGuarantee: DeliveryAtLeastOnce, CommitStrategy: CommitAfterHandlerSuccess, MaxInFlight: 1, RequestTimeout: "0s", RetryBackoff: "1s", DLQ: DLQPolicy{Suffix: DefaultDLQSuffix}},
		"consumer_group": {Adapter: "memory", DeliveryGuarantee: DeliveryAtLeastOnce, CommitStrategy: CommitAfterHandlerSuccess, MaxInFlight: 1, ConsumerGroup: "bad group", RequestTimeout: "1s", RetryBackoff: "1s", DLQ: DLQPolicy{Suffix: DefaultDLQSuffix}},
		"topic":          {Adapter: "memory", DeliveryGuarantee: DeliveryAtLeastOnce, CommitStrategy: CommitAfterHandlerSuccess, MaxInFlight: 1, Topics: []string{"bad topic"}, RequestTimeout: "1s", RetryBackoff: "1s", DLQ: DLQPolicy{Suffix: DefaultDLQSuffix}},
		"dlq":            {Adapter: "memory", DeliveryGuarantee: DeliveryAtLeastOnce, CommitStrategy: CommitAfterHandlerSuccess, MaxInFlight: 1, RequestTimeout: "1s", RetryBackoff: "1s", DLQ: DLQPolicy{Suffix: "bad suffix"}},
		"name":           {Adapter: "memory", Name: "bad name", DeliveryGuarantee: DeliveryAtLeastOnce, CommitStrategy: CommitAfterHandlerSuccess, MaxInFlight: 1, RequestTimeout: "1s", RetryBackoff: "1s", DLQ: DLQPolicy{Suffix: DefaultDLQSuffix}},
		"retry_attempts": {Adapter: "memory", DeliveryGuarantee: DeliveryAtLeastOnce, CommitStrategy: CommitAfterHandlerSuccess, MaxInFlight: 1, RetryAttempts: -1, RequestTimeout: "1s", RetryBackoff: "1s", DLQ: DLQPolicy{Suffix: DefaultDLQSuffix}},
		"subscription":   {Adapter: "memory", DeliveryGuarantee: DeliveryAtLeastOnce, CommitStrategy: CommitAfterHandlerSuccess, MaxInFlight: 1, Subscriptions: []string{"bad sub"}, RequestTimeout: "1s", RetryBackoff: "1s", DLQ: DLQPolicy{Suffix: DefaultDLQSuffix}},
		"retry_backoff":  {Adapter: "memory", DeliveryGuarantee: DeliveryAtLeastOnce, CommitStrategy: CommitAfterHandlerSuccess, MaxInFlight: 1, RequestTimeout: "1s", RetryBackoff: "nonsense", DLQ: DLQPolicy{Suffix: DefaultDLQSuffix}},
		"timeout_parse":  {Adapter: "memory", DeliveryGuarantee: DeliveryAtLeastOnce, CommitStrategy: CommitAfterHandlerSuccess, MaxInFlight: 1, RequestTimeout: "nonsense", RetryBackoff: "1s", DLQ: DLQPolicy{Suffix: DefaultDLQSuffix}},
		"dlq_whitespace": {Adapter: "memory", DeliveryGuarantee: DeliveryAtLeastOnce, CommitStrategy: CommitAfterHandlerSuccess, MaxInFlight: 1, RequestTimeout: "1s", RetryBackoff: "1s", DLQ: DLQPolicy{Suffix: "bad\tsuffix"}},
	}
	for name, cfg := range cases {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			if err := cfg.Validate(); err == nil {
				t.Fatal("expected validation error")
			}
		})
	}
}

func TestValidateTopic(t *testing.T) {
	t.Parallel()

	if err := ValidateTopic("events.created"); err != nil {
		t.Fatalf("valid topic rejected: %v", err)
	}
	if err := ValidateTopic("events created"); err == nil {
		t.Fatal("expected invalid topic error")
	}
	if err := ValidateTopic(""); err == nil {
		t.Fatal("expected empty topic error")
	}
	if err := ValidateTopic(strings.Repeat("a", 250)); err == nil {
		t.Fatal("expected over-length topic error")
	}
	if err := ValidateTopic("events/created"); err == nil {
		t.Fatal("expected path-separator topic error")
	}
	if err := ValidateTopic("events\x01"); err == nil {
		t.Fatal("expected control-character topic error")
	}
	if err := ValidateTopic("events@created"); err == nil {
		t.Fatal("expected unsupported-character topic error")
	}
}
