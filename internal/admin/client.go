package admin

import (
	"bytes"
	"crypto/tls"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"time"

	opcreds "github.com/DavidHoenisch/remotr/internal/operator/credentials"
	"github.com/DavidHoenisch/remotr/internal/tlsconfig"
)

type BootstrapRequest struct {
	Token string `json:"token"`
}

type BootstrapResponse struct {
	OperatorID string `json:"operator_id"`
	CertPEM    string `json:"cert_pem"`
	KeyPEM     string `json:"key_pem"`
	CAPEM      string `json:"ca_pem"`
}

type CreateEnrollTokenRequest struct {
	Fleet      string `json:"fleet"`
	TTLSeconds int64  `json:"ttl_seconds"`
}

type CreateEnrollTokenResponse struct {
	Token     string    `json:"token"`
	Fleet     string    `json:"fleet"`
	ExpiresAt time.Time `json:"expires_at"`
}

type CreateDeploymentTokenRequest struct {
	Label      string `json:"label"`
	Fleet      string `json:"fleet"`
	TTLSeconds int64  `json:"ttl_seconds"`
}

type CreateDeploymentTokenResponse struct {
	Token     string    `json:"token"`
	Label     string    `json:"label"`
	Fleet     string    `json:"fleet"`
	ExpiresAt time.Time `json:"expires_at"`
}

type DeploymentToken struct {
	ID         string     `json:"id"`
	Label      string     `json:"label"`
	Fleet      string     `json:"fleet"`
	ExpiresAt  time.Time  `json:"expires_at"`
	CreatedAt  time.Time  `json:"created_at"`
	RevokedAt  *time.Time `json:"revoked_at,omitempty"`
	LastUsedAt *time.Time `json:"last_used_at,omitempty"`
}

type CheckInSummary struct {
	ReleaseRef string    `json:"release_ref"`
	Digest     string    `json:"digest"`
	At         time.Time `json:"at"`
}

type Endpoint struct {
	ID                   string               `json:"id"`
	Fleet                string               `json:"fleet"`
	CertFingerprint      string               `json:"cert_fingerprint,omitempty"`
	Labels               map[string]string    `json:"labels,omitempty"`
	DesiredAgentVersion  string               `json:"desired_agent_version,omitempty"`
	ReportedAgentVersion string               `json:"reported_agent_version,omitempty"`
	LastCheckIn          *CheckInSummary      `json:"last_check_in,omitempty"`
	AgentUpgrade         *AgentUpgradeSummary `json:"agent_upgrade,omitempty"`
	LastDrift            *DriftSummary        `json:"last_drift,omitempty"`
	LastApplyFailure     *ApplyFailureSummary `json:"last_apply_failure,omitempty"`
}

type AgentUpgradeSummary struct {
	Desired    string    `json:"desired,omitempty"`
	Phase      string    `json:"phase,omitempty"`
	Message    string    `json:"message,omitempty"`
	ReportedAt time.Time `json:"reported_at,omitempty"`
}

type DriftSummary struct {
	ReleaseRef string    `json:"release_ref"`
	Digest     string    `json:"digest"`
	ReportedAt time.Time `json:"reported_at"`
}

type ApplyFailureSummary struct {
	ReleaseRef      string    `json:"release_ref"`
	ResourceAddress string    `json:"resource_address"`
	Message         string    `json:"message"`
	ReportedAt      time.Time `json:"reported_at"`
}

type StateReportItem struct {
	Address     string `json:"address"`
	Name        string `json:"name"`
	Description string `json:"description"`
}

type StateReport struct {
	EndpointID   string               `json:"endpoint_id"`
	Fleet        string               `json:"fleet"`
	ReleaseRef   string               `json:"release_ref,omitempty"`
	Digest       string               `json:"digest,omitempty"`
	ReportedAt   time.Time            `json:"reported_at,omitempty"`
	InCompliance bool                 `json:"in_compliance"`
	Items        []StateReportItem    `json:"items"`
	ApplyFailure *ApplyFailureSummary `json:"apply_failure,omitempty"`
}

func (r StateReport) HasReport() bool {
	return !r.ReportedAt.IsZero()
}

type FleetStateSummary struct {
	Total     int `json:"total"`
	Compliant int `json:"compliant"`
	Drift     int `json:"drift"`
	NoReport  int `json:"no_report"`
}

