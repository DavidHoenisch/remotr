package admin

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"

	"github.com/DavidHoenisch/remotr/internal/secrets"
)

type SecretVersionMetadata = secrets.VersionMetadata

func (c *Client) UploadSecretVersion(name, fleet, endpointID string, material []byte) (SecretVersionMetadata, error) {
	return c.UploadSecretVersionContext(context.Background(), name, fleet, endpointID, material)
}

func (c *Client) UploadSecretVersionContext(ctx context.Context, name, fleet, endpointID string, material []byte) (SecretVersionMetadata, error) {
	query := url.Values{"name": []string{name}}
	if fleet != "" {
		query.Set("fleet", fleet)
	}
	if endpointID != "" {
		query.Set("endpoint_id", endpointID)
	}
	request, err := http.NewRequestWithContext(ctx, http.MethodPost, c.BaseURL+"/v1/admin/secrets/versions?"+query.Encode(), bytes.NewReader(material))
	if err != nil {
		return SecretVersionMetadata{}, err
	}
	request.Header.Set("Content-Type", "application/octet-stream")
	return c.doSecretMetadata(request, http.StatusCreated, "upload secret version")
}

func (c *Client) ListSecretVersions(name string) ([]SecretVersionMetadata, error) {
	return c.ListSecretVersionsContext(context.Background(), name)
}

func (c *Client) ListSecretVersionsContext(ctx context.Context, name string) ([]SecretVersionMetadata, error) {
	request, err := http.NewRequestWithContext(ctx, http.MethodGet, c.BaseURL+"/v1/admin/secrets?name="+url.QueryEscape(name), nil)
	if err != nil {
		return nil, err
	}
	response, err := c.HTTPClient.Do(request)
	if err != nil {
		return nil, err
	}
	defer response.Body.Close()
	raw, err := io.ReadAll(io.LimitReader(response.Body, 2<<20))
	if err != nil {
		return nil, err
	}
	if response.StatusCode != http.StatusOK {
		return nil, &ResponseError{Operation: "list secret versions", StatusCode: response.StatusCode, Body: raw}
	}
	var metadata []SecretVersionMetadata
	if err := json.Unmarshal(raw, &metadata); err != nil {
		return nil, fmt.Errorf("decode secret versions: %w", err)
	}
	return metadata, nil
}

func (c *Client) ActivateSecretVersion(name, version string) (SecretVersionMetadata, error) {
	return c.ActivateSecretVersionContext(context.Background(), name, version)
}

func (c *Client) ActivateSecretVersionContext(ctx context.Context, name, version string) (SecretVersionMetadata, error) {
	return c.secretVersionLifecycle(ctx, "activate", name, version)
}

func (c *Client) RevokeSecretVersion(name, version string) (SecretVersionMetadata, error) {
	return c.RevokeSecretVersionContext(context.Background(), name, version)
}

func (c *Client) RevokeSecretVersionContext(ctx context.Context, name, version string) (SecretVersionMetadata, error) {
	return c.secretVersionLifecycle(ctx, "revoke", name, version)
}

func (c *Client) secretVersionLifecycle(ctx context.Context, action, name, version string) (SecretVersionMetadata, error) {
	body, err := json.Marshal(map[string]string{"name": name, "version": version})
	if err != nil {
		return SecretVersionMetadata{}, err
	}
	request, err := http.NewRequestWithContext(ctx, http.MethodPost, c.BaseURL+"/v1/admin/secrets/"+action, bytes.NewReader(body))
	if err != nil {
		return SecretVersionMetadata{}, err
	}
	request.Header.Set("Content-Type", "application/json")
	return c.doSecretMetadata(request, http.StatusOK, action+" secret version")
}

func (c *Client) doSecretMetadata(request *http.Request, expectedStatus int, operation string) (SecretVersionMetadata, error) {
	response, err := c.HTTPClient.Do(request)
	if err != nil {
		return SecretVersionMetadata{}, err
	}
	defer response.Body.Close()
	raw, err := io.ReadAll(io.LimitReader(response.Body, 1<<20))
	if err != nil {
		return SecretVersionMetadata{}, err
	}
	if response.StatusCode != expectedStatus {
		return SecretVersionMetadata{}, &ResponseError{Operation: operation, StatusCode: response.StatusCode, Body: raw}
	}
	var metadata SecretVersionMetadata
	if err := json.Unmarshal(raw, &metadata); err != nil {
		return SecretVersionMetadata{}, fmt.Errorf("decode secret version metadata: %w", err)
	}
	return metadata, nil
}
