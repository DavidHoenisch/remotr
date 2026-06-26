package postgres

import (
	"context"
	"encoding/json"
	"errors"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"

	"github.com/DavidHoenisch/remotr/internal/diagnostics"
	"github.com/DavidHoenisch/remotr/internal/registry"
	"github.com/DavidHoenisch/remotr/internal/store/postgres/db"
)

func (s *Store) CreateDiagnosticRequest(ctx context.Context, endpointID, requestedBy string, spec diagnostics.Spec) (diagnostics.Request, error) {
	endpointID, err := parseEndpointID(endpointID)
	if err != nil {
		return diagnostics.Request{}, err
	}
	if _, err := s.q.GetEndpointByID(ctx, endpointID); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return diagnostics.Request{}, registry.ErrEndpointNotFound
		}
		return diagnostics.Request{}, err
	}
	if _, err := s.q.GetActiveDiagnosticRequestForEndpoint(ctx, endpointID); err == nil {
		return diagnostics.Request{}, diagnostics.ErrActiveRequest
	} else if !errors.Is(err, pgx.ErrNoRows) {
		return diagnostics.Request{}, err
	}

	id := newUUID()
	requestID := uuid.UUID(id.Bytes).String()
	specJSON, err := json.Marshal(spec)
	if err != nil {
		return diagnostics.Request{}, err
	}
	expiresAt := time.Now().UTC().Add(diagnostics.BundleTTL)
	row, err := s.q.InsertDiagnosticRequest(ctx, db.InsertDiagnosticRequestParams{
		ID:          id,
		EndpointID:  endpointID,
		RequestedBy: requestedBy,
		SpecJson:    specJSON,
		S3Key:       diagnostics.S3Key(endpointID, requestID),
		ExpiresAt:   pgtype.Timestamptz{Time: expiresAt, Valid: true},
	})
	if err != nil {
		return diagnostics.Request{}, err
	}
	return diagnosticRequestFromRow(row)
}

func (s *Store) GetDiagnosticRequest(ctx context.Context, requestID string) (diagnostics.Request, bool, error) {
	id, err := parseRequestUUID(requestID)
	if err != nil {
		return diagnostics.Request{}, false, err
	}
	row, err := s.q.GetDiagnosticRequest(ctx, id)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return diagnostics.Request{}, false, nil
		}
		return diagnostics.Request{}, false, err
	}
	req, err := diagnosticRequestFromRow(row)
	if err != nil {
		return diagnostics.Request{}, false, err
	}
	return req, true, nil
}

func (s *Store) PendingDiagnosticForEndpoint(ctx context.Context, endpointID string) (diagnostics.Request, bool, error) {
	endpointID, err := parseEndpointID(endpointID)
	if err != nil {
		return diagnostics.Request{}, false, err
	}
	row, err := s.q.GetActiveDiagnosticRequestForEndpoint(ctx, endpointID)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return diagnostics.Request{}, false, nil
		}
		return diagnostics.Request{}, false, err
	}
	req, err := diagnosticRequestFromRow(row)
	if err != nil {
		return diagnostics.Request{}, false, err
	}
	return req, true, nil
}

func (s *Store) MarkDiagnosticDispatched(ctx context.Context, requestID string) error {
	id, err := parseRequestUUID(requestID)
	if err != nil {
		return err
	}
	_, err = s.q.MarkDiagnosticRequestDispatched(ctx, id)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil
	}
	return err
}

func (s *Store) MarkDiagnosticRunning(ctx context.Context, requestID string) error {
	id, err := parseRequestUUID(requestID)
	if err != nil {
		return err
	}
	_, err = s.q.MarkDiagnosticRequestRunning(ctx, id)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil
	}
	return err
}

func (s *Store) CompleteDiagnosticRequest(ctx context.Context, result diagnostics.ResultPayload) error {
	id, err := parseRequestUUID(result.RequestID)
	if err != nil {
		return err
	}
	status := result.Status
	if status != diagnostics.StatusReady && status != diagnostics.StatusFailed {
		status = diagnostics.StatusFailed
	}
	_, err = s.q.CompleteDiagnosticRequest(ctx, db.CompleteDiagnosticRequestParams{
		ID:           id,
		Status:       status,
		Sha256:       result.SHA256,
		SizeBytes:    result.SizeBytes,
		ErrorMessage: result.Message,
	})
	return err
}

func (s *Store) ExpireDiagnosticRequests(ctx context.Context) error {
	return s.q.ExpireDiagnosticRequests(ctx)
}

func (s *Store) DeleteExpiredDiagnosticRequests(ctx context.Context) ([]diagnostics.Request, error) {
	rows, err := s.q.DeleteExpiredDiagnosticRequests(ctx)
	if err != nil {
		return nil, err
	}
	out := make([]diagnostics.Request, 0, len(rows))
	for _, row := range rows {
		req, err := diagnosticRequestFromRow(row)
		if err != nil {
			return nil, err
		}
		out = append(out, req)
	}
	return out, nil
}

func diagnosticRequestFromRow(row db.DiagnosticRequest) (diagnostics.Request, error) {
	var spec diagnostics.Spec
	if err := json.Unmarshal(row.SpecJson, &spec); err != nil {
		return diagnostics.Request{}, err
	}
	id := ""
	if row.ID.Valid {
		id = uuid.UUID(row.ID.Bytes).String()
	}
	req := diagnostics.Request{
		ID:           id,
		EndpointID:   row.EndpointID,
		RequestedBy:  row.RequestedBy,
		Status:       row.Status,
		Spec:         spec,
		S3Key:        row.S3Key,
		SHA256:       row.Sha256,
		SizeBytes:    row.SizeBytes,
		ErrorMessage: row.ErrorMessage,
	}
	if row.CreatedAt.Valid {
		req.CreatedAt = row.CreatedAt.Time
	}
	if row.DispatchedAt.Valid {
		t := row.DispatchedAt.Time
		req.DispatchedAt = &t
	}
	if row.CompletedAt.Valid {
		t := row.CompletedAt.Time
		req.CompletedAt = &t
	}
	if row.ExpiresAt.Valid {
		req.ExpiresAt = row.ExpiresAt.Time
	}
	return req, nil
}

func parseRequestUUID(requestID string) (pgtype.UUID, error) {
	parsed, err := uuid.Parse(requestID)
	if err != nil {
		return pgtype.UUID{}, err
	}
	return pgtype.UUID{Bytes: parsed, Valid: true}, nil
}
