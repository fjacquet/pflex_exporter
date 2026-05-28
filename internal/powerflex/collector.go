package powerflex

import (
	"context"
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
}

// NewCollector creates a collection loop over the given per-cluster clients.
func NewCollector(clients []Client, store *SnapshotStore, interval, timeout time.Duration, tp trace.TracerProvider) *Collector {
	return &Collector{
		clients:  clients,
		store:    store,
		interval: interval,
		timeout:  timeout,
		tracing:  NewTracerWrapper(tp, "pflex-exporter/collector"),
	}
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

	results := make([]*ClusterSnapshot, len(c.clients))
	g, gctx := errgroup.WithContext(ctx)
	for i, client := range c.clients {
		i, client := i, client
		g.Go(func() error {
			results[i] = c.collectCluster(gctx, client)
			return nil // graceful degradation: never fail the group on one cluster
		})
	}
	_ = g.Wait()
	return BuildSnapshot(results)
}

func (c *Collector) collectCluster(ctx context.Context, client Client) *ClusterSnapshot {
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
		stats, err := client.GetStatisticsV5(ctx)
		if err != nil {
			log.Warnf("cluster %q: v5 statistics fetch failed: %v", client.Name(), err)
			cs.ScrapeError = err.Error()
			return cs
		}
		cs.Samples = buildSamplesGen2(client.Name(), instances, relations, stats)
		cs.Up = true
		return cs
	}

	// Gen1 (and unknown) use the querySelectedStatistics path.
	stats, err := client.GetStatistics(ctx)
	if err != nil {
		log.Warnf("cluster %q: statistics fetch failed: %v", client.Name(), err)
		cs.ScrapeError = err.Error()
		return cs
	}
	cs.Samples = buildSamplesGen1(client.Name(), instances, relations, stats)
	cs.Up = true
	return cs
}

// buildSamplesGen1 derives every metric sample for one Gen1 cluster (querySelectedStatistics).
func buildSamplesGen1(clusterName string, in *models.Instances, rel *models.Relations, stats *models.Statistics) []Sample {
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
			samples = append(samples, deriveSamples(prefix, labels, sm)...)
		}
	}
	return samples
}

// buildSamplesGen2 derives every metric sample for one Gen2 cluster (v5 metrics API).
func buildSamplesGen2(clusterName string, in *models.Instances, rel *models.Relations, stats *StatisticsV5) []Sample {
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
			samples = append(samples, deriveSamplesV5(prefix, labels, sm, mapping)...)
		}
	}
	return samples
}

func systemIDOf(in *models.Instances) string {
	if in.System != nil {
		return in.System.ID
	}
	return ""
}
