package sync

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"
)

// DueQueryPayload is a scheduled or ad-hoc osquery query dispatched on Sync.
type DueQueryPayload struct {
	RunID        string    `json:"runId"`
	RequestID    string    `json:"requestId,omitempty"`
	TriggerType  string    `json:"triggerType"`
	Name         string    `json:"name"`
	SQL          string    `json:"sql"`
	TimeoutSec   int       `json:"timeoutSec"`
	MaxRows      int       `json:"maxRows"`
	ScheduledFor time.Time `json:"scheduledFor,omitempty"`
}

// QueryResultPayload is an endpoint's bounded result for one dispatched query.
type QueryResultPayload struct {
	RunID        string              `json:"runId"`
	RequestID    string              `json:"requestId,omitempty"`
	TriggerType  string              `json:"triggerType,omitempty"`
	Name         string              `json:"name"`
	Status       string              `json:"status"`
	Rows         []map[string]string `json:"rows,omitempty"`
	RowCount     int                 `json:"rowCount"`
	Truncated    bool                `json:"truncated,omitempty"`
	DurationMS   int64               `json:"durationMs"`
	ErrorSummary string              `json:"errorSummary,omitempty"`
	ScheduledFor *time.Time          `json:"scheduledFor,omitempty"`
	CompletedAt  time.Time           `json:"completedAt,omitempty"`
}

// PostQueryResults uploads executed query results to POST /v1/sync/query-results.
func (c *Client) PostQueryResults(ctx context.Context, results []QueryResultPayload) error {
	if len(results) == 0 {
		return nil
	}
	raw, err := json.Marshal(map[string]any{"results": results})
	if err != nil {
		return err
	}
	endpoint := strings.TrimRight(c.BaseURL, "/") + "/v1/sync/query-results"
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, bytes.NewReader(raw))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")
	httpClient := c.HTTPClient
	if httpClient == nil {
		httpClient = http.DefaultClient
	}
	resp, err := httpClient.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		body, _ := io.ReadAll(io.LimitReader(resp.Body, 4096))
		return fmt.Errorf("post query results: HTTP %d: %s", resp.StatusCode, strings.TrimSpace(string(body)))
	}
	return nil
}
