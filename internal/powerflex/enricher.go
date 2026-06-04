package powerflex

// Enricher resolves optional Kubernetes workload labels for volume and SDC metrics.
// It is satisfied by the real k8s-backed enricher and by a no-op default, so the
// collector has no hard dependency on client-go.
//
// Label-key consistency (load-bearing): when Enabled() is true, VolumeLabels and
// SDCLabels MUST return the same label keys in the same order for every call,
// using empty values when a workload cannot be resolved. Enabled() is constant for
// the lifetime of the process, so the per-metric label-key set stays uniform across
// all series (the invariant TestMixedGenerationMetricsValid guards).
type Enricher interface {
	// Enabled reports whether enrichment labels should be appended at all.
	Enabled() bool
	// VolumeLabels returns the k8s labels for a PowerFlex volume ID (empty values when
	// the volume is not backed by a known PersistentVolume).
	VolumeLabels(volumeID string) []Label
	// SDCLabels returns the k8s labels for an SDC IP (empty value when no node matches).
	SDCLabels(sdcIP string) []Label
}

// Enrichment label keys, in canonical order. Kept here so the no-op and the real
// enricher agree on the exact key set and ordering.
const (
	labelNamespace             = "namespace"
	labelPersistentVolumeClaim = "persistent_volume_claim"
	labelPersistentVolume      = "persistent_volume"
	labelStorageClass          = "storage_class"
	labelK8sNode               = "k8s_node"
)

// VolumeEnrichmentLabels builds the canonical volume enrichment label slice.
func VolumeEnrichmentLabels(namespace, pvc, pv, storageClass string) []Label {
	return []Label{
		{Name: labelNamespace, Value: namespace},
		{Name: labelPersistentVolumeClaim, Value: pvc},
		{Name: labelPersistentVolume, Value: pv},
		{Name: labelStorageClass, Value: storageClass},
	}
}

// SDCEnrichmentLabels builds the canonical SDC enrichment label slice.
func SDCEnrichmentLabels(node string) []Label {
	return []Label{{Name: labelK8sNode, Value: node}}
}

// noopEnricher is the default when k8s enrichment is disabled or unavailable.
type noopEnricher struct{}

// NoopEnricher returns an Enricher that appends nothing.
func NoopEnricher() Enricher { return noopEnricher{} }

func (noopEnricher) Enabled() bool               { return false }
func (noopEnricher) VolumeLabels(string) []Label { return nil }
func (noopEnricher) SDCLabels(string) []Label    { return nil }
