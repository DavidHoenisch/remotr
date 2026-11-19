package admin

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"time"
)

type QuerySummary struct {
	Fleet       string `json:"fleet"`
	Name        string `json:"name"`
	Description string `json:"description,omitempty"`
	Schedule    string `json:"schedule"`
	Mode        string `json:"mode"`
	MaxRows     int    `json:"max_rows"`
	TotalHosts  int    `json:"total_hosts"`
	Reported    int    `json:"reported"`
	Failed      int    `json:"failed"`
	Unsupported int    `json:"unsupported"`
}

type QueryExecutionStatus struct {
	RunID        string     `json:"run_id"`
	EndpointID   string     `json:"endpoint_id"`
	Fleet        string     `json:"fleet"`
	QueryName    string     `json:"query_name"`
	TriggerType  string     `json:"trigger_type"`
	Status       string     `json:"status"`
	RowCount     int        `json:"row_count"`
	Truncated    bool       `json:"truncated"`
	DurationMS   int64      `json:"duration_ms"`
	ErrorSummary string     `json:"error_summary,omitempty"`
	ScheduledFor *time.Time `json:"scheduled_for,omitempty"`
	StartedAt    time.Time  `json:"started_at"`
	CompletedAt  *time.Time `json:"completed_at,omitempty"`
}

type QuerySnapshot struct {
	EndpointID string              `json:"endpoint_id"`
	QueryName  string              `json:"query_name"`
	Fleet      string              `json:"fleet"`
	RunID      string              `json:"run_id"`
	Rows       []map[string]string `json:"rows"`
	RowCount   int                 `json:"row_count"`
	Truncated  bool                `json:"truncated"`
	UpdatedAt  time.Time           `json:"updated_at"`
}

type QueryResultsResponse struct {
	QueryName string                 `json:"query_name"`
	Fleet     string                 `json:"fleet"`
	Endpoint  string                 `json:"endpoint"`
	Snapshots []QuerySnapshot        `json:"snapshots"`
	Statuses  []QueryExecutionStatus `json:"statuses"`
}

type QueryDiff struct {
	RunID        string              `json:"run_id"`
	EndpointID   string              `json:"endpoint_id"`
	Fleet        string              `json:"fleet"`
	QueryName    string              `json:"query_name"`
	AddedRows    []map[string]string `json:"added_rows"`
	RemovedRows  []map[string]string `json:"removed_rows"`
	AddedCount   int                 `json:"added_count"`
	RemovedCount int                 `json:"removed_count"`
	CreatedAt    time.Time           `json:"created_at"`
}

type AdhocQueryRequest struct {
	RequestID  string    `json:"request_id"`
	Actor      string    `json:"actor"`
	Fleet      string    `json:"fleet,omitempty"`
	EndpointID string    `json:"endpoint_id,omitempty"`
	QueryName  string    `json:"query_name,omitempty"`
	SQL        string    `json:"sql"`
	IsInline   bool      `json:"is_inline"`
	TimeoutSec int       `json:"timeout_sec"`
	MaxRows    int       `json:"max_rows"`
	CreatedAt  time.Time `json:"created_at"`
	ExpiresAt  time.Time `json:"expires_at"`
}

type AdhocQueryResult struct {
	RequestID    string              `json:"request_id"`
	EndpointID   string              `json:"endpoint_id"`
	Status       string              `json:"status"`
	Rows         []map[string]string `json:"rows"`
	RowCount     int                 `json:"row_count"`
	Truncated    bool                `json:"truncated"`
	DurationMS   int64               `json:"duration_ms"`
	ErrorSummary string              `json:"error_summary,omitempty"`
	CompletedAt  time.Time           `json:"completed_at"`
}

type AdhocQueryRunResponse struct {
	Request         AdhocQueryRequest  `json:"request"`
	ExpectedTargets int                `json:"expected_targets"`
	Results         []AdhocQueryResult `json:"results"`
}

type RunQueryOptions struct {
	QueryName  string
	SQL        string
	Fleet      string
	EndpointID string
	TimeoutSec int
	MaxRows    int
	TTLSec     int
}

func (c *Client) ListQueries(ctx context.Context, fleet string) ([]QuerySummary, error) {
	u := c.BaseURL + "/v1/admin/queries"
	if fleet != "" {
		u += "?fleet=" + url.QueryEscape(fleet)
	}
	var out struct {
		Queries []QuerySummary `json:"queries"`
	}
	if err := c.doJSON(ctx, http.MethodGet, u, nil, &out, "list queries"); err != nil {
		return nil, err
	}
	return out.Queries, nil
}

