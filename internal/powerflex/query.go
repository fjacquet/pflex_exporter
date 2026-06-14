package powerflex

import (
	_ "embed"
	"encoding/json"
)

// queryStatsBody is the POST body for /api/instances/querySelectedStatistics: the
// full Gen1 statistic set across all 7 object types, embedded at build time.
//
//go:embed querySelectedStatistics.json
var queryStatsBody []byte

// gen1SelectedStatistics mirrors the embedded querySelectedStatistics.json shape.
type gen1SelectedStatistics struct {
	SelectedStatisticsList []json.RawMessage `json:"selectedStatisticsList"`
}

// gen1PerTypeBodies splits the embedded querySelectedStatistics.json into one request body
// per object type, each a selectedStatisticsList with a single entry. This lets Gen1 stats
// be fetched per type (one bad type cannot fail the others; see ADR 0002). Keyed by the
// entry's "type".
func gen1PerTypeBodies() (map[string][]byte, error) {
	var doc gen1SelectedStatistics
	if err := json.Unmarshal(queryStatsBody, &doc); err != nil {
		return nil, err
	}
	out := make(map[string][]byte, len(doc.SelectedStatisticsList))
	for _, entry := range doc.SelectedStatisticsList {
		var meta struct {
			Type string `json:"type"`
		}
		if err := json.Unmarshal(entry, &meta); err != nil {
			return nil, err
		}
		body, err := json.Marshal(map[string]any{
			"selectedStatisticsList": []json.RawMessage{entry},
		})
		if err != nil {
			return nil, err
		}
		out[meta.Type] = body
	}
	return out, nil
}
