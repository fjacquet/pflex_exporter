package powerflex

import (
	"context"
	"strings"
	"time"

	"github.com/fjacquet/pflex_exporter/internal/models"
	log "github.com/sirupsen/logrus"
	"go.opentelemetry.io/otel/trace"
	"golang.org/x/sync/errgroup"
)

// Collector runs the background collection loop: every interval it polls all clusters
// in parallel and publishes a fresh Snapshot to the store. One cluster's failure does
// not affect the others (graceful degradation).
type Collector struct {
	clients  []Client
	store    *SnapshotStore
	interval time.Duration
	timeout  time.Duration
	tracing  *TracerWrapper

	enricher      Enricher
	maxConcurrent int      // cap on clusters polled in parallel; 0 = unlimited
	slowEveryN    int      // decimation multiplier for slowTypes; <=1 disables
	slowTypes     []string // Gen2 object types collected only every slowEveryN cycles

	cycle int // collection cycle counter (serial; only touched in collectAll)
}

// CollectorOption configures optional Collector behavior.
type CollectorOption func(*Collector)

// WithEnricher injects a Kubernetes workload enricher. Defaults to a no-op.
func WithEnricher(e Enricher) CollectorOption {
	return func(c *Collector) {
		if e != nil {
			c.enricher = e
		}
	}
}

// WithMaxConcurrentClusters caps how many clusters are polled in parallel (0 = unlimited).
func WithMaxConcurrentClusters(n int) CollectorOption {
	return func(c *Collector) {
		if n > 0 {
			c.maxConcurrent = n
		}
	}
}

// WithDecimation collects the given (Gen2) object types only every everyN cycles,
// reusing the prior cycle's samples in between. everyN <= 1 or empty types disables it.
func WithDecimation(everyN int, slowTypes []string) CollectorOption {
	return func(c *Collector) {
		if everyN > 1 && len(slowTypes) > 0 {
			c.slowEveryN = everyN
			c.slowTypes = append([]string(nil), slowTypes...)
		}
	}
}

// NewCollector creates a collection loop over the given per-cluster clients.
func NewCollector(clients []Client, store *SnapshotStore, interval, timeout time.Duration, tp trace.TracerProvider, opts ...CollectorOption) *Collector {
	c := &Collector{
		clients:    clients,
		store:      store,
		interval:   interval,
		timeout:    timeout,
		tracing:    NewTracerWrapper(tp, "pflex-exporter/collector"),
		enricher:   NoopEnricher(),
		slowEveryN: 1,
	}
	for _, opt := range opts {
		opt(c)
	}
	return c
}

// CollectOnce runs a single collection cycle and publishes the result. Used for the
// synchronous startup cycle and for --once.
func (c *Collector) CollectOnce(ctx context.Context) *Snapshot {
	snap := c.collectAll(ctx)
	c.store.Store(snap)
	return snap
}

// Run drives the collection loop until ctx is cancelled. It assumes an initial
// CollectOnce has already populated the store.
func (c *Collector) Run(ctx context.Context) {
	ticker := time.NewTicker(c.interval)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			c.store.Store(c.collectAll(ctx))
		}
	}
}

func (c *Collector) collectAll(ctx context.Context) *Snapshot {
	ctx, span := c.tracing.StartSpan(ctx, "collect.cycle", trace.SpanKindInternal)
	defer span.End()

	prior := c.store.Load() // previous cycle's snapshot (for decimation reuse)

	results := make([]*ClusterSnapshot, len(c.clients))
	g, gctx := errgroup.WithContext(ctx)
	if c.maxConcurrent > 0 {
		g.SetLimit(c.maxConcurrent)
	}
	for i, client := range c.clients {
		i, client := i, client
		var priorCS *ClusterSnapshot
		if prior != nil {
			priorCS = prior.PerCluster[client.Name()]
		}
		g.Go(func() error {
			results[i] = c.collectCluster(gctx, client, priorCS)
			return nil // graceful degradation: never fail the group on one cluster
		})
	}
	_ = g.Wait()

	c.cycle++ // advance after the cycle (serial caller; no concurrent access)
	return BuildSnapshot(results)
}

