// Package k8s provides optional Kubernetes workload enrichment for PowerFlex metrics.
//
// When the exporter runs in (or is configured for) a cluster that uses the Dell
// PowerFlex CSI driver, volume metrics gain namespace / PVC / PV / storageClass labels
// and SDC metrics gain a node label, resolved from the cluster's PersistentVolumes and
// Nodes. It is portable: when no usable cluster configuration is found, the Enricher
// degrades to a no-op (satisfying powerflex.Enricher with Enabled() == false).
package k8s

import (
	"context"
	"strings"
	"sync"
	"time"

	log "github.com/sirupsen/logrus"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/rest"
	"k8s.io/client-go/tools/clientcmd"

	"github.com/fjacquet/pflex_exporter/internal/powerflex"
)

// defaultCSIDriver is the Dell PowerFlex (VxFlex OS) CSI provisioner name.
const defaultCSIDriver = "csi-vxflexos.dellemc.com"

// volumeMeta is the Kubernetes workload context for a PowerFlex volume.
type volumeMeta struct {
	namespace    string
	pvc          string
	pv           string
	storageClass string
}

// Config configures a k8s Enricher.
type Config struct {
	Enabled       bool
	Kubeconfig    string        // explicit kubeconfig path; in-cluster/default used when empty
	CSIDriverName string        // CSI driver to match (defaults to csi-vxflexos.dellemc.com)
	Refresh       time.Duration // PV/Node cache refresh period
}

// Enricher resolves Kubernetes labels for PowerFlex volumes and SDCs. It implements
// powerflex.Enricher. The lookup maps are refreshed periodically behind an RWMutex.
type Enricher struct {
	clientset kubernetes.Interface
	csiDriver string
	refresh   time.Duration
	enabled   bool

	mu      sync.RWMutex
	volumes map[string]volumeMeta // PowerFlex volume ID -> workload context
	nodes   map[string]string     // SDC IP -> node name
}

// Compile-time assertion that *Enricher satisfies the collector's enrichment contract.
var _ powerflex.Enricher = (*Enricher)(nil)

// NewEnricher builds an Enricher from config. When enrichment is disabled, or when no
// usable cluster configuration can be found, it returns a no-op Enricher (Enabled()
// reports false) and never a hard error — keeping the exporter portable.
func NewEnricher(cfg Config) *Enricher {
	e := &Enricher{
		csiDriver: cfg.CSIDriverName,
		refresh:   cfg.Refresh,
		volumes:   map[string]volumeMeta{},
		nodes:     map[string]string{},
	}
	if e.csiDriver == "" {
		e.csiDriver = defaultCSIDriver
	}
	if !cfg.Enabled {
		return e
	}
	cs, err := buildClientset(cfg.Kubeconfig)
	if err != nil {
		log.Warnf("kubernetes enrichment enabled but no usable cluster config (%v); enrichment disabled", err)
		return e
	}
	e.clientset = cs
	e.enabled = true
	return e
}

// NewEnricherWithClientset builds an enabled Enricher over a provided clientset (used in
// tests with a fake clientset).
func NewEnricherWithClientset(cs kubernetes.Interface, csiDriver string) *Enricher {
	if csiDriver == "" {
		csiDriver = defaultCSIDriver
	}
	return &Enricher{
		clientset: cs,
		csiDriver: csiDriver,
		enabled:   true,
		volumes:   map[string]volumeMeta{},
		nodes:     map[string]string{},
	}
}

// Enabled reports whether enrichment labels should be appended.
func (e *Enricher) Enabled() bool { return e.enabled }

// VolumeLabels returns the canonical volume enrichment labels for a PowerFlex volume ID
// (empty values when the volume is not backed by a known PersistentVolume).
func (e *Enricher) VolumeLabels(volumeID string) []powerflex.Label {
	if !e.enabled {
		return nil
	}
	e.mu.RLock()
	m := e.volumes[volumeID]
	e.mu.RUnlock()
	return powerflex.VolumeEnrichmentLabels(m.namespace, m.pvc, m.pv, m.storageClass)
}