type FleetStateReport struct {
	Fleet     string            `json:"fleet"`
	Summary   FleetStateSummary `json:"summary"`
	Endpoints []StateReport     `json:"endpoints"`
}

type Client struct {
	BaseURL    string
	StateDir   string
	HTTPClient *http.Client
}

func NewClient(baseURL, stateDir string, tlsCfg *tls.Config) (*Client, error) {
	c := &Client{
		BaseURL:  baseURL,
		StateDir: stateDir,
	}
	if DemoEnabled() {
		hc, err := demoHTTPClient()
		if err != nil {
			return nil, err
		}
		c.HTTPClient = hc
		return c, nil
	}
	c.HTTPClient = &http.Client{
		Transport: &http.Transport{TLSClientConfig: tlsCfg},
	}
	return c, nil
}

func NewClientFromState(baseURL, stateDir string) (*Client, error) {
	if DemoEnabled() {
		return NewClient(baseURL, stateDir, nil)
	}
	layout, err := opcreds.Layout(stateDir)
	if err != nil {
		return nil, err
	}
	tlsCfg, err := tlsconfig.ClientTLSConfig(layout.Cert, layout.Key, layout.CA)
	if err != nil {
		return nil, err
	}
	return NewClient(baseURL, stateDir, tlsCfg)
}

