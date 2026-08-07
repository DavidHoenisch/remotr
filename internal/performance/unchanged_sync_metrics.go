package performance

import (
	"sync"
	"time"
)

type BoundedMeasure struct {
	Count uint64 `json:"count"`
	Total uint64 `json:"total"`
	Max   uint64 `json:"max"`
}

type UnchangedSyncFastPathMetrics struct {
	Hits                       uint64            `json:"hits"`
	Misses                     map[string]uint64 `json:"misses"`
	Invalidations              map[string]uint64 `json:"invalidations"`
	Evictions                  map[string]uint64 `json:"evictions"`
	DocumentRequests           map[string]uint64 `json:"documentRequests"`
	Checkpoints                map[string]uint64 `json:"checkpoints"`
	Disabled                   map[string]uint64 `json:"disabled"`
	Backends                   map[string]uint64 `json:"backends"`
	RedisErrors                map[string]uint64 `json:"redisErrors"`
	Fallbacks                  map[string]uint64 `json:"fallbacks"`
	DecisionLatencyNanoseconds BoundedMeasure    `json:"decisionLatencyNanoseconds"`
	ResponseBytes              BoundedMeasure    `json:"responseBytes"`
	DatabaseOperations         BoundedMeasure    `json:"databaseOperations"`
}

var unchangedSyncMetrics = struct {
	sync.Mutex
	value UnchangedSyncFastPathMetrics
}{value: newUnchangedSyncFastPathMetrics()}

func newUnchangedSyncFastPathMetrics() UnchangedSyncFastPathMetrics {
	return UnchangedSyncFastPathMetrics{
		Misses: map[string]uint64{}, Invalidations: map[string]uint64{}, Evictions: map[string]uint64{},
		DocumentRequests: map[string]uint64{}, Checkpoints: map[string]uint64{}, Disabled: map[string]uint64{},
		Backends: map[string]uint64{}, RedisErrors: map[string]uint64{}, Fallbacks: map[string]uint64{},
	}
}

func RecordFastPathHit(elapsed time.Duration, responseBytes, databaseOperations uint64) {
	unchangedSyncMetrics.Lock()
	defer unchangedSyncMetrics.Unlock()
	unchangedSyncMetrics.value.Hits++
	observeBounded(&unchangedSyncMetrics.value.DecisionLatencyNanoseconds, uint64(max(elapsed.Nanoseconds(), 0)))
	observeBounded(&unchangedSyncMetrics.value.ResponseBytes, responseBytes)
	observeBounded(&unchangedSyncMetrics.value.DatabaseOperations, databaseOperations)
}

func RecordFastPathMiss(reason string, elapsed time.Duration) {
	recordBoundedCounter(unchangedSyncMetrics.value.Misses, allowed(reason, []string{"ineligible", "absent", "deadline", "identity", "delivery", "documents", "authority"}), elapsed)
}

func RecordFastPathInvalidation(scope string)     { recordCounter("invalidation", scope) }
func RecordFastPathEviction(reason string)        { recordCounter("eviction", reason) }
func RecordFastPathDocumentRequest(domain string) { recordCounter("document", domain) }
func RecordFastPathCheckpoint(outcome string)     { recordCounter("checkpoint", outcome) }
func RecordFastPathDisabled(reason string)        { recordCounter("disabled", reason) }
func RecordFastPathBackend(backend string)        { recordCounter("backend", backend) }
func RecordFastPathRedisError(class string)       { recordCounter("redis_error", class) }
func RecordFastPathFallback(reason string)        { recordCounter("fallback", reason) }

func recordCounter(kind, value string) {
	unchangedSyncMetrics.Lock()
	defer unchangedSyncMetrics.Unlock()
	switch kind {
	case "invalidation":
		unchangedSyncMetrics.value.Invalidations[allowed(value, []string{"global", "fleet", "endpoint"})]++
	case "eviction":
		unchangedSyncMetrics.value.Evictions[allowed(value, []string{"lru", "ttl", "mutation"})]++
	case "document":
		unchangedSyncMetrics.value.DocumentRequests[allowed(value, []string{"capability", "system_information", "delivery", "targeting", "changed"})]++
	case "checkpoint":
		unchangedSyncMetrics.value.Checkpoints[allowed(value, []string{"due", "success", "failure"})]++
	case "disabled":
		unchangedSyncMetrics.value.Disabled[allowed(value, []string{"disabled_by_configuration", "multiple_serving_processes_without_coordinator", "redis_configuration"})]++
	case "backend":
		unchangedSyncMetrics.value.Backends[allowed(value, []string{"disabled", "memory", "redis"})]++
	case "redis_error":
		unchangedSyncMetrics.value.RedisErrors[allowed(value, []string{"lookup", "fill", "snapshot", "mutation", "checkpoint"})]++
	case "fallback":
		unchangedSyncMetrics.value.Fallbacks[allowed(value, []string{"redis_unavailable"})]++
	}
}

func recordBoundedCounter(counters map[string]uint64, reason string, elapsed time.Duration) {
	unchangedSyncMetrics.Lock()
	defer unchangedSyncMetrics.Unlock()
	counters[reason]++
	observeBounded(&unchangedSyncMetrics.value.DecisionLatencyNanoseconds, uint64(max(elapsed.Nanoseconds(), 0)))
}

func allowed(value string, values []string) string {
	for _, candidate := range values {
		if value == candidate {
			return value
		}
	}
	return "other"
}

func observeBounded(measure *BoundedMeasure, value uint64) {
	measure.Count++
	measure.Total += value
	if value > measure.Max {
		measure.Max = value
	}
}

func SnapshotUnchangedSyncFastPathMetrics() UnchangedSyncFastPathMetrics {
	unchangedSyncMetrics.Lock()
	defer unchangedSyncMetrics.Unlock()
	result := unchangedSyncMetrics.value
	result.Misses = cloneCounters(result.Misses)
	result.Invalidations = cloneCounters(result.Invalidations)
	result.Evictions = cloneCounters(result.Evictions)
	result.DocumentRequests = cloneCounters(result.DocumentRequests)
	result.Checkpoints = cloneCounters(result.Checkpoints)
	result.Disabled = cloneCounters(result.Disabled)
	result.Backends = cloneCounters(result.Backends)
	result.RedisErrors = cloneCounters(result.RedisErrors)
	result.Fallbacks = cloneCounters(result.Fallbacks)
	return result
}

func cloneCounters(input map[string]uint64) map[string]uint64 {
	result := make(map[string]uint64, len(input))
	for key, value := range input {
		result[key] = value
	}
	return result
}