func (c *Client) GetQueryStatus(ctx context.Context, name, fleet string) ([]QueryExecutionStatus, error) {
	u := c.BaseURL + "/v1/admin/queries/" + url.PathEscape(name) + "/status"
	if fleet != "" {
		u += "?fleet=" + url.QueryEscape(fleet)
	}
	var out struct {
		Statuses []QueryExecutionStatus `json:"statuses"`
	}
	if err := c.doJSON(ctx, http.MethodGet, u, nil, &out, "get query status"); err != nil {
		return nil, err
	}
	return out.Statuses, nil
}

func (c *Client) GetQueryResults(ctx context.Context, name, fleet, endpointID string) (QueryResultsResponse, error) {
	params := url.Values{}
	if fleet != "" {
		params.Set("fleet", fleet)
	}
	if endpointID != "" {
		params.Set("endpoint", endpointID)
	}
	u := c.BaseURL + "/v1/admin/queries/" + url.PathEscape(name) + "/results"
	if encoded := params.Encode(); encoded != "" {
		u += "?" + encoded
	}
	var out QueryResultsResponse
	if err := c.doJSON(ctx, http.MethodGet, u, nil, &out, "get query results"); err != nil {
		return QueryResultsResponse{}, err
	}
	return out, nil
}

func (c *Client) GetQueryDiff(ctx context.Context, name, fleet, endpointID string, since time.Time, limit int) ([]QueryDiff, error) {
	params := url.Values{}
	if fleet != "" {
		params.Set("fleet", fleet)
	}
	if endpointID != "" {
		params.Set("endpoint", endpointID)
	}
	if !since.IsZero() {
		params.Set("since", since.UTC().Format(time.RFC3339))
	}
	if limit > 0 {
		params.Set("limit", strconv.Itoa(limit))
	}
	u := c.BaseURL + "/v1/admin/queries/" + url.PathEscape(name) + "/diff"
	if encoded := params.Encode(); encoded != "" {
		u += "?" + encoded
	}
	var out struct {
		Diffs []QueryDiff `json:"diffs"`
	}
	if err := c.doJSON(ctx, http.MethodGet, u, nil, &out, "get query diff"); err != nil {
		return nil, err
	}
	return out.Diffs, nil
}

func (c *Client) RunAdhocQuery(ctx context.Context, opts RunQueryOptions) (AdhocQueryRunResponse, error) {
	path := "/v1/admin/queries/run"
	if opts.SQL != "" {
		path = "/v1/admin/queries/run-sql"
	}
	payload := map[string]any{
		"query_name":  opts.QueryName,
		"sql":         opts.SQL,
		"fleet":       opts.Fleet,
		"endpoint_id": opts.EndpointID,
		"timeout_sec": opts.TimeoutSec,
		"max_rows":    opts.MaxRows,
		"ttl_sec":     opts.TTLSec,
	}
	var out AdhocQueryRunResponse
	if err := c.doJSON(ctx, http.MethodPost, c.BaseURL+path, payload, &out, "run query"); err != nil {
		return AdhocQueryRunResponse{}, err
	}
	return out, nil
}

func (c *Client) GetAdhocQueryRun(ctx context.Context, requestID string) (AdhocQueryRunResponse, error) {
	u := c.BaseURL + "/v1/admin/queries/runs/" + url.PathEscape(requestID)
	var out AdhocQueryRunResponse
	if err := c.doJSON(ctx, http.MethodGet, u, nil, &out, "get adhoc query run"); err != nil {
		return AdhocQueryRunResponse{}, err
	}
	return out, nil
}

func (c *Client) doJSON(ctx context.Context, method, rawURL string, body any, dst any, op string) error {
	var rdr io.Reader
	if body != nil {
		raw, err := json.Marshal(body)
		if err != nil {
			return err
		}
		rdr = bytes.NewReader(raw)
	}
	req, err := http.NewRequestWithContext(ctx, method, rawURL, rdr)
	if err != nil {
		return err
	}
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	resp, err := c.HTTPClient.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	respBody, err := io.ReadAll(io.LimitReader(resp.Body, 16<<20))
	if err != nil {
		return err
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return &ResponseError{Operation: op, StatusCode: resp.StatusCode, Body: respBody}
	}
	if dst != nil && len(bytes.TrimSpace(respBody)) > 0 {
		if err := json.Unmarshal(respBody, dst); err != nil {
			return fmt.Errorf("%s: decode response: %w", op, err)
		}
	}
	return nil
}
