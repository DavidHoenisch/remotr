package admin

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"
)

// EndpointReservation is a reserved endpoint ID with its single-use bound token.
type EndpointReservation struct {
	EndpointID string    `json:"endpoint_id"`
	Fleet      string    `json:"fleet"`
	Token      string    `json:"token"`
	ExpiresAt  time.Time `json:"expires_at"`
}

// EndpointUnreserveResult reports a released reservation and any forced-release warning.
type EndpointUnreserveResult struct {
	EndpointID string `json:"endpoint_id"`
	Forced     bool   `json:"forced,omitempty"`
	Warning    string `json:"warning,omitempty"`
}

// ErrReservationConflict carries the server's explanation for a refused
// reservation or release so operators see why it failed.
var ErrReservationConflict = errors.New("endpoint reservation conflict")

func reservationError(operation string, status int, body []byte) error {
	msg := strings.TrimSpace(string(body))
	switch status {
	case http.StatusConflict, http.StatusNotFound, http.StatusBadRequest:
		return fmt.Errorf("%w: %s", ErrReservationConflict, msg)
	}
	return &ResponseError{Operation: operation, StatusCode: status, Body: body}
}

// ReserveEndpoint reserves endpointID in fleet and returns its bound enrollment token.
func (c *Client) ReserveEndpoint(ctx context.Context, fleet, endpointID string, ttl time.Duration) (EndpointReservation, error) {
	body, err := json.Marshal(map[string]any{"fleet": fleet, "endpoint_id": endpointID, "ttl_seconds": int64(ttl.Seconds())})
	if err != nil {
		return EndpointReservation{}, err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.BaseURL+"/v1/admin/endpoint-reservations", bytes.NewReader(body))
	if err != nil {
		return EndpointReservation{}, err
	}
	req.Header.Set("Content-Type", "application/json")
	resp, err := c.HTTPClient.Do(req)
	if err != nil {
		return EndpointReservation{}, err
	}
	defer resp.Body.Close()
	raw, err := io.ReadAll(resp.Body)
	if err != nil {
		return EndpointReservation{}, err
	}
	if resp.StatusCode != http.StatusOK {
		return EndpointReservation{}, reservationError("reserve endpoint", resp.StatusCode, raw)
	}
	var out EndpointReservation
	if err := json.Unmarshal(raw, &out); err != nil {
		return EndpointReservation{}, fmt.Errorf("decode reservation response: %w", err)
	}
	if out.Token == "" || out.EndpointID == "" {
		return EndpointReservation{}, fmt.Errorf("incomplete reservation response")
	}
	return out, nil
}

// UnreserveEndpoint releases an unclaimed reservation. Without force the
// server refuses when the current release already contains its override.
func (c *Client) UnreserveEndpoint(ctx context.Context, endpointID string, force bool) (EndpointUnreserveResult, error) {
	target := c.BaseURL + "/v1/admin/endpoint-reservations/" + url.PathEscape(endpointID)
	if force {
		target += "?force=true"
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodDelete, target, nil)
	if err != nil {
		return EndpointUnreserveResult{}, err
	}
	resp, err := c.HTTPClient.Do(req)
	if err != nil {
		return EndpointUnreserveResult{}, err
	}
	defer resp.Body.Close()
	raw, err := io.ReadAll(resp.Body)
	if err != nil {
		return EndpointUnreserveResult{}, err
	}
	if resp.StatusCode != http.StatusOK {
		return EndpointUnreserveResult{}, reservationError("unreserve endpoint", resp.StatusCode, raw)
	}
	var out EndpointUnreserveResult
	if err := json.Unmarshal(raw, &out); err != nil {
		return EndpointUnreserveResult{}, fmt.Errorf("decode unreserve response: %w", err)
	}
	return out, nil
}
