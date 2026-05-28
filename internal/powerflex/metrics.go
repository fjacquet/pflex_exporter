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
	models.TypeStorageNode:      "pflex_storagenode", // Gen2
	models.TypeDeviceGroup:      "pflex_devicegroup", // Gen2
	models.TypeSdt:              "pflex_sdt",         // Gen2
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

// labelBuildersGen1 maps each Gen1 per-object type to its label builder. System is
// handled separately (flat stats, no per-object iteration).
var labelBuildersGen1 = map[string]labelBuilder{
	models.TypeSds:              buildSdsLabels,
	models.TypeDevice:           buildDeviceLabelsGen1,
	models.TypeVolume:           buildVolumeLabelsGen1,
	models.TypeStoragePool:      buildStoragePoolLabels,
	models.TypeSdc:              buildSdcLabels,
	models.TypeProtectionDomain: buildProtectionDomainLabels,
}

// labelBuildersGen2 maps each Gen2 per-object type to its label builder. StoragePool,
// Sdc and ProtectionDomain reuse the Gen1 builders (identical label sets). Volume and
// Device use Gen2 variants that share a union label-key set with their Gen1 forms.
var labelBuildersGen2 = map[string]labelBuilder{
	models.TypeStorageNode:      buildStorageNodeLabels,
	models.TypeDevice:           buildDeviceLabelsGen2,
	models.TypeVolume:           buildVolumeLabelsGen2,
	models.TypeStoragePool:      buildStoragePoolLabels,
	models.TypeSdc:              buildSdcLabels,
	models.TypeDeviceGroup:      buildDeviceGroupLabels,
	models.TypeProtectionDomain: buildProtectionDomainLabels,
	models.TypeSdt:              buildSdtLabels,
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

// deviceLabels builds the union Device label set in canonical order so Gen1 and Gen2
// device metrics share identical label keys (required by Prometheus). Inapplicable
// values are passed empty.
func deviceLabels(clusterName, systemID, sds, sdsID, sn, snID, devName, devID, devPath, poolID, poolName, pdID, pdName string) []Label {
	labels := baseLabels(clusterName, systemID)
	return append(labels,
		Label{"sds", sds},
		Label{"sds_id", sdsID},
		Label{"storage_node_name", sn},
		Label{"storage_node_id", snID},
		Label{"device_name", devName},
		Label{"device_id", devID},
		Label{"device_path", devPath},
		Label{"storage_pool_id", poolID},
		Label{"storage_pool_name", poolName},
		Label{"protection_domain_id", pdID},
		Label{"protection_domain_name", pdName},
	)
}

func buildDeviceLabelsGen1(clusterName, systemID string, dev *models.Instance, in *models.Instances, rel *models.Relations) ([]Label, bool) {
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
	return deviceLabels(clusterName, systemID,
		sds.DisplayName(), sds.ID, "", "",
		dev.DisplayName(), dev.ID, devPath,
		pool.ID, pool.DisplayName(), pd.ID, pd.DisplayName()), true
}

// buildDeviceLabelsGen2 resolves the Gen2 device parent chain (StorageNode -> PD;
// devices belong to DeviceGroups, not StoragePools).
func buildDeviceLabelsGen2(clusterName, systemID string, dev *models.Instance, in *models.Instances, rel *models.Relations) ([]Label, bool) {
	sn := rel.FirstParent(dev.ID, models.TypeStorageNode, in.Get(models.TypeStorageNode))
	if sn == nil {
		return nil, false
	}
	pd := rel.FirstParent(sn.ID, models.TypeProtectionDomain, in.Get(models.TypeProtectionDomain))
	if pd == nil {
		return nil, false
	}
	devPath := strings.TrimPrefix(dev.DeviceCurrentPathName, "/dev/")
	return deviceLabels(clusterName, systemID,
		"", "", sn.DisplayName(), sn.ID,
		dev.DisplayName(), dev.ID, devPath,
		"", "", pd.ID, pd.DisplayName()), true
}

// volumeLabels builds the union Volume label set in canonical order (Gen1 passes an
// empty volume_type) so Gen1 and Gen2 volume metrics share identical label keys.
func volumeLabels(clusterName, systemID, volName, volID, volType, poolID, poolName, pdID, pdName string) []Label {
	labels := baseLabels(clusterName, systemID)
	return append(labels,
		Label{"volume_name", volName},
		Label{"volume_id", volID},
		Label{"volume_type", volType},
		Label{"storage_pool_id", poolID},
		Label{"storage_pool_name", poolName},
		Label{"protection_domain_id", pdID},
		Label{"protection_domain_name", pdName},
	)
}

func buildVolumeLabelsGen1(clusterName, systemID string, vol *models.Instance, in *models.Instances, rel *models.Relations) ([]Label, bool) {
	pool := rel.FirstParent(vol.ID, models.TypeStoragePool, in.Get(models.TypeStoragePool))
	if pool == nil {
		return nil, false
	}
	pd := rel.FirstParent(pool.ID, models.TypeProtectionDomain, in.Get(models.TypeProtectionDomain))
	if pd == nil {
		return nil, false
	}
	return volumeLabels(clusterName, systemID,
		vol.DisplayName(), vol.ID, "",
		pool.ID, pool.DisplayName(), pd.ID, pd.DisplayName()), true
}

// gen2VolumeType maps the raw Gen2 volumeType to Dell's display values.
func gen2VolumeType(raw string) string {
	switch raw {
	case "ThinProvisioned":
		return "BaseVolume"
	case "Snapshot":
		return "ThinClone"
	case "":
		return "unknown"
	default:
		return raw
	}
}

func buildVolumeLabelsGen2(clusterName, systemID string, vol *models.Instance, in *models.Instances, rel *models.Relations) ([]Label, bool) {
	pool := rel.FirstParent(vol.ID, models.TypeStoragePool, in.Get(models.TypeStoragePool))
	if pool == nil {
		return nil, false
	}
	pd := rel.FirstParent(pool.ID, models.TypeProtectionDomain, in.Get(models.TypeProtectionDomain))
	if pd == nil {
		return nil, false
	}
	return volumeLabels(clusterName, systemID,
		vol.DisplayName(), vol.ID, gen2VolumeType(vol.VolumeType),
		pool.ID, pool.DisplayName(), pd.ID, pd.DisplayName()), true
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

// buildStorageNodeLabels is the Gen2 analog of buildSdsLabels (SDS was renamed
// StorageNode in Gen2). Parent: ProtectionDomain.
func buildStorageNodeLabels(clusterName, systemID string, node *models.Instance, in *models.Instances, rel *models.Relations) ([]Label, bool) {
	pd := rel.FirstParent(node.ID, models.TypeProtectionDomain, in.Get(models.TypeProtectionDomain))
	if pd == nil {
		return nil, false
	}
	labels := baseLabels(clusterName, systemID)
	labels = append(labels,
		Label{"storage_node_name", node.DisplayName()},
		Label{"storage_node_id", node.ID},
		Label{"protection_domain_id", pd.ID},
		Label{"protection_domain_name", pd.DisplayName()},
	)
	return labels, true
}

// buildDeviceGroupLabels (Gen2) groups devices by media type under a ProtectionDomain.
func buildDeviceGroupLabels(clusterName, systemID string, dg *models.Instance, in *models.Instances, rel *models.Relations) ([]Label, bool) {
	pd := rel.FirstParent(dg.ID, models.TypeProtectionDomain, in.Get(models.TypeProtectionDomain))
	if pd == nil {
		return nil, false
	}
	mediaType := dg.MediaType
	if mediaType == "" {
		mediaType = "unknown"
	}
	labels := baseLabels(clusterName, systemID)
	labels = append(labels,
		Label{"device_group_name", dg.DisplayName()},
		Label{"device_group_id", dg.ID},
		Label{"media_type", mediaType},
		Label{"protection_domain_id", pd.ID},
		Label{"protection_domain_name", pd.DisplayName()},
	)
	return labels, true
}

// buildSdtLabels (Gen2) is the NVMe/TCP target, under a ProtectionDomain.
func buildSdtLabels(clusterName, systemID string, sdt *models.Instance, in *models.Instances, rel *models.Relations) ([]Label, bool) {
	pd := rel.FirstParent(sdt.ID, models.TypeProtectionDomain, in.Get(models.TypeProtectionDomain))
	if pd == nil {
		return nil, false
	}
	labels := baseLabels(clusterName, systemID)
	labels = append(labels,
		Label{"sdt_name", sdt.DisplayName()},
		Label{"sdt_id", sdt.ID},
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