func (c *Collector) collectCluster(ctx context.Context, client Client, prior *ClusterSnapshot) *ClusterSnapshot {
	ctx, cancel := context.WithTimeout(ctx, c.timeout)
	defer cancel()

	cs := &ClusterSnapshot{Cluster: client.Name(), LastScrape: time.Now()}

	instances, relations, err := client.GetInstances(ctx)
	if err != nil {
		log.Warnf("cluster %q: instances fetch failed: %v", client.Name(), err)
		cs.ScrapeError = err.Error()
		return cs
	}

	cs.Generation = detectGeneration(instances)
	if cs.Generation == GenerationGen2 {
		if c.collectGen2(ctx, client, instances, relations, prior, cs) {
			return cs
		}
		// Gen2 stats path failed; fall back to the Gen1 querySelectedStatistics path.
		if c.collectGen1(ctx, client, instances, relations, cs) {
			log.Infof("cluster %q: recovered via Gen1 statistics fallback", client.Name())
			return cs
		}
		return cs
	}

	// Gen1 (and unknown) use the querySelectedStatistics path.
	if c.collectGen1(ctx, client, instances, relations, cs) {
		return cs
	}
	// Gen1 stats path failed; fall back to the Gen2 v5 metrics path.
	if c.collectGen2(ctx, client, instances, relations, prior, cs) {
		log.Infof("cluster %q: recovered via Gen2 statistics fallback", client.Name())
		return cs
	}
	return cs
}

// collectGen2 fetches Gen2 statistics (honoring decimation) and fills cs on success.
// Returns true when samples were produced. On failure it records cs.ScrapeError and
// returns false so the caller can attempt a fallback.
func (c *Collector) collectGen2(ctx context.Context, client Client, instances *models.Instances, relations *models.Relations, prior *ClusterSnapshot, cs *ClusterSnapshot) bool {
	skip := c.slowSkipFor(prior)
	stats, err := client.GetStatisticsV5(ctx, skip...)
	if err != nil {
		log.Warnf("cluster %q: v5 statistics fetch failed: %v", client.Name(), err)
		cs.ScrapeError = err.Error()
		return false
	}
	samples := buildSamplesGen2(client.Name(), instances, relations, stats, c.enricher)
	if len(skip) > 0 {
		samples = append(samples, reusePriorStatSamples(prior, skip)...)
	}
	cs.Generation = GenerationGen2
	cs.Samples = samples
	cs.Up = true
	cs.ScrapeError = ""
	return true
}

// collectGen1 fetches Gen1 statistics and fills cs on success. Returns true when
// samples were produced; on failure it records cs.ScrapeError and returns false.
func (c *Collector) collectGen1(ctx context.Context, client Client, instances *models.Instances, relations *models.Relations, cs *ClusterSnapshot) bool {
	stats, err := client.GetStatistics(ctx)
	if err != nil {
		log.Warnf("cluster %q: statistics fetch failed: %v", client.Name(), err)
		cs.ScrapeError = err.Error()
		return false
	}
	cs.Samples = buildSamplesGen1(client.Name(), instances, relations, stats, c.enricher)
	cs.Up = true
	cs.ScrapeError = ""
	return true
}

// slowSkipFor returns the slow object types to skip this cycle, or nil when slow types
// must be collected (decimation disabled, an Nth cycle, or no reusable prior samples).
func (c *Collector) slowSkipFor(prior *ClusterSnapshot) []string {
	if c.slowEveryN <= 1 || len(c.slowTypes) == 0 || c.cycle%c.slowEveryN == 0 {
		return nil
	}
	if prior == nil || prior.Generation != GenerationGen2 || len(prior.Samples) == 0 {
		return nil // nothing safe to reuse; collect in full
	}
	return c.slowTypes
}

