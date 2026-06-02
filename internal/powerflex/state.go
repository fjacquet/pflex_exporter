package powerflex

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
