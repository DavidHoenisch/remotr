package server

import (
	"net/http"
	"strconv"
	"time"
)

const (
	defaultSyncRetryAfter = 5 * time.Second
	maxSyncRetryAfter     = 5 * time.Minute
)

// SyncAdmission limits concurrent authenticated Sync handling. It is injected
// so overload behavior remains deterministic in tests and controlled runners.
type SyncAdmission interface {
	Acquire() (release func(), retryAfter time.Duration, admitted bool)
}

type syncLimiter struct {
	slots      chan struct{}
	retryAfter time.Duration
}

func newSyncLimiter(maxConcurrent int, retryAfter time.Duration) *syncLimiter {
	if retryAfter <= 0 {
		retryAfter = defaultSyncRetryAfter
	}
	if retryAfter > maxSyncRetryAfter {
		retryAfter = maxSyncRetryAfter
	}
	return &syncLimiter{slots: make(chan struct{}, maxConcurrent), retryAfter: retryAfter}
}

func (l *syncLimiter) Acquire() (func(), time.Duration, bool) {
	select {
	case l.slots <- struct{}{}:
		return func() { <-l.slots }, 0, true
	default:
		return nil, l.retryAfter, false
	}
}

func writeSyncOverload(w http.ResponseWriter, retryAfter time.Duration) {
	if retryAfter <= 0 {
		retryAfter = defaultSyncRetryAfter
	}
	if retryAfter > maxSyncRetryAfter {
		retryAfter = maxSyncRetryAfter
	}
	seconds := int64((retryAfter + time.Second - 1) / time.Second)
	w.Header().Set("Retry-After", strconv.FormatInt(seconds, 10))
	http.Error(w, "sync overloaded", http.StatusServiceUnavailable)
}
