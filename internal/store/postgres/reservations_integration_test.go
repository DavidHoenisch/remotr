//go:build postgres

package postgres

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"os"
	"sync"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/DavidHoenisch/remotr/internal/registry"
)

func reservationStore(t *testing.T) (*Store, func(string) string) {
	t.Helper()
	databaseURL := os.Getenv("REMOTR_TEST_DATABASE_URL")
	if databaseURL == "" {
		t.Fatal("REMOTR_TEST_DATABASE_URL is required for Postgres integration evidence")
	}
	pool, err := pgxpool.New(context.Background(), databaseURL)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(pool.Close)
	raw := make([]byte, 4)
	_, _ = rand.Read(raw)
	suffix := hex.EncodeToString(raw)
	return NewFromPool(pool), func(name string) string { return name + "-" + suffix }
}

func newToken(t *testing.T) string {
	t.Helper()
	raw := make([]byte, 24)
	if _, err := rand.Read(raw); err != nil {
		t.Fatal(err)
	}
	return hex.EncodeToString(raw)
}

// OS-ENR-001/003/005/006/007/011/012/013/014 at the persistence boundary.
func TestPostgresEndpointReservationLifecycle(t *testing.T) {
	ctx := context.Background()
	s, name := reservationStore(t)
	id, fleet, otherFleet := name("runner"), name("servers"), name("desktops")
	hour := time.Now().Add(time.Hour)

	first := newToken(t)
	if err := s.ReserveEndpoint(ctx, id, fleet, first, hour); err != nil {
		t.Fatal(err)
	}
	ep, err := s.endpointByID(ctx, id)
	if err != nil || !ep.Reserved() || ep.Fleet != fleet || ep.CertFingerprint != "" {
		t.Fatalf("reserved endpoint = %+v, %v", ep, err)
	}
	if bound, ok, err := s.EnrollmentTokenBinding(ctx, first); err != nil || !ok || bound != id {
		t.Fatalf("binding = %q %v %v", bound, ok, err)
	}

	// Cross-fleet conflict leaves the reservation and its token intact.
	if err := s.ReserveEndpoint(ctx, id, otherFleet, newToken(t), hour); !errors.Is(err, registry.ErrReservationFleetConflict) {
		t.Fatalf("cross-fleet err = %v", err)
	}

	// Re-issue revokes the previous token.
	second := newToken(t)
	if err := s.ReserveEndpoint(ctx, id, fleet, second, hour); err != nil {
		t.Fatal(err)
	}
	if _, err := s.ClaimReservedEndpoint(ctx, first, id, "fp-first"); !errors.Is(err, registry.ErrBoundTokenInvalid) {
		t.Fatalf("revoked token claim err = %v", err)
	}

	// Unbound redemption cannot consume a bound token, and unbound
	// registration cannot take a reserved ID.
	if _, err := s.ConsumeEnrollmentToken(ctx, second); err == nil {
		t.Fatal("unbound redemption consumed a bound token")
	}
	if _, err := s.RegisterEndpoint(ctx, id, fleet, "fp-intruder"); !errors.Is(err, registry.ErrEndpointExists) {
		t.Fatalf("unbound register err = %v", err)
	}

	gotFleet, err := s.ClaimReservedEndpoint(ctx, second, id, "fp-"+id)
	if err != nil || gotFleet != fleet {
		t.Fatalf("claim fleet = %q err = %v", gotFleet, err)
	}
	ep, err = s.endpointByFingerprint(ctx, "fp-"+id)
	if err != nil || ep.ID != id || ep.Reserved() {
		t.Fatalf("claimed endpoint = %+v, %v", ep, err)
	}
	if _, err := s.ClaimReservedEndpoint(ctx, second, id, "fp-reuse"); !errors.Is(err, registry.ErrBoundTokenInvalid) {
		t.Fatalf("reuse err = %v", err)
	}
	if err := s.ReserveEndpoint(ctx, id, fleet, newToken(t), hour); !errors.Is(err, registry.ErrEndpointEnrolled) {
		t.Fatalf("reserve enrolled err = %v", err)
	}
	if err := s.UnreserveEndpoint(ctx, id); !errors.Is(err, registry.ErrEndpointEnrolled) {
		t.Fatalf("unreserve enrolled err = %v", err)
	}
	if err := s.UnreserveEndpoint(ctx, name("never")); !errors.Is(err, registry.ErrReservationNotFound) {
		t.Fatalf("unreserve unknown err = %v", err)
	}
}