func (c *Client) Bootstrap(token string) (BootstrapResponse, error) {
	body, err := json.Marshal(BootstrapRequest{Token: token})
	if err != nil {
		return BootstrapResponse{}, err
	}

	req, err := http.NewRequest(http.MethodPost, c.BaseURL+"/v1/admin/bootstrap", bytes.NewReader(body))
	if err != nil {
		return BootstrapResponse{}, err
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := c.HTTPClient.Do(req)
	if err != nil {
		return BootstrapResponse{}, err
	}
	defer resp.Body.Close()

	raw, err := io.ReadAll(resp.Body)
	if err != nil {
		return BootstrapResponse{}, err
	}
	if resp.StatusCode != http.StatusOK {
		return BootstrapResponse{}, fmt.Errorf("bootstrap status %d: %s", resp.StatusCode, raw)
	}

	var out BootstrapResponse
	if err := json.Unmarshal(raw, &out); err != nil {
		return BootstrapResponse{}, fmt.Errorf("decode bootstrap response: %w", err)
	}
	if out.OperatorID == "" || out.CertPEM == "" || out.KeyPEM == "" || out.CAPEM == "" {
		return BootstrapResponse{}, fmt.Errorf("incomplete bootstrap response")
	}
	return out, nil
}

func (c *Client) CreateEnrollToken(fleet string, ttl time.Duration) (CreateEnrollTokenResponse, error) {
	body, err := json.Marshal(CreateEnrollTokenRequest{
		Fleet:      fleet,
		TTLSeconds: int64(ttl.Seconds()),
	})
	if err != nil {
		return CreateEnrollTokenResponse{}, err
	}

	req, err := http.NewRequest(http.MethodPost, c.BaseURL+"/v1/admin/enroll-tokens", bytes.NewReader(body))
	if err != nil {
		return CreateEnrollTokenResponse{}, err
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := c.HTTPClient.Do(req)
	if err != nil {
		return CreateEnrollTokenResponse{}, err
	}
	defer resp.Body.Close()

	raw, err := io.ReadAll(resp.Body)
	if err != nil {
		return CreateEnrollTokenResponse{}, err
	}
	if resp.StatusCode != http.StatusOK {
		return CreateEnrollTokenResponse{}, fmt.Errorf("create enroll token status %d: %s", resp.StatusCode, raw)
	}

	var out CreateEnrollTokenResponse
	if err := json.Unmarshal(raw, &out); err != nil {
		return CreateEnrollTokenResponse{}, fmt.Errorf("decode enroll token response: %w", err)
	}
	if out.Token == "" || out.Fleet == "" {
		return CreateEnrollTokenResponse{}, fmt.Errorf("incomplete enroll token response")
	}
	return out, nil
}

func (c *Client) CreateDeploymentToken(label, fleet string, ttl time.Duration) (CreateDeploymentTokenResponse, error) {
	body, err := json.Marshal(CreateDeploymentTokenRequest{
		Label:      label,
		Fleet:      fleet,
		TTLSeconds: int64(ttl.Seconds()),
	})
	if err != nil {
		return CreateDeploymentTokenResponse{}, err
	}

	req, err := http.NewRequest(http.MethodPost, c.BaseURL+"/v1/admin/deployment-tokens", bytes.NewReader(body))
	if err != nil {
		return CreateDeploymentTokenResponse{}, err
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := c.HTTPClient.Do(req)
	if err != nil {
		return CreateDeploymentTokenResponse{}, err
	}
	defer resp.Body.Close()

	raw, err := io.ReadAll(resp.Body)
	if err != nil {
		return CreateDeploymentTokenResponse{}, err
	}
	if resp.StatusCode == http.StatusConflict {
		return CreateDeploymentTokenResponse{}, fmt.Errorf("deployment token label already exists")
	}
	if resp.StatusCode != http.StatusOK {
		return CreateDeploymentTokenResponse{}, fmt.Errorf("create deployment token status %d: %s", resp.StatusCode, raw)
	}

	var out CreateDeploymentTokenResponse
	if err := json.Unmarshal(raw, &out); err != nil {
		return CreateDeploymentTokenResponse{}, fmt.Errorf("decode deployment token response: %w", err)
	}
	if out.Token == "" || out.Label == "" || out.Fleet == "" {
		return CreateDeploymentTokenResponse{}, fmt.Errorf("incomplete deployment token response")
	}
	return out, nil
}

func (c *Client) ListDeploymentTokens() ([]DeploymentToken, error) {
	req, err := http.NewRequest(http.MethodGet, c.BaseURL+"/v1/admin/deployment-tokens", nil)
	if err != nil {
		return nil, err
	}

	resp, err := c.HTTPClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	raw, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("list deployment tokens status %d: %s", resp.StatusCode, raw)
	}

	var out []DeploymentToken
	if err := json.Unmarshal(raw, &out); err != nil {
		return nil, fmt.Errorf("decode deployment tokens response: %w", err)
	}
	return out, nil
}

func (c *Client) GetDeploymentToken(label string) (DeploymentToken, error) {
	req, err := http.NewRequest(http.MethodGet, c.BaseURL+"/v1/admin/deployment-tokens/"+label, nil)
	if err != nil {
		return DeploymentToken{}, err
	}

	resp, err := c.HTTPClient.Do(req)
	if err != nil {
		return DeploymentToken{}, err
	}
	defer resp.Body.Close()

	raw, err := io.ReadAll(resp.Body)
	if err != nil {
		return DeploymentToken{}, err
	}
	if resp.StatusCode == http.StatusNotFound {
		return DeploymentToken{}, fmt.Errorf("deployment token not found")
	}
	if resp.StatusCode != http.StatusOK {
		return DeploymentToken{}, fmt.Errorf("get deployment token status %d: %s", resp.StatusCode, raw)
	}

	var out DeploymentToken
	if err := json.Unmarshal(raw, &out); err != nil {
		return DeploymentToken{}, fmt.Errorf("decode deployment token response: %w", err)
	}
	return out, nil
}

func (c *Client) RevokeDeploymentToken(label string) error {
	req, err := http.NewRequest(http.MethodDelete, c.BaseURL+"/v1/admin/deployment-tokens/"+label, nil)
	if err != nil {
		return err
	}

	resp, err := c.HTTPClient.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	raw, err := io.ReadAll(resp.Body)
	if err != nil {
		return err
	}
	if resp.StatusCode == http.StatusNotFound {
		return fmt.Errorf("deployment token not found")
	}
	if resp.StatusCode != http.StatusNoContent {
		return fmt.Errorf("revoke deployment token status %d: %s", resp.StatusCode, raw)
	}
	return nil
}

func (c *Client) TriggerGitSync() error {
	req, err := http.NewRequest(http.MethodPost, c.BaseURL+"/v1/admin/git-sync", nil)
	if err != nil {
		return err
	}

	resp, err := c.HTTPClient.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	raw, err := io.ReadAll(resp.Body)
	if err != nil {
		return err
	}
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("git sync status %d: %s", resp.StatusCode, raw)
	}
	return nil
}

func (c *Client) ListEndpoints() ([]Endpoint, error) {
	req, err := http.NewRequest(http.MethodGet, c.BaseURL+"/v1/admin/endpoints", nil)
	if err != nil {
		return nil, err
	}

	resp, err := c.HTTPClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	raw, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("list endpoints status %d: %s", resp.StatusCode, raw)
	}

	var out []Endpoint
	if err := json.Unmarshal(raw, &out); err != nil {
		return nil, fmt.Errorf("decode endpoints response: %w", err)
	}
	return out, nil
}

