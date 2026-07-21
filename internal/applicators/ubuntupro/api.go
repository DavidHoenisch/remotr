package ubuntupro

import (
	"bytes"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/DavidHoenisch/remotr/internal/executil"
)

const (
	proExecutable           = "/usr/bin/pro"
	fullTokenAttachEndpoint = "u.pro.attach.token.full_token_attach.v1"
	isAttachedEndpoint      = "u.pro.status.is_attached.v1"
	maxAPIInputBytes        = 1 << 20
	maxAPIOutputBytes       = 64 << 10
)

type APIClient struct {
	runner executil.Runner
}

type AttachResult struct {
	Enabled        []string
	RebootRequired bool
	ClientVersion  string
}

type AttachmentStatus struct {
	Attached      bool
	ClientVersion string
}

func NewAPIClient(runner executil.Runner) *APIClient {
	return &APIClient{runner: runner}
}

func (client *APIClient) IsAttached() (AttachmentStatus, error) {
	if client == nil || client.runner == nil {
		return AttachmentStatus{}, fmt.Errorf("Ubuntu Pro API runner is unavailable")
	}
	stdout, _, err := client.runner.Run(proExecutable, "api", isAttachedEndpoint)
	if err != nil {
		return AttachmentStatus{}, fmt.Errorf("Ubuntu Pro attachment probe failed")
	}
	if len(stdout) == 0 || len(stdout) > maxAPIOutputBytes {
		return AttachmentStatus{}, fmt.Errorf("Ubuntu Pro attachment probe returned invalid output size")
	}
	envelope, err := decodeEnvelope(stdout)
	if err != nil {
		return AttachmentStatus{}, err
	}
	var attributes struct {
		Attached *bool `json:"is_attached"`
	}
	if err := json.Unmarshal(envelope.Data.Attributes, &attributes); err != nil || attributes.Attached == nil {
		return AttachmentStatus{}, fmt.Errorf("Ubuntu Pro attachment probe returned invalid attributes")
	}
	return AttachmentStatus{Attached: *attributes.Attached, ClientVersion: envelope.Version}, nil
}

// FullTokenAttach invokes only Canonical's versioned, stdin-parameterized API.
// The caller-provided token and the marshaled request are cleared on every
// return path to reduce the lifetime of bearer-token copies.
func (client *APIClient) FullTokenAttach(token []byte) (AttachResult, error) {
	defer clear(token)
	if client == nil || client.runner == nil {
		return AttachResult{}, fmt.Errorf("Ubuntu Pro API runner is unavailable")
	}
	inputRunner, ok := client.runner.(executil.InputRunner)
	if !ok {
		return AttachResult{}, fmt.Errorf("Ubuntu Pro API requires protected stdin")
	}
	if len(token) == 0 || len(token) > maxAPIInputBytes {
		return AttachResult{}, fmt.Errorf("Ubuntu Pro token has invalid size")
	}
	request, err := json.Marshal(struct {
		Token              string `json:"token"`
		AutoEnableServices bool   `json:"auto_enable_services"`
	}{Token: string(token), AutoEnableServices: false})
	if err != nil {
		return AttachResult{}, fmt.Errorf("encode Ubuntu Pro attach request")
	}
	defer clear(request)
	if len(request) > maxAPIInputBytes {
		return AttachResult{}, fmt.Errorf("Ubuntu Pro attach request exceeds bound")
	}
	stdout, _, err := inputRunner.RunInput(proExecutable, request, "api", fullTokenAttachEndpoint, "--data", "-")
	if err != nil {
		return AttachResult{}, fmt.Errorf("Ubuntu Pro attach API failed")
	}
	if len(stdout) == 0 || len(stdout) > maxAPIOutputBytes {
		return AttachResult{}, fmt.Errorf("Ubuntu Pro attach API returned invalid output size")
	}

	envelope, err := decodeEnvelope(stdout)
	if err != nil {
		return AttachResult{}, err
	}
	var attributes struct {
		Enabled        *[]string `json:"enabled"`
		RebootRequired *bool     `json:"reboot_required"`
	}
	if err := json.Unmarshal(envelope.Data.Attributes, &attributes); err != nil || attributes.Enabled == nil || attributes.RebootRequired == nil {
		return AttachResult{}, fmt.Errorf("Ubuntu Pro attach API returned invalid attributes")
	}
	return AttachResult{
		Enabled: append([]string(nil), (*attributes.Enabled)...), RebootRequired: *attributes.RebootRequired,
		ClientVersion: envelope.Version,
	}, nil
}

type apiEnvelope struct {
	SchemaVersion string `json:"_schema_version"`
	Data          struct {
		Attributes json.RawMessage `json:"attributes"`
		Type       string          `json:"type"`
	} `json:"data"`
	Errors   []apiIssue `json:"errors"`
	Result   string     `json:"result"`
	Version  string     `json:"version"`
	Warnings []apiIssue `json:"warnings"`
}

type apiIssue struct {
	Code string `json:"code"`
}

func decodeEnvelope(raw []byte) (apiEnvelope, error) {
	var envelope apiEnvelope
	decoder := json.NewDecoder(bytes.NewReader(raw))
	if err := decoder.Decode(&envelope); err != nil {
		return apiEnvelope{}, fmt.Errorf("Ubuntu Pro API returned malformed envelope")
	}
	if decoder.More() {
		return apiEnvelope{}, fmt.Errorf("Ubuntu Pro API returned multiple values")
	}
	if envelope.SchemaVersion != "v1" || envelope.Result != "success" || len(envelope.Errors) != 0 ||
		strings.TrimSpace(envelope.Version) == "" || len(envelope.Version) > 128 ||
		strings.TrimSpace(envelope.Data.Type) == "" || len(envelope.Data.Attributes) == 0 {
		return apiEnvelope{}, fmt.Errorf("Ubuntu Pro API returned invalid envelope")
	}
	return envelope, nil
}
