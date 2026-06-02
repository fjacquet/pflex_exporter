package powerflex

import "testing"

func TestSeverityOf(t *testing.T) {
	cases := map[string]float64{
		"Connected":                 0,
		"Joined":                    0,
		"NoMaintenance":             0,
		"Normal":                    0,
		"InMaintenance":             1,
		"JoinPending":               1,
		"NormalTesting":             1,
		"RemovePending":             1,
		"SetMaintenanceInProgress":  1,
		"ExitMaintenanceInProgress": 1,
		"DeviceInfoPending":         1,
		"Reserved":                  1,
		"Disconnected":              2,
		"Decoupled":                 2,
		"Failed":                    2,
		"":                          2, // missing signal is surfaced, not silently healthy
		"SomethingUnrecognized":     2,
	}
	for state, want := range cases {
		if got := severityOf(state); got != want {
			t.Errorf("severityOf(%q) = %v, want %v", state, got, want)
		}
	}
}
