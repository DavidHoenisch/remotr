package diagnostics

import (
	"errors"
	"fmt"
	"slices"
	"strings"
	"time"
)

const (
	MaxTimeSpan       = 7 * 24 * time.Hour
	DefaultLookback   = 24 * time.Hour
	MaxCollectorBytes = 5 << 20  // 5 MiB
	MaxBundleBytes    = 32 << 20 // 32 MiB
	BundleTTL         = 24 * time.Hour
)

// Collector IDs for allowlisted diagnostic sources.
const (
	CollectorSystemInfo       = "system_info"
	CollectorNetworkState     = "network_state"
	CollectorJournalRemotr    = "journal_remotr"
	CollectorJournalKernel    = "journal_kernel"
	CollectorJournalAudit     = "journal_audit"
	CollectorDmesg            = "dmesg"
	CollectorRemotrAgentState = "remotr_agent_state"
)

var allCollectors = []string{
	CollectorSystemInfo,
	CollectorNetworkState,
	CollectorJournalRemotr,
	CollectorJournalKernel,
	CollectorJournalAudit,
	CollectorDmesg,
	CollectorRemotrAgentState,
}

// Spec is the validated collection request sent to the agent.
type Spec struct {
	Collectors []string  `json:"collectors"`
	Since      time.Time `json:"since"`
	Until      time.Time `json:"until"`
}

// RequestStatus values for diagnostic_requests.status.
const (
	StatusPending    = "pending"
	StatusDispatched = "dispatched"
	StatusRunning    = "running"
	StatusReady      = "ready"
	StatusFailed     = "failed"
	StatusExpired    = "expired"
)

var (
	ErrInvalidCollector   = errors.New("invalid collector")
	ErrInvalidTimeRange   = errors.New("invalid time range")
	ErrActiveRequest      = errors.New("endpoint already has an active diagnostic request")
	ErrDiagnosticsUnavail = errors.New("diagnostics unavailable")
)

// DefaultCollectors returns all v1 collector IDs.
func DefaultCollectors() []string {
	out := make([]string, len(allCollectors))
	copy(out, allCollectors)
	return out
}

// ValidCollector reports whether name is an allowlisted collector ID.
func ValidCollector(name string) bool {
	return slices.Contains(allCollectors, strings.TrimSpace(name))
}

// NormalizeCollectors validates and deduplicates collector names.
func NormalizeCollectors(names []string) ([]string, error) {
	if len(names) == 0 {
		return DefaultCollectors(), nil
	}
	seen := make(map[string]struct{}, len(names))
	out := make([]string, 0, len(names))
	for _, raw := range names {
		name := strings.TrimSpace(raw)
		if name == "" {
			continue
		}
		if !ValidCollector(name) {
			return nil, fmt.Errorf("%w: %q", ErrInvalidCollector, name)
		}
		if _, ok := seen[name]; ok {
			continue
		}
		seen[name] = struct{}{}
		out = append(out, name)
	}
	if len(out) == 0 {
		return nil, fmt.Errorf("%w: empty collector list", ErrInvalidCollector)
	}
	return out, nil
}

// NormalizeTimeRange validates since/until with defaults and bounds.
func NormalizeTimeRange(since, until time.Time, now time.Time) (time.Time, time.Time, error) {
	if now.IsZero() {
		now = time.Now().UTC()
	}
	if until.IsZero() {
		until = now
	} else {
		until = until.UTC()
	}
	if since.IsZero() {
		since = until.Add(-DefaultLookback)
	} else {
		since = since.UTC()
	}
	if until.After(now) {
		return time.Time{}, time.Time{}, fmt.Errorf("%w: until is in the future", ErrInvalidTimeRange)
	}
	if !since.Before(until) && !since.Equal(until) {
		return time.Time{}, time.Time{}, fmt.Errorf("%w: since must be before until", ErrInvalidTimeRange)
	}
	if until.Sub(since) > MaxTimeSpan {
		return time.Time{}, time.Time{}, fmt.Errorf("%w: span exceeds %s", ErrInvalidTimeRange, MaxTimeSpan)
	}
	return since, until, nil
}

// NormalizeSpec validates collectors and time range.
func NormalizeSpec(collectors []string, since, until time.Time) (Spec, error) {
	names, err := NormalizeCollectors(collectors)
	if err != nil {
		return Spec{}, err
	}
	s, u, err := NormalizeTimeRange(since, until, time.Now().UTC())
	if err != nil {
		return Spec{}, err
	}
	return Spec{Collectors: names, Since: s, Until: u}, nil
}

// S3Key returns the object key for a diagnostic bundle.
func S3Key(endpointID, requestID string) string {
	return fmt.Sprintf("diagnostics/%s/%s.tar.gz", endpointID, requestID)
}
