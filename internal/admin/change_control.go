package admin

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"

	"github.com/DavidHoenisch/remotr/internal/changecontrol"
)

type ChangeRequest = changecontrol.ChangeRequest
type RolloutSpec = changecontrol.RolloutSpec
type RolloutAuthorization = changecontrol.RolloutAuthorization
type BaselineAuthorization = changecontrol.BaselineAuthorization
type FleetPlan = changecontrol.FleetPlan

func (c *Client) ListChangeRequests() ([]ChangeRequest, error) {
	var out []ChangeRequest
	if err := c.changeControlJSON(http.MethodGet, "/v1/admin/change-requests", nil, &out); err != nil {
		return nil, err
	}
	return out, nil
}

func (c *Client) GetChangeRequest(id string) (ChangeRequest, error) {
	var out ChangeRequest
	err := c.changeControlJSON(http.MethodGet, "/v1/admin/change-requests/"+id, nil, &out)
	return out, err
}

func (c *Client) AuthorizeChangeRequest(id string, spec RolloutSpec, justification string) (RolloutAuthorization, error) {
	body := struct {
		RolloutSpec
		Justification string `json:"justification"`
	}{RolloutSpec: spec, Justification: justification}
	var out RolloutAuthorization
	err := c.changeControlJSON(http.MethodPost, "/v1/admin/change-requests/"+id+"/authorize", body, &out)
	return out, err
}

func (c *Client) ChangeRequestLifecycle(id, action string) (ChangeRequest, error) {
	var out ChangeRequest
	err := c.changeControlJSON(http.MethodPost, "/v1/admin/change-requests/"+id+"/"+action, struct{}{}, &out)
	return out, err
}

func (c *Client) PromoteChangeBaseline(id, resourceAddress string, acknowledgeExceptions bool) (BaselineAuthorization, error) {
	var out BaselineAuthorization
	err := c.changeControlJSON(http.MethodPost, "/v1/admin/change-requests/"+id+"/baseline", map[string]any{"resource_address": resourceAddress, "acknowledge_exceptions": acknowledgeExceptions}, &out)
	return out, err
}

func (c *Client) CreateBaselineAdoption(fleet string, plan FleetPlan) (ChangeRequest, error) {
	var out ChangeRequest
	err := c.changeControlJSON(http.MethodPost, "/v1/admin/fleets/"+fleet+"/baseline-adoptions", plan, &out)
	return out, err
}

func (c *Client) changeControlJSON(method, path string, body, out any) error {
	var reader io.Reader
	if body != nil {
		raw, err := json.Marshal(body)
		if err != nil {
			return err
		}
		reader = bytes.NewReader(raw)
	}
	req, err := http.NewRequest(method, c.BaseURL+path, reader)
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
		return fmt.Errorf("change control status %d: %s", resp.StatusCode, raw)
	}
	if out == nil || len(raw) == 0 {
		return nil
	}
	return json.Unmarshal(raw, out)
}
