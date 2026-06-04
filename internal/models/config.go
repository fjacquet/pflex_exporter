// Package models defines the core data structures for the PowerFlex exporter:
// application configuration and the PowerFlex REST API response types.
package models

import (
	"errors"
	"fmt"
	"strconv"
	"time"
)

// ClusterConfig holds the connection details for a single PowerFlex cluster (Gen1 or Gen2).
// One exporter process monitors many clusters; the Name becomes the `cluster` label
// on every metric emitted for this cluster.
type ClusterConfig struct {
	Name               string `yaml:"name"`
	Gateway            string `yaml:"gateway"`
	Username           string `yaml:"username"`
	Password           string `yaml:"password"`
	PasswordFile       string `yaml:"passwordFile"`
	InsecureSkipVerify bool   `yaml:"insecureSkipVerify"`
}

// GatewayBaseURL returns the HTTPS base URL for the cluster's PowerFlex gateway.
// PowerFlex only serves the REST API over HTTPS, so the scheme is fixed.
func (c ClusterConfig) GatewayBaseURL() string {
	return "https://" + c.Gateway
}

// MaskPassword returns a masked password suitable for logging.
func (c ClusterConfig) MaskPassword() string {
	if len(c.Password) <= 8 {
		return "****"
	}
	return c.Password[:2] + "****" + c.Password[len(c.Password)-2:]
}

// OTelExportConfig holds the settings shared by the metrics-push and tracing exporters.
type OTelExportConfig struct {
	Enabled  bool   `yaml:"enabled"`
	Endpoint string `yaml:"endpoint"`
	Insecure bool   `yaml:"insecure"`
	// Interval is the OTLP metric push period (metrics only). Ignored for tracing.
	Interval string `yaml:"interval"`
	// SamplingRate is the trace sampling ratio 0.0-1.0 (tracing only). Ignored for metrics.
	SamplingRate float64 `yaml:"samplingRate"`
}

// Config represents the complete application configuration for the PowerFlex exporter.
type Config struct {
	Server struct {
		Host    string `yaml:"host"`
		Port    string `yaml:"port"`
		URI     string `yaml:"uri"`
		LogName string `yaml:"logName"`
	} `yaml:"server"`

	Collection struct {
		Interval string `yaml:"interval"` // background collection loop period (e.g. "10s")
		Timeout  string `yaml:"timeout"`  // per-cluster collection timeout (e.g. "8s")
		// MaxConcurrentClusters caps how many clusters are polled in parallel per cycle.
		// 0 (default) means unlimited (every cluster polled concurrently).
		MaxConcurrentClusters int `yaml:"maxConcurrentClusters"`
		// SlowResourceEveryN, when > 1, collects the SlowResourceTypes statistics only
		// every Nth cycle (reusing prior samples in between) to reduce API load on large
		// arrays. 1 (default) disables decimation. Gen2 only (see SlowResourceTypes).
		SlowResourceEveryN int `yaml:"slowResourceEveryN"`
		// SlowResourceTypes lists the PowerFlex object types treated as slow-changing for
		// decimation (e.g. DeviceGroup, Sdt, ProtectionDomain). Empty (default) disables
		// it. Only effective for Gen2, whose statistics are fetched per resource type;
		// Gen1 statistics arrive in a single query and are always collected in full.
		SlowResourceTypes []string `yaml:"slowResourceTypes"`
	} `yaml:"collection"`

	// Kubernetes enables optional workload enrichment: volume metrics gain namespace /
	// PVC / PV / storageClass labels and SDC metrics gain a node label, resolved from the
	// cluster's PersistentVolumes and Nodes. It is portable: when the exporter is not
	// running in (or configured for) a reachable cluster it degrades to a no-op.
	Kubernetes struct {
		Enabled         bool   `yaml:"enabled"`
		RefreshInterval string `yaml:"refreshInterval"` // PV/Node cache refresh period (e.g. "60s")
		CSIDriverName   string `yaml:"csiDriverName"`   // CSI driver to match (default csi-vxflexos.dellemc.com)
		Kubeconfig      string `yaml:"kubeconfig"`      // explicit kubeconfig path (optional; in-cluster used otherwise)
	} `yaml:"kubernetes"`

	OpenTelemetry struct {
		Metrics OTelExportConfig `yaml:"metrics"`
		Tracing OTelExportConfig `yaml:"tracing"`
	} `yaml:"opentelemetry"`

	Clusters []ClusterConfig `yaml:"clusters"`
}

