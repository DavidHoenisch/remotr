package audit

import (
	"encoding/base64"
	"encoding/json"
	"fmt"
	"time"
)

type cursorPayload struct {
	OccurredAt time.Time `json:"t"`
	ID         string    `json:"id"`
}

// EncodeCursor returns an opaque pagination cursor for audit event listing.
func EncodeCursor(occurredAt time.Time, id string) (string, error) {
	if occurredAt.IsZero() || id == "" {
		return "", nil
	}
	raw, err := json.Marshal(cursorPayload{OccurredAt: occurredAt.UTC(), ID: id})
	if err != nil {
		return "", err
	}
	return base64.RawURLEncoding.EncodeToString(raw), nil
}

// DecodeCursor parses an opaque pagination cursor.
func DecodeCursor(cursor string) (time.Time, string, error) {
	if cursor == "" {
		return time.Time{}, "", nil
	}
	raw, err := base64.RawURLEncoding.DecodeString(cursor)
	if err != nil {
		return time.Time{}, "", fmt.Errorf("invalid cursor")
	}
	var payload cursorPayload
	if err := json.Unmarshal(raw, &payload); err != nil {
		return time.Time{}, "", fmt.Errorf("invalid cursor")
	}
	if payload.OccurredAt.IsZero() || payload.ID == "" {
		return time.Time{}, "", fmt.Errorf("invalid cursor")
	}
	return payload.OccurredAt.UTC(), payload.ID, nil
}