// SDCLabels returns the canonical SDC enrichment label for an SDC IP (empty value when
// no node matches).
func (e *Enricher) SDCLabels(sdcIP string) []powerflex.Label {
	if !e.enabled {
		return nil
	}
	e.mu.RLock()
	node := e.nodes[sdcIP]
	e.mu.RUnlock()
	return powerflex.SDCEnrichmentLabels(node)
}

// Refresh rebuilds the volume and node lookup maps from the cluster's PVs and Nodes.
func (e *Enricher) Refresh(ctx context.Context) error {
	if !e.enabled {
		return nil
	}

	pvs, err := e.clientset.CoreV1().PersistentVolumes().List(ctx, metav1.ListOptions{})
	if err != nil {
		return err
	}
	volumes := make(map[string]volumeMeta, len(pvs.Items))
	for i := range pvs.Items {
		pv := &pvs.Items[i]
		if pv.Spec.CSI == nil || pv.Spec.CSI.Driver != e.csiDriver {
			continue
		}
		volID := volumeIDFromHandle(pv.Spec.CSI.VolumeHandle)
		if volID == "" {
			continue
		}
		m := volumeMeta{pv: pv.Name, storageClass: pv.Spec.StorageClassName}
		if ref := pv.Spec.ClaimRef; ref != nil {
			m.namespace = ref.Namespace
			m.pvc = ref.Name
		}
		volumes[volID] = m
	}

	nodes, err := e.clientset.CoreV1().Nodes().List(ctx, metav1.ListOptions{})
	if err != nil {
		return err
	}
	nodeByIP := make(map[string]string, len(nodes.Items))
	for i := range nodes.Items {
		n := &nodes.Items[i]
		for _, addr := range n.Status.Addresses {
			if addr.Type == corev1.NodeInternalIP || addr.Type == corev1.NodeExternalIP {
				nodeByIP[addr.Address] = n.Name
			}
		}
	}

	e.mu.Lock()
	e.volumes = volumes
	e.nodes = nodeByIP
	e.mu.Unlock()
	log.Debugf("kubernetes enrichment refreshed: %d volumes, %d node IPs", len(volumes), len(nodeByIP))
	return nil
}

// Run refreshes the lookup maps on the configured interval until ctx is cancelled.
// It is a no-op when enrichment is disabled.
func (e *Enricher) Run(ctx context.Context) {
	if !e.enabled || e.refresh <= 0 {
		return
	}
	ticker := time.NewTicker(e.refresh)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			if err := e.Refresh(ctx); err != nil {
				log.Warnf("kubernetes enrichment refresh failed: %v", err)
			}
		}
	}
}

// volumeIDFromHandle extracts the PowerFlex volume ID from a CSI volume handle. The Dell
// csi-vxflexos handle format is "<systemID>-<volumeID>" (confirmed against Dell CSM
// docs), so the volume ID is the substring after the first hyphen.
func volumeIDFromHandle(handle string) string {
	if i := strings.Index(handle, "-"); i >= 0 && i+1 < len(handle) {
		return handle[i+1:]
	}
	return handle
}

// buildClientset resolves a Kubernetes client config (explicit kubeconfig -> in-cluster
// -> default loading rules) and returns a clientset.
func buildClientset(kubeconfig string) (kubernetes.Interface, error) {
	cfg, err := resolveConfig(kubeconfig)
	if err != nil {
		return nil, err
	}
	return kubernetes.NewForConfig(cfg)
}

func resolveConfig(kubeconfig string) (*rest.Config, error) {
	if kubeconfig != "" {
		return clientcmd.BuildConfigFromFlags("", kubeconfig)
	}
	if cfg, err := rest.InClusterConfig(); err == nil {
		return cfg, nil
	}
	// Fall back to the default loading rules (KUBECONFIG env / ~/.kube/config).
	return clientcmd.NewNonInteractiveDeferredLoadingClientConfig(
		clientcmd.NewDefaultClientConfigLoadingRules(),
		&clientcmd.ConfigOverrides{},
	).ClientConfig()
}