// DefaultCSIDriverName is the Dell PowerFlex (VxFlex OS) CSI driver provisioner name,
// used as the default match for Kubernetes workload enrichment.
const DefaultCSIDriverName = "csi-vxflexos.dellemc.com"

// validSlowResourceTypes is the set of object-type names accepted in
// Collection.SlowResourceTypes (mirrors the normalized type names in instances.go).
var validSlowResourceTypes = map[string]struct{}{
	TypeSystem: {}, TypeSds: {}, TypeSdc: {}, TypeVolume: {}, TypeStoragePool: {},
	TypeDevice: {}, TypeProtectionDomain: {}, TypeStorageNode: {}, TypeDeviceGroup: {}, TypeSdt: {},
}

// SetDefaults sets default values for optional configuration fields.
func (c *Config) SetDefaults() {
	if c.Server.Host == "" {
		c.Server.Host = "0.0.0.0"
	}
	if c.Server.Port == "" {
		c.Server.Port = "2112"
	}
	if c.Server.URI == "" {
		c.Server.URI = "/metrics"
	}
	if c.Collection.Interval == "" {
		c.Collection.Interval = "10s"
	}
	if c.Collection.Timeout == "" {
		c.Collection.Timeout = "8s"
	}
	if c.OpenTelemetry.Metrics.Interval == "" {
		c.OpenTelemetry.Metrics.Interval = c.Collection.Interval
	}
	if c.Collection.MaxConcurrentClusters < 0 {
		c.Collection.MaxConcurrentClusters = 0
	}
	if c.Collection.SlowResourceEveryN <= 0 {
		c.Collection.SlowResourceEveryN = 1
	}
	if c.Kubernetes.Enabled {
		if c.Kubernetes.RefreshInterval == "" {
			c.Kubernetes.RefreshInterval = "60s"
		}
		if c.Kubernetes.CSIDriverName == "" {
			c.Kubernetes.CSIDriverName = DefaultCSIDriverName
		}
	}
}

// Validate checks the configuration and returns an error on the first problem found.
// SetDefaults is applied first so optional fields have sensible values.
func (c *Config) Validate() error {
	c.SetDefaults()

	if err := c.validateServer(); err != nil {
		return err
	}
	if err := c.validateCollection(); err != nil {
		return err
	}
	if err := c.validateClusters(); err != nil {
		return err
	}
	if err := c.validateOTel("metrics", c.OpenTelemetry.Metrics); err != nil {
		return err
	}
	return c.validateOTel("tracing", c.OpenTelemetry.Tracing)
}

func (c *Config) validateServer() error {
	if c.Server.Host == "" {
		return errors.New("server host is required")
	}
	if err := validatePort(c.Server.Port); err != nil {
		return fmt.Errorf("invalid server port: %s", c.Server.Port)
	}
	if c.Server.URI == "" {
		return errors.New("server URI is required")
	}
	return nil
}

func (c *Config) validateCollection() error {
	if _, err := time.ParseDuration(c.Collection.Interval); err != nil {
		return fmt.Errorf("invalid collection interval '%s': %w (expected format: 10s, 1m)", c.Collection.Interval, err)
	}
	if _, err := time.ParseDuration(c.Collection.Timeout); err != nil {
		return fmt.Errorf("invalid collection timeout '%s': %w (expected format: 8s, 30s)", c.Collection.Timeout, err)
	}
	for _, t := range c.Collection.SlowResourceTypes {
		if _, ok := validSlowResourceTypes[t]; !ok {
			return fmt.Errorf("invalid collection slowResourceTypes entry %q (expected a PowerFlex object type, e.g. DeviceGroup, Sdt, ProtectionDomain)", t)
		}
	}
	if c.Kubernetes.Enabled {
		if _, err := time.ParseDuration(c.Kubernetes.RefreshInterval); err != nil {
			return fmt.Errorf("invalid kubernetes refreshInterval '%s': %w (expected format: 60s, 2m)", c.Kubernetes.RefreshInterval, err)
		}
	}
	return nil
}

func (c *Config) validateClusters() error {
	if len(c.Clusters) == 0 {
		return errors.New("at least one cluster must be configured")
	}
	seen := make(map[string]struct{}, len(c.Clusters))
	for i, cl := range c.Clusters {
		if cl.Name == "" {
			return fmt.Errorf("cluster[%d]: name is required", i)
		}
		if _, dup := seen[cl.Name]; dup {
			return fmt.Errorf("duplicate cluster name: %s", cl.Name)
		}
		seen[cl.Name] = struct{}{}
		if cl.Gateway == "" {
			return fmt.Errorf("cluster %q: gateway is required", cl.Name)
		}
		if cl.Username == "" {
			return fmt.Errorf("cluster %q: username is required", cl.Name)
		}
		if cl.Password == "" {
			return fmt.Errorf("cluster %q: password is required (set password or passwordFile)", cl.Name)
		}
	}
	return nil
}

