// Package loadtest drives authenticated Sync workloads against an explicitly
// configured disposable Remotr server and Postgres database.
package loadtest

import (
	"bufio"
	"context"
	"crypto/rand"
	"crypto/sha256"
	"crypto/tls"
	"crypto/x509"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"runtime"
	"sort"
	"strconv"
	"strings"
	"sync"
	"syscall"
	"time"

	"github.com/DavidHoenisch/remotr/internal/agent/enroll"
	"github.com/DavidHoenisch/remotr/internal/agent/polling"
	agentsync "github.com/DavidHoenisch/remotr/internal/agent/sync"
	pgstore "github.com/DavidHoenisch/remotr/internal/store/postgres"
	"github.com/DavidHoenisch/remotr/internal/tlsconfig"
	"github.com/jackc/pgx/v5/pgxpool"
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
	database  *pgxpool.Pool
}

type endpoint struct {
	id             string
	client         *agentsync.Client
	lastDigest     string
	lastReleaseRef string
}

// Sample is one authenticated Sync attempt.
type Sample struct {
	StartedAt     time.Time
	Latency       time.Duration
	RequestBytes  int64
	ResponseBytes int64
	Unchanged     bool
	Err           error
}

// Summary aggregates bounded client-observed Sync measurements.
type Summary struct {
	Requests          int
	Successes         int
	Errors            int
	Overloaded        int
	Unchanged         int
	RequestBytes      int64
	ResponseBytes     int64
	P50               time.Duration
	P95               time.Duration
	Max               time.Duration
	StartSpread       time.Duration
	MaxStartsPer100ms int
}

// Wave names one measured workload phase and its client-observed result.
type Wave struct {
	Name    string
	Summary Summary
}

// FaultController applies and recovers an explicitly authorized outage in a
// disposable environment. It lets the harness retain endpoint state across the
// failed and recovered waves without embedding a container runtime dependency.
type FaultController interface {
	Degrade(context.Context) error
	Recover(context.Context) error
}

// ProcessMetrics is a point-in-time measurement of the load-generator process.
type ProcessMetrics struct {
	CPU            time.Duration
	RSSBytes       uint64
	HeapAllocBytes uint64
	Goroutines     int
}

// DatabaseMetrics contains pool state for the harness and database-wide
// counters from pg_stat_database for the configured database.
type DatabaseMetrics struct {
	PoolAcquireCount         int64
	PoolAcquiredConns        int32
	PoolIdleConns            int32
	PoolTotalConns           int32
	PoolEmptyAcquireCount    int64
	PoolCanceledAcquireCount int64
	XactCommit               int64
	XactRollback             int64
	BlocksRead               int64
	BlocksHit                int64
	TuplesReturned           int64
	TuplesFetched            int64
	TuplesInserted           int64
	TempFiles                int64
	TempBytes                int64
	Deadlocks                int64
	Backends                 int32
}

// DatabaseDelta reports counter changes over a workload and the ending pool
// state. PostgreSQL counters can include unrelated activity in the same
// disposable database, which is why raw snapshots are kept in Report too.
type DatabaseDelta struct {
	PoolAcquireCount         int64
	PoolAcquiredConns        int32
	PoolIdleConns            int32
	PoolTotalConns           int32
	PoolEmptyAcquireCount    int64
	PoolCanceledAcquireCount int64
	XactCommit               int64
	XactRollback             int64
	BlocksRead               int64
	BlocksHit                int64
	TuplesReturned           int64
	TuplesFetched            int64
	TuplesInserted           int64
	TempFiles                int64
	TempBytes                int64
	Deadlocks                int64
	Backends                 int32
}

