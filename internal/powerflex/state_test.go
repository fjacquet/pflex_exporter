package powerflex

import (
	"context"
	"testing"
	"time"
)

func TestSeverityOf(t *testing.T) {
	cases := map[string]float64{
		"Connected":                 0,
		"Joined":                    0,
		"NoMaintenance":             0,
		"Normal":                    0,
		"InMaintenance":             1,
		"JoinPending":               1,
		"NormalTesting":             1,
		"RemovePending":             1,
		"SetMaintenanceInProgress":  1,
		"ExitMaintenanceInProgress": 1,
		"DeviceInfoPending":         1,
		"Reserved":                  1,
		"Disconnected":              2,
		"Decoupled":                 2,
		"Failed":                    2,
		"":                          2, // missing signal is surfaced, not silently healthy
		"SomethingUnrecognized":     2,
	}
	for state, want := range cases {
		if got := severityOf(state); got != want {
			t.Errorf("severityOf(%q) = %v, want %v", state, got, want)
		}
	}
}

func gen1Snapshot(t *testing.T) *Snapshot {
	t.Helper()
	g := newMockGateway(t) // default instances.json -> gen1
	store := NewSnapshotStore()
	c := NewCollector([]Client{g.clientNamed(t, "gen1-cluster")}, store, time.Second, 5*time.Second, nil)
	c.CollectOnce(context.Background())
	return store.Load()
}

func TestGen1StateSamples(t *testing.T) {
	snap := gen1Snapshot(t)

	assertSample(t, snap, "pflex_sds_health", map[string]string{"sds_id": "sds1"}, 0)
	assertSample(t, snap, "pflex_device_health", map[string]string{"device_id": "dev1"}, 0)
	assertSample(t, snap, "pflex_sdc_health", map[string]string{"sdc_id": "sdc1"}, 0)

	assertSample(t, snap, "pflex_sds_info", map[string]string{"sds_id": "sds1"}, 1)
	assertLabel(t, snap, "pflex_sds_info", map[string]string{"sds_id": "sds1"}, "mdm_connection_state", "Connected")
	assertLabel(t, snap, "pflex_sds_info", map[string]string{"sds_id": "sds1"}, "membership_state", "Joined")
	assertLabel(t, snap, "pflex_sds_info", map[string]string{"sds_id": "sds1"}, "maintenance_state", "NoMaintenance")
	assertLabel(t, snap, "pflex_device_info", map[string]string{"device_id": "dev1"}, "device_state", "Normal")
	assertLabel(t, snap, "pflex_sdc_info", map[string]string{"sdc_id": "sdc1"}, "mdm_connection_state", "Connected")
}

func TestGen2StateSamples(t *testing.T) {
	g := newMockGateway(t)
	g.instancesFixture = "instances-gen2.json"
	store := NewSnapshotStore()
	c := NewCollector([]Client{g.clientNamed(t, "gen2-cluster")}, store, time.Second, 5*time.Second, nil)
	c.CollectOnce(context.Background())
	snap := store.Load()

	assertSample(t, snap, "pflex_storagenode_health", map[string]string{"storage_node_id": "sn1"}, 0)
	assertSample(t, snap, "pflex_device_health", map[string]string{"device_id": "dev1"}, 0)
	assertSample(t, snap, "pflex_sdc_health", map[string]string{"sdc_id": "sdc1"}, 0)
	assertLabel(t, snap, "pflex_storagenode_info", map[string]string{"storage_node_id": "sn1"}, "membership_state", "Joined")
}

func TestStateHealthDegradedAndFailed(t *testing.T) {
	g := newMockGateway(t)
	g.instancesFixture = "instances-unhealthy.json"
	store := NewSnapshotStore()
	c := NewCollector([]Client{g.clientNamed(t, "unhealthy")}, store, time.Second, 5*time.Second, nil)
	c.CollectOnce(context.Background())
	snap := store.Load()

	// mdm_connection_state=Disconnected -> severity 2; worst-of-N across the SDS's
	// three state fields (Disconnected=2, Joined=0, NoMaintenance=0) must be 2.
	assertSample(t, snap, "pflex_sds_health", map[string]string{"sds_id": "sds1"}, 2)
	assertLabel(t, snap, "pflex_sds_info", map[string]string{"sds_id": "sds1"}, "mdm_connection_state", "Disconnected")
}

func TestVolumeMappedSdc(t *testing.T) {
	// Gen1
	snap1 := gen1Snapshot(t)
	assertSample(t, snap1, "pflex_volume_mapped_sdc",
		map[string]string{"volume_id": "vol1", "sdc_id": "sdc1"}, 1)
	assertLabel(t, snap1, "pflex_volume_mapped_sdc",
		map[string]string{"volume_id": "vol1", "sdc_id": "sdc1"}, "sdc_ip", "10.0.0.5")

	// Gen2
	g := newMockGateway(t)
	g.instancesFixture = "instances-gen2.json"
	store := NewSnapshotStore()
	c := NewCollector([]Client{g.clientNamed(t, "gen2-cluster")}, store, time.Second, 5*time.Second, nil)
	c.CollectOnce(context.Background())
	snap2 := store.Load()
	assertSample(t, snap2, "pflex_volume_mapped_sdc",
		map[string]string{"volume_id": "vol1", "sdc_id": "sdc1"}, 1)
	assertLabel(t, snap2, "pflex_volume_mapped_sdc",
		map[string]string{"volume_id": "vol1", "sdc_id": "sdc1"}, "sdc_ip", "10.0.0.9")
}
