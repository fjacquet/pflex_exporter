package powerflex

import (
	"context"
	"fmt"
	"os"
	"sort"
	"strings"
	"testing"
	"time"

	"github.com/prometheus/client_golang/prometheus"
)

// TestDumpEmittedMetricNames runs one collection cycle per generation against the
// existing mock-gateway fixtures, gathers the PromCollector output, and prints the
// sorted set of concrete metric family names for each generation. This is an audit
// helper: the printed set is the ground-truth list of names the exporter emits.
func TestDumpEmittedMetricNames(t *testing.T) {
	if os.Getenv("PFLEX_DUMP_METRICS") == "" {
		t.Skip("audit-only: set PFLEX_DUMP_METRICS=1 to dump the emitted metric-name set")
	}
	dump := func(gen, instancesFixture string) {
		g := newMockGateway(t)
		g.instancesFixture = instancesFixture
		store := NewSnapshotStore()
		c := NewCollector([]Client{g.clientNamed(t, gen+"-cluster")}, store, time.Second, 5*time.Second, nil)
		c.CollectOnce(context.Background())

		reg := prometheus.NewRegistry()
		reg.MustRegister(NewPromCollector(store))
		mfs, err := reg.Gather()
		if err != nil {
			t.Fatalf("%s gather: %v", gen, err)
		}

		names := make([]string, 0, len(mfs))
		for _, mf := range mfs {
			names = append(names, mf.GetName())
		}
		sort.Strings(names)

		// Print to stdout at column 0 so the audit grep (^(===|pflex_)) matches;
		// t.Logf would prefix file:line and break the matcher.
		var b strings.Builder
		fmt.Fprintf(&b, "=== EMITTED %s (%d) ===\n", gen, len(names))
		for _, n := range names {
			b.WriteString(n)
			b.WriteByte('\n')
		}
		if _, err := os.Stdout.WriteString(b.String()); err != nil {
			t.Fatalf("%s write: %v", gen, err)
		}
	}

	dump("gen1", "instances.json")
	dump("gen2", "instances-gen2.json")
}