func (c *Client) ListFleets() ([]string, error) {
	req, err := http.NewRequest(http.MethodGet, c.BaseURL+"/v1/admin/fleets", nil)
	if err != nil {
		return nil, err
	}

	resp, err := c.HTTPClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	raw, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("list fleets status %d: %s", resp.StatusCode, raw)
	}

	var out []string
	if err := json.Unmarshal(raw, &out); err != nil {
		return nil, fmt.Errorf("decode fleets response: %w", err)
	}
	return out, nil
}

func (c *Client) GetEndpoint(id string) (Endpoint, error) {
	req, err := http.NewRequest(http.MethodGet, c.BaseURL+"/v1/admin/endpoints/"+id, nil)
	if err != nil {
		return Endpoint{}, err
	}

	resp, err := c.HTTPClient.Do(req)
	if err != nil {
		return Endpoint{}, err
	}
	defer resp.Body.Close()

	raw, err := io.ReadAll(resp.Body)
	if err != nil {
		return Endpoint{}, err
	}
	if resp.StatusCode == http.StatusNotFound {
		return Endpoint{}, fmt.Errorf("endpoint not found")
	}
	if resp.StatusCode != http.StatusOK {
		return Endpoint{}, fmt.Errorf("get endpoint status %d: %s", resp.StatusCode, raw)
	}

	var out Endpoint
	if err := json.Unmarshal(raw, &out); err != nil {
		return Endpoint{}, fmt.Errorf("decode endpoint response: %w", err)
	}
	return out, nil
}

func (c *Client) RequestEndpointAgentUpgrade(id, version string) error {
	body, err := json.Marshal(map[string]string{"version": version})
	if err != nil {
		return err
	}
	req, err := http.NewRequest(http.MethodPost, c.BaseURL+"/v1/admin/endpoints/"+url.PathEscape(id)+"/agent-upgrade", bytes.NewReader(body))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")
	resp, err := c.HTTPClient.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	raw, _ := io.ReadAll(resp.Body)
	if resp.StatusCode == http.StatusNotFound {
		return fmt.Errorf("endpoint not found")
	}
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("agent upgrade status %d: %s", resp.StatusCode, raw)
	}
	return nil
}

func (c *Client) RequestFleetAgentUpgrade(fleet, version string) (int, error) {
	body, err := json.Marshal(map[string]string{"version": version})
	if err != nil {
		return 0, err
	}
	req, err := http.NewRequest(http.MethodPost, c.BaseURL+"/v1/admin/fleets/"+url.PathEscape(fleet)+"/agent-upgrade", bytes.NewReader(body))
	if err != nil {
		return 0, err
	}
	req.Header.Set("Content-Type", "application/json")
	resp, err := c.HTTPClient.Do(req)
	if err != nil {
		return 0, err
	}
	defer resp.Body.Close()
	raw, err := io.ReadAll(resp.Body)
	if err != nil {
		return 0, err
	}
	if resp.StatusCode != http.StatusOK {
		return 0, fmt.Errorf("fleet agent upgrade status %d: %s", resp.StatusCode, raw)
	}
	var out struct {
		Version   string `json:"version"`
		Endpoints int    `json:"endpoints"`
	}
	if err := json.Unmarshal(raw, &out); err != nil {
		return 0, err
	}
	return out.Endpoints, nil
}

func (c *Client) GetEndpointStateReport(id string) (StateReport, error) {
	req, err := http.NewRequest(http.MethodGet, c.BaseURL+"/v1/admin/endpoints/"+url.PathEscape(id)+"/state-report", nil)
	if err != nil {
		return StateReport{}, err
	}

	resp, err := c.HTTPClient.Do(req)
	if err != nil {
		return StateReport{}, err
	}
	defer resp.Body.Close()

	raw, err := io.ReadAll(resp.Body)
	if err != nil {
		return StateReport{}, err
	}
	if resp.StatusCode == http.StatusNotFound {
		return StateReport{}, fmt.Errorf("endpoint not found")
	}
	if resp.StatusCode != http.StatusOK {
		return StateReport{}, fmt.Errorf("get endpoint state report status %d: %s", resp.StatusCode, raw)
	}

	var out StateReport
	if err := json.Unmarshal(raw, &out); err != nil {
		return StateReport{}, fmt.Errorf("decode endpoint state report: %w", err)
	}
	return out, nil
}

