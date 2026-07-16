package admin

import (
	"bytes"
	"context"
	"crypto/tls"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"time"

	"github.com/DavidHoenisch/remotr/internal/apppackages"
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

type BootstrapResponseError struct {
	StatusCode int
	Body       []byte
}

func (e *BootstrapResponseError) Error() string {
	return fmt.Sprintf("bootstrap status %d: %s", e.StatusCode, e.Body)
}

type BootstrapPayloadError struct {
	Err error
}

func (e *BootstrapPayloadError) Error() string {
	return e.Err.Error()
}

func (e *BootstrapPayloadError) Unwrap() error {
	return e.Err
}

// ResponseError preserves a non-success Admin API status for typed callers.
// Desktop and other presentation layers must map Body to a safe public error
// rather than returning server response content directly.
type ResponseError struct {
	Operation  string
	StatusCode int
	Body       []byte
}

func (e *ResponseError) Error() string {
	return fmt.Sprintf("%s status %d: %s", e.Operation, e.StatusCode, e.Body)
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
	Usernames            []string             `json:"usernames,omitempty"`
	LastCheckIn          *CheckInSummary      `json:"last_check_in,omitempty"`
	AgentUpgrade         *AgentUpgradeSummary `json:"agent_upgrade,omitempty"`
	LastDrift            *DriftSummary        `json:"last_drift,omitempty"`
	LastApplyFailure     *ApplyFailureSummary `json:"last_apply_failure,omitempty"`
	SystemInfo           *SystemInfoSummary   `json:"system_info,omitempty"`
}

type SystemInfoSummary struct {
	Digest     string          `json:"digest,omitempty"`
	ReportedAt time.Time       `json:"reported_at,omitempty"`
	Report     json.RawMessage `json:"report,omitempty"`
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

type StateReportStatus string

const (
	StateCompliant   StateReportStatus = "compliant"
	StateDrifted     StateReportStatus = "drifted"
	StateUnsupported StateReportStatus = "unsupported"
	StateCheckFailed StateReportStatus = "check_failed"
	StateDeferred    StateReportStatus = "deferred"
	StateApplyFailed StateReportStatus = "apply_failed"
	StateNoReport    StateReportStatus = "no_report"
)

type StateReportItem struct {
	Address             string                 `json:"address"`
	Name                string                 `json:"name"`
	Description         string                 `json:"description"`
	Provider            string                 `json:"provider,omitempty"`
	Status              StateReportStatus      `json:"status,omitempty"`
	ReasonCode          string                 `json:"reasonCode,omitempty"`
	DesiredSummary      string                 `json:"desiredSummary,omitempty"`
	ObservedSummary     string                 `json:"observedSummary,omitempty"`
	Subresults          []StateReportSubresult `json:"subresults,omitempty"`
	SubresultsTruncated bool                   `json:"subresultsTruncated,omitempty"`
}

type StateReportSubresult struct {
	Target          string            `json:"target"`
	Status          StateReportStatus `json:"status"`
	ReasonCode      string            `json:"reasonCode"`
	DesiredSummary  string            `json:"desiredSummary,omitempty"`
	ObservedSummary string            `json:"observedSummary,omitempty"`
}

type StateReportActivation struct {
	Kind   string `json:"kind"`
	Target string `json:"target,omitempty"`
}

type StateReportApplyItem struct {
	Address         string                  `json:"address"`
	Name            string                  `json:"name"`
	Provider        string                  `json:"provider,omitempty"`
	Status          string                  `json:"status"`
	ReasonCode      string                  `json:"reasonCode,omitempty"`
	DesiredSummary  string                  `json:"desiredSummary,omitempty"`
	ObservedSummary string                  `json:"observedSummary,omitempty"`
	Activation      []StateReportActivation `json:"activation,omitempty"`
	RebootRequired  string                  `json:"rebootRequired,omitempty"`
	RollbackClass   string                  `json:"rollbackClass,omitempty"`
	RollbackStatus  string                  `json:"rollbackStatus,omitempty"`
	Diagnostics     []string                `json:"diagnostics,omitempty"`
}

type StateReportScheduleRuntime struct {
	Address           string `json:"address"`
	Name              string `json:"name"`
	Provider          string `json:"provider,omitempty"`
	Status            string `json:"status"`
	ExitCode          *int   `json:"exitCode,omitempty"`
	MissedRunBehavior string `json:"missedRunBehavior"`
}

type StateReportRebootSource struct {
	Address  string `json:"address"`
	Name     string `json:"name,omitempty"`
	Provider string `json:"provider,omitempty"`
}

type StateReportRebootIntent struct {
	Generation        string    `json:"generation"`
	Phase             string    `json:"phase"`
	PriorBootID       string    `json:"priorBootId"`
	CurrentBootID     string    `json:"currentBootId,omitempty"`
	PreparedAt        time.Time `json:"preparedAt"`
	NotBefore         time.Time `json:"notBefore"`
	Deadline          time.Time `json:"deadline,omitempty"`
	AttemptedAt       time.Time `json:"attemptedAt,omitempty"`
	AttemptDeadline   time.Time `json:"attemptDeadline,omitempty"`
	AttemptGeneration uint64    `json:"attemptGeneration,omitempty"`
	Reason            string    `json:"reason,omitempty"`
}

type StateReportRebootCompletion struct {
	Generation        string    `json:"generation"`
	BootID            string    `json:"bootId"`
	AttemptGeneration uint64    `json:"attemptGeneration"`
	CompletedAt       time.Time `json:"completedAt"`
}

type StateReportRebootRequired struct {
	Required          bool                         `json:"required"`
	Sources           []StateReportRebootSource    `json:"sources,omitempty"`
	Intent            *StateReportRebootIntent     `json:"intent,omitempty"`
	Completion        *StateReportRebootCompletion `json:"completion,omitempty"`
	AttemptGeneration uint64                       `json:"attemptGeneration,omitempty"`
}

type StateReport struct {
	EndpointID      string                       `json:"endpoint_id"`
	Fleet           string                       `json:"fleet"`
	ReleaseRef      string                       `json:"release_ref,omitempty"`
	Digest          string                       `json:"digest,omitempty"`
	ReportedAt      time.Time                    `json:"reported_at,omitempty"`
	InCompliance    bool                         `json:"in_compliance"`
	Status          StateReportStatus            `json:"status"`
	Items           []StateReportItem            `json:"items"`
	Apply           []StateReportApplyItem       `json:"apply,omitempty"`
	ScheduleRuntime []StateReportScheduleRuntime `json:"schedule_runtime,omitempty"`
	RebootRequired  *StateReportRebootRequired   `json:"reboot_required,omitempty"`
	ApplyFailure    *ApplyFailureSummary         `json:"apply_failure,omitempty"`
}

func (r StateReport) HasReport() bool {
	return !r.ReportedAt.IsZero()
}

type FleetStateSummary struct {
	Total       int `json:"total"`
	Compliant   int `json:"compliant"`
	Drift       int `json:"drift"`
	Unsupported int `json:"unsupported"`
	CheckFailed int `json:"check_failed"`
	Deferred    int `json:"deferred"`
	ApplyFailed int `json:"apply_failed"`
	NoReport    int `json:"no_report"`
}

type FleetStateReport struct {
	Fleet     string            `json:"fleet"`
	Summary   FleetStateSummary `json:"summary"`
	Endpoints []StateReport     `json:"endpoints"`
}

type CronJobStatus struct {
	Name             string    `json:"name"`
	Schedule         string    `json:"schedule,omitempty"`
	Applicable       bool      `json:"applicable"`
	LastScheduledFor time.Time `json:"last_scheduled_for,omitempty"`
	LastStatus       string    `json:"last_status,omitempty"`
	LastMessage      string    `json:"last_message,omitempty"`
	LastCompletedAt  time.Time `json:"last_completed_at,omitempty"`
}

type CronReport struct {
	EndpointID  string          `json:"endpoint_id"`
	Fleet       string          `json:"fleet"`
	CronsDigest string          `json:"crons_digest,omitempty"`
	Jobs        []CronJobStatus `json:"jobs"`
}

type CronSummary struct {
	Total      int `json:"total"`
	Applicable int `json:"applicable"`
	Success    int `json:"success"`
	Failed     int `json:"failed"`
	Running    int `json:"running"`
	NeverRun   int `json:"never_run"`
}

type FleetCronReport struct {
	Fleet     string       `json:"fleet"`
	Summary   CronSummary  `json:"summary"`
	Endpoints []CronReport `json:"endpoints"`
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
	return c.BootstrapContext(context.Background(), token)
}

func (c *Client) BootstrapContext(ctx context.Context, token string) (BootstrapResponse, error) {
	body, err := json.Marshal(BootstrapRequest{Token: token})
	if err != nil {
		return BootstrapResponse{}, err
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.BaseURL+"/v1/admin/bootstrap", bytes.NewReader(body))
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
		return BootstrapResponse{}, &BootstrapResponseError{StatusCode: resp.StatusCode, Body: raw}
	}

	var out BootstrapResponse
	if err := json.Unmarshal(raw, &out); err != nil {
		return BootstrapResponse{}, &BootstrapPayloadError{Err: fmt.Errorf("decode bootstrap response: %w", err)}
	}
	if out.OperatorID == "" || out.CertPEM == "" || out.KeyPEM == "" || out.CAPEM == "" {
		return BootstrapResponse{}, &BootstrapPayloadError{Err: errors.New("incomplete bootstrap response")}
	}
	return out, nil
}

func (c *Client) CreateEnrollToken(fleet string, ttl time.Duration) (CreateEnrollTokenResponse, error) {
	return c.CreateEnrollTokenContext(context.Background(), fleet, ttl)
}

func (c *Client) CreateEnrollTokenContext(ctx context.Context, fleet string, ttl time.Duration) (CreateEnrollTokenResponse, error) {
	body, err := json.Marshal(CreateEnrollTokenRequest{
		Fleet:      fleet,
		TTLSeconds: int64(ttl.Seconds()),
	})
	if err != nil {
		return CreateEnrollTokenResponse{}, err
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.BaseURL+"/v1/admin/enroll-tokens", bytes.NewReader(body))
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
		return CreateEnrollTokenResponse{}, &ResponseError{Operation: "create enroll token", StatusCode: resp.StatusCode, Body: raw}
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
	return c.CreateDeploymentTokenContext(context.Background(), label, fleet, ttl)
}

func (c *Client) CreateDeploymentTokenContext(ctx context.Context, label, fleet string, ttl time.Duration) (CreateDeploymentTokenResponse, error) {
	body, err := json.Marshal(CreateDeploymentTokenRequest{
		Label:      label,
		Fleet:      fleet,
		TTLSeconds: int64(ttl.Seconds()),
	})
	if err != nil {
		return CreateDeploymentTokenResponse{}, err
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.BaseURL+"/v1/admin/deployment-tokens", bytes.NewReader(body))
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
	if resp.StatusCode != http.StatusOK {
		return CreateDeploymentTokenResponse{}, &ResponseError{Operation: "create deployment token", StatusCode: resp.StatusCode, Body: raw}
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
	return c.ListDeploymentTokensContext(context.Background())
}

func (c *Client) ListDeploymentTokensContext(ctx context.Context) ([]DeploymentToken, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, c.BaseURL+"/v1/admin/deployment-tokens", nil)
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
		return nil, &ResponseError{Operation: "list deployment tokens", StatusCode: resp.StatusCode, Body: raw}
	}

	var out []DeploymentToken
	if err := json.Unmarshal(raw, &out); err != nil {
		return nil, fmt.Errorf("decode deployment tokens response: %w", err)
	}
	return out, nil
}

