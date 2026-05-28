// Package powerflex provides the PowerFlex Gen1 REST API client, authentication,
// data collection, and the dual (Prometheus + OTLP) metric export paths.
package powerflex

import (
	"context"

	"github.com/fjacquet/pflex_exporter/internal/models"
)

// Client is the per-cluster PowerFlex API client abstraction. It is satisfied by
// ClusterClient and mocked in tests so the collector can run without a live gateway.
type Client interface {
	// Name returns the configured cluster name (used as the `cluster` label).
	Name() string

	// GetInstances fetches GET /api/instances and returns typed objects plus the
	// parent/child relations graph.
	GetInstances(ctx context.Context) (*models.Instances, *models.Relations, error)

	// GetStatistics fetches POST /api/instances/querySelectedStatistics.
	GetStatistics(ctx context.Context) (*models.Statistics, error)

	// Close releases the client's HTTP resources.
	Close() error
}