// buildSamplesGen1 derives every metric sample for one Gen1 cluster (querySelectedStatistics).
func buildSamplesGen1(clusterName string, in *models.Instances, rel *models.Relations, stats *models.Statistics, enricher Enricher) []Sample {
	systemID := systemIDOf(in)

	var samples []Sample

	// System: flat statistics, no per-object iteration.
	if in.System != nil && stats.System != nil {
		base := baseLabels(clusterName, systemID)
		samples = append(samples, deriveSamples(metricPrefix[models.TypeSystem], base, stats.System)...)
	}

	for objType, builder := range labelBuildersGen1 {
		prefix := metricPrefix[objType]
		for _, obj := range in.Get(objType) {
			sm := stats.Object(objType, obj.ID)
			if sm == nil {
				continue
			}
			labels, ok := builder(clusterName, systemID, obj, in, rel)
			if !ok {
				continue
			}
			labels = appendEnrichment(enricher, objType, obj, labels)
			samples = append(samples, deriveSamples(prefix, labels, sm)...)
		}
	}
	samples = append(samples, deriveStateSamples(clusterName, systemID, in, rel, GenerationGen1)...)
	samples = append(samples, inventorySamples(clusterName, systemID, in)...)
	return samples
}

// buildSamplesGen2 derives every metric sample for one Gen2 cluster (v5 metrics API).
func buildSamplesGen2(clusterName string, in *models.Instances, rel *models.Relations, stats *StatisticsV5, enricher Enricher) []Sample {
	systemID := systemIDOf(in)

	var samples []Sample

	// System: flat v5 stats keyed by the System object id.
	if in.System != nil {
		if sm := stats.Object(models.TypeSystem, in.System.ID); sm != nil {
			base := baseLabels(clusterName, systemID)
			samples = append(samples, deriveSamplesV5(metricPrefix[models.TypeSystem], base, sm, v5Metrics[models.TypeSystem])...)
		}
	}

	for objType, builder := range labelBuildersGen2 {
		prefix := metricPrefix[objType]
		mapping := v5Metrics[objType]
		for _, obj := range in.Get(objType) {
			sm := stats.Object(objType, obj.ID)
			if sm == nil {
				continue
			}
			labels, ok := builder(clusterName, systemID, obj, in, rel)
			if !ok {
				continue
			}
			labels = appendEnrichment(enricher, objType, obj, labels)
			samples = append(samples, deriveSamplesV5(prefix, labels, sm, mapping)...)
		}
	}
	samples = append(samples, deriveStateSamples(clusterName, systemID, in, rel, GenerationGen2)...)
	samples = append(samples, inventorySamples(clusterName, systemID, in)...)
	return samples
}

// appendEnrichment appends k8s workload labels to Volume and SDC metrics when the
// enricher is active. The label keys are always appended (empty when unresolved) so the
// per-metric-name label-key set stays uniform across Gen1/Gen2 and mapped/unmapped
// objects — the invariant TestMixedGenerationMetricsValid guards.
func appendEnrichment(enricher Enricher, objType string, obj *models.Instance, labels []Label) []Label {
	if enricher == nil || !enricher.Enabled() {
		return labels
	}
	switch objType {
	case models.TypeVolume:
		return append(labels, enricher.VolumeLabels(obj.ID)...)
	case models.TypeSdc:
		return append(labels, enricher.SDCLabels(obj.SdcIP)...)
	default:
		return labels
	}
}

// reusePriorStatSamples returns the prior cycle's statistics samples for the given slow
// object types (used when their fresh collection was skipped this cycle). State metrics
// (_health/_info/_mapped_sdc) are excluded because they are always re-derived fresh from
// the instance list, which is fetched every cycle.
func reusePriorStatSamples(prior *ClusterSnapshot, slowTypes []string) []Sample {
	if prior == nil || len(prior.Samples) == 0 {
		return nil
	}
	prefixes := make([]string, 0, len(slowTypes))
	for _, t := range slowTypes {
		if p := metricPrefix[t]; p != "" {
			prefixes = append(prefixes, p+"_")
		}
	}
	var reused []Sample
	for _, s := range prior.Samples {
		if isStateMetric(s.Name) {
			continue
		}
		for _, p := range prefixes {
			if strings.HasPrefix(s.Name, p) {
				reused = append(reused, s)
				break
			}
		}
	}
	return reused
}

// isStateMetric reports whether a metric name is an operational-state metric (derived
// from instance properties rather than the statistics API).
func isStateMetric(name string) bool {
	return strings.HasSuffix(name, "_health") ||
		strings.HasSuffix(name, "_info") ||
		strings.HasSuffix(name, "_mapped_sdc")
}

func systemIDOf(in *models.Instances) string {
	if in.System != nil {
		return in.System.ID
	}
	return ""
}