func (c *Client) GetDeploymentToken(label string) (DeploymentToken, error) {
	return c.GetDeploymentTokenContext(context.Background(), label)
}

func (c *Client) GetDeploymentTokenContext(ctx context.Context, label string) (DeploymentToken, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, c.BaseURL+"/v1/admin/deployment-tokens/"+url.PathEscape(label), nil)
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
	if resp.StatusCode != http.StatusOK {
		return DeploymentToken{}, &ResponseError{Operation: "get deployment token", StatusCode: resp.StatusCode, Body: raw}
	}

	var out DeploymentToken
	if err := json.Unmarshal(raw, &out); err != nil {
		return DeploymentToken{}, fmt.Errorf("decode deployment token response: %w", err)
	}
	return out, nil
}

func (c *Client) RevokeDeploymentToken(label string) error {
	return c.RevokeDeploymentTokenContext(context.Background(), label)
}

func (c *Client) RevokeDeploymentTokenContext(ctx context.Context, label string) error {
	req, err := http.NewRequestWithContext(ctx, http.MethodDelete, c.BaseURL+"/v1/admin/deployment-tokens/"+url.PathEscape(label), nil)
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
	if resp.StatusCode != http.StatusNoContent {
		return &ResponseError{Operation: "revoke deployment token", StatusCode: resp.StatusCode, Body: raw}
	}
	return nil
}

