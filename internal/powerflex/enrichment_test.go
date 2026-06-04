package powerflex

import (
	"context"
	"testing"
	"time"

	"github.com/fjacquet/pflex_exporter/internal/models"
	"github.com/prometheus/client_golang/prometheus"
)

// stubEnricher is an always-on Enricher returning deterministic labels, used to verify
// the collector appends enrichment keys consistently without a real k8s cluster.
type stubEnricher struct{}

func (stubEnricher) Enabled() bool { return true }
func (stubEnricher) VolumeLabels(id string) []Label {
	return VolumeEnrichmentLabels("ns-"+id, "pvc-"+id, "pv-"+id, "sc1")
}
func (stubEnricher) SDCLabels(ip string) []Label { return SDCEnrichmentLabels("node-" + ip) }

func TestEnrichmentAppendsConsistentLabels(t *testing.T) {
	gen1 := newMockGateway(t) // instances.json -> gen1
	gen2 := newMockGateway(t)
	gen2.instancesFixture = "instances-gen2.json"

	store := NewSnapshotStore()
	c := NewCollector(
		[]Client{gen1.clientNamed(t, "g1"), gen2.clientNamed(t, "g2")},
		store, time.Second, 5*time.Second, nil,
		WithEnricher(stubEnricher{}),
	)
	c.CollectOnce(context.Background())
	snap := store.Load()

	// Both generations' volume series carry the enrichment keys, so a mixed /metrics
	// gather still succeeds (consistent label-key set per metric name).
	reg := prometheus.NewRegistry()
	reg.MustRegister(NewPromCollector(store))
	if _, err := reg.Gather(); err != nil {
		t.Fatalf("gather must succeed with enrichment enabled across generations: %v", err)
	}

	// Volume gains namespace/pvc/pv/storage_class; value derived from the volume ID.
	assertLabel(t, snap, "pflex_volume_iops",
		map[string]string{"cluster": "g1", "volume_id": "vol1"}, "namespace", "ns-vol1")
	assertLabel(t, snap, "pflex_volume_iops",
		map[string]string{"cluster": "g2", "volume_id": "vol1"}, "storage_class", "sc1")
	// SDC gains the k8s_node key, resolved from the SDC IP (sdc1 -> 10.0.0.5).
	if _, ok := findSample(snap, "pflex_sdc_iops", map[string]string{"cluster": "g1", "sdc_id": "sdc1"}); ok {
		assertLabel(t, snap, "pflex_sdc_iops",
			map[string]string{"cluster": "g1", "sdc_id": "sdc1"}, "k8s_node", "node-10.0.0.5")
	}
}

func TestNoEnrichmentByDefault(t *testing.T) {
	g := newMockGateway(t)
	store := NewSnapshotStore()
	c := NewCollector([]Client{g.client(t)}, store, time.Second, 5*time.Second, nil)
	c.CollectOnce(context.Background())
	snap := store.Load()

	s, ok := findSample(snap, "pflex_volume_iops", map[string]string{"volume_id": "vol1"})
	if !ok {
		t.Fatal("expected a volume sample")
	}
	for _, l := range s.Labels {
		if l.Name == "namespace" || l.Name == "k8s_node" {
			t.Errorf("did not expect enrichment label %q when enrichment is disabled", l.Name)
		}
	}
}

func TestGen2StatsFallbackToGen1(t *testing.T) {
	g := newMockGateway(t)
	g.instancesFixture = "instances-gen2.json" // detected as Gen2
	g.failStatsV5 = true                       // v5 endpoint broken -> total failure

	store := NewSnapshotStore()
	c := NewCollector([]Client{g.clientNamed(t, "gen2-cluster")}, store, time.Second, 5*time.Second, nil)
	c.CollectOnce(context.Background())

	cs := store.Load().PerCluster["gen2-cluster"]
	if cs == nil || !cs.Up {
		t.Fatalf("expected cluster up via Gen1 fallback, got %+v", cs)
	}
	if cs.ScrapeError != "" {
		t.Errorf("expected cleared scrape error after fallback, got %q", cs.ScrapeError)
	}
}

func TestGen1StatsFallbackToGen2(t *testing.T) {
	g := newMockGateway(t) // instances.json -> Gen1
	g.failStats = true     // Gen1 querySelectedStatistics broken

	store := NewSnapshotStore()
	c := NewCollector([]Client{g.clientNamed(t, "gen1-cluster")}, store, time.Second, 5*time.Second, nil)
	c.CollectOnce(context.Background())

	cs := store.Load().PerCluster["gen1-cluster"]
	if cs == nil || !cs.Up {
		t.Fatalf("expected cluster up via Gen2 fallback, got %+v", cs)
	}
}

func TestDecimationSkipsSlowTypesAndReusesSamples(t *testing.T) {
	g := newMockGateway(t)
	g.instancesFixture = "instances-gen2.json"

	store := NewSnapshotStore()
	c := NewCollector([]Client{g.clientNamed(t, "gen2-cluster")}, store, time.Second, 5*time.Second, nil,
		WithDecimation(2, []string{models.TypeDeviceGroup}),
	)

	// Cycle 0 (full): every resource type queried; DeviceGroup samples present.
	c.CollectOnce(context.Background())
	g.mu.Lock()
	full := g.statsV5Count
	g.mu.Unlock()
	if _, ok := findSample(store.Load(), "pflex_devicegroup_iops", map[string]string{"device_group_id": "dg1"}); !ok {
		t.Fatal("expected devicegroup sample on the full cycle")
	}

	// Cycle 1 (skip DeviceGroup): exactly one fewer v5 query, samples reused from prior.
	c.CollectOnce(context.Background())
	g.mu.Lock()
	delta := g.statsV5Count - full
	g.mu.Unlock()
	if delta != full-1 {
		t.Errorf("expected %d v5 queries on decimated cycle (one type skipped), got %d", full-1, delta)
	}
	if _, ok := findSample(store.Load(), "pflex_devicegroup_iops", map[string]string{"device_group_id": "dg1"}); !ok {
		t.Error("expected devicegroup sample to be reused on the decimated cycle")
	}
}

func TestMaxConcurrentClustersCollectsAll(t *testing.T) {
	g := newMockGateway(t)
	store := NewSnapshotStore()
	clients := []Client{
		g.clientNamed(t, "c1"), g.clientNamed(t, "c2"), g.clientNamed(t, "c3"),
	}
	c := NewCollector(clients, store, time.Second, 5*time.Second, nil, WithMaxConcurrentClusters(1))
	c.CollectOnce(context.Background())

	snap := store.Load()
	for _, name := range []string{"c1", "c2", "c3"} {
		if cs := snap.PerCluster[name]; cs == nil || !cs.Up {
			t.Errorf("cluster %q not collected with concurrency cap: %+v", name, cs)
		}
	}
}
