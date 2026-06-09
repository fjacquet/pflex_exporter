package powerflex

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/fjacquet/pflex_exporter/internal/models"
)

// mockGateway is an in-memory PowerFlex gateway for tests.
type mockGateway struct {
	server *httptest.Server

	mu               sync.Mutex
	loginCount       int
	refreshCount     int
	instancesCount   int
	statsCount       int    // Gen1 querySelectedStatistics calls
	statsV5Count     int    // Gen2 v5 metrics/query calls
	failRefresh      bool   // when true, /rest/auth/update-token returns 400
	failInstances    bool   // when true, /api/instances returns 500
	failStats        bool   // when true, Gen1 querySelectedStatistics returns 500
	failStatsV5      bool   // when true, Gen2 v5 metrics/query returns 500
	instancesFixture string // fixture file served by /api/instances

	statsV5Delay      time.Duration // artificial per-call delay for /dtapi metrics/query
	metricsFieldArray bool          // set if any v5 request sent "metrics" as a JSON array
}

func newMockGateway(t *testing.T) *mockGateway {
	t.Helper()
	g := &mockGateway{instancesFixture: "instances.json"}

	mux := http.NewServeMux()
	mux.HandleFunc("/rest/auth/login", func(w http.ResponseWriter, _ *http.Request) {
		g.mu.Lock()
		g.loginCount++
		n := g.loginCount
		g.mu.Unlock()
		writeJSON(w, map[string]string{
			"access_token":  fmt.Sprintf("access-%d", n),
			"refresh_token": fmt.Sprintf("refresh-%d", n),
		})
	})
	mux.HandleFunc("/rest/auth/update-token", func(w http.ResponseWriter, _ *http.Request) {
		g.mu.Lock()
		fail := g.failRefresh
		g.refreshCount++
		n := g.refreshCount
		g.mu.Unlock()
		if fail {
			w.WriteHeader(http.StatusBadRequest)
			writeBytes(w, []byte(`{"message":"refresh token expired"}`))
			return
		}
		writeJSON(w, map[string]string{"access_token": fmt.Sprintf("access-refreshed-%d", n)})
	})
	mux.HandleFunc("/api/instances", func(w http.ResponseWriter, r *http.Request) {
		if !strings.HasPrefix(r.Header.Get("Authorization"), "Bearer ") {
			w.WriteHeader(http.StatusUnauthorized)
			return
		}
		g.mu.Lock()
		g.instancesCount++
		fail := g.failInstances
		fixture := g.instancesFixture
		g.mu.Unlock()
		if fail {
			w.WriteHeader(http.StatusInternalServerError)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		writeBytes(w, readFixture(t, fixture))
	})
	mux.HandleFunc("/api/instances/querySelectedStatistics", func(w http.ResponseWriter, r *http.Request) {
		if !strings.HasPrefix(r.Header.Get("Authorization"), "Bearer ") {
			w.WriteHeader(http.StatusUnauthorized)
			return
		}
		g.mu.Lock()
		g.statsCount++
		fail := g.failStats
		g.mu.Unlock()
		if fail {
			// 404 (not 5xx) so resty does not retry: simulates an endpoint removed in a
			// point release, exercising the collector's stats-path fallback.
			w.WriteHeader(http.StatusNotFound)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		writeBytes(w, readFixture(t, "statistics.json"))
	})
	mux.HandleFunc("/dtapi/rest/v1/metrics/query", func(w http.ResponseWriter, r *http.Request) {
		if !strings.HasPrefix(r.Header.Get("Authorization"), "Bearer ") {
			w.WriteHeader(http.StatusUnauthorized)
			return
		}
		body, _ := io.ReadAll(r.Body)
		var raw map[string]json.RawMessage
		_ = json.Unmarshal(body, &raw)
		var resourceType string
		_ = json.Unmarshal(raw["resource_type"], &resourceType)
		// The documented schema requires "metrics" as a comma-separated string; flag a
		// JSON array so tests can assert the request honors the contract.
		metricsIsArray := len(raw["metrics"]) > 0 && raw["metrics"][0] == '['

		g.mu.Lock()
		g.statsV5Count++
		fail := g.failStatsV5
		delay := g.statsV5Delay
		if metricsIsArray {
			g.metricsFieldArray = true
		}
		g.mu.Unlock()

		if delay > 0 {
			time.Sleep(delay)
		}
		if fail {
			// 404 (not 5xx) so resty does not retry: simulates the v5 endpoint being
			// unavailable, exercising the collector's stats-path fallback.
			w.WriteHeader(http.StatusNotFound)
			return
		}

		resources, ok := readV5Fixture(t)[resourceType]
		if !ok {
			resources = json.RawMessage("[]")
		}
		w.Header().Set("Content-Type", "application/json")
		writeBytes(w, []byte(`{"resources":`+string(resources)+`}`))
	})

	g.server = httptest.NewTLSServer(mux)
	t.Cleanup(g.server.Close)
	return g
}

// client returns a ClusterClient named "test-cluster" pointing at the mock gateway.
func (g *mockGateway) client(t *testing.T) *ClusterClient {
	return g.clientNamed(t, "test-cluster")
}

// clientNamed returns a ClusterClient with a specific name pointing at the mock gateway.
func (g *mockGateway) clientNamed(t *testing.T, name string) *ClusterClient {
	t.Helper()
	host := strings.TrimPrefix(g.server.URL, "https://")
	return NewClusterClient(models.ClusterConfig{
		Name:               name,
		Gateway:            host,
		Username:           "user",
		Password:           "pass",
		InsecureSkipVerify: true,
	})
}

func writeJSON(w http.ResponseWriter, v any) {
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(v)
}

// writeBytes takes io.Writer (not http.ResponseWriter) so the static test fixtures
// don't trip the XSS-on-ResponseWriter lint; these handlers serve fixed JSON only.
func writeBytes(w io.Writer, data []byte) {
	_, _ = w.Write(data)
}

func readFixture(t *testing.T, name string) []byte {
	t.Helper()
	data, err := os.ReadFile(filepath.Join("testdata", name))
	if err != nil {
		t.Fatalf("read fixture %s: %v", name, err)
	}
	return data
}

// readV5Fixture loads the Gen2 v5 statistics fixture as resource_type -> resources array.
func readV5Fixture(t *testing.T) map[string]json.RawMessage {
	t.Helper()
	var m map[string]json.RawMessage
	if err := json.Unmarshal(readFixture(t, "statistics-v5.json"), &m); err != nil {
		t.Fatalf("parse statistics-v5.json: %v", err)
	}
	return m
}

func TestLoginOnFirstRequest(t *testing.T) {
	g := newMockGateway(t)
	c := g.client(t)
	defer func() { _ = c.Close() }()

	if _, _, err := c.GetInstances(context.Background()); err != nil {
		t.Fatalf("GetInstances: %v", err)
	}

	g.mu.Lock()
	defer g.mu.Unlock()
	if g.loginCount != 1 {
		t.Errorf("expected 1 login, got %d", g.loginCount)
	}
	if g.instancesCount != 1 {
		t.Errorf("expected 1 instances call, got %d", g.instancesCount)
	}
}

func TestTokenReusedWithinTTL(t *testing.T) {
	g := newMockGateway(t)
	c := g.client(t)
	defer func() { _ = c.Close() }()

	for i := 0; i < 3; i++ {
		if _, err := c.GetStatistics(context.Background()); err != nil {
			t.Fatalf("GetStatistics #%d: %v", i, err)
		}
	}

	g.mu.Lock()
	defer g.mu.Unlock()
	if g.loginCount != 1 {
		t.Errorf("expected token reuse (1 login), got %d logins", g.loginCount)
	}
	if g.refreshCount != 0 {
		t.Errorf("expected no refresh, got %d", g.refreshCount)
	}
}

func TestRefreshWhenAccessExpired(t *testing.T) {
	g := newMockGateway(t)
	c := g.client(t)
	defer func() { _ = c.Close() }()

	if _, _, err := c.GetInstances(context.Background()); err != nil {
		t.Fatalf("initial GetInstances: %v", err)
	}

	// Force access token expiry while keeping the refresh token valid.
	c.auth.mu.Lock()
	c.auth.accessExpiry = time.Now().Add(-time.Minute)
	c.auth.mu.Unlock()

	if _, _, err := c.GetInstances(context.Background()); err != nil {
		t.Fatalf("second GetInstances: %v", err)
	}

	g.mu.Lock()
	defer g.mu.Unlock()
	if g.loginCount != 1 {
		t.Errorf("expected exactly 1 login (then refresh), got %d", g.loginCount)
	}
	if g.refreshCount != 1 {
		t.Errorf("expected 1 refresh, got %d", g.refreshCount)
	}
}

func TestRefreshFailureFallsBackToLogin(t *testing.T) {
	g := newMockGateway(t)
	g.failRefresh = true
	c := g.client(t)
	defer func() { _ = c.Close() }()

	if _, _, err := c.GetInstances(context.Background()); err != nil {
		t.Fatalf("initial GetInstances: %v", err)
	}

	// Expire both access token (forces refresh) and let refresh fail -> relogin.
	c.auth.mu.Lock()
	c.auth.accessExpiry = time.Now().Add(-time.Minute)
	c.auth.mu.Unlock()

	if _, _, err := c.GetInstances(context.Background()); err != nil {
		t.Fatalf("second GetInstances: %v", err)
	}

	g.mu.Lock()
	defer g.mu.Unlock()
	if g.refreshCount != 1 {
		t.Errorf("expected 1 refresh attempt, got %d", g.refreshCount)
	}
	if g.loginCount != 2 {
		t.Errorf("expected 2 logins (initial + relogin after failed refresh), got %d", g.loginCount)
	}
}

func TestGetInstancesParsesTopology(t *testing.T) {
	g := newMockGateway(t)
	c := g.client(t)
	defer func() { _ = c.Close() }()

	instances, relations, err := c.GetInstances(context.Background())
	if err != nil {
		t.Fatalf("GetInstances: %v", err)
	}
	if instances.System == nil || instances.System.Name != "cluster-one" {
		t.Fatalf("unexpected System: %+v", instances.System)
	}
	if got := len(instances.Get(models.TypeVolume)); got != 1 {
		t.Errorf("expected 1 volume, got %d", got)
	}
	// Device dev1 should resolve a parent SDS (sds1) and StoragePool (sp1).
	if ids := relations.ParentIDs("dev1", models.TypeSds); len(ids) != 1 || ids[0] != "sds1" {
		t.Errorf("device->sds parent wrong: %v", ids)
	}
	if ids := relations.ParentIDs("sp1", models.TypeProtectionDomain); len(ids) != 1 || ids[0] != "pd1" {
		t.Errorf("pool->pd parent wrong: %v", ids)
	}
}

func TestGetStatisticsParses(t *testing.T) {
	g := newMockGateway(t)
	c := g.client(t)
	defer func() { _ = c.Close() }()

	stats, err := c.GetStatistics(context.Background())
	if err != nil {
		t.Fatalf("GetStatistics: %v", err)
	}
	if stats.System == nil {
		t.Fatal("expected System stats")
	}
	if _, ok := stats.System["maxCapacityInKb"]; !ok {
		t.Error("expected maxCapacityInKb in System stats")
	}
	if vol := stats.Object(models.TypeVolume, "vol1"); vol == nil {
		t.Error("expected stats for vol1")
	}
}

// TestGetStatisticsV5ConcurrentAndMetricsContract guards the two Gen2 fixes:
//
//  1. The per-type dtapi queries must run concurrently. The endpoint accepts only one
//     resource_type per call (PowerFlex API 5.0.0), so all types must be fetched in
//     parallel to fit the shared per-cluster timeout — a serial fan-out is what made a
//     slow dtapi blow the 8s budget on real clusters.
//  2. The "metrics" field must be a comma-separated string, per the documented request
//     schema, not a JSON array.
func TestGetStatisticsV5ConcurrentAndMetricsContract(t *testing.T) {
	g := newMockGateway(t)
	const delay = 120 * time.Millisecond
	g.statsV5Delay = delay
	c := g.client(t)
	defer func() { _ = c.Close() }()

	start := time.Now()
	stats, err := c.GetStatisticsV5(context.Background())
	elapsed := time.Since(start)
	if err != nil {
		t.Fatalf("GetStatisticsV5: %v", err)
	}
	if stats == nil {
		t.Fatal("expected non-nil stats")
	}

	// With N delayed types, serial execution costs N*delay; concurrent costs ~delay.
	// There are 9 resource types (>1s serial at 120ms each); require well under half.
	if elapsed > 700*time.Millisecond {
		t.Errorf("v5 queries appear serial: %v elapsed for %d-type fan-out at %v each",
			elapsed, len(v5Metrics), delay)
	}

	g.mu.Lock()
	sawArray := g.metricsFieldArray
	g.mu.Unlock()
	if sawArray {
		t.Error(`v5 request sent "metrics" as a JSON array; the documented schema requires a comma-separated string`)
	}
}

func TestRequestErrorPropagates(t *testing.T) {
	g := newMockGateway(t)
	g.failInstances = true
	c := g.client(t)
	defer func() { _ = c.Close() }()

	if _, _, err := c.GetInstances(context.Background()); err == nil {
		t.Fatal("expected error when gateway returns 500")
	}
}