func (c *Client) GetFleetStateReport(fleet string) (FleetStateReport, error) {
	req, err := http.NewRequest(http.MethodGet, c.BaseURL+"/v1/admin/fleets/"+url.PathEscape(fleet)+"/state-report", nil)
	if err != nil {
		return FleetStateReport{}, err
	}

	resp, err := c.HTTPClient.Do(req)
	if err != nil {
		return FleetStateReport{}, err
	}
	defer resp.Body.Close()

	raw, err := io.ReadAll(resp.Body)
	if err != nil {
		return FleetStateReport{}, err
	}
	if resp.StatusCode != http.StatusOK {
		return FleetStateReport{}, fmt.Errorf("get fleet state report status %d: %s", resp.StatusCode, raw)
	}

	var out FleetStateReport
	if err := json.Unmarshal(raw, &out); err != nil {
		return FleetStateReport{}, fmt.Errorf("decode fleet state report: %w", err)
	}
	return out, nil
}

func (c *Client) RemoveEndpoint(id string) error {
	req, err := http.NewRequest(http.MethodDelete, c.BaseURL+"/v1/admin/endpoints/"+url.PathEscape(id), nil)
	if err != nil {
		return err
	}

	resp, err := c.HTTPClient.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	raw, err := io.ReadAll(resp.Body)
	if err != nil {
		return err
	}
	if resp.StatusCode == http.StatusNotFound {
		return fmt.Errorf("endpoint not found")
	}
	if resp.StatusCode != http.StatusNoContent {
		return fmt.Errorf("remove endpoint status %d: %s", resp.StatusCode, raw)
	}
	return nil
}

type AuditEvent struct {
	ID               string         `json:"id"`
	OccurredAt       time.Time      `json:"occurred_at"`
	RequestID        string         `json:"request_id,omitempty"`
	ActorType        string         `json:"actor_type"`
	ActorID          string         `json:"actor_id,omitempty"`
	ActorFingerprint string         `json:"actor_fingerprint,omitempty"`
	Action           string         `json:"action"`
	Method           string         `json:"method"`
	Path             string         `json:"path"`
	StatusCode       int            `json:"status_code"`
	ResourceType     string         `json:"resource_type,omitempty"`
	ResourceID       string         `json:"resource_id,omitempty"`
	ClientIP         string         `json:"client_ip,omitempty"`
	Details          map[string]any `json:"details,omitempty"`
}

type AuditEventPage struct {
	Events     []AuditEvent `json:"events"`
	NextCursor string       `json:"next_cursor,omitempty"`
}

type AuditListOptions struct {
	Since     time.Time
	Until     time.Time
	Action    string
	ActorType string
	Limit     int
	Cursor    string
}

type AuditExportInfo struct {
	ExportPath string `json:"export_path"`
	PathKey    string `json:"path_key"`
}

type CreateOperatorCredentialResponse struct {
	OperatorID string   `json:"operator_id"`
	Label      string   `json:"label,omitempty"`
	Roles      []string `json:"roles,omitempty"`
	CertPEM    string   `json:"cert_pem"`
	KeyPEM     string   `json:"key_pem"`
	CAPEM      string   `json:"ca_pem"`
}