func (c *Client) TriggerGitSync() error {
	return c.TriggerGitSyncContext(context.Background())
}

func (c *Client) TriggerGitSyncContext(ctx context.Context) error {
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.BaseURL+"/v1/admin/git-sync", nil)
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
		return &ResponseError{Operation: "git sync", StatusCode: resp.StatusCode, Body: raw}
	}
	return nil
}

func (c *Client) ListEndpoints() ([]Endpoint, error) {
	return c.ListEndpointsContext(context.Background())
}

func (c *Client) ListEndpointsContext(ctx context.Context) ([]Endpoint, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, c.BaseURL+"/v1/admin/endpoints", nil)
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
		return nil, &ResponseError{Operation: "list endpoints", StatusCode: resp.StatusCode, Body: raw}
	}

	var out []Endpoint
	if err := json.Unmarshal(raw, &out); err != nil {
		return nil, fmt.Errorf("decode endpoints response: %w", err)
	}
	return out, nil
}

func (c *Client) ListFleets() ([]string, error) {
	return c.ListFleetsContext(context.Background())
}

func (c *Client) ListFleetsContext(ctx context.Context) ([]string, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, c.BaseURL+"/v1/admin/fleets", nil)
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
		return nil, &ResponseError{Operation: "list fleets", StatusCode: resp.StatusCode, Body: raw}
	}

	var out []string
	if err := json.Unmarshal(raw, &out); err != nil {
		return nil, fmt.Errorf("decode fleets response: %w", err)
	}
	return out, nil
}

func (c *Client) GetEndpoint(id string) (Endpoint, error) {
	out, err := c.GetEndpointContext(context.Background(), id)
	var responseError *ResponseError
	if errors.As(err, &responseError) && responseError.StatusCode == http.StatusNotFound {
		return Endpoint{}, fmt.Errorf("endpoint not found")
	}
	return out, err
}

