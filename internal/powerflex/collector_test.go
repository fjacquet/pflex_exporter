package powerflex

import (
	"context"
	"testing"
	"time"

	"github.com/prometheus/client_golang/prometheus"
	dto "github.com/prometheus/client_model/go"
	"go.opentelemetry.io/otel/attribute"
	sdkmetric "go.opentelemetry.io/otel/sdk/metric"
	"go.opentelemetry.io/otel/sdk/metric/metricdata"
)

// newTestCollector wires a collector over a single mock-gateway-backed client.
func newTestCollector(t *testing.T) (*Collector, *SnapshotStore) {
	t.Helper()
	g := newMockGateway(t)
	store := NewSnapshotStore()
	c := NewCollector([]Client{g.client(t)}, store, time.Second, 5*time.Second, nil)
	return c, store
}

func TestCollectOnceBuildsSamples(t *testing.T) {
	c, store := newTestCollector(t)
	c.CollectOnce(context.Background())
	snap := store.Load()

	cs := snap.PerCluster["test-cluster"]
	if cs == nil || !cs.Up {
		t.Fatalf("expected cluster up, got %+v", cs)
	}
	if cs.Generation != GenerationGen1 {
		t.Errorf("expected gen1, got %q", cs.Generation)
	}

	// Volume vol1 userDataReadBwc{numOccured:60,numSeconds:10,totalWeightInKb:1200}
	//   iops = 60/10 = 6 ; bandwidth = 1200/10 = 120 ; io_size = 1200/60 = 20
	assertSample(t, snap, "pflex_volume_iops",
		map[string]string{"op": "userData", "direction": "read", "volume_id": "vol1"}, 6)
	assertSample(t, snap, "pflex_volume_bandwidth_kb_per_second",
		map[string]string{"op": "userData", "direction": "read", "volume_id": "vol1"}, 120)
	assertSample(t, snap, "pflex_volume_io_size_kb",
		map[string]string{"op": "userData", "direction": "read", "volume_id": "vol1"}, 20)
	// userDataSdcReadLatency{60,10,180} -> latency = 180/60 = 3
	assertSample(t, snap, "pflex_volume_latency",
		map[string]string{"op": "userDataSdc", "direction": "read", "volume_id": "vol1"}, 3)

	// System scalar + derived.
	assertSample(t, snap, "pflex_cluster_max_capacity_in_kb",
		map[string]string{"cluster": "test-cluster"}, 1000000)
	assertSample(t, snap, "pflex_cluster_iops",
		map[string]string{"op": "primary", "direction": "read"}, 10)

	// Volume parent labels resolved through the relations graph.
	assertLabel(t, snap, "pflex_volume_iops",
		map[string]string{"volume_id": "vol1"}, "storage_pool_name", "pool-1")
	assertLabel(t, snap, "pflex_volume_iops",
		map[string]string{"volume_id": "vol1"}, "protection_domain_name", "pd-1")
}

func TestPrometheusExportsSnapshot(t *testing.T) {
	c, store := newTestCollector(t)
	c.CollectOnce(context.Background())

	reg := prometheus.NewRegistry()
	reg.MustRegister(NewPromCollector(store))

	mfs, err := reg.Gather()
	if err != nil {
		t.Fatalf("gather: %v", err)
	}

	if v, ok := gatheredValue(mfs, "pflex_up", map[string]string{"cluster": "test-cluster"}); !ok || v != 1 {
		t.Errorf("pflex_up = %v (found=%v), want 1", v, ok)
	}
	if v, ok := gatheredValue(mfs, "pflex_volume_iops", map[string]string{"op": "userData", "direction": "read", "volume_id": "vol1"}); !ok || v != 6 {
		t.Errorf("pflex_volume_iops = %v (found=%v), want 6", v, ok)
	}
	if v, ok := gatheredValue(mfs, "pflex_cluster_generation", map[string]string{"cluster": "test-cluster", "generation": "gen1"}); !ok || v != 1 {
		t.Errorf("pflex_cluster_generation = %v (found=%v), want 1", v, ok)
	}
}

func TestOTLPExportsSnapshot(t *testing.T) {
	c, store := newTestCollector(t)
	c.CollectOnce(context.Background())

	reader := sdkmetric.NewManualReader()
	exp := newOTLPExporter(reader, store, "test")
	if err := exp.EnsureInstruments(); err != nil {
		t.Fatalf("EnsureInstruments: %v", err)
	}

	var rm metricdata.ResourceMetrics
	if err := reader.Collect(context.Background(), &rm); err != nil {
		t.Fatalf("collect: %v", err)
	}

	if v, ok := otlpGaugeValue(rm, "pflex_volume_iops", map[string]string{"op": "userData", "direction": "read", "volume_id": "vol1"}); !ok || v != 6 {
		t.Errorf("OTLP pflex_volume_iops = %v (found=%v), want 6", v, ok)
	}
}

