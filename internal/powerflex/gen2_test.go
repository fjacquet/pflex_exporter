package powerflex

import (
	"context"
	"testing"
	"time"

	"github.com/prometheus/client_golang/prometheus"
)

func gen2Snapshot(t *testing.T) *Snapshot {
	t.Helper()
	g := newMockGateway(t)
	g.instancesFixture = "instances-gen2.json"
	store := NewSnapshotStore()
	c := NewCollector([]Client{g.clientNamed(t, "gen2-cluster")}, store, time.Second, 5*time.Second, nil)
	c.CollectOnce(context.Background())
	return store.Load()
}

func TestGen2SamplesAndLabels(t *testing.T) {
	snap := gen2Snapshot(t)

	// Gen2 metrics are pre-computed: values pass through unchanged (no derivation).
	// Volume vol1: host_read_iops=60, host_read_bandwidth=1200000, avg_host_read_latency=250.
	assertSample(t, snap, "pflex_volume_iops",
		map[string]string{"op": "host", "direction": "read", "volume_id": "vol1"}, 60)
	assertSample(t, snap, "pflex_volume_bandwidth_bytes_per_second",
		map[string]string{"op": "host", "direction": "read", "volume_id": "vol1"}, 1200000)
	assertSample(t, snap, "pflex_volume_latency_microseconds",
		map[string]string{"op": "host", "direction": "read", "volume_id": "vol1"}, 250)
	assertSample(t, snap, "pflex_volume_logical_used",
		map[string]string{"volume_id": "vol1"}, 500000)
	// volumeType ThinProvisioned -> BaseVolume.
	assertLabel(t, snap, "pflex_volume_iops",
		map[string]string{"volume_id": "vol1"}, "volume_type", "BaseVolume")

	// StorageNode (Gen2 SDS): total_device_read_iops=80, raw_total scalar.
	assertSample(t, snap, "pflex_storagenode_iops",
		map[string]string{"op": "device", "direction": "read", "storage_node_id": "sn1"}, 80)
	assertSample(t, snap, "pflex_storagenode_raw_total",
		map[string]string{"storage_node_id": "sn1"}, 9000000)
	assertLabel(t, snap, "pflex_storagenode_iops",
		map[string]string{"storage_node_id": "sn1"}, "protection_domain_name", "pd-1")

	// DeviceGroup: PMEM/WRC ops + media_type label.
	assertSample(t, snap, "pflex_devicegroup_iops",
		map[string]string{"op": "device_pmem", "direction": "read", "device_group_id": "dg1"}, 10)
	assertSample(t, snap, "pflex_devicegroup_iops",
		map[string]string{"op": "wrc", "direction": "read", "device_group_id": "dg1"}, 5)
	assertLabel(t, snap, "pflex_devicegroup_iops",
		map[string]string{"device_group_id": "dg1"}, "media_type", "SSD")

	// Sdt (NVMe/TCP): directionless path latency.
	assertSample(t, snap, "pflex_sdt_latency_microseconds",
		map[string]string{"op": "controller_to_host", "sdt_id": "sdt1"}, 40)

	// Device: Gen2 parent chain resolves StorageNode (sds empty under the union keys).
	assertSample(t, snap, "pflex_device_iops",
		map[string]string{"op": "device", "direction": "read", "device_id": "dev1"}, 80)
	assertLabel(t, snap, "pflex_device_iops",
		map[string]string{"device_id": "dev1"}, "storage_node_name", "node-1")
	assertLabel(t, snap, "pflex_device_iops",
		map[string]string{"device_id": "dev1"}, "sds", "")

	// System scalar + derived.
	assertSample(t, snap, "pflex_cluster_compression_ratio",
		map[string]string{"cluster": "gen2-cluster"}, 2)
	assertSample(t, snap, "pflex_cluster_iops",
		map[string]string{"op": "host", "direction": "read"}, 100)

	// Generation flagged.
	if cs := snap.PerCluster["gen2-cluster"]; cs == nil || cs.Generation != GenerationGen2 {
		t.Errorf("expected generation gen2, got %+v", cs)
	}
}

// TestMixedGenerationMetricsValid ensures a single exporter serving both a Gen1 and a
// Gen2 cluster produces a Prometheus-valid /metrics: shared metric names (Volume, Device)
// keep consistent label keys via the union label set, so Gather succeeds with both series.
func TestMixedGenerationMetricsValid(t *testing.T) {
	gen1 := newMockGateway(t) // default instances.json (MediumGranularity -> gen1)
	gen2 := newMockGateway(t)
	gen2.instancesFixture = "instances-gen2.json"

	store := NewSnapshotStore()
	c := NewCollector(
		[]Client{gen1.clientNamed(t, "g1"), gen2.clientNamed(t, "g2")},
		store, time.Second, 5*time.Second, nil,
	)
	c.CollectOnce(context.Background())

	reg := prometheus.NewRegistry()
	reg.MustRegister(NewPromCollector(store))

	mfs, err := reg.Gather()
	if err != nil {
		t.Fatalf("gather must succeed for mixed gen1+gen2 clusters: %v", err)
	}

	// Both generations' volume IOPS appear under the same metric family with consistent keys.
	if v, ok := gatheredValue(mfs, "pflex_volume_iops", map[string]string{"cluster": "g1", "op": "userData", "direction": "read"}); !ok || v != 6 {
		t.Errorf("gen1 volume iops = %v (found=%v), want 6", v, ok)
	}
	if v, ok := gatheredValue(mfs, "pflex_volume_iops", map[string]string{"cluster": "g2", "op": "host", "direction": "read"}); !ok || v != 60 {
		t.Errorf("gen2 volume iops = %v (found=%v), want 60", v, ok)
	}
	// Both generation labels present.
	if _, ok := gatheredValue(mfs, "pflex_cluster_generation", map[string]string{"cluster": "g1", "generation": "gen1"}); !ok {
		t.Error("expected gen1 generation metric")
	}
	if _, ok := gatheredValue(mfs, "pflex_cluster_generation", map[string]string{"cluster": "g2", "generation": "gen2"}); !ok {
		t.Error("expected gen2 generation metric")
	}
}
