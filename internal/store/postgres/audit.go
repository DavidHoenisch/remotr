package postgres

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"

	"github.com/DavidHoenisch/remotr/internal/audit"
	"github.com/DavidHoenisch/remotr/internal/store/postgres/db"
)

const auditExportPathKeySetting = "audit_export_path_key"

// RecordAuditEvent persists a durable audit record.
func (s *Store) RecordAuditEvent(ctx context.Context, event audit.Event) error {
	id := newUUID()
	if event.ID != "" {
		parsed, err := uuid.Parse(event.ID)
		if err == nil {
			id = pgtype.UUID{Bytes: parsed, Valid: true}
		}
	}

	occurredAt := pgtype.Timestamptz{Time: time.Now().UTC(), Valid: true}
	if !event.OccurredAt.IsZero() {
		occurredAt = pgtype.Timestamptz{Time: event.OccurredAt.UTC(), Valid: true}
	}

	var details []byte
	if len(event.Details) > 0 {
		encoded, err := json.Marshal(event.Details)
		if err != nil {
			return err
		}
		details = encoded
	}

	return s.q.InsertAuditEvent(ctx, db.InsertAuditEventParams{
		ID:               id,
		OccurredAt:       occurredAt,
		RequestID:        textOrNull(event.RequestID),
		ActorType:        event.ActorType,
		ActorID:          textOrNull(event.ActorID),
		ActorFingerprint: textOrNull(event.ActorFingerprint),
		Action:           event.Action,
		Method:           event.Method,
		Path:             event.Path,
		StatusCode:       int32(event.StatusCode),
		ResourceType:     textOrNull(event.ResourceType),
		ResourceID:       textOrNull(event.ResourceID),
		ClientIp:         textOrNull(event.ClientIP),
		Details:          details,
	})
}

// ListAuditEvents returns audit events matching the filter.
func (s *Store) ListAuditEvents(ctx context.Context, filter audit.ListFilter) (audit.Page, error) {
	limit := filter.Limit
	if limit <= 0 {
		limit = 100
	}
	if limit > 1000 {
		limit = 1000
	}

	params := db.ListAuditEventsParams{Limit: int32(limit)}
	if !filter.Since.IsZero() {
		params.Since = pgtype.Timestamptz{Time: filter.Since.UTC(), Valid: true}
	}
	if !filter.Until.IsZero() {
		params.Until = pgtype.Timestamptz{Time: filter.Until.UTC(), Valid: true}
	}
	if filter.Action != "" {
		params.Action = pgtype.Text{String: filter.Action, Valid: true}
	}
	if filter.ActorType != "" {
		params.ActorType = pgtype.Text{String: filter.ActorType, Valid: true}
	}
	if filter.Cursor != "" {
		cursorAt, cursorID, err := audit.DecodeCursor(filter.Cursor)
		if err != nil {
			return audit.Page{}, err
		}
		parsed, err := uuid.Parse(cursorID)
		if err != nil {
			return audit.Page{}, fmt.Errorf("invalid cursor")
		}
		params.CursorAt = pgtype.Timestamptz{Time: cursorAt.UTC(), Valid: true}
		params.CursorID = pgtype.UUID{Bytes: parsed, Valid: true}
	}

	rows, err := s.q.ListAuditEvents(ctx, params)
	if err != nil {
		return audit.Page{}, err
	}

	events := make([]audit.Event, 0, len(rows))
	for _, row := range rows {
		events = append(events, auditEventFromRow(row))
	}

	page := audit.Page{Events: events}
	if len(events) == int(limit) {
		last := events[len(events)-1]
		cursor, err := audit.EncodeCursor(last.OccurredAt, last.ID)
		if err != nil {
			return audit.Page{}, err
		}
		page.NextCursor = cursor
	}
	return page, nil
}

// EnsureAuditExportPathKey returns the stable secret path segment for audit export.
func (s *Store) EnsureAuditExportPathKey(ctx context.Context) (string, error) {
	key, err := s.q.GetServerSetting(ctx, auditExportPathKeySetting)
	if err == nil && key != "" {
		return key, nil
	}
	if err != nil && !errors.Is(err, pgx.ErrNoRows) {
		return "", err
	}

	raw := make([]byte, 32)
	if _, err := rand.Read(raw); err != nil {
		return "", err
	}
	key = hex.EncodeToString(raw)
	if err := s.q.UpsertServerSetting(ctx, db.UpsertServerSettingParams{
		Key:   auditExportPathKeySetting,
		Value: key,
	}); err != nil {
		return "", err
	}
	return key, nil
}

func auditEventFromRow(row db.AuditEvent) audit.Event {
	id, _ := uuidString(row.ID)
	event := audit.Event{
		ID:         id,
		OccurredAt: row.OccurredAt.Time.UTC(),
		ActorType:  row.ActorType,
		Action:     row.Action,
		Method:     row.Method,
		Path:       row.Path,
		StatusCode: int(row.StatusCode),
	}
	if row.RequestID.Valid {
		event.RequestID = row.RequestID.String
	}
	if row.ActorID.Valid {
		event.ActorID = row.ActorID.String
	}
	if row.ActorFingerprint.Valid {
		event.ActorFingerprint = row.ActorFingerprint.String
	}
	if row.ResourceType.Valid {
		event.ResourceType = row.ResourceType.String
	}
	if row.ResourceID.Valid {
		event.ResourceID = row.ResourceID.String
	}
	if row.ClientIp.Valid {
		event.ClientIP = row.ClientIp.String
	}
	if len(row.Details) > 0 {
		var details map[string]any
		if err := json.Unmarshal(row.Details, &details); err == nil {
			event.Details = details
		}
	}
	return event
}

func textOrNull(v string) pgtype.Text {
	if v == "" {
		return pgtype.Text{}
	}
	return pgtype.Text{String: v, Valid: true}
}
