package server

import (
	"os"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/DavidHoenisch/remotr/internal/documenthash"
)

func redisIntegrationConfig(t *testing.T) FastPathConfig {
	t.Helper()
	redisURL := os.Getenv("REMOTR_TEST_REDIS_URL")
	if redisURL == "" {
		t.Skip("REMOTR_TEST_REDIS_URL is not set")
	}
	return FastPathConfig{Enabled: true, Backend: FastPathRedis, RedisURL: redisURL, RedisPrefix: "test-" + strconv.FormatInt(time.Now().UnixNano(), 36), MaxEntries: 8, MaxBytes: 1 << 20, TTL: time.Minute, CheckpointInterval: time.Minute, ServingProcesses: 2}
}

func redisDecisionFixture() (syncRequest, syncResponse) {
	hash := "sha256:" + strings.Repeat("0", 64)
	request := syncRequest{LastReleaseRef: "release", LastDigest: "digest", documentHashes: &documenthash.Summary{Version: documenthash.CurrentVersion, Documents: map[string]string{"capability": hash}}}
	response := syncResponse{Unchanged: true, ReleaseRef: "release", Digest: "digest", AcceptedDocumentHashes: &documenthash.Summary{Version: documenthash.CurrentVersion, Documents: cloneHashes(request.documentHashes.Documents)}}
	return request, response
}

func TestRedisFastPathSurvivesProcessReplacementAndCoordinatesMutation(t *testing.T) {
	config := redisIntegrationConfig(t)
	first := newUnchangedSyncCache(config)
	second := newUnchangedSyncCache(config)
	request, response := redisDecisionFixture()
	now := time.Unix(1_800_000_000, 0).UTC()
	snapshot := first.authoritySnapshot("endpoint-a", "fleet-a")
	if !snapshot.stable {
		t.Fatal("initial Redis authority was not stable")
	}
	first.putWithSnapshot("endpoint-a", "fleet-a", "certificate-a", request, response, now, snapshot)
	if _, hit := second.get("endpoint-a", "certificate-a", request, now.Add(time.Second)); !hit {
		t.Fatal("replacement process did not reuse shared decision")
	}
	complete, err := first.beginMutationChecked(cacheScopeFleet, "fleet-a")
	if err != nil {
		t.Fatal(err)
	}
	if _, hit := second.get("endpoint-a", "certificate-a", request, now.Add(2*time.Second)); hit {
		t.Fatal("shared decision hit while fleet mutation barrier was open")
	}
	if stable := second.authoritySnapshot("endpoint-a", "fleet-a").stable; stable {
		t.Fatal("shared authority reported stable during mutation")
	}
	complete()
	if _, hit := second.get("endpoint-a", "certificate-a", request, now.Add(3*time.Second)); hit {
		t.Fatal("superseded shared decision hit after mutation")
	}
}

func TestRedisFastPathCheckpointClaimSurvivesProcessReplacement(t *testing.T) {
	config := redisIntegrationConfig(t)
	config.CheckpointInterval = time.Second
	first := newUnchangedSyncCache(config)
	second := newUnchangedSyncCache(config)
	request, response := redisDecisionFixture()
	now := time.Unix(1_800_000_000, 0).UTC()
	snapshot := first.authoritySnapshot("endpoint-b", "fleet-a")
	first.putWithSnapshot("endpoint-b", "fleet-a", "certificate-b", request, response, now, snapshot)
	if _, hit, checkpoint := second.getWithCheckpoint("endpoint-b", "certificate-b", request, now.Add(time.Second)); hit || checkpoint == nil {
		t.Fatalf("hit=%t checkpoint=%v", hit, checkpoint)
	}
	if pending, ok := first.pendingCheckpoint("endpoint-b"); !ok || pending.observations != 1 {
		t.Fatalf("shared pending checkpoint = %+v, %t", pending, ok)
	}
	second.completeCheckpoint("endpoint-b")
	if _, ok := first.pendingCheckpoint("endpoint-b"); ok {
		t.Fatal("completed checkpoint remained claimable")
	}
}

