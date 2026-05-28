package models

import "encoding/json"

// StatMap is a single object's statistics: stat name -> raw JSON value. Values are
// either scalar numbers, Bwc accumulators (names ending "Bwc") or latency accumulators
// (names ending "Latency"); the consumer decides how to decode based on the name.
type StatMap map[string]json.RawMessage

// Bwc is PowerFlex's bandwidth/IO accumulator. Latency stats share this shape.
// The PowerFlex API spells the field "numOccured" (single 'r'); NumOccurred is accepted
// as a fallback in case a firmware revision uses the corrected spelling.
type Bwc struct {
	NumOccured      float64 `json:"numOccured"`
	NumOccurred     float64 `json:"numOccurred"`
	NumSeconds      float64 `json:"numSeconds"`
	TotalWeightInKb float64 `json:"totalWeightInKb"`
}

// Occurrences returns the operation count, tolerating either field spelling.
func (b Bwc) Occurrences() float64 {
	if b.NumOccured != 0 {
		return b.NumOccured
	}
	return b.NumOccurred
}

// Statistics is the parsed result of POST /api/instances/querySelectedStatistics:
// flat stats for the cluster (System) plus per-object stats grouped by type.
type Statistics struct {
	System StatMap
	// ByType maps type -> objectID -> StatMap.
	ByType map[string]map[string]StatMap
}

// ParseStatistics decodes a querySelectedStatistics response. The top-level keys are
// type names ("System", "Sds", ...); "System" holds a flat StatMap, others hold
// objectID -> StatMap.
func ParseStatistics(body []byte) (*Statistics, error) {
	var raw map[string]json.RawMessage
	if err := json.Unmarshal(body, &raw); err != nil {
		return nil, err
	}

	stats := &Statistics{ByType: make(map[string]map[string]StatMap)}
	for key, msg := range raw {
		if key == TypeSystem {
			var sys StatMap
			if err := json.Unmarshal(msg, &sys); err != nil {
				return nil, err
			}
			stats.System = sys
			continue
		}
		var byID map[string]StatMap
		if err := json.Unmarshal(msg, &byID); err != nil {
			return nil, err
		}
		stats.ByType[key] = byID
	}
	return stats, nil
}

// Object returns the StatMap for a given type and object ID, or nil if absent.
func (s *Statistics) Object(objType, id string) StatMap {
	if byID, ok := s.ByType[objType]; ok {
		return byID[id]
	}
	return nil
}