func (c *Config) validateOTel(name string, o OTelExportConfig) error {
	if !o.Enabled {
		return nil
	}
	if o.Endpoint == "" {
		return fmt.Errorf("opentelemetry.%s endpoint is required when enabled", name)
	}
	host, port, err := splitHostPort(o.Endpoint)
	if err != nil || host == "" {
		return fmt.Errorf("invalid opentelemetry.%s endpoint: %s (expected host:port)", name, o.Endpoint)
	}
	if err := validatePort(port); err != nil {
		return fmt.Errorf("invalid opentelemetry.%s endpoint port: %s", name, port)
	}
	if name == "metrics" {
		if _, err := time.ParseDuration(o.Interval); err != nil {
			return fmt.Errorf("invalid opentelemetry.metrics interval '%s': %w", o.Interval, err)
		}
	}
	if name == "tracing" && (o.SamplingRate < 0.0 || o.SamplingRate > 1.0) {
		return fmt.Errorf("opentelemetry.tracing samplingRate must be between 0.0 and 1.0, got %f", o.SamplingRate)
	}
	return nil
}

// GetServerAddress returns the host:port the HTTP server binds to.
func (c *Config) GetServerAddress() string {
	return fmt.Sprintf("%s:%s", c.Server.Host, c.Server.Port)
}

// GetCollectionInterval returns the background collection loop period.
func (c *Config) GetCollectionInterval() time.Duration {
	return mustDuration(c.Collection.Interval, 10*time.Second)
}

// GetCollectionTimeout returns the per-cluster collection timeout.
func (c *Config) GetCollectionTimeout() time.Duration {
	return mustDuration(c.Collection.Timeout, 8*time.Second)
}

// GetMetricsPushInterval returns the OTLP metric push period.
func (c *Config) GetMetricsPushInterval() time.Duration {
	return mustDuration(c.OpenTelemetry.Metrics.Interval, c.GetCollectionInterval())
}

// GetMaxConcurrentClusters returns the cap on clusters polled in parallel (0 = unlimited).
func (c *Config) GetMaxConcurrentClusters() int { return c.Collection.MaxConcurrentClusters }

// GetSlowResourceEveryN returns the decimation multiplier for slow resource types (>=1).
func (c *Config) GetSlowResourceEveryN() int {
	if c.Collection.SlowResourceEveryN <= 0 {
		return 1
	}
	return c.Collection.SlowResourceEveryN
}

// GetSlowResourceTypes returns the object types subject to decimation.
func (c *Config) GetSlowResourceTypes() []string { return c.Collection.SlowResourceTypes }

// IsKubernetesEnabled reports whether optional k8s workload enrichment is enabled.
func (c *Config) IsKubernetesEnabled() bool { return c.Kubernetes.Enabled }

// GetKubernetesRefreshInterval returns the PV/Node cache refresh period.
func (c *Config) GetKubernetesRefreshInterval() time.Duration {
	return mustDuration(c.Kubernetes.RefreshInterval, 60*time.Second)
}

// IsOTelMetricsEnabled reports whether OTLP metric push is enabled.
func (c *Config) IsOTelMetricsEnabled() bool { return c.OpenTelemetry.Metrics.Enabled }

// IsOTelTracingEnabled reports whether OTLP tracing is enabled.
func (c *Config) IsOTelTracingEnabled() bool { return c.OpenTelemetry.Tracing.Enabled }

func mustDuration(s string, fallback time.Duration) time.Duration {
	d, err := time.ParseDuration(s)
	if err != nil {
		return fallback
	}
	return d
}

func validatePort(portStr string) error {
	port, err := strconv.Atoi(portStr)
	if err != nil || port < 1 || port > 65535 {
		return errors.New("port must be between 1 and 65535")
	}
	return nil
}

// splitHostPort splits "host:port" handling IPv6 forms like [::1]:4317.
func splitHostPort(endpoint string) (host, port string, err error) {
	lastColon := -1
	for i := len(endpoint) - 1; i >= 0; i-- {
		if endpoint[i] == ':' {
			lastColon = i
			break
		}
	}
	if lastColon == -1 {
		return "", "", errors.New("missing port in endpoint")
	}
	host = endpoint[:lastColon]
	port = endpoint[lastColon+1:]
	if host == "" || port == "" {
		return "", "", errors.New("invalid host:port format")
	}
	return host, port, nil
}
