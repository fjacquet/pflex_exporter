package powerflex

import (
	"context"
	"crypto/tls"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/fjacquet/pflex_exporter/internal/models"
	"github.com/go-resty/resty/v2"
	log "github.com/sirupsen/logrus"
	"go.opentelemetry.io/otel/trace"
	"golang.org/x/sync/errgroup"
)

const (
	defaultTimeout   = 30 * time.Second
	retryCount       = 3
	retryWaitTime    = 5 * time.Second
	retryMaxWaitTime = 60 * time.Second

	maxIdleConns        = 100
	maxIdleConnsPerHost = 20
	idleConnTimeout     = 90 * time.Second

	instancesPath    = "/api/instances"
	statisticsPath   = "/api/instances/querySelectedStatistics" // Gen1
	v5StatisticsPath = "/dtapi/rest/v1/metrics/query"           // Gen2
)

// ClientOption configures optional ClusterClient settings.
type ClientOption func(*clientOptions)

type clientOptions struct {
	tracerProvider trace.TracerProvider
}

// WithTracerProvider injects an OpenTelemetry TracerProvider for HTTP spans.
func WithTracerProvider(tp trace.TracerProvider) ClientOption {
	return func(o *clientOptions) { o.tracerProvider = tp }
}

// ClusterClient is the PowerFlex REST client for a single cluster. It owns the
// authentication token lifecycle and the resty HTTP client.
type ClusterClient struct {
	name    string
	baseURL string
	http    *resty.Client
	auth    *tokenStore
	tracing *TracerWrapper

	mu         sync.Mutex
	activeReqs int32
	closed     bool
	closeChan  chan struct{}
}

// NewClusterClient builds a client for one cluster from its config.
func NewClusterClient(cfg models.ClusterConfig, opts ...ClientOption) *ClusterClient {
	var options clientOptions
	for _, opt := range opts {
		opt(&options)
	}

	if cfg.InsecureSkipVerify {
		log.Warnf("cluster %q: TLS certificate verification disabled (insecureSkipVerify=true)", cfg.Name)
	}

	httpClient := resty.New().
		SetTimeout(defaultTimeout).
		SetRetryCount(retryCount).
		SetRetryWaitTime(retryWaitTime).
		SetRetryMaxWaitTime(retryMaxWaitTime).
		AddRetryCondition(func(r *resty.Response, err error) bool {
			if err != nil {
				return true
			}
			// Retry only rate-limiting (429). Never retry 4xx (auth/bad-request), and
			// never retry 5xx: PowerFlex 5xx here are deterministic (e.g. a malformed
			// query), so retrying with a 5s backoff only buries the real status under a
			// context-deadline timeout — which is exactly what masked the v0.6.2 regression.
			return r.StatusCode() == http.StatusTooManyRequests
		})

	httpClient.GetClient().Transport = &http.Transport{
		MaxIdleConns:        maxIdleConns,
		MaxIdleConnsPerHost: maxIdleConnsPerHost,
		IdleConnTimeout:     idleConnTimeout,
		TLSHandshakeTimeout: 10 * time.Second,
		TLSClientConfig: &tls.Config{
			InsecureSkipVerify: cfg.InsecureSkipVerify, //nolint:gosec // operator-controlled, common for self-signed gateways
			MinVersion:         tls.VersionTLS12,
		},
	}

	baseURL := cfg.GatewayBaseURL()
	return &ClusterClient{
		name:    cfg.Name,
		baseURL: baseURL,
		http:    httpClient,
		auth:    newTokenStore(httpClient, baseURL, cfg.Username, cfg.Password),
		tracing: NewTracerWrapper(options.tracerProvider, "pflex-exporter/http-client"),
	}
}

// Name returns the cluster name.
func (c *ClusterClient) Name() string { return c.name }

// GetInstances fetches and parses GET /api/instances.
func (c *ClusterClient) GetInstances(ctx context.Context) (*models.Instances, *models.Relations, error) {
	body, err := c.get(ctx, instancesPath)
	if err != nil {
		return nil, nil, err
	}
	return models.ParseInstances(body)
}

// GetStatistics fetches and parses POST /api/instances/querySelectedStatistics (Gen1).
func (c *ClusterClient) GetStatistics(ctx context.Context) (*models.Statistics, error) {
	body, err := c.post(ctx, statisticsPath, queryStatsBody)
	if err != nil {
		return nil, err
	}
	return models.ParseStatistics(body)
}

// v5QueryConcurrency bounds how many per-type dtapi metric queries run at once. The
// endpoint accepts a single resource_type per call, so the types are queried in parallel
// to fit the shared per-cluster collection timeout rather than serializing nine
// round-trips under it (a slow dtapi otherwise exhausted the budget mid-fan-out).
const v5QueryConcurrency = 8

