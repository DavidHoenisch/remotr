package admin

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/url"

	"github.com/DavidHoenisch/remotr/internal/changecontrol"
)

type ChangeRequest = changecontrol.ChangeRequest
type RolloutSpec = changecontrol.RolloutSpec
type RecurringWindow = changecontrol.RecurringWindow
type RolloutAuthorization = changecontrol.RolloutAuthorization
type BaselineAuthorization = changecontrol.BaselineAuthorization
type FleetPlan = changecontrol.FleetPlan
type ResourcePlan = changecontrol.ResourcePlan
type PredictedEffect = changecontrol.PredictedEffect

func (c *Client) ListChangeRequests() ([]ChangeRequest, error) {
	return c.ListChangeRequestsContext(context.Background())
}

func (c *Client) ListChangeRequestsContext(ctx context.Context) ([]ChangeRequest, error) {
	var out []ChangeRequest
	if err := c.changeControlJSONContext(ctx, http.MethodGet, "/v1/admin/change-requests", nil, &out); err != nil {
		return nil, err
	}
	return out, nil
}

func (c *Client) GetChangeRequest(id string) (ChangeRequest, error) {
	return c.GetChangeRequestContext(context.Background(), id)
}

func (c *Client) GetChangeRequestContext(ctx context.Context, id string) (ChangeRequest, error) {
	var out ChangeRequest
	err := c.changeControlJSONContext(ctx, http.MethodGet, "/v1/admin/change-requests/"+url.PathEscape(id), nil, &out)
	return out, err
}

func (c *Client) AuthorizeChangeRequest(id string, spec RolloutSpec, justification string) (RolloutAuthorization, error) {
	return c.AuthorizeChangeRequestContext(context.Background(), id, spec, justification)
}

func (c *Client) AuthorizeChangeRequestContext(ctx context.Context, id string, spec RolloutSpec, justification string) (RolloutAuthorization, error) {
	body := struct {
		RolloutSpec
		Justification string `json:"justification"`
	}{RolloutSpec: spec, Justification: justification}
	var out RolloutAuthorization
	err := c.changeControlJSONContext(ctx, http.MethodPost, "/v1/admin/change-requests/"+url.PathEscape(id)+"/authorize", body, &out)
	return out, err
}

func (c *Client) ChangeRequestLifecycle(id, action string) (ChangeRequest, error) {
	return c.ChangeRequestLifecycleContext(context.Background(), id, action)
}

func (c *Client) ChangeRequestLifecycleContext(ctx context.Context, id, action string) (ChangeRequest, error) {
	switch action {
	case "pause", "resume", "revoke":
	default:
		return ChangeRequest{}, &ResponseError{Operation: "change control", StatusCode: http.StatusBadRequest, Body: []byte("unsupported lifecycle action")}
	}
	var out ChangeRequest
	err := c.changeControlJSONContext(ctx, http.MethodPost, "/v1/admin/change-requests/"+url.PathEscape(id)+"/"+action, struct{}{}, &out)
	return out, err
}

func (c *Client) PromoteChangeBaseline(id, resourceAddress string, acknowledgeExceptions bool) (BaselineAuthorization, error) {
	return c.PromoteChangeBaselineContext(context.Background(), id, resourceAddress, acknowledgeExceptions)
}

func (c *Client) PromoteChangeBaselineContext(ctx context.Context, id, resourceAddress string, acknowledgeExceptions bool) (BaselineAuthorization, error) {
	var out BaselineAuthorization
	err := c.changeControlJSONContext(ctx, http.MethodPost, "/v1/admin/change-requests/"+url.PathEscape(id)+"/baseline", map[string]any{"resource_address": resourceAddress, "acknowledge_exceptions": acknowledgeExceptions}, &out)
	return out, err
}

func (c *Client) CreateBaselineAdoption(fleet string) (ChangeRequest, error) {
	return c.CreateBaselineAdoptionContext(context.Background(), fleet)
}

func (c *Client) CreateBaselineAdoptionContext(ctx context.Context, fleet string) (ChangeRequest, error) {
	var out ChangeRequest
	err := c.changeControlJSONContext(ctx, http.MethodPost, "/v1/admin/fleets/"+url.PathEscape(fleet)+"/baseline-adoptions", struct{}{}, &out)
	return out, err
}

func (c *Client) changeControlJSON(method, path string, body, out any) error {
	return c.changeControlJSONContext(context.Background(), method, path, body, out)
}

func (c *Client) changeControlJSONContext(ctx context.Context, method, path string, body, out any) error {
	var reader io.Reader
	if body != nil {
		raw, err := json.Marshal(body)
		if err != nil {
			return err
		}
		reader = bytes.NewReader(raw)
	}
	req, err := http.NewRequestWithContext(ctx, method, c.BaseURL+path, reader)
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
	raw, err := io.ReadAll(resp.Body)
	if err != nil {
		return err
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return &ResponseError{Operation: "change control", StatusCode: resp.StatusCode, Body: raw}
	}
	if out == nil || len(raw) == 0 {
		return nil
	}
	return json.Unmarshal(raw, out)
}
