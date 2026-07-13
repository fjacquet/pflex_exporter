package models

import (
	"testing"

	"gopkg.in/yaml.v2"
)

func validCluster() ClusterConfig {
	return ClusterConfig{Name: "c1", Gateway: "gw1", Username: "u", Password: "p"}
}

func TestValidateAppliesDefaults(t *testing.T) {
	cfg := &Config{Clusters: []ClusterConfig{validCluster()}}
	if err := cfg.Validate(); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if cfg.Server.Port != "9445" || cfg.Server.URI != "/metrics" {
		t.Errorf("server defaults not applied: %+v", cfg.Server)
	}
	if cfg.Collection.Interval != "10s" || cfg.Collection.Timeout != "8s" {
		t.Errorf("collection defaults not applied: %+v", cfg.Collection)
	}
	if cfg.GetMetricsPushInterval() != cfg.GetCollectionInterval() {
		t.Error("metrics push interval should default to collection interval")
	}
}

func TestValidateRejectsBadConfig(t *testing.T) {
	tests := []struct {
		name   string
		mutate func(*Config)
	}{
		{"no clusters", func(c *Config) { c.Clusters = nil }},
		{"duplicate cluster name", func(c *Config) {
			c.Clusters = []ClusterConfig{validCluster(), validCluster()}
		}},
		{"missing gateway", func(c *Config) { c.Clusters[0].Gateway = "" }},
		{"missing password", func(c *Config) { c.Clusters[0].Password = "" }},
		{"bad port", func(c *Config) { c.Server.Port = "99999" }},
		{"bad interval", func(c *Config) { c.Collection.Interval = "soon" }},
		{"otel metrics enabled without endpoint", func(c *Config) {
			c.OpenTelemetry.Metrics.Enabled = true
		}},
		{"otel tracing bad sampling", func(c *Config) {
			c.OpenTelemetry.Tracing.Enabled = true
			c.OpenTelemetry.Tracing.Endpoint = "localhost:4317"
			c.OpenTelemetry.Tracing.SamplingRate = 2.0
		}},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			cfg := &Config{Clusters: []ClusterConfig{validCluster()}}
			tc.mutate(cfg)
			if err := cfg.Validate(); err == nil {
				t.Errorf("expected validation error for %q", tc.name)
			}
		})
	}
}

func TestGatewayBaseURL(t *testing.T) {
	c := ClusterConfig{Gateway: "10.0.0.1"}
	if got := c.GatewayBaseURL(); got != "https://10.0.0.1" {
		t.Errorf("GatewayBaseURL = %q", got)
	}
}

func TestMaskPassword(t *testing.T) {
	if got := (ClusterConfig{Password: "short"}).MaskPassword(); got != "****" {
		t.Errorf("short password mask = %q", got)
	}
	if got := (ClusterConfig{Password: "supersecret"}).MaskPassword(); got != "su****et" {
		t.Errorf("long password mask = %q", got)
	}
}

func TestEnvBoolNativeAndEnvRef(t *testing.T) {
	// native YAML bool
	var native struct {
		Skip EnvBool `yaml:"skip"`
	}
	if err := yaml.Unmarshal([]byte("skip: true\n"), &native); err != nil {
		t.Fatalf("unmarshal native bool: %v", err)
	}
	if !native.Skip.Bool() {
		t.Fatal("native bool true not resolved to true")
	}

	// ${VAR} reference: unresolved until Resolve is called (defaults false)
	var ref struct {
		Skip EnvBool `yaml:"skip"`
	}
	if err := yaml.Unmarshal([]byte("skip: ${PFLEX1_SKIP_CERTIFICATE}\n"), &ref); err != nil {
		t.Fatalf("unmarshal env ref: %v", err)
	}
	if ref.Skip.Bool() {
		t.Fatal("env ref should be false before Resolve")
	}
	// resolve via a fake expander returning "true"
	if err := ref.Skip.Resolve(func(s string) (string, error) { return "true", nil }); err != nil {
		t.Fatalf("resolve: %v", err)
	}
	if !ref.Skip.Bool() {
		t.Fatal("env ref true not resolved")
	}

	// absent field defaults to false and Resolve is a no-op
	var absent struct {
		Skip EnvBool `yaml:"skip"`
	}
	if err := yaml.Unmarshal([]byte("other: 1\n"), &absent); err != nil {
		t.Fatalf("unmarshal absent: %v", err)
	}
	if err := absent.Skip.Resolve(func(s string) (string, error) { return "", nil }); err != nil {
		t.Fatalf("resolve absent: %v", err)
	}
	if absent.Skip.Bool() {
		t.Fatal("absent field should be false")
	}
}

func TestEnvBoolResolveNonBooleanErrors(t *testing.T) {
	var ref struct {
		Skip EnvBool `yaml:"skip"`
	}
	if err := yaml.Unmarshal([]byte("skip: ${PFLEX1_SKIP_CERTIFICATE}\n"), &ref); err != nil {
		t.Fatalf("unmarshal env ref: %v", err)
	}
	err := ref.Skip.Resolve(func(s string) (string, error) { return "not-a-bool", nil })
	if err == nil {
		t.Fatal("expected error resolving a non-boolean value")
	}
}

func TestEnvBoolUnmarshalRejectsNonBoolNonString(t *testing.T) {
	var target struct {
		Skip EnvBool `yaml:"skip"`
	}
	if err := yaml.Unmarshal([]byte("skip: [1, 2, 3]\n"), &target); err == nil {
		t.Fatal("expected error unmarshaling a sequence into EnvBool")
	}
}
