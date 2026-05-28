package powerflex

import (
	"encoding/json"
	"strings"

	"github.com/fjacquet/pflex_exporter/internal/models"
)

// deriveSamples converts one object's raw statistics into exported samples, porting
// Dell's stringifyFields:
//   - "*Bwc" accumulators -> iops, bandwidth (KB/s) and average IO size (KB), each
//     split into op (the prefix) and direction (read/write) labels.
//   - "*Latency" accumulators -> average latency, split into op and direction.
//   - everything else -> a scalar gauge named pflex_<obj>_<snake(stat)>.
//
// prefix is the object's metric prefix (e.g. "pflex_volume"); objLabels are the
// object's identity/parent labels.
func deriveSamples(prefix string, objLabels []Label, stats models.StatMap) []Sample {
	var samples []Sample
	for name, raw := range stats {
		switch {
		case strings.HasSuffix(name, "Bwc"):
			bwc, ok := decodeBwc(raw)
			if !ok {
				continue
			}
			direction, op := splitDirection(name, "Bwc")
			samples = append(samples,
				derived(prefix+"_iops", objLabels, op, direction, persec(bwc.Occurrences(), bwc.NumSeconds)),
				derived(prefix+"_bandwidth_kb_per_second", objLabels, op, direction, persec(bwc.TotalWeightInKb, bwc.NumSeconds)),
				derived(prefix+"_io_size_kb", objLabels, op, direction, ratio(bwc.TotalWeightInKb, bwc.Occurrences())),
			)
		case strings.HasSuffix(name, "Latency"):
			bwc, ok := decodeBwc(raw)
			if !ok {
				continue
			}
			direction, op := splitDirection(name, "Latency")
			samples = append(samples,
				derived(prefix+"_latency", objLabels, op, direction, ratio(bwc.TotalWeightInKb, bwc.Occurrences())),
			)
		default:
			v, ok := decodeFloat(raw)
			if !ok {
				continue
			}
			samples = append(samples, Sample{
				Name:   prefix + "_" + toSnake(name),
				Labels: objLabels,
				Value:  v,
			})
		}
	}
	return samples
}

// splitDirection extracts the read/write direction and the op prefix from a stat name
// for a given accumulator suffix ("Bwc" or "Latency").
// e.g. ("primaryReadBwc","Bwc") -> ("read","primary"); ("totalWriteBwc","Bwc") -> ("write","total").
func splitDirection(name, suffix string) (direction, op string) {
	switch {
	case strings.HasSuffix(name, "Read"+suffix):
		return "read", strings.TrimSuffix(name, "Read"+suffix)
	case strings.HasSuffix(name, "Write"+suffix):
		return "write", strings.TrimSuffix(name, "Write"+suffix)
	default:
		return "", strings.TrimSuffix(name, suffix)
	}
}

// persec returns an integer-truncated per-second rate (matching Dell's persec), or 0
// when seconds is non-positive.
func persec(numerator, seconds float64) float64 {
	if seconds <= 0 {
		return 0
	}
	return float64(int64(numerator / seconds))
}

// ratio returns numerator/denominator, or 0 when either is non-positive (matching
// Dell's guard that avoids division by zero and emits 0 when there is no weight).
func ratio(numerator, denominator float64) float64 {
	if numerator <= 0 || denominator <= 0 {
		return 0
	}
	return numerator / denominator
}

// derived builds a sample for a derived metric, appending op and direction labels to
// a copy of the object's base labels.
func derived(name string, base []Label, op, direction string, value float64) Sample {
	labels := make([]Label, 0, len(base)+2)
	labels = append(labels, base...)
	labels = append(labels, Label{"op", op}, Label{"direction", direction})
	return Sample{Name: name, Labels: labels, Value: value}
}

func decodeBwc(raw json.RawMessage) (models.Bwc, bool) {
	var b models.Bwc
	if err := json.Unmarshal(raw, &b); err != nil {
		return models.Bwc{}, false
	}
	return b, true
}

func decodeFloat(raw json.RawMessage) (float64, bool) {
	var f float64
	if err := json.Unmarshal(raw, &f); err != nil {
		return 0, false
	}
	return f, true
}
