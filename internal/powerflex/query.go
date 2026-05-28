package powerflex

import _ "embed"

// queryStatsBody is the POST body for /api/instances/querySelectedStatistics: the
// full Gen1 statistic set across all 7 object types, embedded at build time.
//
//go:embed querySelectedStatistics.json
var queryStatsBody []byte
