package sync

import (
	"bytes"
	"compress/gzip"
	"crypto/tls"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strconv"
	"strings"
	"time"
)

// DefaultHTTPTimeout is the maximum time allowed for a single POST /v1/sync.
const DefaultHTTPTimeout = 120 * time.Second

type Client struct {
	BaseURL    string
	HTTPClient *http.Client
}

type AgentUpgradeInstruction struct {
	Version    string `json:"version"`
	GitHubRepo string `json:"githubRepo,omitempty"`
}

type Response struct {
	Unchanged            bool                         `json:"unchanged"`
	ReleaseRef           string                       `json:"releaseRef,omitempty"`
	Digest               string                       `json:"digest,omitempty"`
	ArtifactYAML         []byte                       `json:"artifactYaml,omitempty"`
	RemediationPolicy    string                       `json:"remediationPolicy,omitempty"`
	AgentUpgrade         *AgentUpgradeInstruction     `json:"agentUpgrade,omitempty"`
	DueCrons             []DueCronPayload             `json:"dueCrons,omitempty"`
	CronsDigest          string                       `json:"cronsDigest,omitempty"`
	DiagnosticCollection *DiagnosticCollectionPayload `json:"diagnosticCollection,omitempty"`
}

// HTTPStatusError preserves a Sync HTTP failure for retry classification.
type HTTPStatusError struct {
	StatusCode int
	Body       string
	RetryAfter time.Duration
}

func (e *HTTPStatusError) Error() string {
	return fmt.Sprintf("sync status %d: %s", e.StatusCode, strings.TrimSpace(e.Body))
}

// IsPermanent reports statuses that require credential, enrollment, or
// authored-request correction rather than transient retry pressure.
func IsPermanent(err error) bool {
	var status *HTTPStatusError
	if !errors.As(err, &status) {
		return false
	}
	switch status.StatusCode {
	case http.StatusBadRequest, http.StatusUnauthorized, http.StatusForbidden, http.StatusNotFound, http.StatusUnprocessableEntity:
		return true
	default:
		return false
	}
}

// IsOverloaded reports an authenticated server overload response.
func IsOverloaded(err error) bool {
	var status *HTTPStatusError
	return errors.As(err, &status) && (status.StatusCode == http.StatusTooManyRequests || status.StatusCode == http.StatusServiceUnavailable)
}

// RetryAfter returns a positive server-provided overload delay.
func RetryAfter(err error) (time.Duration, bool) {
	var status *HTTPStatusError
	if !errors.As(err, &status) || status.RetryAfter <= 0 {
		return 0, false
	}
	return status.RetryAfter, true
}

func NewClient(baseURL string, tlsCfg *tls.Config) *Client {
	return NewClientWithTimeout(baseURL, tlsCfg, DefaultHTTPTimeout)
}

// NewClientWithTimeout builds a sync client with the given per-request timeout.
func NewClientWithTimeout(baseURL string, tlsCfg *tls.Config, timeout time.Duration) *Client {
	if timeout <= 0 {
		timeout = DefaultHTTPTimeout
	}
	return &Client{
		BaseURL: baseURL,
		HTTPClient: &http.Client{
			Transport: &http.Transport{TLSClientConfig: tlsCfg},
			Timeout:   timeout,
		},
	}
}

func (c *Client) Sync(req Request) (Response, error) {
	body, err := json.Marshal(req)
	if err != nil {
		return Response{}, err
	}

	httpReq, err := http.NewRequest(http.MethodPost, c.BaseURL+"/v1/sync", bytes.NewReader(body))
	if err != nil {
		return Response{}, err
	}
	httpReq.Header.Set("Content-Type", "application/json")
	httpReq.Header.Set("Accept-Encoding", "gzip")

	resp, err := c.HTTPClient.Do(httpReq)
	if err != nil {
		return Response{}, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		b, _ := readResponseBody(resp)
		return Response{}, &HTTPStatusError{
			StatusCode: resp.StatusCode,
			Body:       string(b),
			RetryAfter: retryAfterHeader(resp.Header.Get("Retry-After")),
		}
	}

	reader, err := responseReader(resp)
	if err != nil {
		return Response{}, err
	}
	if closer, ok := reader.(io.Closer); ok {
		defer closer.Close()
	}

	var out Response
	if err := json.NewDecoder(reader).Decode(&out); err != nil {
		return Response{}, err
	}
	return out, nil
}

func retryAfterHeader(raw string) time.Duration {
	seconds, err := strconv.ParseInt(strings.TrimSpace(raw), 10, 64)
	if err != nil || seconds <= 0 {
		return 0
	}
	return time.Duration(seconds) * time.Second
}

func readResponseBody(resp *http.Response) ([]byte, error) {
	reader, err := responseReader(resp)
	if err != nil {
		return nil, err
	}
	if closer, ok := reader.(io.Closer); ok {
		defer closer.Close()
	}
	return io.ReadAll(reader)
}

func responseReader(resp *http.Response) (io.Reader, error) {
	if resp.Header.Get("Content-Encoding") != "gzip" {
		return resp.Body, nil
	}
	gz, err := gzip.NewReader(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("gzip reader: %w", err)
	}
	return gz, nil
}
