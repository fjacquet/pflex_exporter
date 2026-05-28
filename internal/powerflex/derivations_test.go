package powerflex

import (
	"encoding/json"
	"testing"

	"github.com/fjacquet/pflex_exporter/internal/models"
)

func TestSplitDirection(t *testing.T) {
	tests := []struct {
		name, suffix  string
		wantDirection string
		wantOp        string
	}{
		{"primaryReadBwc", "Bwc", "read", "primary"},
		{"totalWriteBwc", "Bwc", "write", "total"},
		{"fwdRebuildReadBwc", "Bwc", "read", "fwdRebuild"},
		{"userDataReadBwc", "Bwc", "read", "userData"},
		{"userDataSdcReadLatency", "Latency", "read", "userDataSdc"},
		{"userDataSdcWriteLatency", "Latency", "write", "userDataSdc"},
		{"weirdBwc", "Bwc", "", "weird"},
	}
	for _, tc := range tests {
		dir, op := splitDirection(tc.name, tc.suffix)
		if dir != tc.wantDirection || op != tc.wantOp {
			t.Errorf("splitDirection(%q,%q) = (%q,%q), want (%q,%q)",
				tc.name, tc.suffix, dir, op, tc.wantDirection, tc.wantOp)
		}
	}
}

func TestPersec(t *testing.T) {
	cases := []struct {
		num, sec, want float64
	}{
		{100, 10, 10},
		{5, 2, 2}, // integer truncation of 2.5
		{5, 0, 0}, // guard against divide-by-zero
		{0, 10, 0},
	}
	for _, c := range cases {
		if got := persec(c.num, c.sec); got != c.want {
			t.Errorf("persec(%v,%v) = %v, want %v", c.num, c.sec, got, c.want)
		}
	}
}

func TestRatio(t *testing.T) {
	cases := []struct {
		num, den, want float64
	}{
		{1200, 60, 20},
		{0, 60, 0},
		{100, 0, 0},
	}
	for _, c := range cases {
		if got := ratio(c.num, c.den); got != c.want {
			t.Errorf("ratio(%v,%v) = %v, want %v", c.num, c.den, got, c.want)
		}
	}
}

func TestToSnake(t *testing.T) {
	cases := map[string]string{
		"maxCapacityInKb":    "max_capacity_in_kb",
		"compressionRatio":   "compression_ratio",
		"numOfMappedVolumes": "num_of_mapped_volumes",
		"avgReadSizeInBytes": "avg_read_size_in_bytes",
	}
	for in, want := range cases {
		if got := toSnake(in); got != want {
			t.Errorf("toSnake(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestDeriveSamples(t *testing.T) {
	stats := models.StatMap{
		"primaryReadBwc":          json.RawMessage(`{"numOccured":100,"numSeconds":10,"totalWeightInKb":2000}`),
		"userDataSdcWriteLatency": json.RawMessage(`{"numOccured":50,"numSeconds":10,"totalWeightInKb":250}`),
		"compressionRatio":        json.RawMessage(`1.5`),
		"fallbackReadBwc":         json.RawMessage(`{"numOccurred":40,"numSeconds":10,"totalWeightInKb":800}`),
	}
	base := []Label{{"cluster", "c1"}, {"cluster_id", "id1"}}
	samples := deriveSamples("pflex_test", base, stats)

	want := map[string]struct {
		op, direction string
		value         float64
	}{
		"pflex_test_iops":                    {"primary", "read", 10},
		"pflex_test_bandwidth_kb_per_second": {"primary", "read", 200},
		"pflex_test_io_size_kb":              {"primary", "read", 20},
		"pflex_test_latency":                 {"userDataSdc", "write", 5},
	}
	for name, w := range want {
		found := false
		for _, s := range samples {
			if s.Name != name {
				continue
			}
			lm := labelMap(s.Labels)
			if lm["op"] == w.op && lm["direction"] == w.direction {
				found = true
				if s.Value != w.value {
					t.Errorf("%s{op=%s,dir=%s} = %v, want %v", name, w.op, w.direction, s.Value, w.value)
				}
			}
		}
		if !found {
			t.Errorf("expected sample %s{op=%s,direction=%s} not found", name, w.op, w.direction)
		}
	}

	// scalar
	if v, ok := scalarValue(samples, "pflex_test_compression_ratio"); !ok || v != 1.5 {
		t.Errorf("compression_ratio = %v (found=%v), want 1.5", v, ok)
	}
	// numOccurred (corrected spelling) fallback is honored for iops.
	if v, ok := derivedValue(samples, "pflex_test_iops", "fallback", "read"); !ok || v != 4 {
		t.Errorf("fallback-spelling iops = %v (found=%v), want 4", v, ok)
	}
}

func labelMap(labels []Label) map[string]string {
	m := make(map[string]string, len(labels))
	for _, l := range labels {
		m[l.Name] = l.Value
	}
	return m
}

func scalarValue(samples []Sample, name string) (float64, bool) {
	for _, s := range samples {
		if s.Name == name {
			return s.Value, true
		}
	}
	return 0, false
}

func derivedValue(samples []Sample, name, op, direction string) (float64, bool) {
	for _, s := range samples {
		if s.Name != name {
			continue
		}
		lm := labelMap(s.Labels)
		if lm["op"] == op && lm["direction"] == direction {
			return s.Value, true
		}
	}
	return 0, false
}