func (c *Client) GetEndpointContext(ctx context.Context, id string) (Endpoint, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, c.BaseURL+"/v1/admin/endpoints/"+url.PathEscape(id), nil)
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
	if resp.StatusCode != http.StatusOK {
		return Endpoint{}, &ResponseError{Operation: "get endpoint", StatusCode: resp.StatusCode, Body: raw}
	}

	var out Endpoint
	if err := json.Unmarshal(raw, &out); err != nil {
		return Endpoint{}, fmt.Errorf("decode endpoint response: %w", err)
	}
	return out, nil
}

func (c *Client) RequestEndpointAgentUpgrade(id, version string) error {
	_, err := c.RequestEndpointAgentUpgradeContext(context.Background(), id, version)
	var responseError *ResponseError
	if errors.As(err, &responseError) {
		if responseError.StatusCode == http.StatusNotFound {
			return fmt.Errorf("endpoint not found")
		}
		return fmt.Errorf("agent upgrade status %d: %s", responseError.StatusCode, responseError.Body)
	}
	return err
}

func (c *Client) RequestEndpointAgentUpgradeContext(ctx context.Context, id, version string) (string, error) {
	body, err := json.Marshal(map[string]string{"version": version})
	if err != nil {
		return "", err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.BaseURL+"/v1/admin/endpoints/"+url.PathEscape(id)+"/agent-upgrade", bytes.NewReader(body))
	if err != nil {
		return "", err
	}
	req.Header.Set("Content-Type", "application/json")
	resp, err := c.HTTPClient.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()
	raw, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", err
	}
	if resp.StatusCode != http.StatusOK {
		return "", &ResponseError{Operation: "request endpoint agent upgrade", StatusCode: resp.StatusCode, Body: raw}
	}
	var out struct {
		Version string `json:"version"`
	}
	if err := json.Unmarshal(raw, &out); err != nil {
		return "", fmt.Errorf("decode endpoint agent upgrade response: %w", err)
	}
	return out.Version, nil
}

func (c *Client) RequestFleetAgentUpgrade(fleet, version string) (int, error) {
	result, err := c.RequestFleetAgentUpgradeContext(context.Background(), fleet, version)
	var responseError *ResponseError
	if errors.As(err, &responseError) {
		return 0, fmt.Errorf("fleet agent upgrade status %d: %s", responseError.StatusCode, responseError.Body)
	}
	return result.Endpoints, err
}

type FleetAgentUpgradeResult struct {
	Version   string `json:"version"`
	Endpoints int    `json:"endpoints"`
}

func (c *Client) RequestFleetAgentUpgradeContext(ctx context.Context, fleet, version string) (FleetAgentUpgradeResult, error) {
	body, err := json.Marshal(map[string]string{"version": version})
	if err != nil {
		return FleetAgentUpgradeResult{}, err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.BaseURL+"/v1/admin/fleets/"+url.PathEscape(fleet)+"/agent-upgrade", bytes.NewReader(body))
	if err != nil {
		return FleetAgentUpgradeResult{}, err
	}
	req.Header.Set("Content-Type", "application/json")
	resp, err := c.HTTPClient.Do(req)
	if err != nil {
		return FleetAgentUpgradeResult{}, err
	}
	defer resp.Body.Close()
	raw, err := io.ReadAll(resp.Body)
	if err != nil {
		return FleetAgentUpgradeResult{}, err
	}
	if resp.StatusCode != http.StatusOK {
		return FleetAgentUpgradeResult{}, &ResponseError{Operation: "request fleet agent upgrade", StatusCode: resp.StatusCode, Body: raw}
	}
	var out FleetAgentUpgradeResult
	if err := json.Unmarshal(raw, &out); err != nil {
		return FleetAgentUpgradeResult{}, fmt.Errorf("decode fleet agent upgrade response: %w", err)
	}
	return out, nil
}

func (c *Client) GetEndpointStateReport(id string) (StateReport, error) {
	out, err := c.GetEndpointStateReportContext(context.Background(), id)
	var responseError *ResponseError
	if errors.As(err, &responseError) && responseError.StatusCode == http.StatusNotFound {
		return StateReport{}, fmt.Errorf("endpoint not found")
	}
	return out, err
}

func (c *Client) GetEndpointStateReportContext(ctx context.Context, id string) (StateReport, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, c.BaseURL+"/v1/admin/endpoints/"+url.PathEscape(id)+"/state-report", nil)
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
	if resp.StatusCode != http.StatusOK {
		return StateReport{}, &ResponseError{Operation: "get endpoint state report", StatusCode: resp.StatusCode, Body: raw}
	}

	var out StateReport
	if err := json.Unmarshal(raw, &out); err != nil {
		return StateReport{}, fmt.Errorf("decode endpoint state report: %w", err)
	}
	return out, nil
}

func (c *Client) GetFleetStateReport(fleet string) (FleetStateReport, error) {
	return c.GetFleetStateReportContext(context.Background(), fleet)
}

func (c *Client) GetFleetStateReportContext(ctx context.Context, fleet string) (FleetStateReport, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, c.BaseURL+"/v1/admin/fleets/"+url.PathEscape(fleet)+"/state-report", nil)
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
		return FleetStateReport{}, &ResponseError{Operation: "get fleet state report", StatusCode: resp.StatusCode, Body: raw}
	}

	var out FleetStateReport
	if err := json.Unmarshal(raw, &out); err != nil {
		return FleetStateReport{}, fmt.Errorf("decode fleet state report: %w", err)
	}
	return out, nil
}

