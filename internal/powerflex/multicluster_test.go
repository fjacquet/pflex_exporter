package powerflex

import (
	"context"
	"testing"
	"time"
)

// TestMultiClusterDegradation verifies that one cluster failing does not prevent the
// others from being collected and exposed.
func TestMultiClusterDegradation(t *testing.T) {
	good := newMockGateway(t)
	bad := newMockGateway(t)
	bad.failInstances = true

	store := NewSnapshotStore()
	c := NewCollector(
		[]Client{good.clientNamed(t, "good"), bad.clientNamed(t, "bad")},
		store, time.Second, 5*time.Second, nil,
	)
	c.CollectOnce(context.Background())
	snap := store.Load()

	goodCS := snap.PerCluster["good"]
	badCS := snap.PerCluster["bad"]
	if goodCS == nil || !goodCS.Up {
		t.Fatalf("good cluster should be up: %+v", goodCS)
	}
	if badCS == nil || badCS.Up {
		t.Fatalf("bad cluster should be down: %+v", badCS)
	}
	if badCS.ScrapeError == "" {
		t.Error("bad cluster should record a scrape error")
	}
	if len(goodCS.Samples) == 0 {
		t.Error("good cluster should still produce samples despite the other failing")
	}

	// The good cluster's metrics carry its own cluster label.
	if _, ok := findSample(snap, "pflex_volume_iops", map[string]string{"cluster": "good", "volume_id": "vol1"}); !ok {
		t.Error("expected good cluster volume metric to be present")
	}
}
