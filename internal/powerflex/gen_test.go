package powerflex

import (
	"context"
	"testing"
	"time"

	"github.com/fjacquet/pflex_exporter/internal/models"
)

func poolWithLayout(layout string) *models.Instances {
	return &models.Instances{
		ByType: map[string][]*models.Instance{
			models.TypeStoragePool: {{ID: "sp1", DataLayout: layout}},
		},
	}
}

func TestDetectGeneration(t *testing.T) {
	cases := []struct {
		name string
		in   *models.Instances
		want string
	}{
		{"medium granularity", poolWithLayout("MediumGranularity"), GenerationGen1},
		{"fine granularity", poolWithLayout("FineGranularity"), GenerationGen1},
		{"erasure coding", poolWithLayout("ErasureCoding"), GenerationGen2},
		{"no pools", &models.Instances{ByType: map[string][]*models.Instance{}}, GenerationUnknown},
		{"unknown layout", poolWithLayout("Something"), GenerationUnknown},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := detectGeneration(tc.in); got != tc.want {
				t.Errorf("detectGeneration = %q, want %q", got, tc.want)
			}
		})
	}
}

// TestGen2ClusterUsesV5Path verifies a Gen2 cluster collects via the v5 metrics API
// (not the Gen1 querySelectedStatistics endpoint) and reports generation gen2.
func TestGen2ClusterUsesV5Path(t *testing.T) {
	g := newMockGateway(t)
	g.instancesFixture = "instances-gen2.json"
	store := NewSnapshotStore()
	c := NewCollector([]Client{g.client(t)}, store, time.Second, 5*time.Second, nil)

	c.CollectOnce(context.Background())
	cs := store.Load().PerCluster["test-cluster"]

	if cs == nil || !cs.Up {
		t.Fatalf("expected cluster up, got %+v", cs)
	}
	if cs.Generation != GenerationGen2 {
		t.Errorf("generation = %q, want gen2", cs.Generation)
	}
	if len(cs.Samples) == 0 {
		t.Error("expected Gen2 cluster to produce samples")
	}

	g.mu.Lock()
	defer g.mu.Unlock()
	if g.statsCount != 0 {
		t.Errorf("Gen1 querySelectedStatistics endpoint must NOT be called for Gen2, got %d", g.statsCount)
	}
	if g.statsV5Count == 0 {
		t.Error("expected the v5 metrics endpoint to be called for Gen2")
	}
}
