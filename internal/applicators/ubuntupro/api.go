package ubuntupro

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"slices"
	"strings"

	"github.com/DavidHoenisch/remotr/internal/executil"
	"github.com/DavidHoenisch/remotr/internal/models"
)

const (
	proExecutable           = "/usr/bin/pro"
	enabledServicesEndpoint = "u.pro.status.enabled_services.v1"
	dependenciesEndpoint    = "u.pro.services.dependencies.v1"
	detachEndpoint          = "u.pro.detach.v1"
	disableEndpoint         = "u.pro.services.disable.v1"
	enableEndpoint          = "u.pro.services.enable.v1"
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
	WarningCodes  []string
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
	WarningCodes  []string
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

type ServiceTransitionResult struct {
	Enabled        []string
	Disabled       []string
	RebootRequired bool
	ClientVersion  string
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
	result := EnabledServicesResult{
		Services: make([]EnabledService, 0, len(*attributes.Services)), ClientVersion: envelope.Version,
		WarningCodes: issueCodes(envelope.Warnings),
	}
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

func (client *APIClient) Enable(service, variant string, accessOnly bool) (ServiceTransitionResult, error) {
	canonical, contract, ok := catalogService(service)
	if !ok || canonical != service || (variant != "" && !slices.Contains(contract.Variants, variant)) ||
		(accessOnly && !slices.Contains(contract.EnableModes, models.UbuntuProEnableAccessOnly)) {
		return ServiceTransitionResult{}, fmt.Errorf("Ubuntu Pro enable request is not cataloged")
	}
	request, err := json.Marshal(struct {
		Service    string `json:"service"`
		Variant    string `json:"variant,omitempty"`
		AccessOnly bool   `json:"access_only"`
	}{Service: service, Variant: variant, AccessOnly: accessOnly})
	if err != nil {
		return ServiceTransitionResult{}, fmt.Errorf("encode Ubuntu Pro enable request")
	}
	defer clear(request)
	envelope, err := client.inputEndpoint(enableEndpoint, "service enable", request)
	if err != nil {
		return ServiceTransitionResult{}, err
	}
	return decodeTransition(envelope, true)
}

func (client *APIClient) Disable(service string, purge bool) (ServiceTransitionResult, error) {
	canonical, contract, ok := catalogService(service)
	if !ok || canonical != service || (purge && !slices.Contains(contract.DisableModes, models.UbuntuProPurgePackages)) {
		return ServiceTransitionResult{}, fmt.Errorf("Ubuntu Pro disable request is not cataloged")
	}
	request, err := json.Marshal(struct {
		Service string `json:"service"`
		Purge   bool   `json:"purge"`
	}{Service: service, Purge: purge})
	if err != nil {
		return ServiceTransitionResult{}, fmt.Errorf("encode Ubuntu Pro disable request")
	}
	defer clear(request)
	envelope, err := client.inputEndpoint(disableEndpoint, "service disable", request)
	if err != nil {
		return ServiceTransitionResult{}, err
	}
	var attributes struct {
		Disabled *[]string `json:"disabled"`
	}
	if err := json.Unmarshal(envelope.Data.Attributes, &attributes); err != nil || attributes.Disabled == nil {
		return ServiceTransitionResult{}, fmt.Errorf("Ubuntu Pro disable operation returned invalid attributes")
	}
	disabled, err := normalizeServiceList(*attributes.Disabled)
	if err != nil {
		return ServiceTransitionResult{}, err
	}
	return ServiceTransitionResult{Disabled: disabled, ClientVersion: envelope.Version}, nil
}

func (client *APIClient) Detach() (ServiceTransitionResult, error) {
	envelope, err := client.readEndpoint(detachEndpoint, "detach")
	if err != nil {
		return ServiceTransitionResult{}, err
	}
	var attributes struct {
		Disabled       *[]string `json:"disabled"`
		RebootRequired *bool     `json:"reboot_required"`
	}
	if err := json.Unmarshal(envelope.Data.Attributes, &attributes); err != nil || attributes.Disabled == nil || attributes.RebootRequired == nil {
		return ServiceTransitionResult{}, fmt.Errorf("Ubuntu Pro detach operation returned invalid attributes")
	}
	disabled, err := normalizeServiceList(*attributes.Disabled)
	if err != nil {
		return ServiceTransitionResult{}, err
	}
	return ServiceTransitionResult{
		Disabled: disabled, RebootRequired: *attributes.RebootRequired, ClientVersion: envelope.Version,
	}, nil
}

func decodeTransition(envelope apiEnvelope, requireReboot bool) (ServiceTransitionResult, error) {
	var attributes struct {
		Enabled        *[]string `json:"enabled"`
		Disabled       *[]string `json:"disabled"`
		RebootRequired *bool     `json:"reboot_required"`
	}
	if err := json.Unmarshal(envelope.Data.Attributes, &attributes); err != nil || attributes.Enabled == nil || attributes.Disabled == nil ||
		(requireReboot && attributes.RebootRequired == nil) {
		return ServiceTransitionResult{}, fmt.Errorf("Ubuntu Pro service operation returned invalid attributes")
	}
	enabled, err := normalizeServiceList(*attributes.Enabled)
	if err != nil {
		return ServiceTransitionResult{}, err
	}
	disabled, err := normalizeServiceList(*attributes.Disabled)
	if err != nil {
		return ServiceTransitionResult{}, err
	}
	result := ServiceTransitionResult{Enabled: enabled, Disabled: disabled, ClientVersion: envelope.Version}
	if attributes.RebootRequired != nil {
		result.RebootRequired = *attributes.RebootRequired
	}
	return result, nil
}

func normalizeServiceList(raw []string) ([]string, error) {
	if len(raw) > 32 {
		return nil, fmt.Errorf("Ubuntu Pro service result exceeds bound")
	}
	result := make([]string, 0, len(raw))
	seen := make(map[string]bool)
	for _, value := range raw {
		name, _, ok := catalogService(value)
		if !ok || seen[name] {
			return nil, fmt.Errorf("Ubuntu Pro service result contains invalid member")
		}
		seen[name] = true
		result = append(result, name)
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
	return AttachmentStatus{Attached: *attributes.Attached, ClientVersion: envelope.Version, WarningCodes: issueCodes(envelope.Warnings)}, nil
}

func issueCodes(issues []apiIssue) []string {
	if len(issues) == 0 {
		return nil
	}
	result := make([]string, len(issues))
	for index, issue := range issues {
		result[index] = issue.Code
	}
	return result
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

func (client *APIClient) inputEndpoint(endpoint, operation string, input []byte) (apiEnvelope, error) {
	if client == nil || client.runner == nil {
		return apiEnvelope{}, fmt.Errorf("Ubuntu Pro API runner is unavailable")
	}
	inputRunner, ok := client.runner.(executil.InputRunner)
	if !ok {
		return apiEnvelope{}, fmt.Errorf("Ubuntu Pro API requires protected stdin")
	}
	if len(input) == 0 || len(input) > maxAPIInputBytes {
		return apiEnvelope{}, fmt.Errorf("Ubuntu Pro %s request exceeds bound", operation)
	}
	stdout, _, err := inputRunner.RunInput(proExecutable, input, "api", endpoint, "--data", "-")
	if err != nil {
		return apiEnvelope{}, fmt.Errorf("Ubuntu Pro %s API failed", operation)
	}
	if len(stdout) == 0 || len(stdout) > maxAPIOutputBytes {
		return apiEnvelope{}, fmt.Errorf("Ubuntu Pro %s API returned invalid output size", operation)
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
	enabled, err := normalizeServiceList(*attributes.Enabled)
	if err != nil {
		return AttachResult{}, err
	}
	return AttachResult{
		Enabled: enabled, RebootRequired: *attributes.RebootRequired,
		ClientVersion: envelope.Version,
	}, nil
}

type apiEnvelope struct {
	SchemaVersion string `json:"_schema_version"`
	Data          struct {
		Attributes json.RawMessage `json:"attributes"`
		Meta       json.RawMessage `json:"meta"`
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

type APIError struct {
	Code string
}

func (err APIError) Error() string { return "Ubuntu Pro API failure: " + err.Code }

func decodeEnvelope(raw []byte) (apiEnvelope, error) {
	if err := validateJSONStructure(raw); err != nil {
		return apiEnvelope{}, fmt.Errorf("Ubuntu Pro API returned malformed envelope")
	}
	var envelope apiEnvelope
	if err := json.Unmarshal(raw, &envelope); err != nil {
		return apiEnvelope{}, fmt.Errorf("Ubuntu Pro API returned malformed envelope")
	}
	attributes := bytes.TrimSpace(envelope.Data.Attributes)
	meta := bytes.TrimSpace(envelope.Data.Meta)
	if envelope.SchemaVersion != "v1" ||
		strings.TrimSpace(envelope.Version) == "" || len(envelope.Version) > 128 ||
		strings.TrimSpace(envelope.Data.Type) == "" || len(envelope.Data.Type) > 128 || len(attributes) == 0 || attributes[0] != '{' ||
		len(meta) == 0 || meta[0] != '{' || envelope.Errors == nil || envelope.Warnings == nil || len(envelope.Errors) > 32 || len(envelope.Warnings) > 32 {
		return apiEnvelope{}, fmt.Errorf("Ubuntu Pro API returned invalid envelope")
	}
	for _, issue := range append(append([]apiIssue(nil), envelope.Errors...), envelope.Warnings...) {
		if !validAPICode(issue.Code) {
			return apiEnvelope{}, fmt.Errorf("Ubuntu Pro API returned invalid issue code")
		}
	}
	switch envelope.Result {
	case "success":
		if len(envelope.Errors) != 0 {
			return apiEnvelope{}, fmt.Errorf("Ubuntu Pro API returned inconsistent success envelope")
		}
	case "failure":
		if len(envelope.Errors) == 0 {
			return apiEnvelope{}, fmt.Errorf("Ubuntu Pro API returned failure without a stable code")
		}
		return apiEnvelope{}, APIError{Code: envelope.Errors[0].Code}
	default:
		return apiEnvelope{}, fmt.Errorf("Ubuntu Pro API returned invalid result")
	}
	return envelope, nil
}

func validAPICode(code string) bool {
	if len(code) == 0 || len(code) > 128 || code != strings.TrimSpace(code) {
		return false
	}
	for index, character := range []byte(code) {
		if (character >= 'a' && character <= 'z') || (character >= '0' && character <= '9') ||
			(index > 0 && (character == '-' || character == '_' || character == '.')) {
			continue
		}
		return false
	}
	return true
}

func validateJSONStructure(raw []byte) error {
	if len(raw) == 0 || len(raw) > maxAPIOutputBytes {
		return fmt.Errorf("JSON size is invalid")
	}
	decoder := json.NewDecoder(bytes.NewReader(raw))
	decoder.UseNumber()
	budget := 4096
	if err := validateJSONValue(decoder, 0, &budget); err != nil {
		return err
	}
	if _, err := decoder.Token(); err != io.EOF {
		if err == nil {
			return fmt.Errorf("multiple JSON values")
		}
		return err
	}
	return nil
}

func validateJSONValue(decoder *json.Decoder, depth int, budget *int) error {
	if depth > 32 || *budget <= 0 {
		return fmt.Errorf("JSON structure exceeds bound")
	}
	*budget--
	token, err := decoder.Token()
	if err != nil {
		return err
	}
	delimiter, composite := token.(json.Delim)
	if !composite {
		if text, ok := token.(string); ok && len(text) > maxAPIOutputBytes {
			return fmt.Errorf("JSON string exceeds bound")
		}
		return nil
	}
	switch delimiter {
	case '{':
		seen := make(map[string]bool)
		for decoder.More() {
			keyToken, err := decoder.Token()
			if err != nil {
				return err
			}
			key, ok := keyToken.(string)
			if !ok || seen[key] {
				return fmt.Errorf("duplicate or invalid JSON member")
			}
			seen[key] = true
			if err := validateJSONValue(decoder, depth+1, budget); err != nil {
				return err
			}
		}
		closing, err := decoder.Token()
		if err != nil || closing != json.Delim('}') {
			return fmt.Errorf("invalid JSON object")
		}
	case '[':
		for decoder.More() {
			if err := validateJSONValue(decoder, depth+1, budget); err != nil {
				return err
			}
		}
		closing, err := decoder.Token()
		if err != nil || closing != json.Delim(']') {
			return fmt.Errorf("invalid JSON array")
		}
	default:
		return fmt.Errorf("invalid JSON delimiter")
	}
	return nil
}
