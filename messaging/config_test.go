package messaging

import "testing"

func TestConfigApplyDefaultsAndValidate(t *testing.T) {
	t.Parallel()

	cfg := Config{DLQ: DLQPolicy{Enabled: false}}
	cfg.ApplyDefaults()

	if cfg.Backend != DefaultBackend {
		t.Fatalf("backend = %q, want %q", cfg.Backend, DefaultBackend)
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
		"backend":        {Backend: "bad backend", DeliveryGuarantee: DeliveryAtLeastOnce, CommitStrategy: CommitAfterHandlerSuccess, MaxInFlight: 1, RequestTimeout: "1s", RetryBackoff: "1s", DLQ: DLQPolicy{Suffix: DefaultDLQSuffix}},
		"delivery":       {Backend: "memory", DeliveryGuarantee: "never", CommitStrategy: CommitAfterHandlerSuccess, MaxInFlight: 1, RequestTimeout: "1s", RetryBackoff: "1s", DLQ: DLQPolicy{Suffix: DefaultDLQSuffix}},
		"commit":         {Backend: "memory", DeliveryGuarantee: DeliveryAtLeastOnce, CommitStrategy: "later", MaxInFlight: 1, RequestTimeout: "1s", RetryBackoff: "1s", DLQ: DLQPolicy{Suffix: DefaultDLQSuffix}},
		"inflight":       {Backend: "memory", DeliveryGuarantee: DeliveryAtLeastOnce, CommitStrategy: CommitAfterHandlerSuccess, MaxInFlight: 0, RequestTimeout: "1s", RetryBackoff: "1s", DLQ: DLQPolicy{Suffix: DefaultDLQSuffix}},
		"timeout":        {Backend: "memory", DeliveryGuarantee: DeliveryAtLeastOnce, CommitStrategy: CommitAfterHandlerSuccess, MaxInFlight: 1, RequestTimeout: "0s", RetryBackoff: "1s", DLQ: DLQPolicy{Suffix: DefaultDLQSuffix}},
		"consumer_group": {Backend: "memory", DeliveryGuarantee: DeliveryAtLeastOnce, CommitStrategy: CommitAfterHandlerSuccess, MaxInFlight: 1, ConsumerGroup: "bad group", RequestTimeout: "1s", RetryBackoff: "1s", DLQ: DLQPolicy{Suffix: DefaultDLQSuffix}},
		"topic":          {Backend: "memory", DeliveryGuarantee: DeliveryAtLeastOnce, CommitStrategy: CommitAfterHandlerSuccess, MaxInFlight: 1, Topics: []string{"bad topic"}, RequestTimeout: "1s", RetryBackoff: "1s", DLQ: DLQPolicy{Suffix: DefaultDLQSuffix}},
		"dlq":            {Backend: "memory", DeliveryGuarantee: DeliveryAtLeastOnce, CommitStrategy: CommitAfterHandlerSuccess, MaxInFlight: 1, RequestTimeout: "1s", RetryBackoff: "1s", DLQ: DLQPolicy{Suffix: "bad suffix"}},
	}
	for name, cfg := range cases {
		cfg := cfg
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
}