// Report combines workload summaries with client-process and database evidence.
type Report struct {
	Waves          []Wave
	ProcessBefore  ProcessMetrics
	ProcessAfter   ProcessMetrics
	ProcessCPUUsed time.Duration
	DatabaseBefore DatabaseMetrics
	DatabaseAfter  DatabaseMetrics
	DatabaseDelta  DatabaseDelta
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
	database, err := pgxpool.New(ctx, h.cfg.DatabaseURL)
	if err != nil {
		return fmt.Errorf("open load database: %w", err)
	}
	store := pgstore.NewFromPool(database)
	serverTLS, err := tlsconfig.TrustOnlyTLSConfig(h.cfg.CAPath)
	if err != nil {
		database.Close()
		return fmt.Errorf("load CA: %w", err)
	}
	enroller := enroll.NewClient(h.cfg.ServerURL, serverTLS)
	enroller.HTTPClient.Timeout = h.cfg.RequestTimeout

	h.endpoints = make([]endpoint, 0, h.cfg.EndpointCount)
	for i := 0; i < h.cfg.EndpointCount; i++ {
		if err := ctx.Err(); err != nil {
			database.Close()
			return err
		}
		id := EndpointID(h.cfg.RunID, i)
		token, err := randomToken()
		if err != nil {
			database.Close()
			return err
		}
		if _, err := store.CreateEnrollmentToken(ctx, token, h.cfg.Fleet, time.Now().UTC().Add(h.cfg.EnrollmentTTL)); err != nil {
			database.Close()
			return fmt.Errorf("create enrollment token for %s: %w", id, err)
		}
		resp, err := enroller.Enroll(token, id)
		if err != nil {
			database.Close()
			return fmt.Errorf("enroll %s: %w", id, err)
		}
		tlsCfg, err := endpointTLS(resp)
		if err != nil {
			database.Close()
			return fmt.Errorf("endpoint TLS %s: %w", id, err)
		}
		client := agentsync.NewClientWithTimeout(h.cfg.ServerURL, tlsCfg, h.cfg.RequestTimeout)
		h.endpoints = append(h.endpoints, endpoint{id: id, client: client})
	}
	h.database = database
	return nil
}

// Close releases the Postgres pool used by the load harness.
func (h *Harness) Close() {
	if h.database != nil {
		h.database.Close()
		h.database = nil
	}
}

// SyncWave sends one authenticated Sync from every provisioned endpoint. Each
// endpoint carries forward its last digest and release reference so later waves
// exercise the unchanged protocol path.
func (h *Harness) SyncWave(ctx context.Context, request agentsync.Request) Summary {
	return h.syncWave(ctx, request, nil)
}

func (h *Harness) syncWave(ctx context.Context, request agentsync.Request, delays map[string]time.Duration) Summary {
	return h.syncWaveRequests(ctx, delays, func(string) agentsync.Request { return request })
}

