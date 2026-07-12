// Package loadtest drives authenticated Sync workloads against an explicitly
// configured disposable Remotr server and Postgres database.
package loadtest

import (
	"context"
	"crypto/rand"
	"crypto/tls"
	"crypto/x509"
	"encoding/hex"
	"errors"
	"fmt"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/DavidHoenisch/remotr/internal/agent/enroll"
	agentsync "github.com/DavidHoenisch/remotr/internal/agent/sync"
	pgstore "github.com/DavidHoenisch/remotr/internal/store/postgres"
	"github.com/DavidHoenisch/remotr/internal/tlsconfig"
)

// Config identifies the disposable infrastructure and bounded workload.
type Config struct {
	ServerURL      string
	CAPath         string
	DatabaseURL    string
	Fleet          string
	RunID          string
	EndpointCount  int
	Concurrency    int
	RequestTimeout time.Duration
	EnrollmentTTL  time.Duration
}

// Harness owns load-test endpoint identities and their authenticated clients.
type Harness struct {
	cfg       Config
	endpoints []endpoint
}

type endpoint struct {
	id     string
	client *agentsync.Client
}

// Sample is one authenticated Sync attempt.
type Sample struct {
	Latency       time.Duration
	ResponseBytes int64
	Err           error
}

// Summary aggregates bounded client-observed Sync measurements.
type Summary struct {
	Requests      int
	Successes     int
	Errors        int
	ResponseBytes int64
	P50           time.Duration
	P95           time.Duration
	Max           time.Duration
}

// New validates an explicitly configured load harness. It does not contact
// infrastructure or create endpoint identities until Provision is called.
func New(cfg Config) (*Harness, error) {
	if strings.TrimSpace(cfg.ServerURL) == "" || strings.TrimSpace(cfg.CAPath) == "" || strings.TrimSpace(cfg.DatabaseURL) == "" || strings.TrimSpace(cfg.Fleet) == "" {
		return nil, errors.New("server URL, CA path, database URL, and fleet are required")
	}
	if cfg.EndpointCount <= 0 {
		return nil, errors.New("endpoint count must be positive")
	}
	if cfg.Concurrency <= 0 || cfg.Concurrency > cfg.EndpointCount {
		cfg.Concurrency = cfg.EndpointCount
	}
	if cfg.RequestTimeout <= 0 {
		cfg.RequestTimeout = 30 * time.Second
	}
	if cfg.EnrollmentTTL <= 0 {
		cfg.EnrollmentTTL = time.Hour
	}
	if strings.TrimSpace(cfg.RunID) == "" {
		cfg.RunID = randomRunID()
	}
	return &Harness{cfg: cfg}, nil
}

// Provision creates unique one-time enrollment tokens in the configured
// database, enrolls each endpoint, and holds only in-memory mTLS credentials.
func (h *Harness) Provision(ctx context.Context) error {
	store, err := pgstore.New(ctx, h.cfg.DatabaseURL)
	if err != nil {
		return fmt.Errorf("open load database: %w", err)
	}
	serverTLS, err := tlsconfig.TrustOnlyTLSConfig(h.cfg.CAPath)
	if err != nil {
		return fmt.Errorf("load CA: %w", err)
	}
	enroller := enroll.NewClient(h.cfg.ServerURL, serverTLS)
	enroller.HTTPClient.Timeout = h.cfg.RequestTimeout

	h.endpoints = make([]endpoint, 0, h.cfg.EndpointCount)
	for i := 0; i < h.cfg.EndpointCount; i++ {
		if err := ctx.Err(); err != nil {
			return err
		}
		id := EndpointID(h.cfg.RunID, i)
		token, err := randomToken()
		if err != nil {
			return err
		}
		if _, err := store.CreateEnrollmentToken(ctx, token, h.cfg.Fleet, time.Now().UTC().Add(h.cfg.EnrollmentTTL)); err != nil {
			return fmt.Errorf("create enrollment token for %s: %w", id, err)
		}
		resp, err := enroller.Enroll(token, id)
		if err != nil {
			return fmt.Errorf("enroll %s: %w", id, err)
		}
		tlsCfg, err := endpointTLS(resp)
		if err != nil {
			return fmt.Errorf("endpoint TLS %s: %w", id, err)
		}
		client := agentsync.NewClientWithTimeout(h.cfg.ServerURL, tlsCfg, h.cfg.RequestTimeout)
		h.endpoints = append(h.endpoints, endpoint{id: id, client: client})
	}
	return nil
}