func (c *Client) GetEndpointCronReport(id string) (CronReport, error) {
	out, err := c.GetEndpointCronReportContext(context.Background(), id)
	var responseError *ResponseError
	if errors.As(err, &responseError) && responseError.StatusCode == http.StatusNotFound {
		return CronReport{}, fmt.Errorf("endpoint not found")
	}
	return out, err
}

func (c *Client) GetEndpointCronReportContext(ctx context.Context, id string) (CronReport, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, c.BaseURL+"/v1/admin/endpoints/"+url.PathEscape(id)+"/cron-report", nil)
	if err != nil {
		return CronReport{}, err
	}

	resp, err := c.HTTPClient.Do(req)
	if err != nil {
		return CronReport{}, err
	}
	defer resp.Body.Close()

	raw, err := io.ReadAll(resp.Body)
	if err != nil {
		return CronReport{}, err
	}
	if resp.StatusCode != http.StatusOK {
		return CronReport{}, &ResponseError{Operation: "get endpoint cron report", StatusCode: resp.StatusCode, Body: raw}
	}

	var out CronReport
	if err := json.Unmarshal(raw, &out); err != nil {
		return CronReport{}, fmt.Errorf("decode endpoint cron report: %w", err)
	}
	return out, nil
}

type FirewallAuditReport struct {
	EndpointID string          `json:"endpoint_id"`
	Digest     string          `json:"digest,omitempty"`
	ReportedAt time.Time       `json:"reported_at,omitempty"`
	Report     json.RawMessage `json:"report,omitempty"`
}

func (c *Client) GetEndpointFirewallAudit(id string) (FirewallAuditReport, error) {
	out, err := c.GetEndpointFirewallAuditContext(context.Background(), id)
	var responseError *ResponseError
	if errors.As(err, &responseError) && responseError.StatusCode == http.StatusNotFound {
		return FirewallAuditReport{}, fmt.Errorf("endpoint not found")
	}
	return out, err
}

func (c *Client) GetEndpointFirewallAuditContext(ctx context.Context, id string) (FirewallAuditReport, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, c.BaseURL+"/v1/admin/endpoints/"+url.PathEscape(id)+"/firewall-audit", nil)
	if err != nil {
		return FirewallAuditReport{}, err
	}

	resp, err := c.HTTPClient.Do(req)
	if err != nil {
		return FirewallAuditReport{}, err
	}
	defer resp.Body.Close()

	raw, err := io.ReadAll(resp.Body)
	if err != nil {
		return FirewallAuditReport{}, err
	}
	if resp.StatusCode != http.StatusOK {
		return FirewallAuditReport{}, &ResponseError{Operation: "get endpoint firewall audit", StatusCode: resp.StatusCode, Body: raw}
	}

	var out FirewallAuditReport
	if err := json.Unmarshal(raw, &out); err != nil {
		return FirewallAuditReport{}, fmt.Errorf("decode endpoint firewall audit: %w", err)
	}
	return out, nil
}

func (c *Client) GetFleetCronReport(fleet string) (FleetCronReport, error) {
	return c.GetFleetCronReportContext(context.Background(), fleet)
}

func (c *Client) GetFleetCronReportContext(ctx context.Context, fleet string) (FleetCronReport, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, c.BaseURL+"/v1/admin/fleets/"+url.PathEscape(fleet)+"/cron-report", nil)
	if err != nil {
		return FleetCronReport{}, err
	}

	resp, err := c.HTTPClient.Do(req)
	if err != nil {
		return FleetCronReport{}, err
	}
	defer resp.Body.Close()

	raw, err := io.ReadAll(resp.Body)
	if err != nil {
		return FleetCronReport{}, err
	}
	if resp.StatusCode != http.StatusOK {
		return FleetCronReport{}, &ResponseError{Operation: "get fleet cron report", StatusCode: resp.StatusCode, Body: raw}
	}

	var out FleetCronReport
	if err := json.Unmarshal(raw, &out); err != nil {
		return FleetCronReport{}, fmt.Errorf("decode fleet cron report: %w", err)
	}
	return out, nil
}

func (c *Client) RemoveEndpoint(id string) error {
	err := c.RemoveEndpointContext(context.Background(), id)
	if err == nil {
		return nil
	}
	var responseError *ResponseError
	if errors.As(err, &responseError) && responseError.StatusCode == http.StatusNotFound {
		return fmt.Errorf("endpoint not found")
	}
	return err
}

func (c *Client) RemoveEndpointContext(ctx context.Context, id string) error {
	req, err := http.NewRequestWithContext(ctx, http.MethodDelete, c.BaseURL+"/v1/admin/endpoints/"+url.PathEscape(id), nil)
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
		return &ResponseError{Operation: "remove endpoint", StatusCode: resp.StatusCode, Body: raw}
	}
	if resp.StatusCode != http.StatusNoContent {
		return &ResponseError{Operation: "remove endpoint", StatusCode: resp.StatusCode, Body: raw}
	}
	return nil
}

