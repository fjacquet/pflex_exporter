package powerflex

import (
	"context"
	"testing"
)

// TestAllObjectTypesProduceSamples verifies every one of the 7 PowerFlex object types
// produces samples with correctly resolved identity/parent labels and values, using the
// shared fixture topology.
func TestAllObjectTypesProduceSamples(t *testing.T) {
	c, store := newTestCollector(t)
	c.CollectOnce(context.Background())
	snap := store.Load()

	// SDS: primaryReadBwc{80,10,1600} -> iops 8; parent PD resolved.
	assertSample(t, snap, "pflex_sds_iops",
		map[string]string{"op": "primary", "direction": "read", "sds_id": "sds1"}, 8)
	assertSample(t, snap, "pflex_sds_iops",
		map[string]string{"op": "total", "direction": "write", "sds_id": "sds1"}, 4)
	assertLabel(t, snap, "pflex_sds_iops",
		map[string]string{"sds_id": "sds1"}, "protection_domain_name", "pd-1")

	// Device: scalars + derived; device_path has /dev/ stripped; full parent chain.
	assertSample(t, snap, "pflex_device_avg_read_size_in_bytes",
		map[string]string{"device_id": "dev1"}, 4096)
	assertSample(t, snap, "pflex_device_avg_read_latency_in_microsec",
		map[string]string{"device_id": "dev1"}, 250)
	assertSample(t, snap, "pflex_device_iops",
		map[string]string{"op": "primary", "direction": "read", "device_id": "dev1"}, 8)
	assertLabel(t, snap, "pflex_device_iops",
		map[string]string{"device_id": "dev1"}, "device_path", "sdb")
	assertLabel(t, snap, "pflex_device_iops",
		map[string]string{"device_id": "dev1"}, "sds", "sds-1")
	assertLabel(t, snap, "pflex_device_iops",
		map[string]string{"device_id": "dev1"}, "storage_pool_name", "pool-1")

	// StoragePool: capacity scalars + parent PD.
	assertSample(t, snap, "pflex_storagepool_max_capacity_in_kb",
		map[string]string{"storage_pool_id": "sp1"}, 1000000)
	assertSample(t, snap, "pflex_storagepool_spare_capacity_in_kb",
		map[string]string{"storage_pool_id": "sp1"}, 100000)
	assertLabel(t, snap, "pflex_storagepool_max_capacity_in_kb",
		map[string]string{"storage_pool_id": "sp1"}, "protection_domain_name", "pd-1")

	// SDC: name falls back to sdcIp when name is null; scalar + derived.
	assertSample(t, snap, "pflex_sdc_num_of_mapped_volumes",
		map[string]string{"sdc_id": "sdc1"}, 3)
	assertSample(t, snap, "pflex_sdc_iops",
		map[string]string{"op": "userData", "direction": "read", "sdc_id": "sdc1"}, 6)
	assertLabel(t, snap, "pflex_sdc_num_of_mapped_volumes",
		map[string]string{"sdc_id": "sdc1"}, "sdc_name", "10.0.0.5")

	// ProtectionDomain: capacity + derived.
	assertSample(t, snap, "pflex_protectiondomain_max_capacity_in_kb",
		map[string]string{"protection_domain_id": "pd1"}, 1000000)
	assertSample(t, snap, "pflex_protectiondomain_iops",
		map[string]string{"op": "primary", "direction": "read", "protection_domain_id": "pd1"}, 10)

	// System (cluster): scalar + derived (already covered elsewhere, re-checked here).
	assertSample(t, snap, "pflex_cluster_unused_capacity_in_kb",
		map[string]string{"cluster": "test-cluster"}, 600000)
	assertSample(t, snap, "pflex_cluster_compression_ratio",
		map[string]string{"cluster": "test-cluster"}, 1.5)
}