func TestPostgresEndpointReservationExpiryAndRelease(t *testing.T) {
	ctx := context.Background()
	s, name := reservationStore(t)
	id, fleet := name("expiring"), name("servers")
	expired := newToken(t)
	if err := s.ReserveEndpoint(ctx, id, fleet, expired, time.Now().Add(-time.Minute)); err != nil {
		t.Fatal(err)
	}
	if _, err := s.ClaimReservedEndpoint(ctx, expired, id, "fp-expired"); !errors.Is(err, registry.ErrBoundTokenInvalid) {
		t.Fatalf("expired claim err = %v", err)
	}
	if err := s.UnreserveEndpoint(ctx, id); err != nil {
		t.Fatal(err)
	}
	if _, ok, _ := s.EnrollmentTokenBinding(ctx, expired); ok {
		t.Fatal("bound token survived unreserve")
	}
	if _, err := s.endpointByID(ctx, id); err == nil {
		t.Fatal("reserved endpoint survived unreserve")
	}
}

// Task 3.4: concurrent redemptions of one bound token enroll exactly once.
func TestPostgresEndpointReservationConcurrentClaim(t *testing.T) {
	ctx := context.Background()
	s, name := reservationStore(t)
	id, fleet := name("race"), name("servers")
	token := newToken(t)
	if err := s.ReserveEndpoint(ctx, id, fleet, token, time.Now().Add(time.Hour)); err != nil {
		t.Fatal(err)
	}
	const racers = 8
	var wg sync.WaitGroup
	results := make(chan error, racers)
	start := make(chan struct{})
	for i := range racers {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			<-start
			_, err := s.ClaimReservedEndpoint(ctx, token, id, name("fp")+string(rune('a'+i)))
			results <- err
		}(i)
	}
	close(start)
	wg.Wait()
	close(results)
	wins := 0
	for err := range results {
		switch {
		case err == nil:
			wins++
		case errors.Is(err, registry.ErrBoundTokenInvalid):
		default:
			t.Fatalf("unexpected err = %v", err)
		}
	}
	if wins != 1 {
		t.Fatalf("successful claims = %d, want 1", wins)
	}
}

// OS-ENR-008 at the persistence boundary: overrides store for reserved
// endpoints before enrollment, and are selected after the claim.
// OS-ENR-009: unknown endpoint IDs fail with ErrEndpointNotRegistered.
func TestPostgresEndpointReservationOverrideArtifacts(t *testing.T) {
	ctx := context.Background()
	s, name := reservationStore(t)
	id, fleet, ref := name("override"), name("servers"), name("rel")
	token := newToken(t)
	if err := s.ReserveEndpoint(ctx, id, fleet, token, time.Now().Add(time.Hour)); err != nil {
		t.Fatal(err)
	}
	if err := s.StoreCompiledArtifactForEndpoint(ctx, id, ref, "desired", []byte("override"), "sha256:override"); err != nil {
		t.Fatalf("store override for reserved endpoint: %v", err)
	}
	err := s.StoreCompiledArtifactForEndpoint(ctx, name("unknown"), ref, "desired", []byte("x"), "sha256:x")
	if !errors.Is(err, ErrEndpointNotRegistered) {
		t.Fatalf("unknown endpoint err = %v", err)
	}
	if _, err := s.ClaimReservedEndpoint(ctx, token, id, "fp-"+id); err != nil {
		t.Fatal(err)
	}
	body, digest, err := s.GetCompiledArtifactForEndpoint(ctx, id, ref, "desired")
	if err != nil || string(body) != "override" || digest != "sha256:override" {
		t.Fatalf("override after claim = %q %q %v", body, digest, err)
	}
}

// Task 2.6: fleet agent upgrades count only enrolled endpoints.
func TestPostgresFleetUpgradeExcludesReservedEndpoints(t *testing.T) {
	ctx := context.Background()
	s, name := reservationStore(t)
	fleet := name("servers")
	if _, err := s.RegisterEndpoint(ctx, name("enrolled"), fleet, name("fp")); err != nil {
		t.Fatal(err)
	}
	if err := s.ReserveEndpoint(ctx, name("reserved"), fleet, newToken(t), time.Now().Add(time.Hour)); err != nil {
		t.Fatal(err)
	}
	n, err := s.RequestFleetAgentUpgrade(ctx, fleet, "0.6.41")
	if err != nil || n != 1 {
		t.Fatalf("upgraded = %d err = %v, want 1", n, err)
	}
}
