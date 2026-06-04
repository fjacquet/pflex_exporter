package k8s

import (
	"context"
	"testing"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes/fake"

	"github.com/fjacquet/pflex_exporter/internal/powerflex"
)

const testDriver = "csi-vxflexos.dellemc.com"

func labelValue(labels []powerflex.Label, name string) (string, bool) {
	for _, l := range labels {
		if l.Name == name {
			return l.Value, true
		}
	}
	return "", false
}

func TestEnricherResolvesVolumesAndNodes(t *testing.T) {
	matchingPV := &corev1.PersistentVolume{
		ObjectMeta: metav1.ObjectMeta{Name: "pv-1"},
		Spec: corev1.PersistentVolumeSpec{
			StorageClassName: "vxflexos",
			ClaimRef:         &corev1.ObjectReference{Namespace: "team-a", Name: "data-claim"},
			PersistentVolumeSource: corev1.PersistentVolumeSource{
				CSI: &corev1.CSIPersistentVolumeSource{
					Driver:       testDriver,
					VolumeHandle: "sysid1-vol123", // <systemID>-<volumeID>
				},
			},
		},
	}
	// A PV from a different CSI driver must be ignored.
	otherPV := &corev1.PersistentVolume{
		ObjectMeta: metav1.ObjectMeta{Name: "pv-other"},
		Spec: corev1.PersistentVolumeSpec{
			PersistentVolumeSource: corev1.PersistentVolumeSource{
				CSI: &corev1.CSIPersistentVolumeSource{Driver: "ebs.csi.aws.com", VolumeHandle: "x-y"},
			},
		},
	}
	node := &corev1.Node{
		ObjectMeta: metav1.ObjectMeta{Name: "worker-1"},
		Status: corev1.NodeStatus{Addresses: []corev1.NodeAddress{
			{Type: corev1.NodeHostName, Address: "worker-1"},
			{Type: corev1.NodeInternalIP, Address: "10.0.0.5"},
		}},
	}

	cs := fake.NewSimpleClientset(matchingPV, otherPV, node)
	e := NewEnricherWithClientset(cs, testDriver)
	if err := e.Refresh(context.Background()); err != nil {
		t.Fatalf("Refresh: %v", err)
	}

	// Volume vol123 resolves to its PV/PVC/namespace/storageClass.
	vl := e.VolumeLabels("vol123")
	if v, _ := labelValue(vl, "namespace"); v != "team-a" {
		t.Errorf("namespace = %q, want team-a", v)
	}
	if v, _ := labelValue(vl, "persistent_volume_claim"); v != "data-claim" {
		t.Errorf("pvc = %q, want data-claim", v)
	}
	if v, _ := labelValue(vl, "persistent_volume"); v != "pv-1" {
		t.Errorf("pv = %q, want pv-1", v)
	}
	if v, _ := labelValue(vl, "storage_class"); v != "vxflexos" {
		t.Errorf("storage_class = %q, want vxflexos", v)
	}

	// Unknown volume still yields the canonical keys with empty values (consistency).
	ul := e.VolumeLabels("does-not-exist")
	if len(ul) != 4 {
		t.Errorf("unmapped volume should still carry 4 keys, got %d", len(ul))
	}
	if v, _ := labelValue(ul, "namespace"); v != "" {
		t.Errorf("unmapped namespace = %q, want empty", v)
	}

	// SDC IP resolves to the node name (internal IP only).
	if v, _ := labelValue(e.SDCLabels("10.0.0.5"), "k8s_node"); v != "worker-1" {
		t.Errorf("k8s_node = %q, want worker-1", v)
	}
	if v, _ := labelValue(e.SDCLabels("203.0.113.9"), "k8s_node"); v != "" {
		t.Errorf("unmatched SDC IP should yield empty node, got %q", v)
	}
}

func TestDisabledEnricherIsNoop(t *testing.T) {
	e := NewEnricher(Config{Enabled: false})
	if e.Enabled() {
		t.Fatal("expected disabled enricher")
	}
	if e.VolumeLabels("vol123") != nil {
		t.Error("disabled enricher should return nil volume labels")
	}
	if e.SDCLabels("10.0.0.5") != nil {
		t.Error("disabled enricher should return nil SDC labels")
	}
	if err := e.Refresh(context.Background()); err != nil {
		t.Errorf("disabled Refresh should be a no-op, got %v", err)
	}
}

func TestVolumeIDFromHandle(t *testing.T) {
	cases := map[string]string{
		"sysid1-vol123":     "vol123",
		"abc-def-ghi":       "def-ghi", // only the first hyphen splits systemID from volumeID
		"novolumeseparator": "novolumeseparator",
	}
	for handle, want := range cases {
		if got := volumeIDFromHandle(handle); got != want {
			t.Errorf("volumeIDFromHandle(%q) = %q, want %q", handle, got, want)
		}
	}
}
