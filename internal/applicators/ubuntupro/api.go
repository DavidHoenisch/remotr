package ubuntupro

import (
	"bytes"
	"encoding/json"
	"fmt"
	"slices"
	"strings"

	"github.com/DavidHoenisch/remotr/internal/executil"
	"github.com/DavidHoenisch/remotr/internal/models"
)

const (
	proExecutable           = "/usr/bin/pro"
	enabledServicesEndpoint = "u.pro.status.enabled_services.v1"
	dependenciesEndpoint    = "u.pro.services.dependencies.v1"
	fullTokenAttachEndpoint = "u.pro.attach.token.full_token_attach.v1"
	isAttachedEndpoint      = "u.pro.status.is_attached.v1"
	rebootRequiredEndpoint  = "u.pro.security.status.reboot_required.v1"
	versionEndpoint         = "u.pro.version.v1"
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

type VersionResult struct {
	InstalledVersion string
	ClientVersion    string
}

type EnabledService struct {
	Name    string
	Variant string
}

type EnabledServicesResult struct {
	Services      []EnabledService
	ClientVersion string
}

type ServiceRelation struct {
	Name string
	Code string
}

type ServiceDependencies struct {
	Name             string
	DependsOn        []ServiceRelation
	IncompatibleWith []ServiceRelation
}

type DependenciesResult struct {
	Services      []ServiceDependencies
	ClientVersion string
}

type RebootRequiredResult struct {
	Required           bool
	LivepatchesApplied bool
	ClientVersion      string
}

func NewAPIClient(runner executil.Runner) *APIClient {
	return &APIClient{runner: runner}
}

func (client *APIClient) Version() (VersionResult, error) {
	envelope, err := client.readEndpoint(versionEndpoint, "version probe")
	if err != nil {
		return VersionResult{}, err
	}
	var attributes struct {
		InstalledVersion string `json:"installed_version"`
	}
	if err := json.Unmarshal(envelope.Data.Attributes, &attributes); err != nil || strings.TrimSpace(attributes.InstalledVersion) == "" || len(attributes.InstalledVersion) > 128 {
		return VersionResult{}, fmt.Errorf("Ubuntu Pro version probe returned invalid attributes")
	}
	return VersionResult{InstalledVersion: attributes.InstalledVersion, ClientVersion: envelope.Version}, nil
}

func (client *APIClient) EnabledServices() (EnabledServicesResult, error) {
	envelope, err := client.readEndpoint(enabledServicesEndpoint, "enabled-services probe")
	if err != nil {
		return EnabledServicesResult{}, err
	}
	var attributes struct {
		Services *[]struct {
			Name           string  `json:"name"`
			VariantEnabled *bool   `json:"variant_enabled"`
			VariantName    *string `json:"variant_name"`
		} `json:"enabled_services"`
	}
	if err := json.Unmarshal(envelope.Data.Attributes, &attributes); err != nil || attributes.Services == nil || len(*attributes.Services) > 32 {
		return EnabledServicesResult{}, fmt.Errorf("Ubuntu Pro enabled-services probe returned invalid attributes")
	}
	known := make(map[string]models.UbuntuProServiceContract)
	aliases := make(map[string]string)
	for _, contract := range models.UbuntuProServiceCatalog() {
		known[contract.Name] = contract
		for _, alias := range contract.StatusAliases {
			aliases[alias] = contract.Name
		}
	}
	result := EnabledServicesResult{Services: make([]EnabledService, 0, len(*attributes.Services)), ClientVersion: envelope.Version}
	seen := make(map[string]bool)
	for _, raw := range *attributes.Services {
		name := raw.Name
		if canonical := aliases[name]; canonical != "" {
			name = canonical
		}
		contract, ok := known[name]
		if !ok || seen[name] || raw.VariantEnabled == nil {
			return EnabledServicesResult{}, fmt.Errorf("Ubuntu Pro enabled-services probe returned invalid service")
		}
		seen[name] = true
		service := EnabledService{Name: name}
		if *raw.VariantEnabled {
			if raw.VariantName == nil || !slices.Contains(contract.Variants, *raw.VariantName) {
				return EnabledServicesResult{}, fmt.Errorf("Ubuntu Pro enabled-services probe returned invalid variant")
			}
			service.Variant = *raw.VariantName
		} else if raw.VariantName != nil && *raw.VariantName != "" {
			return EnabledServicesResult{}, fmt.Errorf("Ubuntu Pro enabled-services probe returned inconsistent variant")
		}
		result.Services = append(result.Services, service)
	}
	return result, nil
}

func (client *APIClient) Dependencies() (DependenciesResult, error) {
	envelope, err := client.readEndpoint(dependenciesEndpoint, "service-dependencies probe")
	if err != nil {
		return DependenciesResult{}, err
	}
	type rawRelation struct {
		Name   string `json:"name"`
		Reason struct {
			Code string `json:"code"`
		} `json:"reason"`
	}
	var attributes struct {
		Services *[]struct {
			Name             string         `json:"name"`
			DependsOn        *[]rawRelation `json:"depends_on"`
			IncompatibleWith *[]rawRelation `json:"incompatible_with"`
		} `json:"services"`
	}
	if err := json.Unmarshal(envelope.Data.Attributes, &attributes); err != nil || attributes.Services == nil || len(*attributes.Services) > 32 {
		return DependenciesResult{}, fmt.Errorf("Ubuntu Pro dependency probe returned invalid attributes")
	}
	result := DependenciesResult{Services: make([]ServiceDependencies, 0, len(*attributes.Services)), ClientVersion: envelope.Version}
	seenServices := make(map[string]bool)
	for _, raw := range *attributes.Services {
		name, _, ok := catalogService(raw.Name)
		if !ok || seenServices[name] || raw.DependsOn == nil || raw.IncompatibleWith == nil || len(*raw.DependsOn) > 32 || len(*raw.IncompatibleWith) > 32 {
			return DependenciesResult{}, fmt.Errorf("Ubuntu Pro dependency probe returned invalid service graph")
		}
		seenServices[name] = true
		service := ServiceDependencies{Name: name}
		for target, destination := range map[*[]rawRelation]*[]ServiceRelation{
			raw.DependsOn: &service.DependsOn, raw.IncompatibleWith: &service.IncompatibleWith,
		} {
			seenRelations := make(map[string]bool)
			for _, relation := range *target {
				relationName, _, known := catalogService(relation.Name)
				if !known || relationName == name || seenRelations[relationName] || strings.TrimSpace(relation.Reason.Code) == "" || len(relation.Reason.Code) > 128 {
					return DependenciesResult{}, fmt.Errorf("Ubuntu Pro dependency probe returned invalid relation")
				}
				seenRelations[relationName] = true
				*destination = append(*destination, ServiceRelation{Name: relationName, Code: relation.Reason.Code})
			}
		}
		result.Services = append(result.Services, service)
	}
	return result, nil
}

func (client *APIClient) RebootRequired() (RebootRequiredResult, error) {
	envelope, err := client.readEndpoint(rebootRequiredEndpoint, "reboot-required probe")
	if err != nil {
		return RebootRequiredResult{}, err
	}
	var attributes struct {
		State string `json:"reboot_required"`
	}
	if err := json.Unmarshal(envelope.Data.Attributes, &attributes); err != nil {
		return RebootRequiredResult{}, fmt.Errorf("Ubuntu Pro reboot-required probe returned invalid attributes")
	}
	result := RebootRequiredResult{ClientVersion: envelope.Version}
	switch attributes.State {
	case "no":
	case "yes":
		result.Required = true
	case "yes-kernel-livepatches-applied":
		result.Required = true
		result.LivepatchesApplied = true
	default:
		return RebootRequiredResult{}, fmt.Errorf("Ubuntu Pro reboot-required probe returned invalid state")
	}
	return result, nil
}

func catalogService(raw string) (string, models.UbuntuProServiceContract, bool) {
	for _, contract := range models.UbuntuProServiceCatalog() {
		if contract.Name == raw {
			return raw, contract, true
		}
		if slices.Contains(contract.StatusAliases, raw) {
			return contract.Name, contract, true
		}
	}
	return "", models.UbuntuProServiceContract{}, false
}

func (client *APIClient) IsAttached() (AttachmentStatus, error) {
	envelope, err := client.readEndpoint(isAttachedEndpoint, "attachment probe")
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

func (client *APIClient) readEndpoint(endpoint, operation string) (apiEnvelope, error) {
	if client == nil || client.runner == nil {
		return apiEnvelope{}, fmt.Errorf("Ubuntu Pro API runner is unavailable")
	}
	stdout, _, err := client.runner.Run(proExecutable, "api", endpoint)
	if err != nil {
		return apiEnvelope{}, fmt.Errorf("Ubuntu Pro %s failed", operation)
	}
	if len(stdout) == 0 || len(stdout) > maxAPIOutputBytes {
		return apiEnvelope{}, fmt.Errorf("Ubuntu Pro %s returned invalid output size", operation)
	}
	return decodeEnvelope(stdout)
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
