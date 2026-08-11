package server

import (
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"math"
	"sort"
	"sync"
	"time"

	"github.com/DavidHoenisch/remotr/internal/changecontrol"
	"github.com/DavidHoenisch/remotr/internal/documenthash"
	"github.com/DavidHoenisch/remotr/internal/performance"
)

type FastPathBackend string

const (
	FastPathDisabled FastPathBackend = "disabled"
	FastPathMemory   FastPathBackend = "memory"
	FastPathRedis    FastPathBackend = "redis"
)

// FastPathConfig controls the unchanged Sync decision backend.
type FastPathConfig struct {
	Enabled            bool
	Backend            FastPathBackend
	RedisURL           string
	RedisPrefix        string
	ServingProcesses   int
	MaxEntries         int
	MaxBytes           int
	MaxObservations    uint64
	TTL                time.Duration
	CheckpointInterval time.Duration
}

// FastPathStatus is a bounded startup status suitable for logs and metrics.
type FastPathStatus struct {
	Enabled bool
	Backend string
	Reason  string
}

func resolveFastPathStatus(config FastPathConfig) FastPathStatus {
	backend := config.Backend
	if backend == "" {
		if config.Enabled {
			backend = FastPathMemory
		} else {
			backend = FastPathDisabled
		}
	}
	if backend == FastPathDisabled {
		return FastPathStatus{Backend: string(backend), Reason: "disabled_by_configuration"}
	}
	if backend == FastPathMemory && config.ServingProcesses > 1 {
		return FastPathStatus{Backend: string(backend), Reason: "multiple_serving_processes_without_coordinator"}
	}
	return FastPathStatus{Enabled: true, Backend: string(backend), Reason: "enabled"}
}

type unchangedSyncEntry struct {
	fingerprint  string
	fleet        string
	hashes       map[string]string
	releaseRef   string
	digest       string
	response     syncResponse
	validUntil   time.Time
	checkpointAt time.Time
	windowStart  time.Time
	sizeBytes    int
	lastUsed     uint64
	observations uint64
	authority    authoritySnapshot
}

type syncCheckpoint struct {
	windowStart  time.Time
	windowEnd    time.Time
	observations uint64
	releaseRef   string
	digest       string
	fleet        string
	sizeBytes    int
}

type cacheScope uint8

const (
	cacheScopeGlobal cacheScope = iota
	cacheScopeFleet
	cacheScopeEndpoint
)

type authoritySnapshot struct {
	epoch    string
	global   uint64
	fleet    uint64
	endpoint uint64
	stable   bool
}

type unchangedSyncCache struct {
	mu                  sync.Mutex
	maxEntries          int
	maxBytes            int
	maxObservations     uint64
	usedBytes           int
	pendingBytes        int
	observations        uint64
	sequence            uint64
	ttl                 time.Duration
	checkpointInterval  time.Duration
	entries             map[string]unchangedSyncEntry
	pendingCheckpoints  map[string]syncCheckpoint
	pendingObservations uint64
	globalGeneration    uint64
	fleetGenerations    map[string]uint64
	endpointGenerations map[string]uint64
	globalUnstable      int
	fleetUnstable       map[string]int
	endpointUnstable    map[string]int
	authorityEpoch      string
	redis               *redisSyncBackend
}

func newUnchangedSyncCache(config FastPathConfig) *unchangedSyncCache {
	status := resolveFastPathStatus(config)
	performance.RecordFastPathBackend(status.Backend)
	if !status.Enabled {
		performance.RecordFastPathDisabled(status.Reason)
		return nil
	}
	if config.MaxEntries <= 0 {
		config.MaxEntries = 10_000
	}
	if config.MaxBytes <= 0 {
		config.MaxBytes = 64 << 20
	}
	if config.MaxObservations == 0 {
		config.MaxObservations = math.MaxUint32
	}
	if config.TTL <= 0 {
		config.TTL = 10 * time.Minute
	}
	if config.CheckpointInterval <= 0 {
		config.CheckpointInterval = 5 * time.Minute
	}
	cache := &unchangedSyncCache{
		maxEntries: config.MaxEntries, maxBytes: config.MaxBytes, maxObservations: config.MaxObservations,
		ttl: config.TTL, checkpointInterval: config.CheckpointInterval,
		entries: make(map[string]unchangedSyncEntry), pendingCheckpoints: make(map[string]syncCheckpoint),
		fleetGenerations: make(map[string]uint64), endpointGenerations: make(map[string]uint64),
		fleetUnstable: make(map[string]int), endpointUnstable: make(map[string]int),
		authorityEpoch: randomAuthorityEpoch(),
	}
	if config.Backend == FastPathRedis {
		backend, err := newRedisSyncBackend(config)
		if err != nil {
			performance.RecordFastPathDisabled("redis_configuration")
			return nil
		}
		cache.redis = backend
	}
	return cache
}

