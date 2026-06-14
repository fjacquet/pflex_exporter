package models

import "testing"

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
