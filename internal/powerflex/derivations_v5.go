package powerflex

// deriveSamplesV5 converts one Gen2 object's pre-computed v5 statistics into samples.
// Unlike Gen1, no math is needed: each v5 metric maps directly to a kind/op/direction.
// Gen2 values are bytes/s, bytes and microseconds, so derived metric names carry explicit
// units distinct from the Gen1 names.
func deriveSamplesV5(prefix string, objLabels []Label, stats map[string]float64, mapping map[string]v5Mapping) []Sample {
	var samples []Sample
	for name, value := range stats {
		m, ok := mapping[name]
		if !ok {
			continue
		}
		switch m.Kind {
		case v5KindIOPS:
			samples = append(samples, derived(prefix+"_iops", objLabels, m.Op, m.Direction, value))
		case v5KindBW:
			samples = append(samples, derived(prefix+"_bandwidth_bytes_per_second", objLabels, m.Op, m.Direction, value))
		case v5KindIOSize:
			samples = append(samples, derived(prefix+"_io_size_bytes", objLabels, m.Op, m.Direction, value))
		case v5KindLatency:
			samples = append(samples, derived(prefix+"_latency_microseconds", objLabels, m.Op, m.Direction, value))
		default: // scalar gauge named from the v5 field
			samples = append(samples, Sample{Name: prefix + "_" + toSnake(m.Op), Labels: objLabels, Value: value})
		}
	}
	return samples
}