type EndpointLabelResult struct {
	Key    string            `json:"key"`
	Value  string            `json:"value"`
	Labels map[string]string `json:"labels"`
}

func (c *Client) SetEndpointLabel(id, key, value string) (EndpointLabelResult, error) {
	out, err := c.SetEndpointLabelContext(context.Background(), id, key, value)
	var responseError *ResponseError
	if errors.As(err, &responseError) {
		if responseError.StatusCode == http.StatusNotFound {
			return EndpointLabelResult{}, fmt.Errorf("endpoint not found")
		}
		return EndpointLabelResult{}, fmt.Errorf("set endpoint label status %d: %s", responseError.StatusCode, responseError.Body)
	}
	return out, err
}

func (c *Client) SetEndpointLabelContext(ctx context.Context, id, key, value string) (EndpointLabelResult, error) {
	body, err := json.Marshal(map[string]string{"value": value})
	if err != nil {
		return EndpointLabelResult{}, err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPut, c.BaseURL+"/v1/admin/endpoints/"+url.PathEscape(id)+"/labels/"+url.PathEscape(key), bytes.NewReader(body))
	if err != nil {
		return EndpointLabelResult{}, err
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := c.HTTPClient.Do(req)
	if err != nil {
		return EndpointLabelResult{}, err
	}
	defer resp.Body.Close()

	raw, err := io.ReadAll(resp.Body)
	if err != nil {
		return EndpointLabelResult{}, err
	}
	if resp.StatusCode != http.StatusOK {
		return EndpointLabelResult{}, &ResponseError{Operation: "set endpoint label", StatusCode: resp.StatusCode, Body: raw}
	}
	var out EndpointLabelResult
	if err := json.Unmarshal(raw, &out); err != nil {
		return EndpointLabelResult{}, fmt.Errorf("decode endpoint label response: %w", err)
	}
	return out, nil
}

func (c *Client) DeleteEndpointLabel(id, key string) error {
	err := c.DeleteEndpointLabelContext(context.Background(), id, key)
	var responseError *ResponseError
	if errors.As(err, &responseError) {
		if responseError.StatusCode == http.StatusNotFound {
			return fmt.Errorf("endpoint or label not found")
		}
		return fmt.Errorf("delete endpoint label status %d: %s", responseError.StatusCode, responseError.Body)
	}
	return err
}

func (c *Client) DeleteEndpointLabelContext(ctx context.Context, id, key string) error {
	req, err := http.NewRequestWithContext(ctx, http.MethodDelete, c.BaseURL+"/v1/admin/endpoints/"+url.PathEscape(id)+"/labels/"+url.PathEscape(key), nil)
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
	if resp.StatusCode != http.StatusNoContent {
		return &ResponseError{Operation: "delete endpoint label", StatusCode: resp.StatusCode, Body: raw}
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

type AppPackage struct {
	ID        string               `json:"id"`
	Name      string               `json:"name"`
	Version   string               `json:"version"`
	S3Key     string               `json:"s3_key"`
	SHA256    string               `json:"sha256"`
	Manifest  apppackages.Manifest `json:"manifest"`
	CreatedAt time.Time            `json:"created_at"`
}

type CreateAppPackageRequest struct {
	Name     string               `json:"name"`
	Version  string               `json:"version"`
	S3Key    string               `json:"s3_key"`
	SHA256   string               `json:"sha256"`
	Manifest apppackages.Manifest `json:"manifest"`
}

func (c *Client) ListAuditEvents(opts AuditListOptions) (AuditEventPage, error) {
	return c.ListAuditEventsContext(context.Background(), opts)
}

func (c *Client) ListAuditEventsContext(ctx context.Context, opts AuditListOptions) (AuditEventPage, error) {
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

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint, nil)
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
		return AuditEventPage{}, &ResponseError{Operation: "list audit events", StatusCode: resp.StatusCode, Body: raw}
	}

	var out AuditEventPage
	if err := json.Unmarshal(raw, &out); err != nil {
		return AuditEventPage{}, fmt.Errorf("decode audit events: %w", err)
	}
	return out, nil
}

func (c *Client) GetAuditExportInfo() (AuditExportInfo, error) {
	return c.GetAuditExportInfoContext(context.Background())
}

func (c *Client) GetAuditExportInfoContext(ctx context.Context) (AuditExportInfo, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, c.BaseURL+"/v1/admin/audit-export", nil)
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
		return AuditExportInfo{}, &ResponseError{Operation: "get audit export info", StatusCode: resp.StatusCode, Body: raw}
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

type OperatorMeResponseError struct {
	StatusCode int
	Body       []byte
}

func (e *OperatorMeResponseError) Error() string {
	return fmt.Sprintf("operator me status %d: %s", e.StatusCode, e.Body)
}

func (c *Client) GetOperatorMe() (OperatorMe, error) {
	return c.GetOperatorMeContext(context.Background())
}

func (c *Client) GetOperatorMeContext(ctx context.Context) (OperatorMe, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, c.BaseURL+"/v1/admin/me", nil)
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
		return OperatorMe{}, &OperatorMeResponseError{StatusCode: resp.StatusCode, Body: raw}
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

func (c *Client) UploadAppPackage(data []byte, s3Key string) (AppPackage, error) {
	u := c.BaseURL + "/v1/admin/app-packages/upload"
	if s3Key != "" {
		u += "?s3_key=" + url.QueryEscape(s3Key)
	}
	httpReq, err := http.NewRequest(http.MethodPost, u, bytes.NewReader(data))
	if err != nil {
		return AppPackage{}, err
	}
	httpReq.Header.Set("Content-Type", "application/zip")
	resp, err := c.HTTPClient.Do(httpReq)
	if err != nil {
		return AppPackage{}, err
	}
	defer resp.Body.Close()
	raw, err := io.ReadAll(resp.Body)
	if err != nil {
		return AppPackage{}, err
	}
	if resp.StatusCode != http.StatusOK {
		return AppPackage{}, fmt.Errorf("upload app package status %d: %s", resp.StatusCode, raw)
	}
	var out AppPackage
	if err := json.Unmarshal(raw, &out); err != nil {
		return AppPackage{}, err
	}
	return out, nil
}

func (c *Client) CreateAppPackage(req CreateAppPackageRequest) (AppPackage, error) {
	body, err := json.Marshal(req)
	if err != nil {
		return AppPackage{}, err
	}
	httpReq, err := http.NewRequest(http.MethodPost, c.BaseURL+"/v1/admin/app-packages", bytes.NewReader(body))
	if err != nil {
		return AppPackage{}, err
	}
	httpReq.Header.Set("Content-Type", "application/json")
	resp, err := c.HTTPClient.Do(httpReq)
	if err != nil {
		return AppPackage{}, err
	}
	defer resp.Body.Close()
	raw, err := io.ReadAll(resp.Body)
	if err != nil {
		return AppPackage{}, err
	}
	if resp.StatusCode != http.StatusOK {
		return AppPackage{}, fmt.Errorf("create app package status %d: %s", resp.StatusCode, raw)
	}
	var out AppPackage
	if err := json.Unmarshal(raw, &out); err != nil {
		return AppPackage{}, err
	}
	return out, nil
}

func (c *Client) ListAppPackages(namePrefix string) ([]AppPackage, error) {
	u := c.BaseURL + "/v1/admin/app-packages"
	if namePrefix != "" {
		u += "?name=" + url.QueryEscape(namePrefix)
	}
	resp, err := c.HTTPClient.Get(u)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	raw, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("list app packages status %d: %s", resp.StatusCode, raw)
	}
	var out []AppPackage
	if err := json.Unmarshal(raw, &out); err != nil {
		return nil, err
	}
	return out, nil
}

