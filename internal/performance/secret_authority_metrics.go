package performance

import "sync"

// SecretAuthorityCacheMetrics contains bounded, material-free agent counters.
type SecretAuthorityCacheMetrics struct {
	Primes        uint64 `json:"primes"`
	Hits          uint64 `json:"hits"`
	DenialHits    uint64 `json:"denialHits"`
	Evictions     uint64 `json:"evictions"`
	Invalidations uint64 `json:"invalidations"`
	FailClosed    uint64 `json:"failClosed"`
	Declined      uint64 `json:"declined"`
}

var secretAuthorityMetrics = struct {
	sync.Mutex
	value SecretAuthorityCacheMetrics
}{}

func RecordSecretAuthorityPrime() {
	recordSecretAuthorityMetric(func(value *SecretAuthorityCacheMetrics) {
		value.Primes++
	})
}

func RecordSecretAuthorityHit(denial bool) {
	recordSecretAuthorityMetric(func(value *SecretAuthorityCacheMetrics) {
		if denial {
			value.DenialHits++
			return
		}
		value.Hits++
	})
}

func RecordSecretAuthorityEviction() {
	recordSecretAuthorityMetric(func(value *SecretAuthorityCacheMetrics) {
		value.Evictions++
	})
}

func RecordSecretAuthorityInvalidation() {
	recordSecretAuthorityMetric(func(value *SecretAuthorityCacheMetrics) {
		value.Invalidations++
	})
}

func RecordSecretAuthorityFailClosed() {
	recordSecretAuthorityMetric(func(value *SecretAuthorityCacheMetrics) {
		value.FailClosed++
	})
}

func RecordSecretAuthorityDeclined() {
	recordSecretAuthorityMetric(func(value *SecretAuthorityCacheMetrics) {
		value.Declined++
	})
}

func recordSecretAuthorityMetric(
	record func(*SecretAuthorityCacheMetrics),
) {
	secretAuthorityMetrics.Lock()
	defer secretAuthorityMetrics.Unlock()
	record(&secretAuthorityMetrics.value)
}

// SnapshotSecretAuthorityCacheMetrics returns a race-safe counter snapshot.
func SnapshotSecretAuthorityCacheMetrics() SecretAuthorityCacheMetrics {
	secretAuthorityMetrics.Lock()
	defer secretAuthorityMetrics.Unlock()
	return secretAuthorityMetrics.value
}
