package diagnostics

import (
	"bytes"
	"context"
	"crypto/tls"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"time"
)

const uploadHTTPTimeout = 5 * time.Minute

// UploadClient requests presigned upload URLs from the server and PUTs bundles to object storage.
type UploadClient struct {
	BaseURL      string
	serverClient *http.Client
	storeClient  *http.Client
}

func NewUploadClient(baseURL string, serverTLS *tls.Config) *UploadClient {
	var serverTransport http.RoundTripper = http.DefaultTransport
	if serverTLS != nil {
		serverTransport = &http.Transport{TLSClientConfig: serverTLS.Clone()}
	}
	return &UploadClient{
		BaseURL: baseURL,
		serverClient: &http.Client{
			Transport: serverTransport,
			Timeout:   uploadHTTPTimeout,
		},
		storeClient: &http.Client{
			Transport: &http.Transport{
				// Nil RootCAs uses the system trust store for public object-store endpoints.
				TLSClientConfig: &tls.Config{MinVersion: tls.VersionTLS12},
			},
			Timeout: uploadHTTPTimeout,
		},
	}
}

type uploadURLResponse struct {
	URL string `json:"url"`
	Key string `json:"key"`
}

// Upload stores bundle bytes via a presigned PUT URL from the server.
func (c *UploadClient) Upload(ctx context.Context, requestID string, bundle Bundle) error {
	putURL, err := c.requestUploadURL(requestID)
	if err != nil {
		return err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPut, putURL, bytes.NewReader(bundle.Data))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/gzip")
	req.ContentLength = bundle.Size

	resp, err := c.storeClient.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		body, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("upload status %d: %s", resp.StatusCode, body)
	}
	return nil
}

func (c *UploadClient) requestUploadURL(requestID string) (string, error) {
	body, err := json.Marshal(map[string]string{"requestId": requestID})
	if err != nil {
		return "", err
	}
	req, err := http.NewRequest(http.MethodPost, c.BaseURL+"/v1/diagnostics/upload-url", bytes.NewReader(body))
	if err != nil {
		return "", err
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := c.serverClient.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()
	raw, _ := io.ReadAll(resp.Body)
	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("upload-url status %d: %s", resp.StatusCode, raw)
	}
	var out uploadURLResponse
	if err := json.Unmarshal(raw, &out); err != nil {
		return "", err
	}
	if out.URL == "" {
		return "", fmt.Errorf("empty upload url")
	}
	return out.URL, nil
}
