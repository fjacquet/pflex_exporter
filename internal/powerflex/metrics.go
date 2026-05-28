package powerflex

import (
	"strings"

	"github.com/fjacquet/pflex_exporter/internal/models"
)

// Label is a single metric label/attribute name-value pair.
type Label struct {
	Name  string
	Value string
}

// Sample is one exported metric data point: a fully-qualified name, ordered labels
// (the first is always "cluster"), and a value. The same shape feeds both the
// Prometheus collector and the OTLP exporter.
type Sample struct {
	Name   string
	Labels []Label
	Value  float64
}

// metricPrefix maps a PowerFlex object type to its metric name prefix, mirroring
// Dell's siometrics.metric (e.g. System -> "pflex_cluster").
var metricPrefix = map[string]string{
	models.TypeSystem:           "pflex_cluster",
	models.TypeSds:              "pflex_sds",
	models.TypeSdc:              "pflex_sdc",
	models.TypeVolume:           "pflex_volume",
	models.TypeStoragePool:      "pflex_storagepool",
	models.TypeDevice:           "pflex_device",
	models.TypeProtectionDomain: "pflex_protectiondomain",
}

// baseLabels returns the cluster identity labels every metric carries.
func baseLabels(clusterName, systemID string) []Label {
	return []Label{
		{Name: "cluster", Value: clusterName},
		{Name: "cluster_id", Value: systemID},
	}
}

// labelBuilder resolves the per-object labels for a given object type, including
// parent labels derived from the relations graph. It returns ok=false when a required
// parent cannot be resolved, in which case the object is skipped (mirrors Dell raising
// and the caller's `except KeyError: continue`).
type labelBuilder func(clusterName, systemID string, obj *models.Instance, in *models.Instances, rel *models.Relations) (labels []Label, ok bool)

// labelBuilders maps each per-object type to its label builder. System is handled
// separately (flat stats, no per-object iteration).
var labelBuilders = map[string]labelBuilder{
	models.TypeSds:              buildSdsLabels,
	models.TypeDevice:           buildDeviceLabels,
	models.TypeVolume:           buildVolumeLabels,
	models.TypeStoragePool:      buildStoragePoolLabels,
	models.TypeSdc:              buildSdcLabels,
	models.TypeProtectionDomain: buildProtectionDomainLabels,
}

func buildSdsLabels(clusterName, systemID string, sds *models.Instance, in *models.Instances, rel *models.Relations) ([]Label, bool) {
	pd := rel.FirstParent(sds.ID, models.TypeProtectionDomain, in.Get(models.TypeProtectionDomain))
	if pd == nil {
		return nil, false
	}
	labels := baseLabels(clusterName, systemID)
	labels = append(labels,
		Label{"sds", sds.DisplayName()},
		Label{"sds_id", sds.ID},
		Label{"protection_domain_id", pd.ID},
		Label{"protection_domain_name", pd.DisplayName()},
	)
	return labels, true
}

func buildDeviceLabels(clusterName, systemID string, dev *models.Instance, in *models.Instances, rel *models.Relations) ([]Label, bool) {
	sds := rel.FirstParent(dev.ID, models.TypeSds, in.Get(models.TypeSds))
	pool := rel.FirstParent(dev.ID, models.TypeStoragePool, in.Get(models.TypeStoragePool))
	if sds == nil || pool == nil {
		return nil, false
	}
	pd := rel.FirstParent(pool.ID, models.TypeProtectionDomain, in.Get(models.TypeProtectionDomain))
	if pd == nil {
		return nil, false
	}
	devPath := strings.TrimPrefix(dev.DeviceCurrentPathName, "/dev/")
	labels := baseLabels(clusterName, systemID)
	labels = append(labels,
		Label{"sds", sds.DisplayName()},
		Label{"sds_id", sds.ID},
		Label{"device_name", dev.DisplayName()},
		Label{"device_id", dev.ID},
		Label{"device_path", devPath},
		Label{"storage_pool_id", pool.ID},
		Label{"storage_pool_name", pool.DisplayName()},
		Label{"protection_domain_id", pd.ID},
		Label{"protection_domain_name", pd.DisplayName()},
	)
	return labels, true
}

func buildVolumeLabels(clusterName, systemID string, vol *models.Instance, in *models.Instances, rel *models.Relations) ([]Label, bool) {
	pool := rel.FirstParent(vol.ID, models.TypeStoragePool, in.Get(models.TypeStoragePool))
	if pool == nil {
		return nil, false
	}
	pd := rel.FirstParent(pool.ID, models.TypeProtectionDomain, in.Get(models.TypeProtectionDomain))
	if pd == nil {
		return nil, false
	}
	labels := baseLabels(clusterName, systemID)
	labels = append(labels,
		Label{"volume_name", vol.DisplayName()},
		Label{"volume_id", vol.ID},
		Label{"storage_pool_id", pool.ID},
		Label{"storage_pool_name", pool.DisplayName()},
		Label{"protection_domain_id", pd.ID},
		Label{"protection_domain_name", pd.DisplayName()},
	)
	return labels, true
}

func buildStoragePoolLabels(clusterName, systemID string, pool *models.Instance, in *models.Instances, rel *models.Relations) ([]Label, bool) {
	pd := rel.FirstParent(pool.ID, models.TypeProtectionDomain, in.Get(models.TypeProtectionDomain))
	if pd == nil {
		return nil, false
	}
	labels := baseLabels(clusterName, systemID)
	labels = append(labels,
		Label{"storage_pool_id", pool.ID},
		Label{"storage_pool_name", pool.DisplayName()},
		Label{"protection_domain_id", pd.ID},
		Label{"protection_domain_name", pd.DisplayName()},
	)
	return labels, true
}

func buildSdcLabels(clusterName, systemID string, sdc *models.Instance, _ *models.Instances, _ *models.Relations) ([]Label, bool) {
	name := sdc.Name
	if name == "" {
		name = sdc.SdcIP
	}
	if name == "" {
		name = sdc.ID
	}
	labels := baseLabels(clusterName, systemID)
	labels = append(labels,
		Label{"sdc_name", name},
		Label{"sdc_id", sdc.ID},
	)
	return labels, true
}

func buildProtectionDomainLabels(clusterName, systemID string, pd *models.Instance, _ *models.Instances, _ *models.Relations) ([]Label, bool) {
	labels := baseLabels(clusterName, systemID)
	labels = append(labels,
		Label{"protection_domain_id", pd.ID},
		Label{"protection_domain_name", pd.DisplayName()},
	)
	return labels, true
}

// toSnake converts a camelCase PowerFlex stat name to snake_case for metric names,
// e.g. "maxCapacityInKb" -> "max_capacity_in_kb".
func toSnake(s string) string {
	var b strings.Builder
	for i, r := range s {
		if r >= 'A' && r <= 'Z' {
			if i > 0 {
				b.WriteByte('_')
			}
			b.WriteRune(r - 'A' + 'a')
		} else {
			b.WriteRune(r)
		}
	}
	return b.String()
}
