package powerflex

import (
	"encoding/json"
	"testing"
)

func TestGen1PerTypeBodies(t *testing.T) {
	bodies, err := gen1PerTypeBodies()
	if err != nil {
		t.Fatalf("gen1PerTypeBodies: %v", err)
	}
	for _, want := range []string{"System", "Sds", "Sdc", "Volume", "StoragePool", "Device", "ProtectionDomain"} {
		body, ok := bodies[want]
		if !ok {
			t.Errorf("missing per-type body for %q", want)
			continue
		}
		if got := countSelectedTypes(t, body, want); got != 1 {
			t.Errorf("body for %q: want exactly 1 entry of that type, got %d", want, got)
		}
	}
}

func countSelectedTypes(t *testing.T, body []byte, typ string) int {
	t.Helper()
	var doc struct {
		SelectedStatisticsList []struct {
			Type string `json:"type"`
		} `json:"selectedStatisticsList"`
	}
	if err := json.Unmarshal(body, &doc); err != nil {
		t.Fatalf("unmarshal body: %v", err)
	}
	n := 0
	for _, e := range doc.SelectedStatisticsList {
		if e.Type == typ {
			n++
		}
	}
	return n
}
