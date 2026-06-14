package powerflex

import "github.com/fjacquet/pflex_exporter/internal/models"

// inventorySamples emits cluster-scoped inventory count gauges (pflex_cluster_num_of_*)
// from the System instance's properties. These are authoritative API counts, not len() of
// the relations graph (which can disagree mid-transition). WS2-13.
func inventorySamples(clusterName, systemID string, in *models.Instances) []Sample {
	if in == nil || in.System == nil {
		return nil
	}
	base := baseLabels(clusterName, systemID)
	prefix := metricPrefix[models.TypeSystem]
	return []Sample{
		{Name: prefix + "_num_of_volumes", Labels: base, Value: float64(in.System.NumOfVolumes)},
		{Name: prefix + "_num_of_sds", Labels: base, Value: float64(in.System.NumOfSds)},
		{Name: prefix + "_num_of_devices", Labels: base, Value: float64(in.System.NumOfDevices)},
	}
}