func (h *Harness) syncWaveRequests(ctx context.Context, delays map[string]time.Duration, requestFor func(string) agentsync.Request) Summary {
	samples := make(chan Sample, len(h.endpoints))
	sem := make(chan struct{}, h.cfg.Concurrency)
	var group sync.WaitGroup
	for i := range h.endpoints {
		endpoint := &h.endpoints[i]
		group.Add(1)
		go func() {
			defer group.Done()
			if delay := delays[endpoint.id]; delay > 0 {
				timer := time.NewTimer(delay)
				defer timer.Stop()
				select {
				case <-ctx.Done():
					samples <- Sample{Err: ctx.Err()}
					return
				case <-timer.C:
				}
			}
			select {
			case sem <- struct{}{}:
				defer func() { <-sem }()
			case <-ctx.Done():
				samples <- Sample{Err: ctx.Err()}
				return
			}
			endpointRequest := requestFor(endpoint.id)
			endpointRequest.LastDigest = endpoint.lastDigest
			endpointRequest.LastReleaseRef = endpoint.lastReleaseRef
			requestBytes, err := json.Marshal(endpointRequest)
			if err != nil {
				samples <- Sample{Err: err}
				return
			}
			started := time.Now()
			response, err := endpoint.client.Sync(endpointRequest)
			if err == nil {
				endpoint.lastDigest = response.Digest
				endpoint.lastReleaseRef = response.ReleaseRef
			}
			samples <- Sample{
				StartedAt:     started,
				Latency:       time.Since(started),
				RequestBytes:  int64(len(requestBytes)),
				ResponseBytes: int64(len(response.ArtifactYAML)),
				Unchanged:     response.Unchanged,
				Err:           err,
			}
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

// SteadyUnchanged sends a warm-up artifact wave followed by the requested
// number of unchanged Sync waves at the supplied polling interval.
func (h *Harness) SteadyUnchanged(ctx context.Context, cycles int, interval time.Duration) []Summary {
	if cycles < 0 {
		cycles = 0
	}
	results := []Summary{h.SyncWave(ctx, agentsync.Request{})}
	for i := 0; i < cycles; i++ {
		if interval > 0 {
			timer := time.NewTimer(interval)
			select {
			case <-ctx.Done():
				timer.Stop()
				return results
			case <-timer.C:
			}
		}
		results = append(results, h.SyncWave(ctx, agentsync.Request{}))
	}
	return results
}

// MeasuredSteadyUnchanged executes the steady workload while collecting
// client-process and PostgreSQL snapshots before and after every wave.
func (h *Harness) MeasuredSteadyUnchanged(ctx context.Context, cycles int, interval time.Duration) (Report, error) {
	return h.measured(ctx, func() ([]Wave, error) {
		summaries := h.SteadyUnchanged(ctx, cycles, interval)
		waves := make([]Wave, 0, len(summaries))
		for i, summary := range summaries {
			name := "artifact-warm-up"
			if i > 0 {
				name = fmt.Sprintf("steady-unchanged-%d", i)
			}
			waves = append(waves, Wave{Name: name, Summary: summary})
		}
		return waves, nil
	})
}

// MeasuredStartupReconnectRecovery runs an initial coordinated Sync, then two
// more coordinated Sync waves after closing every idle client connection. Each
// reconnect uses fresh TLS transport connections but retains Sync digest state.
func (h *Harness) MeasuredStartupReconnectRecovery(ctx context.Context) (Report, error) {
	return h.measured(ctx, func() ([]Wave, error) {
		startup := h.SyncWave(ctx, agentsync.Request{})
		h.closeIdleConnections()
		reconnect := h.SyncWave(ctx, agentsync.Request{})
		h.closeIdleConnections()
		recovery := h.SyncWave(ctx, agentsync.Request{})
		return []Wave{
			{Name: "simultaneous-startup", Summary: startup},
			{Name: "simultaneous-reconnect", Summary: reconnect},
			{Name: "post-reconnect-recovery", Summary: recovery},
		}, nil
	})
}

// MeasuredReleaseFanout delivers a changed fleet artifact to every endpoint,
// then adds a one-endpoint override at that release. The original release ref
// is restored before the report returns so a disposable shared stack is left
// in its previous state.
func (h *Harness) MeasuredReleaseFanout(ctx context.Context) (Report, error) {
	return h.measured(ctx, func() ([]Wave, error) {
		return h.releaseFanout(ctx)
	})
}

// MeasuredTelemetryHeavy sends a baseline artifact wave followed by an
// unchanged Sync wave carrying bounded labels, users, inventory, drift, and
// firewall telemetry that the current server persists.
func (h *Harness) MeasuredTelemetryHeavy(ctx context.Context) (Report, error) {
	return h.measured(ctx, func() ([]Wave, error) {
		baseline := h.SyncWave(ctx, agentsync.Request{})
		telemetry := h.syncWaveRequests(ctx, nil, telemetryHeavyRequest)
		return []Wave{
			{Name: "baseline-artifact", Summary: baseline},
			{Name: "telemetry-heavy-unchanged", Summary: telemetry},
		}, nil
	})
}

// MeasuredOutageRecovery records a successful baseline, a controlled outage
// wave, and a recovered wave while retaining endpoint Sync state.
func (h *Harness) MeasuredOutageRecovery(ctx context.Context, controller FaultController) (Report, error) {
	if controller == nil {
		return Report{}, errors.New("fault controller is required")
	}
	return h.measured(ctx, func() (waves []Wave, err error) {
		baseline := h.SyncWave(ctx, agentsync.Request{})
		if err = controller.Degrade(ctx); err != nil {
			return nil, fmt.Errorf("degrade disposable service: %w", err)
		}
		degraded := true
		defer func() {
			if !degraded {
				return
			}
			restoreCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
			defer cancel()
			if restoreErr := controller.Recover(restoreCtx); restoreErr != nil {
				err = errors.Join(err, fmt.Errorf("recover disposable service: %w", restoreErr))
			}
		}()
		outage := h.SyncWave(ctx, agentsync.Request{})
		if err = controller.Recover(ctx); err != nil {
			return nil, fmt.Errorf("recover disposable service: %w", err)
		}
		degraded = false
		recovery := h.SyncWave(ctx, agentsync.Request{})
		return []Wave{
			{Name: "baseline-before-outage", Summary: baseline},
			{Name: "controlled-outage", Summary: outage},
			{Name: "post-outage-recovery", Summary: recovery},
		}, nil
	})
}

// MeasuredShapedOutageRecovery uses the same startup, stable-success, and
// transient-backoff delays as the agent polling loop while exercising real
// endpoint Sync calls. It records start-window density for each phase so a
// coordinated launch cannot silently become a coordinated retry wave.
func (h *Harness) MeasuredShapedOutageRecovery(ctx context.Context, interval time.Duration, controller FaultController) (Report, error) {
	if controller == nil {
		return Report{}, errors.New("fault controller is required")
	}
	policy := polling.NewPolicy(interval)
	return h.measured(ctx, func() (waves []Wave, err error) {
		startup := h.syncWave(ctx, agentsync.Request{}, h.startupDelays(policy))
		if err = controller.Degrade(ctx); err != nil {
			return nil, fmt.Errorf("degrade disposable service: %w", err)
		}
		degraded := true
		defer func() {
			if !degraded {
				return
			}
			restoreCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
			defer cancel()
			if restoreErr := controller.Recover(restoreCtx); restoreErr != nil {
				err = errors.Join(err, fmt.Errorf("recover disposable service: %w", restoreErr))
			}
		}()
		outage := h.syncWave(ctx, agentsync.Request{}, h.successDelays(policy))
		if err = controller.Recover(ctx); err != nil {
			return nil, fmt.Errorf("recover disposable service: %w", err)
		}
		degraded = false
		recovery := h.syncWave(ctx, agentsync.Request{}, h.retryDelays(policy))
		return []Wave{
			{Name: "policy-shaped-startup", Summary: startup},
			{Name: "policy-shaped-outage", Summary: outage},
			{Name: "policy-shaped-recovery", Summary: recovery},
		}, nil
	})
}

// MeasuredOverload records a concurrent Sync wave against a server that the
// caller has deliberately configured with a bounded admission limit.
func (h *Harness) MeasuredOverload(ctx context.Context) (Report, error) {
	return h.measured(ctx, func() ([]Wave, error) {
		return []Wave{{Name: "controlled-overload", Summary: h.SyncWave(ctx, agentsync.Request{})}}, nil
	})
}

func (h *Harness) measured(ctx context.Context, workload func() ([]Wave, error)) (Report, error) {
	beforeProcess, err := SnapshotProcess()
	if err != nil {
		return Report{}, fmt.Errorf("snapshot load process before workload: %w", err)
	}
	beforeDatabase, err := h.SnapshotDatabase(ctx)
	if err != nil {
		return Report{}, fmt.Errorf("snapshot database before workload: %w", err)
	}
	waves, err := workload()
	if err != nil {
		return Report{}, err
	}
	afterProcess, err := SnapshotProcess()
	if err != nil {
		return Report{}, fmt.Errorf("snapshot load process after workload: %w", err)
	}
	afterDatabase, err := h.SnapshotDatabase(ctx)
	if err != nil {
		return Report{}, fmt.Errorf("snapshot database after workload: %w", err)
	}
	return Report{
		Waves:          waves,
		ProcessBefore:  beforeProcess,
		ProcessAfter:   afterProcess,
		ProcessCPUUsed: afterProcess.CPU - beforeProcess.CPU,
		DatabaseBefore: beforeDatabase,
		DatabaseAfter:  afterDatabase,
		DatabaseDelta:  afterDatabase.Delta(beforeDatabase),
	}, nil
}

func (h *Harness) closeIdleConnections() {
	for i := range h.endpoints {
		if client := h.endpoints[i].client; client != nil && client.HTTPClient != nil {
			client.HTTPClient.CloseIdleConnections()
		}
	}
}

func (h *Harness) startupDelays(policy polling.Policy) map[string]time.Duration {
	delays := make(map[string]time.Duration, len(h.endpoints))
	random := polling.SystemRandom()
	for i := range h.endpoints {
		delays[h.endpoints[i].id] = policy.StartupDelay(random)
	}
	return delays
}

func (h *Harness) successDelays(policy polling.Policy) map[string]time.Duration {
	delays := make(map[string]time.Duration, len(h.endpoints))
	for i := range h.endpoints {
		delays[h.endpoints[i].id] = policy.SuccessDelay(h.endpoints[i].id)
	}
	return delays
}

func (h *Harness) retryDelays(policy polling.Policy) map[string]time.Duration {
	delays := make(map[string]time.Duration, len(h.endpoints))
	for i := range h.endpoints {
		backoff := polling.NewBackoff(policy, polling.SystemRandom())
		delays[h.endpoints[i].id] = backoff.NextDelay()
	}
	return delays
}

func (h *Harness) releaseFanout(ctx context.Context) (waves []Wave, err error) {
	if h.database == nil {
		return nil, errors.New("load endpoints have not been provisioned")
	}
	if len(h.endpoints) == 0 {
		return nil, errors.New("release fan-out requires at least one endpoint")
	}
	store := pgstore.NewFromPool(h.database)
	originalRelease, err := store.GetReleaseRef(ctx)
	if err != nil {
		return nil, fmt.Errorf("read current release ref: %w", err)
	}
	if originalRelease == "" {
		return nil, errors.New("current release ref is empty")
	}
	defer func() {
		restoreCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		if restoreErr := store.SetReleaseRef(restoreCtx, originalRelease); restoreErr != nil {
			err = errors.Join(err, fmt.Errorf("restore release ref %q: %w", originalRelease, restoreErr))
		}
	}()

	baseline := h.SyncWave(ctx, agentsync.Request{})
	currentArtifact, _, err := store.GetCompiledArtifactForFleet(ctx, h.cfg.Fleet, originalRelease, "desired")
	if err != nil {
		return nil, fmt.Errorf("load current fleet artifact: %w", err)
	}
	fanoutYAML, fanoutDigest := fanoutArtifact(currentArtifact, "release fan-out "+h.cfg.RunID)
	fanoutRelease := "load-fanout-" + h.cfg.RunID
	if err := store.StoreCompiledArtifactForFleet(ctx, h.cfg.Fleet, fanoutRelease, "desired", fanoutYAML, fanoutDigest); err != nil {
		return nil, fmt.Errorf("store fan-out artifact: %w", err)
	}
	if err := store.SetReleaseRef(ctx, fanoutRelease); err != nil {
		return nil, fmt.Errorf("advance release ref: %w", err)
	}
	fanout := h.SyncWave(ctx, agentsync.Request{})

	overrideArtifact, overrideDigest := fanoutArtifact(fanoutYAML, "endpoint override "+h.endpoints[0].id)
	if err := store.StoreCompiledArtifactForEndpoint(ctx, h.endpoints[0].id, fanoutRelease, "desired", overrideArtifact, overrideDigest); err != nil {
		return nil, fmt.Errorf("store endpoint override: %w", err)
	}
	override := h.SyncWave(ctx, agentsync.Request{})
	return []Wave{
		{Name: "baseline-artifact", Summary: baseline},
		{Name: "release-fan-out", Summary: fanout},
		{Name: "endpoint-override", Summary: override},
	}, nil
}

func fanoutArtifact(artifact []byte, label string) ([]byte, string) {
	updated := append([]byte(nil), artifact...)
	updated = append(updated, []byte("\n# load harness "+label+"\n")...)
	digest := sha256.Sum256(updated)
	return updated, stringDigest(digest)
}

func stringDigest(digest [sha256.Size]byte) string {
	return hex.EncodeToString(digest[:])
}

func telemetryHeavyRequest(endpointID string) agentsync.Request {
	inventory := strings.Repeat("load-telemetry-", 1800)
	driftDetail := strings.Repeat("drift-detail-", 500)
	firewallDetail := strings.Repeat("observed-rule-", 250)
	return agentsync.Request{
		Labels: map[string]string{
			"environment": "load",
			"location":    "test-lab",
			"owner":       "load-harness",
			"platform":    "linux",
			"role":        "telemetry",
		},
		AgentVersion: "load-harness/1",
		Usernames:    []string{"load-user", "operator"},
		SystemInfo: &agentsync.SystemInfoPayload{
			Digest: "system-" + endpointID,
			Report: json.RawMessage(fmt.Sprintf(`{"endpoint":%q,"inventory":%q}`, endpointID, inventory)),
		},
		Drift: &agentsync.DriftPayload{
			Digest: "drift-" + endpointID,
			Report: json.RawMessage(fmt.Sprintf(`{"inCompliance":false,"items":[{"resourceAddress":"load.telemetry","status":"drifted","detail":%q}]}`, driftDetail)),
		},
		FirewallAudit: &agentsync.FirewallAuditPayload{
			Digest: "firewall-" + endpointID,
			Report: json.RawMessage(fmt.Sprintf(`{"endpoint":%q,"observedRules":%q}`, endpointID, firewallDetail)),
		},
	}
}

// SnapshotDatabase reads bounded pool state and database-wide workload counters.
func (h *Harness) SnapshotDatabase(ctx context.Context) (DatabaseMetrics, error) {
	if h.database == nil {
		return DatabaseMetrics{}, errors.New("load endpoints have not been provisioned")
	}
	stat := h.database.Stat()
	metrics := DatabaseMetrics{
		PoolAcquireCount:         stat.AcquireCount(),
		PoolAcquiredConns:        stat.AcquiredConns(),
		PoolIdleConns:            stat.IdleConns(),
		PoolTotalConns:           stat.TotalConns(),
		PoolEmptyAcquireCount:    stat.EmptyAcquireCount(),
		PoolCanceledAcquireCount: stat.CanceledAcquireCount(),
	}
	err := h.database.QueryRow(ctx, `
		SELECT xact_commit, xact_rollback, blks_read, blks_hit,
		       tup_returned, tup_fetched, tup_inserted, temp_files,
		       temp_bytes, deadlocks, numbackends
		FROM pg_stat_database
		WHERE datname = current_database()
	`).Scan(
		&metrics.XactCommit, &metrics.XactRollback, &metrics.BlocksRead, &metrics.BlocksHit,
		&metrics.TuplesReturned, &metrics.TuplesFetched, &metrics.TuplesInserted, &metrics.TempFiles,
		&metrics.TempBytes, &metrics.Deadlocks, &metrics.Backends,
	)
	if err != nil {
		return DatabaseMetrics{}, err
	}
	return metrics, nil
}

// Delta subtracts cumulative database counters and retains ending pool state.
func (after DatabaseMetrics) Delta(before DatabaseMetrics) DatabaseDelta {
	return DatabaseDelta{
		PoolAcquireCount:         after.PoolAcquireCount - before.PoolAcquireCount,
		PoolAcquiredConns:        after.PoolAcquiredConns,
		PoolIdleConns:            after.PoolIdleConns,
		PoolTotalConns:           after.PoolTotalConns,
		PoolEmptyAcquireCount:    after.PoolEmptyAcquireCount - before.PoolEmptyAcquireCount,
		PoolCanceledAcquireCount: after.PoolCanceledAcquireCount - before.PoolCanceledAcquireCount,
		XactCommit:               after.XactCommit - before.XactCommit,
		XactRollback:             after.XactRollback - before.XactRollback,
		BlocksRead:               after.BlocksRead - before.BlocksRead,
		BlocksHit:                after.BlocksHit - before.BlocksHit,
		TuplesReturned:           after.TuplesReturned - before.TuplesReturned,
		TuplesFetched:            after.TuplesFetched - before.TuplesFetched,
		TuplesInserted:           after.TuplesInserted - before.TuplesInserted,
		TempFiles:                after.TempFiles - before.TempFiles,
		TempBytes:                after.TempBytes - before.TempBytes,
		Deadlocks:                after.Deadlocks - before.Deadlocks,
		Backends:                 after.Backends,
	}
}

// SnapshotProcess captures the load-generator's CPU, current RSS, heap, and
// goroutine count. Linux /proc is intentional because Remotr's agents and
// controlled performance runners are Linux targets.
func SnapshotProcess() (ProcessMetrics, error) {
	var usage syscall.Rusage
	if err := syscall.Getrusage(syscall.RUSAGE_SELF, &usage); err != nil {
		return ProcessMetrics{}, err
	}
	status, err := os.Open("/proc/self/status")
	if err != nil {
		return ProcessMetrics{}, err
	}
	defer status.Close()
	rss, err := parseRSSBytes(status)
	if err != nil {
		return ProcessMetrics{}, err
	}
	var memory runtime.MemStats
	runtime.ReadMemStats(&memory)
	return ProcessMetrics{
		CPU:            timevalDuration(usage.Utime) + timevalDuration(usage.Stime),
		RSSBytes:       rss,
		HeapAllocBytes: memory.HeapAlloc,
		Goroutines:     runtime.NumGoroutine(),
	}, nil
}

func timevalDuration(value syscall.Timeval) time.Duration {
	return time.Duration(value.Sec)*time.Second + time.Duration(value.Usec)*time.Microsecond
}

func parseRSSBytes(input io.Reader) (uint64, error) {
	scanner := bufio.NewScanner(input)
	for scanner.Scan() {
		fields := strings.Fields(scanner.Text())
		if len(fields) != 3 || fields[0] != "VmRSS:" || fields[2] != "kB" {
			continue
		}
		value, err := strconv.ParseUint(fields[1], 10, 64)
		if err != nil {
			return 0, fmt.Errorf("parse VmRSS: %w", err)
		}
		return value * 1024, nil
	}
	if err := scanner.Err(); err != nil {
		return 0, err
	}
	return 0, errors.New("VmRSS not found")
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
	starts := make([]time.Time, 0, len(samples))
	for _, sample := range samples {
		latencies = append(latencies, sample.Latency)
		if !sample.StartedAt.IsZero() {
			starts = append(starts, sample.StartedAt)
		}
		summary.ResponseBytes += sample.ResponseBytes
		summary.RequestBytes += sample.RequestBytes
		if sample.Unchanged {
			summary.Unchanged++
		}
		if sample.Err != nil {
			summary.Errors++
			if agentsync.IsOverloaded(sample.Err) {
				summary.Overloaded++
			}
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
	if len(starts) > 0 {
		sort.Slice(starts, func(i, j int) bool { return starts[i].Before(starts[j]) })
		summary.StartSpread = starts[len(starts)-1].Sub(starts[0])
		buckets := make(map[int]int)
		for _, started := range starts {
			bucket := int(started.Sub(starts[0]) / (100 * time.Millisecond))
			buckets[bucket]++
			if buckets[bucket] > summary.MaxStartsPer100ms {
				summary.MaxStartsPer100ms = buckets[bucket]
			}
		}
	}
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