func (c *Client) GetAppPackage(name, version string) (AppPackage, error) {
	u := c.BaseURL + "/v1/admin/app-packages/detail?name=" + url.QueryEscape(name) + "&version=" + url.QueryEscape(version)
	resp, err := c.HTTPClient.Get(u)
	if err != nil {
		return AppPackage{}, err
	}
	defer resp.Body.Close()
	raw, err := io.ReadAll(resp.Body)
	if err != nil {
		return AppPackage{}, err
	}
	if resp.StatusCode != http.StatusOK {
		return AppPackage{}, fmt.Errorf("get app package status %d: %s", resp.StatusCode, raw)
	}
	var out AppPackage
	if err := json.Unmarshal(raw, &out); err != nil {
		return AppPackage{}, err
	}
	return out, nil
}

func (c *Client) DeleteAppPackage(name, version string, deleteObject bool) error {
	u := c.BaseURL + "/v1/admin/app-packages/detail?name=" + url.QueryEscape(name) + "&version=" + url.QueryEscape(version)
	if deleteObject {
		u += "&delete_object=true"
	}
	req, err := http.NewRequest(http.MethodDelete, u, nil)
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
	if resp.StatusCode != http.StatusNoContent {
		return fmt.Errorf("delete app package status %d: %s", resp.StatusCode, raw)
	}
	return nil
}

// DiagnosticRequest is a server-side diagnostic collection job.
type DiagnosticRequest struct {
	ID           string         `json:"id"`
	EndpointID   string         `json:"endpoint_id"`
	RequestedBy  string         `json:"requested_by,omitempty"`
	Status       string         `json:"status"`
	Spec         DiagnosticSpec `json:"spec"`
	SHA256       string         `json:"sha256,omitempty"`
	SizeBytes    int64          `json:"size_bytes,omitempty"`
	ErrorMessage string         `json:"error_message,omitempty"`
	CreatedAt    time.Time      `json:"created_at"`
	DispatchedAt *time.Time     `json:"dispatched_at,omitempty"`
	CompletedAt  *time.Time     `json:"completed_at,omitempty"`
	ExpiresAt    time.Time      `json:"expires_at"`
}

// DiagnosticSpec is the validated collection parameters.
type DiagnosticSpec struct {
	Collectors []string  `json:"collectors"`
	Since      time.Time `json:"since"`
	Until      time.Time `json:"until"`
}

