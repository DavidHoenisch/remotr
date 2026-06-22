package inventory

import "time"

const (
	defaultReportInterval = time.Hour
	defaultChangeMinGap   = 5 * time.Minute
)

// ThrottleState tracks when system info was last reported to the server.
type ThrottleState struct {
	LastSentAt     time.Time `json:"lastSentAt,omitempty"`
	LastSentDigest string    `json:"lastSentDigest,omitempty"`
}

// Throttler decides whether a snapshot should be reported on sync.
type Throttler struct {
	Interval       time.Duration
	ChangeMinGap   time.Duration
	LastSentAt     time.Time
	LastSentDigest string
}

// NewThrottler returns a throttler with the given interval and change minimum gap.
func NewThrottler(interval, changeMinGap time.Duration) *Throttler {
	if interval <= 0 {
		interval = defaultReportInterval
	}
	if changeMinGap <= 0 {
		changeMinGap = defaultChangeMinGap
	}
	return &Throttler{
		Interval:     interval,
		ChangeMinGap: changeMinGap,
	}
}

// LoadState restores throttle state from persisted credentials metadata.
func (t *Throttler) LoadState(st ThrottleState) {
	t.LastSentAt = st.LastSentAt
	t.LastSentDigest = st.LastSentDigest
}

// State returns the current throttle state for persistence.
func (t *Throttler) State() ThrottleState {
	return ThrottleState{
		LastSentAt:     t.LastSentAt,
		LastSentDigest: t.LastSentDigest,
	}
}

// ShouldReport reports whether the snapshot with the given digest should be sent now.
func (t *Throttler) ShouldReport(now time.Time, digest string) bool {
	if t.LastSentAt.IsZero() {
		return true
	}
	elapsed := now.Sub(t.LastSentAt)
	if elapsed >= t.Interval {
		return true
	}
	if digest != "" && digest != t.LastSentDigest && elapsed >= t.ChangeMinGap {
		return true
	}
	return false
}

// MarkSent records a successful report at now with the given digest.
func (t *Throttler) MarkSent(now time.Time, digest string) {
	t.LastSentAt = now
	t.LastSentDigest = digest
}