func (c *Client) ListAuditEvents(opts AuditListOptions) (AuditEventPage, error) {
	q := url.Values{}
	if !opts.Since.IsZero() {
		q.Set("since", opts.Since.UTC().Format(time.RFC3339))
	}
	if !opts.Until.IsZero() {
		q.Set("until", opts.Until.UTC().Format(time.RFC3339))
	}
	if opts.Action != "" {
		q.Set("action", opts.Action)
	}
	if opts.ActorType != "" {
		q.Set("actor_type", opts.ActorType)
	}
	if opts.Limit > 0 {
		q.Set("limit", strconv.Itoa(opts.Limit))
	}
	if opts.Cursor != "" {
		q.Set("cursor", opts.Cursor)
	}

	endpoint := c.BaseURL + "/v1/admin/audit-events"
	if encoded := q.Encode(); encoded != "" {
		endpoint += "?" + encoded
	}

	req, err := http.NewRequest(http.MethodGet, endpoint, nil)
	if err != nil {
		return AuditEventPage{}, err
	}

	resp, err := c.HTTPClient.Do(req)
	if err != nil {
		return AuditEventPage{}, err
	}
	defer resp.Body.Close()

	raw, err := io.ReadAll(resp.Body)
	if err != nil {
		return AuditEventPage{}, err
	}
	if resp.StatusCode != http.StatusOK {
		return AuditEventPage{}, fmt.Errorf("list audit events status %d: %s", resp.StatusCode, raw)
	}

	var out AuditEventPage
	if err := json.Unmarshal(raw, &out); err != nil {
		return AuditEventPage{}, fmt.Errorf("decode audit events: %w", err)
	}
	return out, nil
}

func (c *Client) GetAuditExportInfo() (AuditExportInfo, error) {
	req, err := http.NewRequest(http.MethodGet, c.BaseURL+"/v1/admin/audit-export", nil)
	if err != nil {
		return AuditExportInfo{}, err
	}

	resp, err := c.HTTPClient.Do(req)
	if err != nil {
		return AuditExportInfo{}, err
	}
	defer resp.Body.Close()

	raw, err := io.ReadAll(resp.Body)
	if err != nil {
		return AuditExportInfo{}, err
	}
	if resp.StatusCode != http.StatusOK {
		return AuditExportInfo{}, fmt.Errorf("audit export info status %d: %s", resp.StatusCode, raw)
	}

	var out AuditExportInfo
	if err := json.Unmarshal(raw, &out); err != nil {
		return AuditExportInfo{}, fmt.Errorf("decode audit export info: %w", err)
	}
	return out, nil
}

func (c *Client) ExportAuditEvents(pathKey string, opts AuditListOptions) (AuditEventPage, error) {
	q := url.Values{}
	if !opts.Since.IsZero() {
		q.Set("since", opts.Since.UTC().Format(time.RFC3339))
	}
	if !opts.Until.IsZero() {
		q.Set("until", opts.Until.UTC().Format(time.RFC3339))
	}
	if opts.Action != "" {
		q.Set("action", opts.Action)
	}
	if opts.ActorType != "" {
		q.Set("actor_type", opts.ActorType)
	}
	if opts.Limit > 0 {
		q.Set("limit", strconv.Itoa(opts.Limit))
	}
	if opts.Cursor != "" {
		q.Set("cursor", opts.Cursor)
	}

	endpoint := c.BaseURL + "/v1/exports/audit/" + url.PathEscape(pathKey)
	if encoded := q.Encode(); encoded != "" {
		endpoint += "?" + encoded
	}

	req, err := http.NewRequest(http.MethodGet, endpoint, nil)
	if err != nil {
		return AuditEventPage{}, err
	}

	resp, err := c.HTTPClient.Do(req)
	if err != nil {
		return AuditEventPage{}, err
	}
	defer resp.Body.Close()

	raw, err := io.ReadAll(resp.Body)
	if err != nil {
		return AuditEventPage{}, err
	}
	if resp.StatusCode != http.StatusOK {
		return AuditEventPage{}, fmt.Errorf("export audit events status %d: %s", resp.StatusCode, raw)
	}

	var out AuditEventPage
	if err := json.Unmarshal(raw, &out); err != nil {
		return AuditEventPage{}, fmt.Errorf("decode audit export: %w", err)
	}
	return out, nil
}

type RBACRole struct {
	Name        string     `json:"name"`
	Description string     `json:"description"`
	BuiltIn     bool       `json:"built_in"`
	Rules       []RBACRule `json:"rules"`
}

type RBACRule struct {
	ID          string `json:"id,omitempty"`
	Method      string `json:"method"`
	PathPattern string `json:"path_pattern"`
}

type OperatorInfo struct {
	ID              string    `json:"id"`
	CertFingerprint string    `json:"cert_fingerprint"`
	Roles           []string  `json:"roles"`
	CreatedAt       time.Time `json:"created_at"`
}