// CollectDiagnosticsOptions configures a diagnostic collection request.
type CollectDiagnosticsOptions struct {
	Collectors []string
	Since      time.Time
	Until      time.Time
}

func (c *Client) RequestDiagnosticsCollect(endpointID string, opts CollectDiagnosticsOptions) (DiagnosticRequest, error) {
	out, err := c.RequestDiagnosticsCollectContext(context.Background(), endpointID, opts)
	if err == nil {
		return out, nil
	}
	var responseError *ResponseError
	if errors.As(err, &responseError) {
		switch responseError.StatusCode {
		case http.StatusNotFound:
			return DiagnosticRequest{}, fmt.Errorf("endpoint not found")
		case http.StatusConflict:
			return DiagnosticRequest{}, fmt.Errorf("endpoint already has an active diagnostic request")
		}
	}
	return DiagnosticRequest{}, err
}

func (c *Client) RequestDiagnosticsCollectContext(ctx context.Context, endpointID string, opts CollectDiagnosticsOptions) (DiagnosticRequest, error) {
	body, err := json.Marshal(map[string]any{
		"collectors": opts.Collectors,
		"since":      opts.Since,
		"until":      opts.Until,
	})
	if err != nil {
		return DiagnosticRequest{}, err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.BaseURL+"/v1/admin/endpoints/"+url.PathEscape(endpointID)+"/diagnostics/collect", bytes.NewReader(body))
	if err != nil {
		return DiagnosticRequest{}, err
	}
	req.Header.Set("Content-Type", "application/json")
	resp, err := c.HTTPClient.Do(req)
	if err != nil {
		return DiagnosticRequest{}, err
	}
	defer resp.Body.Close()
	raw, err := io.ReadAll(resp.Body)
	if err != nil {
		return DiagnosticRequest{}, err
	}
	if resp.StatusCode != http.StatusOK {
		return DiagnosticRequest{}, &ResponseError{
			Operation:  "diagnostics collect",
			StatusCode: resp.StatusCode,
			Body:       raw,
		}
	}
	var out DiagnosticRequest
	if err := json.Unmarshal(raw, &out); err != nil {
		return DiagnosticRequest{}, err
	}
	return out, nil
}

func (c *Client) GetDiagnosticRequest(requestID string) (DiagnosticRequest, error) {
	out, err := c.GetDiagnosticRequestContext(context.Background(), requestID)
	if err == nil {
		return out, nil
	}
	var responseError *ResponseError
	if errors.As(err, &responseError) && responseError.StatusCode == http.StatusNotFound {
		return DiagnosticRequest{}, fmt.Errorf("diagnostic request not found")
	}
	return DiagnosticRequest{}, err
}

func (c *Client) GetDiagnosticRequestContext(ctx context.Context, requestID string) (DiagnosticRequest, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, c.BaseURL+"/v1/admin/diagnostics/"+url.PathEscape(requestID), nil)
	if err != nil {
		return DiagnosticRequest{}, err
	}
	resp, err := c.HTTPClient.Do(req)
	if err != nil {
		return DiagnosticRequest{}, err
	}
	defer resp.Body.Close()
	raw, err := io.ReadAll(resp.Body)
	if err != nil {
		return DiagnosticRequest{}, err
	}
	if resp.StatusCode != http.StatusOK {
		return DiagnosticRequest{}, &ResponseError{
			Operation:  "get diagnostic request",
			StatusCode: resp.StatusCode,
			Body:       raw,
		}
	}
	var out DiagnosticRequest
	if err := json.Unmarshal(raw, &out); err != nil {
		return DiagnosticRequest{}, err
	}
	return out, nil
}

func (c *Client) DownloadDiagnosticBundle(requestID string) ([]byte, error) {
	var bundle bytes.Buffer
	_, err := c.DownloadDiagnosticBundleToContext(context.Background(), requestID, &bundle)
	if err == nil {
		return bundle.Bytes(), nil
	}
	var responseError *ResponseError
	if errors.As(err, &responseError) {
		switch responseError.StatusCode {
		case http.StatusNotFound:
			return nil, fmt.Errorf("diagnostic bundle not found")
		case http.StatusConflict:
			return nil, fmt.Errorf("diagnostic bundle not ready")
		}
	}
	return nil, err
}

func (c *Client) DownloadDiagnosticBundleToContext(ctx context.Context, requestID string, destination io.Writer) (int64, error) {
	if destination == nil {
		return 0, errors.New("diagnostic bundle destination is required")
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, c.BaseURL+"/v1/admin/diagnostics/"+url.PathEscape(requestID)+"/download", nil)
	if err != nil {
		return 0, err
	}
	resp, err := c.HTTPClient.Do(req)
	if err != nil {
		return 0, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		raw, readErr := io.ReadAll(resp.Body)
		if readErr != nil {
			return 0, readErr
		}
		return 0, &ResponseError{
			Operation:  "download diagnostic bundle",
			StatusCode: resp.StatusCode,
			Body:       raw,
		}
	}
	return io.Copy(destination, resp.Body)
}
