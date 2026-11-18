package postgres

import (
	"context"
	"errors"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"

	"github.com/DavidHoenisch/remotr/internal/registry"
	"github.com/DavidHoenisch/remotr/internal/store/postgres/db"
)

// withTx runs fn in one transaction when a pool is available, otherwise on
// the configured queries (test fakes).
func (s *Store) withTx(ctx context.Context, fn func(Querier) error) error {
	if s.pool == nil {
		return fn(s.q)
	}
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback(ctx) }()
	if err := fn(db.New(tx)); err != nil {
		return err
	}
	return tx.Commit(ctx)
}

func endpointIDText(id string) pgtype.Text { return pgtype.Text{String: id, Valid: id != ""} }

// ReserveEndpoint creates or re-issues a reservation and binds token to it.
func (s *Store) ReserveEndpoint(ctx context.Context, id, fleet, token string, expiresAt time.Time) error {
	parsedID, err := parseEndpointID(id)
	if err != nil {
		return err
	}
	if err := s.ensureFleet(ctx, fleet); err != nil {
		return err
	}
	return s.withTx(ctx, func(q Querier) error {
		if _, err := q.ReserveEndpoint(ctx, db.ReserveEndpointParams{ID: parsedID, Fleet: fleet}); err != nil {
			if !errors.Is(err, pgx.ErrNoRows) {
				return err
			}
			existing, getErr := q.GetEndpointByID(ctx, parsedID)
			if getErr != nil {
				return getErr
			}
			if existing.EnrollmentState != registry.EnrollmentStateReserved {
				return registry.ErrEndpointEnrolled
			}
			return registry.ErrReservationFleetConflict
		}
		if _, err := q.RevokeBoundEnrollmentTokens(ctx, endpointIDText(parsedID)); err != nil {
			return err
		}
		_, err := q.CreateBoundEnrollmentToken(ctx, db.CreateBoundEnrollmentTokenParams{
			Token:      token,
			Fleet:      fleet,
			ExpiresAt:  pgtype.Timestamptz{Time: expiresAt.UTC(), Valid: true},
			EndpointID: endpointIDText(parsedID),
		})
		return err
	})
}

// UnreserveEndpoint removes an unclaimed reservation; bound tokens cascade.
func (s *Store) UnreserveEndpoint(ctx context.Context, id string) error {
	parsedID, err := parseEndpointID(id)
	if err != nil {
		return err
	}
	n, err := s.q.DeleteReservedEndpoint(ctx, parsedID)
	if err != nil {
		return err
	}
	if n > 0 {
		return nil
	}
	if _, err := s.q.GetEndpointByID(ctx, parsedID); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return registry.ErrReservationNotFound
		}
		return err
	}
	return registry.ErrEndpointEnrolled
}

// EnrollmentTokenBinding reports the endpoint a token is bound to.
func (s *Store) EnrollmentTokenBinding(ctx context.Context, token string) (string, bool, error) {
	bound, err := s.q.GetEnrollmentTokenBinding(ctx, token)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return "", false, nil
		}
		return "", false, err
	}
	return bound.String, bound.Valid, nil
}

// ClaimReservedEndpoint consumes the bound token and enrolls the reserved
// endpoint in one transaction; a failed claim leaves the token unconsumed.
func (s *Store) ClaimReservedEndpoint(ctx context.Context, token, id, certFingerprint string) (string, error) {
	parsedID, err := parseEndpointID(id)
	if err != nil {
		return "", registry.ErrBoundTokenInvalid
	}
	var fleet string
	err = s.withTx(ctx, func(q Querier) error {
		tok, err := q.ConsumeBoundEnrollmentToken(ctx, db.ConsumeBoundEnrollmentTokenParams{Token: token, EndpointID: endpointIDText(parsedID)})
		if err != nil {
			if errors.Is(err, pgx.ErrNoRows) {
				return registry.ErrBoundTokenInvalid
			}
			return err
		}
		if _, err := q.ClaimReservedEndpoint(ctx, db.ClaimReservedEndpointParams{
			ID: parsedID, Fleet: tok.Fleet, CertFingerprint: textFingerprint(certFingerprint),
		}); err != nil {
			if errors.Is(err, pgx.ErrNoRows) {
				return registry.ErrBoundTokenInvalid
			}
			return err
		}
		fleet = tok.Fleet
		return nil
	})
	return fleet, err
}

var _ registry.EndpointReservations = (*RegistryAdmin)(nil)
var _ registry.EndpointReservations = (*RegistryEnroller)(nil)

func (r *RegistryAdmin) ReserveEndpoint(id, fleet, token string, expiresAt time.Time) error {
	return r.Store.ReserveEndpoint(context.Background(), id, fleet, token, expiresAt)
}

func (r *RegistryAdmin) UnreserveEndpoint(id string) error {
	return r.Store.UnreserveEndpoint(context.Background(), id)
}

func (r *RegistryAdmin) EnrollmentTokenBinding(token string) (string, bool) {
	id, ok, err := r.Store.EnrollmentTokenBinding(context.Background(), token)
	return id, ok && err == nil
}

func (r *RegistryAdmin) ClaimReservedEndpoint(token, id, certFingerprint string) (string, error) {
	return r.Store.ClaimReservedEndpoint(context.Background(), token, id, certFingerprint)
}

func (r *RegistryEnroller) ReserveEndpoint(id, fleet, token string, expiresAt time.Time) error {
	return r.Store.ReserveEndpoint(context.Background(), id, fleet, token, expiresAt)
}

func (r *RegistryEnroller) UnreserveEndpoint(id string) error {
	return r.Store.UnreserveEndpoint(context.Background(), id)
}

func (r *RegistryEnroller) EnrollmentTokenBinding(token string) (string, bool) {
	id, ok, err := r.Store.EnrollmentTokenBinding(context.Background(), token)
	return id, ok && err == nil
}

func (r *RegistryEnroller) ClaimReservedEndpoint(token, id, certFingerprint string) (string, error) {
	return r.Store.ClaimReservedEndpoint(context.Background(), token, id, certFingerprint)
}
