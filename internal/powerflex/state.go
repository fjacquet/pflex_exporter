package powerflex

import "github.com/fjacquet/pflex_exporter/internal/models"

// healthSeverity maps a PowerFlex operational-state string to a numeric severity used
// by the *_health gauges: 0 = healthy, 1 = degraded, 2 = failed/disconnected. Values
// not in this table (including the empty string) map to 2 via severityOf, so a missing
// or unrecognized signal is surfaced and alertable rather than silently healthy.
var healthSeverity = map[string]float64{
	// connection / membership / maintenance / device "good" states
	"Connected":     0,
	"Joined":        0,
	"NoMaintenance": 0,
	"Normal":        0,
	// degraded / transitional states
	"JoinPending":               1,
	"RemovePending":             1,
	"SetMaintenanceInProgress":  1,
	"InMaintenance":             1,
	"ExitMaintenanceInProgress": 1,
	"NormalTesting":             1,
	"DeviceInfoPending":         1,
	"Reserved":                  1,
	// failed / disconnected states
	"Disconnected": 2,
	"Decoupled":    2,
	"Failed":       2,
}

// severityOf returns the numeric severity for an operational-state string. Empty or
// unrecognized states return 2 (treated as failed/unknown).
func severityOf(state string) float64 {
	if sev, ok := healthSeverity[state]; ok {
		return sev
	}
	return 2
}

// stateLabelsFn returns the state-field labels (label name -> raw state string) for one
// object. The returned labels are appended to the base identity labels on the _info series.
type stateLabelsFn func(o *models.Instance) []Label

func nodeStateLabels(o *models.Instance) []Label {
	return []Label{
		{"mdm_connection_state", o.MdmConnectionState},
		{"membership_state", o.MembershipState},
		{"maintenance_state", o.MaintenanceState},
	}
}

func deviceStateLabels(o *models.Instance) []Label {
	return []Label{{"device_state", o.DeviceState}}
}

func sdcStateLabels(o *models.Instance) []Label {
	return []Label{{"mdm_connection_state", o.MdmConnectionState}}
}

// deriveStateSamples turns instance operational-state properties into health gauges and
// info metrics for one cluster. It reuses the generation's label builders so the new
// series carry the same identity/parent labels (and union label-key sets) as the
// performance metrics. gen is GenerationGen1 or GenerationGen2.
func deriveStateSamples(clusterName, systemID string, in *models.Instances, rel *models.Relations, gen string) []Sample {
	builders := labelBuildersGen1
	nodeType := models.TypeSds
	if gen == GenerationGen2 {
		builders = labelBuildersGen2
		nodeType = models.TypeStorageNode
	}

	var samples []Sample
	samples = append(samples, emitState(clusterName, systemID, in, rel, builders, nodeType, metricPrefix[nodeType], nodeStateLabels)...)
	samples = append(samples, emitState(clusterName, systemID, in, rel, builders, models.TypeDevice, metricPrefix[models.TypeDevice], deviceStateLabels)...)
	samples = append(samples, emitState(clusterName, systemID, in, rel, builders, models.TypeSdc, metricPrefix[models.TypeSdc], sdcStateLabels)...)
	samples = append(samples, volumeMappingSamples(clusterName, systemID, in, rel, builders)...)
	return samples
}

// volumeMappingSamples emits one pflex_volume_mapped_sdc info series (value 1) per
// volume->SDC mapping, correlating a volume with each host consuming it (no k8s needed).
// Volume identity/parent labels come from the generation's union volume label builder,
// then sdc_id and sdc_ip are appended in the same order for both generations.
func volumeMappingSamples(clusterName, systemID string, in *models.Instances, rel *models.Relations, builders map[string]labelBuilder) []Sample {
	builder, ok := builders[models.TypeVolume]
	if !ok {
		return nil
	}
	var samples []Sample
	for _, vol := range in.Get(models.TypeVolume) {
		base, ok := builder(clusterName, systemID, vol, in, rel)
		if !ok {
			continue
		}
		for _, m := range vol.MappedSdcInfo {
			labels := make([]Label, 0, len(base)+2)
			labels = append(labels, base...)
			labels = append(labels, Label{"sdc_id", m.SdcID}, Label{"sdc_ip", m.SdcIP})
			samples = append(samples, Sample{Name: "pflex_volume_mapped_sdc", Labels: labels, Value: 1})
		}
	}
	return samples
}

// emitState returns a <prefix>_health gauge (worst severity across the object's state
// fields) and a <prefix>_info metric (raw state strings as labels, value 1) for every
// object of objType whose label builder resolves.
func emitState(clusterName, systemID string, in *models.Instances, rel *models.Relations,
	builders map[string]labelBuilder, objType, prefix string, fn stateLabelsFn,
) []Sample {
	builder, ok := builders[objType]
	if !ok {
		return nil
	}
	var samples []Sample
	for _, obj := range in.Get(objType) {
		base, ok := builder(clusterName, systemID, obj, in, rel)
		if !ok {
			continue
		}
		stateLabels := fn(obj)
		health := 0.0
		for _, sl := range stateLabels {
			if sev := severityOf(sl.Value); sev > health {
				health = sev
			}
		}
		samples = append(samples, Sample{Name: prefix + "_health", Labels: base, Value: health})

		info := make([]Label, 0, len(base)+len(stateLabels))
		info = append(info, base...)
		info = append(info, stateLabels...)
		samples = append(samples, Sample{Name: prefix + "_info", Labels: info, Value: 1})
	}
	return samples
}
