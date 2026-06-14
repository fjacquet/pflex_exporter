package models

import "testing"

func TestStatisticsMerge(t *testing.T) {
	sys, err := ParseStatistics([]byte(`{"System":{"maxCapacityInKb":10}}`))
	if err != nil {
		t.Fatal(err)
	}
	sds, err := ParseStatistics([]byte(`{"Sds":{"sds1":{"primaryReadBwc":{"numOccured":1,"numSeconds":1,"totalWeightInKb":2}}}}`))
	if err != nil {
		t.Fatal(err)
	}
	agg := &Statistics{ByType: map[string]map[string]StatMap{}}
	agg.Merge(sys)
	agg.Merge(sds)
	if _, ok := agg.System["maxCapacityInKb"]; !ok {
		t.Error("merged System missing maxCapacityInKb")
	}
	if agg.Object("Sds", "sds1") == nil {
		t.Error("merged ByType missing Sds/sds1")
	}
	agg.Merge(nil) // must be a no-op, not panic
}