type OperatorMe struct {
	OperatorID string   `json:"operator_id"`
	Roles      []string `json:"roles"`
}

func (c *Client) GetOperatorMe() (OperatorMe, error) {
	req, err := http.NewRequest(http.MethodGet, c.BaseURL+"/v1/admin/me", nil)
	if err != nil {
		return OperatorMe{}, err
	}
	resp, err := c.HTTPClient.Do(req)
	if err != nil {
		return OperatorMe{}, err
	}
	defer resp.Body.Close()
	raw, err := io.ReadAll(resp.Body)
	if err != nil {
		return OperatorMe{}, err
	}
	if resp.StatusCode != http.StatusOK {
		return OperatorMe{}, fmt.Errorf("operator me status %d: %s", resp.StatusCode, raw)
	}
	var out OperatorMe
	if err := json.Unmarshal(raw, &out); err != nil {
		return OperatorMe{}, err
	}
	return out, nil
}

func (c *Client) ListRBACRoles() ([]RBACRole, error) {
	req, err := http.NewRequest(http.MethodGet, c.BaseURL+"/v1/admin/rbac/roles", nil)
	if err != nil {
		return nil, err
	}
	resp, err := c.HTTPClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	raw, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("list rbac roles status %d: %s", resp.StatusCode, raw)
	}
	var out []RBACRole
	if err := json.Unmarshal(raw, &out); err != nil {
		return nil, err
	}
	return out, nil
}

func (c *Client) GetRBACRole(name string) (RBACRole, error) {
	req, err := http.NewRequest(http.MethodGet, c.BaseURL+"/v1/admin/rbac/roles/"+url.PathEscape(name), nil)
	if err != nil {
		return RBACRole{}, err
	}
	resp, err := c.HTTPClient.Do(req)
	if err != nil {
		return RBACRole{}, err
	}
	defer resp.Body.Close()
	raw, err := io.ReadAll(resp.Body)
	if err != nil {
		return RBACRole{}, err
	}
	if resp.StatusCode == http.StatusNotFound {
		return RBACRole{}, fmt.Errorf("role not found")
	}
	if resp.StatusCode != http.StatusOK {
		return RBACRole{}, fmt.Errorf("get rbac role status %d: %s", resp.StatusCode, raw)
	}
	var out RBACRole
	if err := json.Unmarshal(raw, &out); err != nil {
		return RBACRole{}, err
	}
	return out, nil
}

func (c *Client) CreateRBACRole(name, description string) (RBACRole, error) {
	body, err := json.Marshal(map[string]string{"name": name, "description": description})
	if err != nil {
		return RBACRole{}, err
	}
	req, err := http.NewRequest(http.MethodPost, c.BaseURL+"/v1/admin/rbac/roles", bytes.NewReader(body))
	if err != nil {
		return RBACRole{}, err
	}
	req.Header.Set("Content-Type", "application/json")
	resp, err := c.HTTPClient.Do(req)
	if err != nil {
		return RBACRole{}, err
	}
	defer resp.Body.Close()
	raw, err := io.ReadAll(resp.Body)
	if err != nil {
		return RBACRole{}, err
	}
	if resp.StatusCode != http.StatusOK {
		return RBACRole{}, fmt.Errorf("create rbac role status %d: %s", resp.StatusCode, raw)
	}
	var out RBACRole
	if err := json.Unmarshal(raw, &out); err != nil {
		return RBACRole{}, err
	}
	return out, nil
}

func (c *Client) DeleteRBACRole(name string) error {
	req, err := http.NewRequest(http.MethodDelete, c.BaseURL+"/v1/admin/rbac/roles/"+url.PathEscape(name), nil)
	if err != nil {
		return err
	}
	resp, err := c.HTTPClient.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	raw, err := io.ReadAll(resp.Body)
	if err != nil {
		return err
	}
	if resp.StatusCode == http.StatusNotFound {
		return fmt.Errorf("role not found")
	}
	if resp.StatusCode != http.StatusNoContent {
		return fmt.Errorf("delete rbac role status %d: %s", resp.StatusCode, raw)
	}
	return nil
}

