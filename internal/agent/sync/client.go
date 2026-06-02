package sync

import (
	"bytes"
	"compress/gzip"
	"crypto/tls"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
)

type Client struct {
	BaseURL    string
	HTTPClient *http.Client
}

type Request struct {
	LastDigest string `json:"lastDigest"`
}

type Response struct {
	Unchanged          bool   `json:"unchanged"`
	ReleaseRef         string `json:"releaseRef,omitempty"`
	Digest             string `json:"digest,omitempty"`
	ArtifactYAML       []byte `json:"artifactYaml,omitempty"`
	RemediationPolicy  string `json:"remediationPolicy,omitempty"`
}

func NewClient(baseURL string, tlsCfg *tls.Config) *Client {
	return &Client{
		BaseURL: baseURL,
		HTTPClient: &http.Client{
			Transport: &http.Transport{TLSClientConfig: tlsCfg},
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
		b, _ := io.ReadAll(resp.Body)
		return Response{}, fmt.Errorf("sync status %d: %s", resp.StatusCode, b)
	}

	var reader io.Reader = resp.Body
	if resp.Header.Get("Content-Encoding") == "gzip" {
		gz, err := gzip.NewReader(resp.Body)
		if err != nil {
			return Response{}, fmt.Errorf("gzip reader: %w", err)
		}
		defer gz.Close()
		reader = gz
	}

	var out Response
	if err := json.NewDecoder(reader).Decode(&out); err != nil {
		return Response{}, err
	}
	return out, nil
}