func TestRedisFastPathAbandonedCheckpointClaimIsRetryable(t *testing.T) {
	config := redisIntegrationConfig(t)
	config.CheckpointInterval = time.Second
	first := newUnchangedSyncCache(config)
	second := newUnchangedSyncCache(config)
	request, response := redisDecisionFixture()
	now := time.Unix(1_800_000_000, 0).UTC()
	snapshot := first.authoritySnapshot("endpoint-retry", "fleet")
	first.putWithSnapshot("endpoint-retry", "fleet", "certificate", request, response, now, snapshot)
	if _, _, checkpoint := first.getWithCheckpoint("endpoint-retry", "certificate", request, now.Add(time.Second)); checkpoint == nil {
		t.Fatal("first checkpoint claim missing")
	}
	if _, err := first.redis.client.command("DEL", first.redis.key("checkpoint", redisHash("endpoint-retry"))); err != nil {
		t.Fatal(err)
	}
	if _, hit, checkpoint := second.getWithCheckpoint("endpoint-retry", "certificate", request, now.Add(2*time.Second)); hit || checkpoint == nil || checkpoint.observations != 2 {
		t.Fatalf("retry hit=%t checkpoint=%+v", hit, checkpoint)
	}
}

func TestRedisFastPathUnavailableFailsClosed(t *testing.T) {
	config := FastPathConfig{Enabled: true, Backend: FastPathRedis, RedisURL: "redis://:canary@127.0.0.1:1", RedisPrefix: "outage", ServingProcesses: 2}
	cache := newUnchangedSyncCache(config)
	request, _ := redisDecisionFixture()
	if _, hit := cache.get("endpoint", "certificate", request, time.Now()); hit {
		t.Fatal("unavailable Redis produced a hit")
	}
	if snapshot := cache.authoritySnapshot("endpoint", "fleet"); snapshot.stable {
		t.Fatal("unavailable Redis produced stable authority")
	}
	if _, err := cache.beginMutationChecked(cacheScopeGlobal, ""); err == nil {
		t.Fatal("unavailable Redis permitted mutation barrier")
	}
}

func TestRedisFastPathIdentityExpiryBoundsAndMalformedValues(t *testing.T) {
	config := redisIntegrationConfig(t)
	config.MaxEntries = 2
	config.TTL = 2 * time.Second
	cache := newUnchangedSyncCache(config)
	request, response := redisDecisionFixture()
	now := time.Unix(1_800_000_000, 0).UTC()
	put := func(id string, at time.Time) {
		snapshot := cache.authoritySnapshot(id, "fleet")
		cache.putWithSnapshot(id, "fleet", "certificate", request, response, at, snapshot)
	}
	put("one", now)
	if _, hit := cache.get("one", "wrong-certificate", request, now.Add(time.Second)); hit {
		t.Fatal("identity mismatch hit shared decision")
	}
	isolatedConfig := config
	isolatedConfig.RedisPrefix = config.RedisPrefix + "-other"
	isolated := newUnchangedSyncCache(isolatedConfig)
	if _, hit := isolated.get("one", "certificate", request, now.Add(time.Second)); hit {
		t.Fatal("namespace isolation failed")
	}
	if _, hit := cache.get("one", "certificate", request, now.Add(3*time.Second)); hit {
		t.Fatal("expired decision hit")
	}
	config.TTL = time.Minute
	cache = newUnchangedSyncCache(config)
	put = func(id string, at time.Time) {
		snapshot := cache.authoritySnapshot(id, "fleet")
		cache.putWithSnapshot(id, "fleet", "certificate", request, response, at, snapshot)
	}
	put("one", now)
	put("two", now.Add(time.Millisecond))
	put("three", now.Add(2*time.Millisecond))
	if _, hit := cache.get("one", "certificate", request, now.Add(time.Second)); hit {
		t.Fatal("oldest decision survived entry bound")
	}
	small := config
	small.RedisPrefix = config.RedisPrefix + "-small"
	small.MaxBytes = 32
	smallCache := newUnchangedSyncCache(small)
	snapshot := smallCache.authoritySnapshot("oversize", "fleet")
	smallCache.putWithSnapshot("oversize", "fleet", "certificate", request, response, now, snapshot)
	if _, hit := smallCache.get("oversize", "certificate", request, now.Add(time.Second)); hit {
		t.Fatal("oversized decision was admitted")
	}
	malformedKey := cache.redis.endpointKey("malformed")
	if _, err := cache.redis.client.command("SET", malformedKey, "{", "PX", "60000"); err != nil {
		t.Fatal(err)
	}
	if _, hit := cache.get("malformed", "certificate", request, now.Add(time.Second)); hit {
		t.Fatal("malformed Redis value produced hit")
	}
}