func (c *Client) AddRBACRule(roleName, method, pathPattern string) (RBACRule, error) {
	body, err := json.Marshal(map[string]string{"method": method, "path_pattern": pathPattern})
	if err != nil {
		return RBACRule{}, err
	}
	req, err := http.NewRequest(http.MethodPost, c.BaseURL+"/v1/admin/rbac/roles/"+url.PathEscape(roleName)+"/rules", bytes.NewReader(body))
	if err != nil {
		return RBACRule{}, err
	}
	req.Header.Set("Content-Type", "application/json")
	resp, err := c.HTTPClient.Do(req)
	if err != nil {
		return RBACRule{}, err
	}
	defer resp.Body.Close()
	raw, err := io.ReadAll(resp.Body)
	if err != nil {
		return RBACRule{}, err
	}
	if resp.StatusCode != http.StatusOK {
		return RBACRule{}, fmt.Errorf("add rbac rule status %d: %s", resp.StatusCode, raw)
	}
	var out RBACRule
	if err := json.Unmarshal(raw, &out); err != nil {
		return RBACRule{}, err
	}
	return out, nil
}

func (c *Client) DeleteRBACRule(roleName, ruleID string) error {
	req, err := http.NewRequest(http.MethodDelete, c.BaseURL+"/v1/admin/rbac/roles/"+url.PathEscape(roleName)+"/rules/"+url.PathEscape(ruleID), nil)
	if err != nil {
		return err
	}
	resp, err := c.HTTPClient.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	raw, err := io.ReadAll(resp.Body)
	if err != nil {
		return err
	}
	if resp.StatusCode == http.StatusNotFound {
		return fmt.Errorf("rule not found")
	}
	if resp.StatusCode != http.StatusNoContent {
		return fmt.Errorf("delete rbac rule status %d: %s", resp.StatusCode, raw)
	}
	return nil
}

func (c *Client) ListOperators() ([]OperatorInfo, error) {
	req, err := http.NewRequest(http.MethodGet, c.BaseURL+"/v1/admin/operators", nil)
	if err != nil {
		return nil, err
	}
	resp, err := c.HTTPClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	raw, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("list operators status %d: %s", resp.StatusCode, raw)
	}
	var out []OperatorInfo
	if err := json.Unmarshal(raw, &out); err != nil {
		return nil, err
	}
	return out, nil
}

func (c *Client) SetOperatorRoles(operatorID string, roles []string) error {
	body, err := json.Marshal(map[string][]string{"roles": roles})
	if err != nil {
		return err
	}
	req, err := http.NewRequest(http.MethodPut, c.BaseURL+"/v1/admin/operators/"+url.PathEscape(operatorID)+"/roles", bytes.NewReader(body))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")
	resp, err := c.HTTPClient.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	raw, err := io.ReadAll(resp.Body)
	if err != nil {
		return err
	}
	if resp.StatusCode != http.StatusNoContent {
		return fmt.Errorf("set operator roles status %d: %s", resp.StatusCode, raw)
	}
	return nil
}

func (c *Client) CreateOperatorCredential(label string, roles []string) (CreateOperatorCredentialResponse, error) {
	body, err := json.Marshal(map[string]any{"label": label, "roles": roles})
	if err != nil {
		return CreateOperatorCredentialResponse{}, err
	}

	req, err := http.NewRequest(http.MethodPost, c.BaseURL+"/v1/admin/operator-credentials", bytes.NewReader(body))
	if err != nil {
		return CreateOperatorCredentialResponse{}, err
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := c.HTTPClient.Do(req)
	if err != nil {
		return CreateOperatorCredentialResponse{}, err
	}
	defer resp.Body.Close()

	raw, err := io.ReadAll(resp.Body)
	if err != nil {
		return CreateOperatorCredentialResponse{}, err
	}
	if resp.StatusCode != http.StatusOK {
		return CreateOperatorCredentialResponse{}, fmt.Errorf("create operator credential status %d: %s", resp.StatusCode, raw)
	}

	var out CreateOperatorCredentialResponse
	if err := json.Unmarshal(raw, &out); err != nil {
		return CreateOperatorCredentialResponse{}, fmt.Errorf("decode operator credential response: %w", err)
	}
	if out.OperatorID == "" || out.CertPEM == "" || out.KeyPEM == "" || out.CAPEM == "" {
		return CreateOperatorCredentialResponse{}, fmt.Errorf("incomplete operator credential response")
	}
	return out, nil
}