// SyncWave sends one authenticated Sync from every provisioned endpoint.
func (h *Harness) SyncWave(ctx context.Context, request agentsync.Request) Summary {
	samples := make(chan Sample, len(h.endpoints))
	sem := make(chan struct{}, h.cfg.Concurrency)
	var group sync.WaitGroup
	for _, endpoint := range h.endpoints {
		endpoint := endpoint
		group.Add(1)
		go func() {
			defer group.Done()
			select {
			case sem <- struct{}{}:
				defer func() { <-sem }()
			case <-ctx.Done():
				samples <- Sample{Err: ctx.Err()}
				return
			}
			started := time.Now()
			response, err := endpoint.client.Sync(request)
			samples <- Sample{Latency: time.Since(started), ResponseBytes: int64(len(response.ArtifactYAML)), Err: err}
		}()
	}
	group.Wait()
	close(samples)

	out := make([]Sample, 0, len(h.endpoints))
	for sample := range samples {
		out = append(out, sample)
	}
	return Summarize(out)
}

// EndpointID derives a bounded valid endpoint identity for one load run.
func EndpointID(runID string, index int) string {
	clean := strings.Map(func(r rune) rune {
		switch {
		case r >= 'a' && r <= 'z', r >= 'A' && r <= 'Z', r >= '0' && r <= '9', r == '-':
			return r
		default:
			return -1
		}
	}, runID)
	clean = strings.Trim(clean, "-")
	if clean == "" {
		clean = "run"
	}
	if len(clean) > 48 {
		clean = clean[:48]
	}
	return fmt.Sprintf("load-%s-%04d", clean, index)
}

// Summarize calculates deterministic nearest-rank percentile summaries.
func Summarize(samples []Sample) Summary {
	summary := Summary{Requests: len(samples)}
	latencies := make([]time.Duration, 0, len(samples))
	for _, sample := range samples {
		latencies = append(latencies, sample.Latency)
		summary.ResponseBytes += sample.ResponseBytes
		if sample.Err != nil {
			summary.Errors++
		} else {
			summary.Successes++
		}
	}
	if len(latencies) == 0 {
		return summary
	}
	sort.Slice(latencies, func(i, j int) bool { return latencies[i] < latencies[j] })
	summary.P50 = nearestRank(latencies, 50)
	summary.P95 = nearestRank(latencies, 95)
	summary.Max = latencies[len(latencies)-1]
	return summary
}

func nearestRank(values []time.Duration, percentile int) time.Duration {
	index := (len(values)*percentile + 99) / 100
	if index < 1 {
		index = 1
	}
	return values[index-1]
}

func endpointTLS(resp enroll.Response) (*tls.Config, error) {
	certificate, err := tls.X509KeyPair([]byte(resp.CertPEM), []byte(resp.KeyPEM))
	if err != nil {
		return nil, err
	}
	pool := x509.NewCertPool()
	if !pool.AppendCertsFromPEM([]byte(resp.CAPEM)) {
		return nil, errors.New("parse enrollment CA")
	}
	return &tls.Config{MinVersion: tls.VersionTLS12, RootCAs: pool, Certificates: []tls.Certificate{certificate}}, nil
}

func randomToken() (string, error) {
	raw := make([]byte, 24)
	if _, err := rand.Read(raw); err != nil {
		return "", err
	}
	return hex.EncodeToString(raw), nil
}

func randomRunID() string {
	raw := make([]byte, 6)
	if _, err := rand.Read(raw); err != nil {
		return "run"
	}
	return hex.EncodeToString(raw)
}