func randomAuthorityEpoch() string {
	var value [32]byte
	if _, err := rand.Read(value[:]); err != nil {
		return ""
	}
	return hex.EncodeToString(value[:])
}

func (c *unchangedSyncCache) get(endpointID, fingerprint string, request syncRequest, now time.Time) (syncResponse, bool) {
	response, hit, _ := c.getWithCheckpoint(endpointID, fingerprint, request, now)
	return response, hit
}

func (c *unchangedSyncCache) getWithCheckpoint(endpointID, fingerprint string, request syncRequest, now time.Time) (syncResponse, bool, *syncCheckpoint) {
	started := time.Now()
	if c == nil || !eligibleHashOnlyRequest(request) {
		performance.RecordFastPathMiss("ineligible", time.Since(started))
		return syncResponse{}, false, nil
	}
	if c.redis != nil {
		response, hit, checkpoint, err := c.redis.get(endpointID, fingerprint, request, now)
		if err != nil {
			performance.RecordFastPathRedisError("lookup")
			performance.RecordFastPathFallback("redis_unavailable")
			performance.RecordFastPathMiss("backend", time.Since(started))
			return syncResponse{}, false, nil
		}
		if hit {
			encoded, _ := json.Marshal(response)
			performance.RecordFastPathHit(time.Since(started), uint64(len(encoded)), 0)
		}
		if checkpoint != nil {
			c.mu.Lock()
			c.pendingCheckpoints[endpointID] = *checkpoint
			c.mu.Unlock()
		}
		return response, hit, checkpoint
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	entry, ok := c.entries[endpointID]
	if ok && !now.Before(entry.checkpointAt) {
		performance.RecordFastPathCheckpoint("due")
		performance.RecordFastPathMiss("deadline", time.Since(started))
		observations := entry.observations
		if c.observations+c.pendingObservations < c.maxObservations {
			observations++
		}
		checkpoint := syncCheckpoint{
			windowStart: entry.windowStart, windowEnd: now, observations: observations,
			releaseRef: entry.releaseRef, digest: entry.digest, fleet: entry.fleet, sizeBytes: entry.sizeBytes,
		}
		if previous, exists := c.pendingCheckpoints[endpointID]; exists {
			c.pendingObservations -= previous.observations
			c.pendingBytes -= previous.sizeBytes
		}
		c.pendingCheckpoints[endpointID] = checkpoint
		c.pendingObservations += checkpoint.observations
		c.pendingBytes += checkpoint.sizeBytes
		c.remove(endpointID)
		return syncResponse{}, false, &checkpoint
	}
	if !ok || entry.fingerprint != fingerprint || !now.Before(entry.validUntil) ||
		entry.releaseRef != request.LastReleaseRef || entry.digest != request.LastDigest ||
		!equalDocumentHashes(entry.hashes, request.documentHashes.Documents) ||
		!c.authorityCurrent(endpointID, entry.fleet, entry.authority) {
		if ok && !now.Before(entry.validUntil) {
			c.remove(endpointID)
			performance.RecordFastPathEviction("ttl")
		}
		reason := "absent"
		if ok && entry.fingerprint != fingerprint {
			reason = "identity"
		}
		if ok && (entry.releaseRef != request.LastReleaseRef || entry.digest != request.LastDigest) {
			reason = "delivery"
		}
		if ok && !equalDocumentHashes(entry.hashes, request.documentHashes.Documents) {
			reason = "documents"
		}
		if ok && !c.authorityCurrent(endpointID, entry.fleet, entry.authority) {
			reason = "authority"
		}
		performance.RecordFastPathMiss(reason, time.Since(started))
		return syncResponse{}, false, nil
	}
	c.sequence++
	entry.lastUsed = c.sequence
	if c.observations < c.maxObservations {
		entry.observations++
		c.observations++
	}
	c.entries[endpointID] = entry
	response := cloneSyncResponse(entry.response)
	encoded, _ := json.Marshal(response)
	performance.RecordFastPathHit(time.Since(started), uint64(len(encoded)), 0)
	return response, true, nil
}

func (c *unchangedSyncCache) put(endpointID, fingerprint string, request syncRequest, response syncResponse, now time.Time) {
	snapshot := c.authoritySnapshot(endpointID, "")
	c.putWithSnapshot(endpointID, "", fingerprint, request, response, now, snapshot)
}

func (c *unchangedSyncCache) putWithSnapshot(endpointID, fleet, fingerprint string, request syncRequest, response syncResponse, now time.Time, snapshot authoritySnapshot) {
	if c == nil || !quietCacheableResponse(response) || response.AcceptedDocumentHashes == nil || len(response.AcceptedDocumentHashes.Documents) == 0 {
		return
	}
	if request.LastReleaseRef != response.ReleaseRef || request.LastDigest != response.Digest {
		return
	}
	if c.redis != nil {
		if c.redis.put(endpointID, fleet, fingerprint, request, response, now, snapshot) != nil {
			performance.RecordFastPathRedisError("fill")
		}
		return
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	if !c.authorityCurrent(endpointID, fleet, snapshot) {
		return
	}
	if _, pending := c.pendingCheckpoints[endpointID]; pending {
		return
	}
	if _, exists := c.entries[endpointID]; exists {
		c.remove(endpointID)
	}
	checkpointAt := now.Add(c.checkpointInterval)
	validUntil := deriveValidUntil(now, c.ttl)
	if checkpointAt.Before(validUntil) {
		validUntil = checkpointAt
	}
	entry := unchangedSyncEntry{
		fingerprint:  fingerprint,
		fleet:        fleet,
		hashes:       cloneHashes(response.AcceptedDocumentHashes.Documents),
		releaseRef:   response.ReleaseRef,
		digest:       response.Digest,
		response:     cloneSyncResponse(response),
		validUntil:   validUntil,
		checkpointAt: checkpointAt,
		windowStart:  now,
		authority:    snapshot,
	}
	entry.sizeBytes = cacheEntrySize(endpointID, entry)
	if entry.sizeBytes > c.maxBytes {
		return
	}
	for len(c.entries)+len(c.pendingCheckpoints) >= c.maxEntries || c.usedBytes+c.pendingBytes+entry.sizeBytes > c.maxBytes {
		if !c.evictLRU() {
			return
		}
	}
	c.sequence++
	entry.lastUsed = c.sequence
	c.entries[endpointID] = entry
	c.usedBytes += entry.sizeBytes
}

func (c *unchangedSyncCache) pendingCheckpoint(endpointID string) (syncCheckpoint, bool) {
	if c == nil {
		return syncCheckpoint{}, false
	}
	if c.redis != nil {
		c.mu.Lock()
		local, ok := c.pendingCheckpoints[endpointID]
		c.mu.Unlock()
		if ok {
			return local, true
		}
		return c.redis.pending(endpointID)
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	checkpoint, ok := c.pendingCheckpoints[endpointID]
	return checkpoint, ok
}

func (c *unchangedSyncCache) completeCheckpoint(endpointID string) {
	if c == nil {
		return
	}
	if c.redis != nil {
		c.mu.Lock()
		delete(c.pendingCheckpoints, endpointID)
		c.mu.Unlock()
		c.redis.completeCheckpoint(endpointID)
		return
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	if checkpoint, ok := c.pendingCheckpoints[endpointID]; ok {
		c.pendingObservations -= checkpoint.observations
		c.pendingBytes -= checkpoint.sizeBytes
		delete(c.pendingCheckpoints, endpointID)
	}
}

func (c *unchangedSyncCache) authoritySnapshot(endpointID, fleet string) authoritySnapshot {
	if c == nil {
		return authoritySnapshot{}
	}
	if c.redis != nil {
		snapshot, err := c.redis.snapshot(endpointID, fleet)
		if err != nil {
			performance.RecordFastPathRedisError("snapshot")
			return authoritySnapshot{}
		}
		return snapshot
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	return authoritySnapshot{
		epoch:  c.authorityEpoch,
		global: c.globalGeneration, fleet: c.fleetGenerations[fleet], endpoint: c.endpointGenerations[endpointID],
		stable: c.authorityEpoch != "" && c.globalUnstable == 0 && c.fleetUnstable[fleet] == 0 && c.endpointUnstable[endpointID] == 0,
	}
}

func (c *unchangedSyncCache) secretAuthorityToken(endpointID, fleet string) string {
	snapshot := c.authoritySnapshot(endpointID, fleet)
	return secretAuthorityTokenFromSnapshot(snapshot)
}

func secretAuthorityTokenFromSnapshot(snapshot authoritySnapshot) string {
	if !snapshot.stable || snapshot.epoch == "" {
		return ""
	}
	digest := sha256.New()
	_, _ = fmt.Fprintf(digest, "remotr-secret-authority-v1\x00%s\x00%d\x00%d\x00%d",
		snapshot.epoch, snapshot.global, snapshot.fleet, snapshot.endpoint)
	return hex.EncodeToString(digest.Sum(nil))
}

// authorityCurrent is called only while c.mu is held.
func (c *unchangedSyncCache) authorityCurrent(endpointID, fleet string, snapshot authoritySnapshot) bool {
	return snapshot.stable && c.globalUnstable == 0 && c.fleetUnstable[fleet] == 0 && c.endpointUnstable[endpointID] == 0 &&
		snapshot.global == c.globalGeneration && snapshot.fleet == c.fleetGenerations[fleet] && snapshot.endpoint == c.endpointGenerations[endpointID]
}

func (c *unchangedSyncCache) beginMutation(scope cacheScope, key string) func() {
	complete, _ := c.beginMutationChecked(scope, key)
	if complete == nil {
		return func() {}
	}
	return complete
}

func (c *unchangedSyncCache) beginMutationChecked(scope cacheScope, key string) (func(), error) {
	if c == nil {
		return func() {}, nil
	}
	if c.redis != nil {
		complete, err := c.redis.mutation(scope, key)
		if err != nil {
			performance.RecordFastPathRedisError("mutation")
		}
		return complete, err
	}
	c.mu.Lock()
	scopeName := "global"
	if scope == cacheScopeFleet {
		scopeName = "fleet"
	}
	if scope == cacheScopeEndpoint {
		scopeName = "endpoint"
	}
	performance.RecordFastPathInvalidation(scopeName)
	switch scope {
	case cacheScopeGlobal:
		c.globalGeneration++
		c.globalUnstable++
		for endpointID := range c.entries {
			c.remove(endpointID)
		}
		c.pendingCheckpoints = make(map[string]syncCheckpoint)
		c.pendingObservations = 0
		c.pendingBytes = 0
	case cacheScopeFleet:
		c.fleetGenerations[key]++
		c.fleetUnstable[key]++
		for endpointID, entry := range c.entries {
			if entry.fleet == key {
				c.remove(endpointID)
			}
		}
		for endpointID, checkpoint := range c.pendingCheckpoints {
			if checkpoint.fleet == key {
				c.pendingObservations -= checkpoint.observations
				c.pendingBytes -= checkpoint.sizeBytes
				delete(c.pendingCheckpoints, endpointID)
			}
		}
	case cacheScopeEndpoint:
		c.endpointGenerations[key]++
		c.endpointUnstable[key]++
		c.remove(key)
		if checkpoint, ok := c.pendingCheckpoints[key]; ok {
			c.pendingObservations -= checkpoint.observations
			c.pendingBytes -= checkpoint.sizeBytes
			delete(c.pendingCheckpoints, key)
		}
	}
	c.mu.Unlock()

	var once sync.Once
	return func() {
		once.Do(func() {
			c.mu.Lock()
			defer c.mu.Unlock()
			switch scope {
			case cacheScopeGlobal:
				c.globalUnstable--
			case cacheScopeFleet:
				c.fleetUnstable[key]--
			case cacheScopeEndpoint:
				c.endpointUnstable[key]--
			}
		})
	}, nil
}

func cacheEntrySize(endpointID string, entry unchangedSyncEntry) int {
	size := len(endpointID) + len(entry.fingerprint) + len(entry.fleet) + len(entry.releaseRef) + len(entry.digest) + 64
	for name, hash := range entry.hashes {
		size += len(name) + len(hash)
	}
	if encoded, err := json.Marshal(entry.response); err == nil {
		size += len(encoded)
	}
	return size
}

func (c *unchangedSyncCache) remove(endpointID string) {
	entry, ok := c.entries[endpointID]
	if !ok {
		return
	}
	delete(c.entries, endpointID)
	c.usedBytes -= entry.sizeBytes
	c.observations -= entry.observations
}

func (c *unchangedSyncCache) evictLRU() bool {
	if len(c.entries) == 0 {
		return false
	}
	ids := make([]string, 0, len(c.entries))
	for id := range c.entries {
		ids = append(ids, id)
	}
	sort.Strings(ids)
	victim := ids[0]
	for _, id := range ids[1:] {
		if c.entries[id].lastUsed < c.entries[victim].lastUsed {
			victim = id
		}
	}
	c.remove(victim)
	performance.RecordFastPathEviction("lru")
	return true
}

func (c *unchangedSyncCache) bounds() (entries, bytes int, observations uint64) {
	if c == nil {
		return 0, 0, 0
	}
	if c.redis != nil {
		return 0, 0, 0
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	return len(c.entries) + len(c.pendingCheckpoints), c.usedBytes + c.pendingBytes, c.observations + c.pendingObservations
}

func deriveValidUntil(now time.Time, cacheTTL time.Duration) time.Time {
	return now.Add(cacheTTL)
}

func eligibleHashOnlyRequest(request syncRequest) bool {
	return request.documentHashes != nil && len(request.documentHashes.Documents) > 0 &&
		len(request.CapabilityDocument) == 0 && request.SystemInfo == nil && len(request.Labels) == 0 && len(request.Usernames) == 0 &&
		request.AgentUpgradeStatus == nil && request.Drift == nil && request.ApplyFailure == nil && len(request.CronResults) == 0 &&
		request.DiagnosticResult == nil && request.FirewallAudit == nil && len(request.ChangePreflights) == 0 &&
		request.RebootIntent == nil && request.NetworkIntent == nil
}

func quietCacheableResponse(response syncResponse) bool {
	return response.Unchanged && len(response.ArtifactYAML) == 0 && response.CapabilityBlocked == nil &&
		response.AgentUpgrade == nil && len(response.DueCrons) == 0 && response.DiagnosticCollection == nil &&
		len(response.ExecutionLeases) == 0 && response.RebootAcknowledged == "" && response.NetworkAcknowledged == "" &&
		len(response.RequestedDocuments) == 0
}

func equalDocumentHashes(left, right map[string]string) bool {
	if len(left) != len(right) {
		return false
	}
	for name, hash := range left {
		if !documenthash.Equal(hash, right[name]) {
			return false
		}
	}
	return true
}

func cloneHashes(input map[string]string) map[string]string {
	output := make(map[string]string, len(input))
	for name, hash := range input {
		output[name] = hash
	}
	return output
}

func cloneSyncResponse(input syncResponse) syncResponse {
	output := input
	output.ArtifactYAML = append([]byte(nil), input.ArtifactYAML...)
	output.DueCrons = append([]dueCronPayload(nil), input.DueCrons...)
	output.ExecutionLeases = append([]changecontrol.ExecutionLease(nil), input.ExecutionLeases...)
	output.RequestedDocuments = append([]string(nil), input.RequestedDocuments...)
	if input.AcceptedDocumentHashes != nil {
		output.AcceptedDocumentHashes = &documenthash.Summary{
			Version: input.AcceptedDocumentHashes.Version, Documents: cloneHashes(input.AcceptedDocumentHashes.Documents),
		}
	}
	return output
}
