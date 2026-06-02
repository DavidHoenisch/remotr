package admin

import (
	"bytes"
	"crypto/tls"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
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

type Endpoint struct {
	ID              string            `json:"id"`
	Fleet           string            `json:"fleet"`
	CertFingerprint string            `json:"cert_fingerprint,omitempty"`
	Labels          map[string]string `json:"labels,omitempty"`
	LastDrift       *DriftSummary     `json:"last_drift,omitempty"`
}

type DriftSummary struct {
	ReleaseRef string    `json:"release_ref"`
	Digest     string    `json:"digest"`
	ReportedAt time.Time `json:"reported_at"`
}

type Client struct {
	BaseURL    string
	StateDir   string
	HTTPClient *http.Client
}

func NewClient(baseURL, stateDir string, tlsCfg *tls.Config) *Client {
	return &Client{
		BaseURL:  baseURL,
		StateDir: stateDir,
		HTTPClient: &http.Client{
			Transport: &http.Transport{TLSClientConfig: tlsCfg},
		},
	}
}

func NewClientFromState(baseURL, stateDir string) (*Client, error) {
	layout, err := opcreds.Layout(stateDir)
	if err != nil {
		return nil, err
	}
	tlsCfg, err := tlsconfig.ClientTLSConfig(layout.Cert, layout.Key, layout.CA)
	if err != nil {
		return nil, err
	}
	return NewClient(baseURL, stateDir, tlsCfg), nil
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