// --- assertion helpers ---

func findSample(snap *Snapshot, name string, match map[string]string) (Sample, bool) {
	for _, s := range snap.SamplesByName(name) {
		if labelsMatch(s.Labels, match) {
			return s, true
		}
	}
	return Sample{}, false
}

func assertSample(t *testing.T, snap *Snapshot, name string, match map[string]string, want float64) {
	t.Helper()
	s, ok := findSample(snap, name, match)
	if !ok {
		t.Errorf("sample %s%v not found", name, match)
		return
	}
	if s.Value != want {
		t.Errorf("%s%v = %v, want %v", name, match, s.Value, want)
	}
}

func assertLabel(t *testing.T, snap *Snapshot, name string, match map[string]string, labelName, want string) {
	t.Helper()
	s, ok := findSample(snap, name, match)
	if !ok {
		t.Errorf("sample %s%v not found", name, match)
		return
	}
	for _, l := range s.Labels {
		if l.Name == labelName {
			if l.Value != want {
				t.Errorf("%s label %s = %q, want %q", name, labelName, l.Value, want)
			}
			return
		}
	}
	t.Errorf("%s has no label %q", name, labelName)
}

func labelsMatch(labels []Label, match map[string]string) bool {
	have := make(map[string]string, len(labels))
	for _, l := range labels {
		have[l.Name] = l.Value
	}
	for k, v := range match {
		if have[k] != v {
			return false
		}
	}
	return true
}

// TestGen1NewCoverageMetrics asserts the WS2-11/12/15/17 stats added to
// querySelectedStatistics.json are emitted by the automatic Gen1 derivation.
func TestGen1NewCoverageMetrics(t *testing.T) {
	c, store := newTestCollector(t)
	c.CollectOnce(context.Background())

	reg := prometheus.NewRegistry()
	reg.MustRegister(NewPromCollector(store))
	mfs, err := reg.Gather()
	if err != nil {
		t.Fatalf("gather: %v", err)
	}

	// Scalar capacity / job-count gauges (WS2-11/12/15).
	for _, name := range []string{
		"pflex_cluster_degraded_healthy_capacity_in_kb",
		"pflex_cluster_fwd_rebuild_capacity_in_kb",
		"pflex_cluster_snap_capacity_in_use_in_kb",
		"pflex_storagepool_degraded_healthy_capacity_in_kb",
		"pflex_storagepool_rebalance_capacity_in_kb",
		"pflex_storagepool_pending_moving_in_bck_rebuild_jobs",
		"pflex_protectiondomain_bck_rebuild_capacity_in_kb",
	} {
		if _, ok := gatheredValue(mfs, name, map[string]string{}); !ok {
			t.Errorf("missing expected Gen1 coverage metric %q", name)
		}
	}

	// Target/journaler latency (WS2-17) — the Latency suffix splits into op/direction.
	if _, ok := gatheredValue(mfs, "pflex_protectiondomain_latency", map[string]string{"op": "target", "direction": "read"}); !ok {
		t.Error("missing pflex_protectiondomain_latency{op=target,direction=read}")
	}
	if _, ok := gatheredValue(mfs, "pflex_cluster_latency", map[string]string{"op": "journaler", "direction": "write"}); !ok {
		t.Error("missing pflex_cluster_latency{op=journaler,direction=write}")
	}
}

func gatheredValue(mfs []*dto.MetricFamily, name string, match map[string]string) (float64, bool) {
	for _, mf := range mfs {
		if mf.GetName() != name {
			continue
		}
		for _, m := range mf.GetMetric() {
			if dtoLabelsMatch(m, match) {
				return m.GetGauge().GetValue(), true
			}
		}
	}
	return 0, false
}

func dtoLabelsMatch(m *dto.Metric, match map[string]string) bool {
	have := make(map[string]string, len(m.GetLabel()))
	for _, l := range m.GetLabel() {
		have[l.GetName()] = l.GetValue()
	}
	for k, v := range match {
		if have[k] != v {
			return false
		}
	}
	return true
}

func otlpGaugeValue(rm metricdata.ResourceMetrics, name string, match map[string]string) (float64, bool) {
	for _, sm := range rm.ScopeMetrics {
		for _, m := range sm.Metrics {
			if m.Name != name {
				continue
			}
			g, ok := m.Data.(metricdata.Gauge[float64])
			if !ok {
				continue
			}
			for _, dp := range g.DataPoints {
				if otlpAttrsMatch(dp, match) {
					return dp.Value, true
				}
			}
		}
	}
	return 0, false
}

func otlpAttrsMatch(dp metricdata.DataPoint[float64], match map[string]string) bool {
	for k, v := range match {
		val, ok := dp.Attributes.Value(attribute.Key(k))
		if !ok || val.AsString() != v {
			return false
		}
	}
	return true
}