// GetStatisticsV5 fetches Gen2 metrics by querying the v5 endpoint once per resource
// type, concurrently. A failed per-type query is logged and skipped (graceful
// degradation). Resource types listed in skipTypes are not queried at all.
func (c *ClusterClient) GetStatisticsV5(ctx context.Context, skipTypes ...string) (*StatisticsV5, error) {
	skip := make(map[string]struct{}, len(skipTypes))
	for _, t := range skipTypes {
		skip[t] = struct{}{}
	}

	stats := &StatisticsV5{ByType: make(map[string]map[string]map[string]float64, len(v5Metrics))}
	var mu sync.Mutex // guards stats.ByType
	var succeeded int32

	var g errgroup.Group
	g.SetLimit(v5QueryConcurrency)

	cycleStart := time.Now()
	attempted := 0
	for typeName, mapping := range v5Metrics {
		if _, skipped := skip[typeName]; skipped {
			continue
		}
		v5type, ok := v5ResourceType[typeName]
		if !ok || len(mapping) == 0 {
			continue
		}
		metricNames := make([]string, 0, len(mapping))
		for name := range mapping {
			metricNames = append(metricNames, name)
		}
		// The live dtapi accepts "metrics" only as a JSON array; a comma-separated string
		// gets an instant HTTP 500. This matches Dell's reference siocli/sio_sdk tool. (The
		// PowerFlex 5.0 PDF documents a comma-separated string, but that is wrong — trusting
		// it shipped a regression in v0.6.2.)
		reqBody, err := json.Marshal(map[string]any{"resource_type": v5type, "metrics": metricNames})
		if err != nil {
			return nil, err
		}

		attempted++
		typeName, reqBody := typeName, reqBody
		g.Go(func() error {
			start := time.Now()
			respBody, err := c.post(ctx, v5StatisticsPath, reqBody)
			elapsed := time.Since(start).Round(time.Millisecond)
			if err != nil {
				// Elapsed distinguishes a fast server reject (e.g. HTTP 500) from a slow
				// timeout — the very ambiguity that masked the dtapi failures.
				log.Warnf("cluster %q: v5 metrics query for %s failed after %s: %v", c.name, typeName, elapsed, err)
				return nil // graceful degradation: one failed type must not sink the rest
			}
			byID, err := parseV5Response(respBody)
			if err != nil {
				log.Warnf("cluster %q: failed to parse v5 response for %s: %v", c.name, typeName, err)
				return nil
			}
			mu.Lock()
			stats.ByType[typeName] = byID
			mu.Unlock()
			atomic.AddInt32(&succeeded, 1)
			log.Debugf("cluster %q: v5 query %s -> %d resources in %s", c.name, typeName, len(byID), elapsed)
			return nil
		})
	}
	_ = g.Wait()

	ok := atomic.LoadInt32(&succeeded)
	log.Debugf("cluster %q: v5 stats %d/%d types ok in %s", c.name, ok, attempted, time.Since(cycleStart).Round(time.Millisecond))

	// Partial failures degrade gracefully, but a total failure (no type succeeded) is a
	// hard error so the collector can fall back to the other statistics path.
	if attempted > 0 && ok == 0 {
		return nil, fmt.Errorf("cluster %q: all %d v5 metric queries failed", c.name, attempted)
	}
	return stats, nil
}

func (c *ClusterClient) get(ctx context.Context, path string) ([]byte, error) {
	return c.do(ctx, http.MethodGet, path, nil)
}

func (c *ClusterClient) post(ctx context.Context, path string, body []byte) ([]byte, error) {
	return c.do(ctx, http.MethodPost, path, body)
}

// do executes an authenticated request and returns the validated JSON body.
func (c *ClusterClient) do(ctx context.Context, method, path string, body []byte) ([]byte, error) {
	c.mu.Lock()
	if c.closed {
		c.mu.Unlock()
		return nil, fmt.Errorf("client is closed")
	}
	atomic.AddInt32(&c.activeReqs, 1)
	c.mu.Unlock()
	defer c.requestDone()

	ctx, span := c.tracing.StartSpan(ctx, "powerflex.request", trace.SpanKindClient)
	defer span.End()

	token, err := c.auth.ensureValidToken(ctx)
	if err != nil {
		return nil, fmt.Errorf("cluster %q: %w", c.name, err)
	}

	req := c.http.R().
		SetContext(ctx).
		SetHeader("Authorization", "Bearer "+token)

	url := c.baseURL + path
	var resp *resty.Response
	if method == http.MethodPost {
		req.SetHeader("Content-Type", "application/json").SetBody(body)
		resp, err = req.Post(url)
	} else {
		resp, err = req.Get(url)
	}
	if err != nil {
		return nil, fmt.Errorf("cluster %q: request to %s failed: %w", c.name, path, err)
	}
	if resp.IsError() {
		return nil, fmt.Errorf("cluster %q: request to %s failed: status=%d body=%s",
			c.name, path, resp.StatusCode(), truncate(resp.Body(), 200))
	}

	if ct := resp.Header().Get("Content-Type"); ct != "" && !strings.Contains(ct, "application/json") {
		return nil, fmt.Errorf("cluster %q: %s returned non-JSON content-type %q", c.name, path, ct)
	}
	if !json.Valid(resp.Body()) {
		return nil, fmt.Errorf("cluster %q: %s returned invalid JSON", c.name, path)
	}
	return resp.Body(), nil
}

func (c *ClusterClient) requestDone() {
	if atomic.AddInt32(&c.activeReqs, -1) == 0 {
		c.mu.Lock()
		if c.closed && c.closeChan != nil {
			close(c.closeChan)
			c.closeChan = nil
		}
		c.mu.Unlock()
	}
}

// Close waits up to 30s for in-flight requests then releases idle connections.
func (c *ClusterClient) Close() error {
	ch, err := c.beginClose()
	if err != nil {
		return err
	}

	if ch != nil {
		ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer cancel()
		select {
		case <-ch:
		case <-ctx.Done():
			log.Warnf("cluster %q: timeout waiting for in-flight requests during shutdown", c.name)
		}
	}

	if c.http != nil {
		c.http.GetClient().CloseIdleConnections()
	}
	return nil
}

// beginClose marks the client closed and returns a channel to wait on if requests
// are still in flight (nil otherwise). The lock is fully scoped to this helper.
func (c *ClusterClient) beginClose() (chan struct{}, error) {
	c.mu.Lock()
	defer c.mu.Unlock()

	if c.closed {
		return nil, fmt.Errorf("client already closed")
	}
	c.closed = true

	if atomic.LoadInt32(&c.activeReqs) > 0 {
		c.closeChan = make(chan struct{})
		return c.closeChan, nil
	}
	return nil, nil
}
