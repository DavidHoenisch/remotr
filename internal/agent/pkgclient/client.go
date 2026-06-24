package pkgclient

import (
	"bytes"
	"context"
	"crypto/tls"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"time"

	"github.com/DavidHoenisch/remotr/internal/apppackages"
)

// Client resolves presigned download URLs from the Remotr server.
type Client struct {
	BaseURL    string
	HTTPClient *http.Client
}

// New builds an mTLS client for package download URL grants.
func New(baseURL string, tlsCfg *tls.Config) *Client {
	return &Client{
		BaseURL: baseURL,
		HTTPClient: &http.Client{
			Transport: &http.Transport{TLSClientConfig: tlsCfg},
			Timeout:   60 * time.Second,
		},
	}
}

type downloadURLRequest struct {
	Name    string `json:"name"`
	Version string `json:"version"`
}

type downloadURLResponse struct {
	URL       string    `json:"url"`
	SHA256    string    `json:"sha256"`
	ExpiresAt time.Time `json:"expires_at"`
}

func (c *Client) DownloadURL(ctx context.Context, name, version string) (apppackages.DownloadURL, error) {
	body, err := json.Marshal(downloadURLRequest{Name: name, Version: version})
	if err != nil {
		return apppackages.DownloadURL{}, err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.BaseURL+"/v1/app-packages/download-url", bytes.NewReader(body))
	if err != nil {
		return apppackages.DownloadURL{}, err
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := c.HTTPClient.Do(req)
	if err != nil {
		return apppackages.DownloadURL{}, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		b, _ := io.ReadAll(resp.Body)
		return apppackages.DownloadURL{}, fmt.Errorf("download-url status %d: %s", resp.StatusCode, b)
	}
	var out downloadURLResponse
	if err := json.NewDecoder(resp.Body).Decode(&out); err != nil {
		return apppackages.DownloadURL{}, err
	}
	return apppackages.DownloadURL{
		URL:       out.URL,
		SHA256:    out.SHA256,
		ExpiresAt: out.ExpiresAt,
	}, nil
}
