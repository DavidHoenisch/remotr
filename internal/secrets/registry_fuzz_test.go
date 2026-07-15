package secrets

import (
	"bytes"
	"context"
	"encoding/json"
	"strings"
	"testing"
	"time"
)

func FuzzRollbackReferenceMetadataIsBoundedAndSecretFree(f *testing.F) {
	f.Add("base/service", "sha256:artifact", int32(1), int32(60))
	f.Add("missing-slash", "sha256:artifact", int32(1), int32(60))
	f.Add("base/service", "", int32(0), int32(0))

	f.Fuzz(func(t *testing.T, resourceAddress, artifactDigest string, attempt, expiryMinutes int32) {
		if len(resourceAddress)+len(artifactDigest) > 512 {
			return
		}
		now := time.Date(2026, 7, 15, 12, 0, 0, 0, time.UTC)
		service := newTestRegistryService(t, nil, nil)
		service.now = func() time.Time { return now }
		service.random = bytes.NewReader(make([]byte, 16))
		service.envelope.random = bytes.NewReader(make([]byte, 64))
		const canary = "rollback-secret-canary"
		if _, err := service.Upload(context.Background(), UploadRequest{
			Name: "service/token", Fleet: "engineering", Material: []byte(canary), ActorID: "operator",
		}); err != nil {
			t.Fatal(err)
		}

		expiresAt := now.Add(time.Duration(expiryMinutes) * time.Minute)
		metadata, err := service.RetainRollbackReference(context.Background(), RollbackReferenceRequest{
			Name: "service/token", Version: "1", ResourceAddress: resourceAddress,
			ArtifactDigest: artifactDigest, Attempt: int(attempt), ExpiresAt: expiresAt,
		})
		valid := strings.TrimSpace(resourceAddress) != "" && resourceAddress == strings.TrimSpace(resourceAddress) && strings.Contains(resourceAddress, "/") &&
			strings.TrimSpace(artifactDigest) != "" && artifactDigest == strings.TrimSpace(artifactDigest) && len(artifactDigest) <= 256 &&
			attempt > 0 && expiresAt.After(now) && !expiresAt.After(now.Add(MaxOfflineRecoveryAge))
		if !valid {
			if err == nil {
				t.Fatalf("invalid rollback metadata was accepted: %+v", metadata)
			}
			return
		}
		if err != nil {
			t.Fatal(err)
		}
		if metadata.Reference != "remotr:service/token@1" || metadata.ResourceAddress != resourceAddress || metadata.ArtifactDigest != artifactDigest || metadata.Attempt != int(attempt) || metadata.Status != RollbackReferenceArmed {
			t.Fatalf("rollback metadata changed: %+v", metadata)
		}
		raw, err := json.Marshal(metadata)
		if err != nil {
			t.Fatal(err)
		}
		if len(raw) > 4096 || bytes.Contains(raw, []byte(canary)) {
			t.Fatalf("rollback metadata is unbounded or leaked material: %d bytes", len(raw))
		}
	})
}
