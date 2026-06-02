package enroll

import (
	"bytes"
	"crypto/tls"
	"encoding/json"
	"fmt"
	"io"
	"net/http"

	"github.com/DavidHoenisch/remotr/internal/pki"
)

type Request struct {
	Token      string `json:"token"`
	CSRPEM     string `json:"csr_pem,omitempty"`
	EndpointID string `json:"endpoint_id,omitempty"`
}

type Response struct {
	EndpointID string `json:"endpoint_id"`
	CertPEM    string `json:"cert_pem"`
	KeyPEM     string `json:"key_pem,omitempty"`
	CAPEM      string `json:"ca_pem"`
}

type Client struct {
	BaseURL    string
	HTTPClient *http.Client
}

func NewClient(baseURL string, tlsCfg *tls.Config) *Client {
	return &Client{
		BaseURL: baseURL,
		HTTPClient: &http.Client{
			Transport: &http.Transport{TLSClientConfig: tlsCfg},
		},
	}
}

// Enroll exchanges a one-time token for endpoint credentials using a locally generated key and CSR.
func (c *Client) Enroll(token string, endpointID string) (Response, error) {
	keyPEM, csrPEM, err := pki.GenerateEndpointCSR()
	if err != nil {
		return Response{}, fmt.Errorf("generate csr: %w", err)
	}

	resp, err := c.enroll(Request{Token: token, CSRPEM: string(csrPEM), EndpointID: endpointID})
	if err != nil {
		return Response{}, err
	}
	if resp.KeyPEM != "" {
		return Response{}, fmt.Errorf("unexpected key_pem in csr enroll response")
	}
	resp.KeyPEM = string(keyPEM)
	return resp, nil
}

// EnrollWithServerKey exchanges a token for server-generated credentials (legacy path).
func (c *Client) EnrollWithServerKey(token string, endpointID string) (Response, error) {
	resp, err := c.enroll(Request{Token: token, EndpointID: endpointID})
	if err != nil {
		return Response{}, err
	}
	if resp.KeyPEM == "" {
		return Response{}, fmt.Errorf("incomplete enroll response: missing key_pem")
	}
	return resp, nil
}

func (c *Client) enroll(req Request) (Response, error) {
	body, err := json.Marshal(req)
	if err != nil {
		return Response{}, err
	}

	httpReq, err := http.NewRequest(http.MethodPost, c.BaseURL+"/v1/enroll", bytes.NewReader(body))
	if err != nil {
		return Response{}, err
	}
	httpReq.Header.Set("Content-Type", "application/json")

	resp, err := c.HTTPClient.Do(httpReq)
	if err != nil {
		return Response{}, err
	}
	defer resp.Body.Close()

	raw, err := io.ReadAll(resp.Body)
	if err != nil {
		return Response{}, err
	}
	if resp.StatusCode != http.StatusOK {
		return Response{}, fmt.Errorf("enroll status %d: %s", resp.StatusCode, raw)
	}

	var out Response
	if err := json.Unmarshal(raw, &out); err != nil {
		return Response{}, fmt.Errorf("decode enroll response: %w", err)
	}
	if out.EndpointID == "" || out.CertPEM == "" || out.CAPEM == "" {
		return Response{}, fmt.Errorf("incomplete enroll response")
	}
	return out, nil
}
