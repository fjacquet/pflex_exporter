package models

import "testing"

func TestParseInstancesStateFields(t *testing.T) {
	body := []byte(`{
		"System": {"id": "clu1", "name": "c1", "links": []},
		"sdsList": [{"id": "sds1", "name": "s1", "mdmConnectionState": "Connected",
			"membershipState": "Joined", "maintenanceState": "NoMaintenance", "links": []}],
		"deviceList": [{"id": "dev1", "name": "d1", "deviceState": "Normal", "links": []}],
		"volumeList": [{"id": "vol1", "name": "v1",
			"mappedSdcInfo": [{"sdcId": "sdc1", "sdcIp": "10.0.0.5"}], "links": []}]
	}`)

	in, _, err := ParseInstances(body)
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	sds := in.Get(TypeSds)[0]
	if sds.MdmConnectionState != "Connected" || sds.MembershipState != "Joined" || sds.MaintenanceState != "NoMaintenance" {
		t.Errorf("sds state = %q/%q/%q", sds.MdmConnectionState, sds.MembershipState, sds.MaintenanceState)
	}
	if got := in.Get(TypeDevice)[0].DeviceState; got != "Normal" {
		t.Errorf("device state = %q, want Normal", got)
	}
	vol := in.Get(TypeVolume)[0]
	if len(vol.MappedSdcInfo) != 1 || vol.MappedSdcInfo[0].SdcID != "sdc1" || vol.MappedSdcInfo[0].SdcIP != "10.0.0.5" {
		t.Errorf("mappedSdcInfo = %+v", vol.MappedSdcInfo)
	}
}

func TestParseInstancesCoverageFields(t *testing.T) {
	body := []byte(`{
		"System": {"id": "clu1", "name": "c1", "numOfVolumes": 12, "numOfSds": 4, "numOfDevices": 30, "links": []},
		"sdtList": [{"id": "sdt1", "name": "t1", "sdtState": "Normal", "links": []}],
		"deviceList": [{"id": "dev1", "name": "d1", "deviceState": "Normal",
			"temperatureState": "NormalTemperature", "ssdEndOfLifeState": "NormalEndOfLife", "errorState": "None", "links": []}]
	}`)

	in, _, err := ParseInstances(body)
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	if got := in.Get(TypeSdt)[0].SdtState; got != "Normal" {
		t.Errorf("SdtState = %q, want Normal", got)
	}
	d := in.Get(TypeDevice)[0]
	if d.TemperatureState != "NormalTemperature" || d.SsdEndOfLifeState != "NormalEndOfLife" || d.ErrorState != "None" {
		t.Errorf("device wear = %q/%q/%q", d.TemperatureState, d.SsdEndOfLifeState, d.ErrorState)
	}
	sys := in.System
	if sys.NumOfVolumes != 12 || sys.NumOfSds != 4 || sys.NumOfDevices != 30 {
		t.Errorf("counts = %d/%d/%d", sys.NumOfVolumes, sys.NumOfSds, sys.NumOfDevices)
	}
}
