package ubuntupro

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"slices"
	"sort"
	"sync"

	"github.com/DavidHoenisch/remotr/internal/agent/facts"
	appErr "github.com/DavidHoenisch/remotr/internal/errors"
	"github.com/DavidHoenisch/remotr/internal/executil"
	"github.com/DavidHoenisch/remotr/internal/executor"
	"github.com/DavidHoenisch/remotr/internal/models"
	"github.com/DavidHoenisch/remotr/internal/rollbackstore"
	"github.com/DavidHoenisch/remotr/internal/types"
)

const maxRollbackPayloadBytes = 4096

type TokenResolver func(context.Context, string) ([]byte, error)

type AttachmentState string

const (
	AttachmentAttached   AttachmentState = "attached"
	AttachmentUnattached AttachmentState = "unattached"
)

type ContractHealth string

const (
	ContractInvalid ContractHealth = "invalid"
	ContractExpired ContractHealth = "expired"
)

type EntitlementOutcome string

const (
	EntitlementUnavailable EntitlementOutcome = "unavailable"
	EntitlementUnentitled  EntitlementOutcome = "unentitled"
)

type ApplyOutcome string

const (
	OutcomeChanged  ApplyOutcome = "changed"
	OutcomeNoChange ApplyOutcome = "no-change"
	OutcomeFailed   ApplyOutcome = "failed"
)

type ResidualEffects string

const (
	ResidualNone     ResidualEffects = "none"
	ResidualPossible ResidualEffects = "possible-native-effects"
)

type ServiceState struct {
	Name    string `json:"name"`
	Enabled bool   `json:"enabled"`
	Variant string `json:"variant,omitempty"`
}

// StateReport is the bounded, safe projection returned through public Check.
// It intentionally contains no account, contract, token, or raw API data.
type StateReport struct {
	Attachment      AttachmentState            `json:"attachment,omitempty"`
	ContractHealth  ContractHealth             `json:"contractHealth,omitempty"`
	Entitlement     EntitlementOutcome         `json:"entitlement,omitempty"`
	Services        []ServiceState             `json:"services,omitempty"`
	WarningCodes    []string                   `json:"warningCodes,omitempty"`
	LastOutcome     ApplyOutcome               `json:"lastOutcome,omitempty"`
	RollbackClass   executor.RollbackClass     `json:"rollbackClass,omitempty"`
	ResidualEffects ResidualEffects            `json:"residualEffects,omitempty"`
	RebootRequired  executor.RebootRequirement `json:"rebootRequired,omitempty"`
}

type Applicator struct {
	resource     models.UbuntuProResource
	facts        facts.Facts
	api          *APIClient
	resolve      TokenResolver
	changed      bool
	reboot       bool
	reportMu     sync.RWMutex
	lastOutcome  ApplyOutcome
	lastRollback executor.RollbackClass
	lastResidual ResidualEffects
	lastReboot   executor.RebootRequirement
	rollback     *rollbackstore.Handle
}

type rollbackSnapshot struct {
	Version            int                    `json:"version"`
	OriginallyAttached bool                   `json:"originallyAttached"`
	Services           []rollbackServiceState `json:"services,omitempty"`
}

type rollbackServiceState struct {
	Name    string `json:"name"`
	Enabled bool   `json:"enabled"`
	Variant string `json:"variant,omitempty"`
}

func New(resource models.UbuntuProResource, endpoint facts.Facts, runner executil.Runner, resolver TokenResolver) *Applicator {
	return &Applicator{resource: resource, facts: endpoint.Normalized(), api: NewAPIClient(runner), resolve: resolver}
}

func (applicator *Applicator) ConfigureRollback(store *rollbackstore.Store, address, artifactDigest string) error {
	handle, err := rollbackstore.NewHandle(store, address, artifactDigest, true)
	if err != nil {
		return err
	}
	applicator.rollback = handle
	return nil
}

func (applicator *Applicator) PreflightRollback(ctx context.Context) error {
	if applicator.rollback == nil {
		return errors.New("protected Ubuntu Pro rollback is not configured")
	}
	return applicator.rollback.Preflight(ctx, maxRollbackPayloadBytes)
}

// Preflight exposes Ubuntu Pro's non-mutating platform eligibility check to
// the generic high-risk execution contract.
func (applicator *Applicator) Preflight(ctx context.Context) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	return applicator.preflight()
}

func (applicator *Applicator) Name() string { return "ubuntu-pro:" + applicator.resource.Name }

func (applicator *Applicator) Description() string {
	return "Ubuntu Pro subscription attachment"
}

func (applicator *Applicator) State(ctx context.Context) (any, bool) {
	check := applicator.Check(ctx)
	return check.Actual, check.Status == executor.Compliant
}

func (applicator *Applicator) Check(ctx context.Context) executor.CheckResult {
	desired := executor.RedactedSummary("Ubuntu Pro attachment is " + string(applicator.resource.Lifecycle))
	if err := applicator.preflight(); err != nil {
		return executor.CheckResult{Status: executor.CheckFailed, ReasonCode: executor.ReasonPreflightFailed, DesiredSummary: desired, Err: err}
	}
	api := applicator.api.WithContext(ctx)
	status, err := api.IsAttached()
	if err != nil {
		return classifyCheckError(desired, err)
	}
	attachment := AttachmentUnattached
	if status.Attached {
		attachment = AttachmentAttached
	}
	report := applicator.stateReport(attachment)
	report.WarningCodes = slices.Clone(status.WarningCodes)
	if len(status.WarningCodes) != 0 {
		return warningCheckResult(desired, report)
	}
	desiredAttached := applicator.resource.Lifecycle == models.UbuntuProAttached
	if status.Attached != desiredAttached {
		return executor.CheckResult{Status: executor.Drifted, ReasonCode: executor.ReasonStateDrift, DesiredSummary: desired, ObservedSummary: executor.RedactedSummary("Ubuntu Pro attachment is " + string(attachment)), Actual: report}
	}
	if !status.Attached || len(applicator.resource.Services) == 0 {
		return executor.CheckResult{Status: executor.Compliant, ReasonCode: executor.ReasonCompliant, DesiredSummary: desired, ObservedSummary: executor.RedactedSummary("Ubuntu Pro attachment is " + string(attachment)), Actual: report}
	}

	enabled, err := api.EnabledServices()
	if err != nil {
		return classifyCheckError(desired, err, report)
	}
	report.WarningCodes = slices.Clone(enabled.WarningCodes)
	enabledByName := make(map[string]EnabledService, len(enabled.Services))
	for _, service := range enabled.Services {
		enabledByName[service.Name] = service
	}
	compliant := true
	unobservableMode := false
	for _, declared := range applicator.resource.Services {
		observed, isEnabled := enabledByName[declared.Name]
		report.Services = append(report.Services, ServiceState{Name: declared.Name, Enabled: isEnabled, Variant: observed.Variant})
		wantEnabled := declared.State == models.UbuntuProServiceEnabled
		if isEnabled && declared.EnableMode != "" {
			unobservableMode = true
		}
		if wantEnabled != isEnabled || (isEnabled && declared.Variant != observed.Variant) {
			compliant = false
		}
	}
	if len(report.WarningCodes) != 0 {
		return warningCheckResult(desired, report)
	}
	if unobservableMode {
		return executor.CheckResult{
			Status: executor.Unsupported, ReasonCode: "ubuntu_pro_mode_unobservable", DesiredSummary: desired,
			ObservedSummary: "requested Ubuntu Pro enable mode is not durably observable", Actual: report,
		}
	}
	if compliant {
		return executor.CheckResult{Status: executor.Compliant, ReasonCode: executor.ReasonCompliant, DesiredSummary: desired, ObservedSummary: "declared Ubuntu Pro state matches", Actual: report}
	}
	return executor.CheckResult{Status: executor.Drifted, ReasonCode: executor.ReasonStateDrift, DesiredSummary: desired, ObservedSummary: "declared Ubuntu Pro state differs", Actual: report}
}

func warningCheckResult(desired executor.RedactedSummary, report StateReport) executor.CheckResult {
	return executor.CheckResult{
		Status: executor.CheckFailed, ReasonCode: "ubuntu_pro_warning", DesiredSummary: desired,
		ObservedSummary: "Ubuntu Pro API reported a stable warning", Actual: report,
		Err: errors.New("Ubuntu Pro API reported a stable warning"),
	}
}

func classifyCheckError(desired executor.RedactedSummary, err error, reports ...StateReport) executor.CheckResult {
	report := StateReport{}
	var actual any
	if len(reports) != 0 {
		report = reports[0]
		actual = report
	}
	if errors.Is(err, context.Canceled) {
		return executor.CheckResult{Status: executor.CheckFailed, ReasonCode: "ubuntu_pro_canceled", DesiredSummary: desired, Actual: actual, Err: context.Canceled}
	}
	if errors.Is(err, context.DeadlineExceeded) {
		return executor.CheckResult{Status: executor.CheckFailed, ReasonCode: "ubuntu_pro_timeout", DesiredSummary: desired, Actual: actual, Err: context.DeadlineExceeded}
	}
	if errors.Is(err, errContextRunnerRequired) {
		return executor.CheckResult{Status: executor.CheckFailed, ReasonCode: "ubuntu_pro_context_runner_required", DesiredSummary: desired, Actual: actual, Err: errContextRunnerRequired}
	}
	var apiError APIError
	if errors.As(err, &apiError) {
		switch apiError.Code {
		case "invalid-contract":
			report.ContractHealth = ContractInvalid
			actual = report
			return executor.CheckResult{Status: executor.CheckFailed, ReasonCode: "ubuntu_pro_contract_invalid", DesiredSummary: desired, Actual: actual, Err: apiError}
		case "expired-contract":
			report.ContractHealth = ContractExpired
			actual = report
			return executor.CheckResult{Status: executor.CheckFailed, ReasonCode: "ubuntu_pro_contract_expired", DesiredSummary: desired, Actual: actual, Err: apiError}
		case "operation-in-progress":
			return executor.CheckResult{Status: executor.Deferred, ReasonCode: executor.ReasonNativeLockContended, DesiredSummary: desired, Actual: actual, Err: executor.ErrNativeLockContended}
		case "service-unavailable":
			report.Entitlement = EntitlementUnavailable
			actual = report
			return executor.CheckResult{Status: executor.Unsupported, ReasonCode: "ubuntu_pro_service_unavailable", DesiredSummary: desired, Actual: actual, Err: apiError}
		case "service-not-entitled":
			report.Entitlement = EntitlementUnentitled
			actual = report
			return executor.CheckResult{Status: executor.Unsupported, ReasonCode: "ubuntu_pro_service_unentitled", DesiredSummary: desired, Actual: actual, Err: apiError}
		}
	}
	return executor.CheckResult{Status: executor.CheckFailed, ReasonCode: executor.ReasonProbeFailed, DesiredSummary: desired, Actual: actual, Err: fmt.Errorf("Ubuntu Pro versioned API probe failed")}
}

func (applicator *Applicator) Apply(ctx context.Context) error {
	applicator.changed = false
	applicator.reboot = false
	if err := applicator.preflight(); err != nil {
		return err
	}
	api := applicator.api.WithContext(ctx)
	status, err := api.IsAttached()
	if err != nil {
		return err
	}
	if len(status.WarningCodes) != 0 {
		return errors.New("Ubuntu Pro attachment probe reported a stable warning")
	}
	if applicator.resource.Lifecycle == models.UbuntuProDetached {
		if !status.Attached {
			return nil
		}
		result, err := api.Detach()
		if err != nil {
			return err
		}
		applicator.changed = true
		applicator.reboot = result.RebootRequired
		observed, err := api.IsAttached()
		if err != nil {
			return err
		}
		if observed.Attached || len(observed.WarningCodes) != 0 {
			return errors.New("Ubuntu Pro detach post-check failed")
		}
		return nil
	}
	attachedHere := false
	if !status.Attached {
		if applicator.resolve == nil {
			return fmt.Errorf("Ubuntu Pro token resolver is unavailable")
		}
		token, err := applicator.resolve(ctx, applicator.resource.TokenRef)
		if err != nil {
			if errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
				return err
			}
			return errors.New("Ubuntu Pro token resolution failed")
		}
		defer clear(token)
		// Secret files commonly carry one terminal line ending. It is not part
		// of Canonical's enrollment token and must not cross the API boundary.
		token = bytes.TrimSuffix(token, []byte{'\n'})
		token = bytes.TrimSuffix(token, []byte{'\r'})
		if err := applicator.armRollbackSnapshot(ctx, rollbackSnapshot{Version: 1, OriginallyAttached: false}); err != nil {
			return err
		}
		result, err := api.FullTokenAttach(token)
		if err != nil {
			var apiError APIError
			if errors.As(err, &apiError) || errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
				return err
			}
			recovery := applicator.Check(ctx)
			return fmt.Errorf("Ubuntu Pro attachment outcome is ambiguous after API failure; recovery check: %s", recovery.ReasonCode)
		}
		if len(result.Enabled) != 0 {
			return fmt.Errorf("Ubuntu Pro attachment enabled unexpected services")
		}
		applicator.changed = true
		applicator.reboot = result.RebootRequired
		observed, err := api.IsAttached()
		if err != nil {
			return err
		}
		if !observed.Attached || len(observed.WarningCodes) != 0 {
			reason := executor.ReasonStateDrift
			if len(observed.WarningCodes) != 0 {
				reason = "ubuntu_pro_warning"
			}
			return fmt.Errorf("Ubuntu Pro attachment post-check failed: %s", reason)
		}
		attachedHere = true
	}
	if err := applicator.convergeServices(ctx, api); err != nil {
		if attachedHere {
			return rollbackNewAttachment(api, err)
		}
		return err
	}
	if applicator.rollback != nil {
		return applicator.rollback.Acknowledge(ctx)
	}
	return nil
}

func rollbackNewAttachment(api *APIClient, cause error) error {
	result, err := api.Detach()
	if err != nil || len(result.WarningCodes) != 0 {
		return fmt.Errorf("%v; attachment rollback failed", cause)
	}
	observed, err := api.IsAttached()
	if err != nil || observed.Attached || len(observed.WarningCodes) != 0 {
		return fmt.Errorf("%v; attachment rollback check failed", cause)
	}
	return fmt.Errorf("%v; attachment rollback restored", cause)
}

func (applicator *Applicator) armRollbackSnapshot(ctx context.Context, snapshot rollbackSnapshot) error {
	if applicator.rollback == nil {
		return nil
	}
	payload, err := json.Marshal(snapshot)
	if err != nil || len(payload) > maxRollbackPayloadBytes {
		clear(payload)
		return errors.New("encode Ubuntu Pro rollback snapshot")
	}
	defer clear(payload)
	return applicator.rollback.Arm(ctx, payload)
}

func (applicator *Applicator) ApplyResult(ctx context.Context) executor.ApplyResult {
	err := applicator.Apply(ctx)
	rollbackClass := executor.RollbackBestEffort
	if applicator.resource.Lifecycle == models.UbuntuProDetached {
		rollbackClass = executor.RollbackNone
	}
	for _, service := range applicator.resource.Services {
		if service.DisableMode == models.UbuntuProPurgePackages || !serviceControlRestorable(service.Name) {
			rollbackClass = executor.RollbackNone
		}
	}
	result := executor.ApplyResult{Status: executor.Changed, RebootRequired: executor.RebootNotRequired, RollbackClass: rollbackClass}
	switch {
	case err != nil:
		result.Status = executor.Failed
		result.Err = err
	case !applicator.changed:
		result.Status = executor.NoChange
	case applicator.reboot:
		result.RebootRequired = executor.RebootRequired
		result.Activation = []executor.ActivationSignal{{Kind: executor.ActivationRebootRequired}}
	}
	applicator.recordApplyResult(result)
	return result
}

func (applicator *Applicator) stateReport(attachment AttachmentState) StateReport {
	applicator.reportMu.RLock()
	defer applicator.reportMu.RUnlock()
	return StateReport{
		Attachment: attachment, LastOutcome: applicator.lastOutcome, RollbackClass: applicator.lastRollback,
		ResidualEffects: applicator.lastResidual, RebootRequired: applicator.lastReboot,
	}
}

func (applicator *Applicator) recordApplyResult(result executor.ApplyResult) {
	outcome := OutcomeChanged
	switch result.Status {
	case executor.NoChange:
		outcome = OutcomeNoChange
	case executor.Failed:
		outcome = OutcomeFailed
	}
	residual := ResidualNone
	if result.Status == executor.Failed && applicator.changed {
		residual = ResidualPossible
	}
	applicator.reportMu.Lock()
	defer applicator.reportMu.Unlock()
	applicator.lastOutcome = outcome
	applicator.lastRollback = result.RollbackClass
	applicator.lastResidual = residual
	applicator.lastReboot = result.RebootRequired
}

func (applicator *Applicator) convergeServices(ctx context.Context, api *APIClient) error {
	if len(applicator.resource.Services) == 0 {
		return nil
	}
	for _, declared := range applicator.resource.Services {
		if declared.EnableMode != "" {
			return errors.New("Ubuntu Pro selected enable mode is not durably observable")
		}
	}
	enabled, err := api.EnabledServices()
	if err != nil {
		return err
	}
	if len(enabled.WarningCodes) != 0 {
		return errors.New("Ubuntu Pro enabled-services probe reported a stable warning")
	}
	enabledByName := make(map[string]EnabledService, len(enabled.Services))
	for _, service := range enabled.Services {
		enabledByName[service.Name] = service
	}
	graph, err := api.Dependencies()
	if err != nil {
		return err
	}
	if len(graph.WarningCodes) != 0 {
		return errors.New("Ubuntu Pro dependency probe reported a stable warning")
	}
	relationsByName := make(map[string]ServiceDependencies, len(graph.Services))
	for _, service := range graph.Services {
		relationsByName[service.Name] = service
	}
	declaredByName := make(map[string]models.UbuntuProService, len(applicator.resource.Services))
	for _, service := range applicator.resource.Services {
		declaredByName[service.Name] = service
	}
	for _, declared := range applicator.resource.Services {
		if declared.State != models.UbuntuProServiceEnabled {
			continue
		}
		relations := relationsByName[declared.Name]
		for _, dependency := range relations.DependsOn {
			owned, declaredDependency := declaredByName[dependency.Name]
			if _, alreadyEnabled := enabledByName[dependency.Name]; !alreadyEnabled &&
				(!declaredDependency || owned.State != models.UbuntuProServiceEnabled) {
				return fmt.Errorf("%s: service %s requires explicitly enabled %s", executor.ReasonDependencyBlocked, declared.Name, dependency.Name)
			}
		}
		for _, incompatible := range relations.IncompatibleWith {
			owned, declaredIncompatible := declaredByName[incompatible.Name]
			if _, alreadyEnabled := enabledByName[incompatible.Name]; alreadyEnabled &&
				(!declaredIncompatible || owned.State != models.UbuntuProServiceDisabled) {
				return fmt.Errorf("%s: service %s requires explicitly disabled %s", executor.ReasonDependencyBlocked, declared.Name, incompatible.Name)
			}
		}
	}
	operationOrder, err := serviceOperationOrder(applicator.resource.Services, enabledByName, relationsByName)
	if err != nil {
		return err
	}
	if applicator.rollback != nil && !applicator.rollback.Owned() {
		prior := make([]rollbackServiceState, 0, len(operationOrder))
		for _, declared := range operationOrder {
			observed, isEnabled := enabledByName[declared.Name]
			wantEnabled := declared.State == models.UbuntuProServiceEnabled
			if serviceControlRestorable(declared.Name) && (wantEnabled != isEnabled || (wantEnabled && observed.Variant != declared.Variant)) {
				prior = append(prior, rollbackServiceState{Name: declared.Name, Enabled: isEnabled, Variant: observed.Variant})
			}
		}
		if len(prior) != 0 {
			if err := applicator.armRollbackSnapshot(ctx, rollbackSnapshot{Version: 1, OriginallyAttached: true, Services: prior}); err != nil {
				return err
			}
		}
	}
	changed := false
	changes := make([]serviceChange, 0, len(operationOrder))
	for _, declared := range operationOrder {
		observed, isEnabled := enabledByName[declared.Name]
		wantEnabled := declared.State == models.UbuntuProServiceEnabled
		if wantEnabled && (!isEnabled || observed.Variant != declared.Variant) {
			transition, err := api.Enable(declared.Name, declared.Variant, false)
			if err != nil {
				return restoreServiceChanges(api, changes, err)
			}
			if len(transition.WarningCodes) != 0 || !slices.Equal(transition.Enabled, []string{declared.Name}) || len(transition.Disabled) != 0 {
				if slices.Contains(transition.Enabled, declared.Name) {
					changes = append(changes, serviceChange{Name: declared.Name, WasEnabled: isEnabled, Variant: observed.Variant, Restorable: serviceControlRestorable(declared.Name)})
				}
				for _, name := range append(slices.Clone(transition.Enabled), transition.Disabled...) {
					if name != declared.Name {
						changes = append(changes, serviceChange{Name: name, Restorable: false})
					}
				}
				return restoreServiceChanges(api, changes, errors.New("Ubuntu Pro enable operation returned unexpected effects"))
			}
			changes = append(changes, serviceChange{Name: declared.Name, WasEnabled: isEnabled, Variant: observed.Variant, Restorable: serviceControlRestorable(declared.Name)})
			applicator.changed = true
			applicator.reboot = applicator.reboot || transition.RebootRequired
			changed = true
		} else if !wantEnabled && isEnabled {
			purge := declared.DisableMode == models.UbuntuProPurgePackages
			transition, err := api.Disable(declared.Name, purge)
			if err != nil {
				return restoreServiceChanges(api, changes, err)
			}
			if len(transition.WarningCodes) != 0 || !slices.Equal(transition.Disabled, []string{declared.Name}) || len(transition.Enabled) != 0 {
				if slices.Contains(transition.Disabled, declared.Name) {
					changes = append(changes, serviceChange{Name: declared.Name, WasEnabled: true, Variant: observed.Variant, Restorable: !purge && serviceControlRestorable(declared.Name)})
				}
				for _, name := range append(slices.Clone(transition.Enabled), transition.Disabled...) {
					if name != declared.Name {
						changes = append(changes, serviceChange{Name: name, Restorable: false})
					}
				}
				return restoreServiceChanges(api, changes, errors.New("Ubuntu Pro disable operation returned unexpected effects"))
			}
			changes = append(changes, serviceChange{Name: declared.Name, WasEnabled: true, Variant: observed.Variant, Restorable: !purge && serviceControlRestorable(declared.Name)})
			applicator.changed = true
			applicator.reboot = applicator.reboot || transition.RebootRequired
			changed = true
		}
	}
	if !changed {
		return nil
	}
	check := applicator.Check(ctx)
	if check.Status != executor.Compliant {
		return restoreServiceChanges(api, changes, fmt.Errorf("Ubuntu Pro service post-check failed: %s", check.ReasonCode))
	}
	return nil
}

func serviceControlRestorable(name string) bool {
	contract, cataloged := models.UbuntuProServiceContractFor(name)
	return cataloged && contract.Recovery == models.UbuntuProRecoverBestEffort
}

type serviceChange struct {
	Name       string
	WasEnabled bool
	Variant    string
	Restorable bool
}

func restoreServiceChanges(api *APIClient, changes []serviceChange, cause error) error {
	if len(changes) == 0 {
		return cause
	}
	fullyRestorable := true
	for index := len(changes) - 1; index >= 0; index-- {
		change := changes[index]
		if !change.Restorable {
			fullyRestorable = false
			continue
		}
		if change.WasEnabled {
			result, err := api.Enable(change.Name, change.Variant, false)
			if err != nil || len(result.WarningCodes) != 0 || !slices.Equal(result.Enabled, []string{change.Name}) || len(result.Disabled) != 0 {
				return fmt.Errorf("%v; rollback failed", cause)
			}
		} else {
			result, err := api.Disable(change.Name, false)
			if err != nil || len(result.WarningCodes) != 0 || !slices.Equal(result.Disabled, []string{change.Name}) || len(result.Enabled) != 0 {
				return fmt.Errorf("%v; rollback failed", cause)
			}
		}
	}
	observed, err := api.EnabledServices()
	if err != nil || len(observed.WarningCodes) != 0 {
		return fmt.Errorf("%v; rollback check failed", cause)
	}
	enabled := make(map[string]EnabledService, len(observed.Services))
	for _, service := range observed.Services {
		enabled[service.Name] = service
	}
	for _, change := range changes {
		if !change.Restorable {
			continue
		}
		service, isEnabled := enabled[change.Name]
		if isEnabled != change.WasEnabled || (isEnabled && service.Variant != change.Variant) {
			return fmt.Errorf("%v; rollback check failed", cause)
		}
	}
	if !fullyRestorable {
		return fmt.Errorf("%v; rollback incomplete", cause)
	}
	return fmt.Errorf("%v; rollback restored", cause)
}

func serviceOperationOrder(
	declared []models.UbuntuProService,
	enabled map[string]EnabledService,
	graph map[string]ServiceDependencies,
) ([]models.UbuntuProService, error) {
	byName := make(map[string]models.UbuntuProService, len(declared))
	for _, service := range declared {
		byName[service.Name] = service
	}
	preDisable := make(map[string]bool)
	for _, service := range declared {
		if service.State != models.UbuntuProServiceEnabled {
			continue
		}
		for _, incompatible := range graph[service.Name].IncompatibleWith {
			owned, ok := byName[incompatible.Name]
			if _, active := enabled[incompatible.Name]; active && ok && owned.State == models.UbuntuProServiceDisabled {
				preDisable[incompatible.Name] = true
			}
		}
	}
	order := make([]models.UbuntuProService, 0, len(declared))
	preNames := make([]string, 0, len(preDisable))
	for name := range preDisable {
		preNames = append(preNames, name)
	}
	sort.Strings(preNames)
	for _, name := range preNames {
		order = append(order, byName[name])
	}

	state := make(map[string]uint8)
	var visit func(string) error
	visit = func(name string) error {
		if state[name] == 2 {
			return nil
		}
		if state[name] == 1 {
			return fmt.Errorf("%s: Ubuntu Pro dependency graph contains a cycle", executor.ReasonDependencyBlocked)
		}
		service, ok := byName[name]
		if !ok || service.State != models.UbuntuProServiceEnabled {
			return nil
		}
		state[name] = 1
		dependencies := make([]string, 0, len(graph[name].DependsOn))
		for _, dependency := range graph[name].DependsOn {
			owned, declaredDependency := byName[dependency.Name]
			if declaredDependency && owned.State == models.UbuntuProServiceEnabled {
				dependencies = append(dependencies, dependency.Name)
			}
		}
		sort.Strings(dependencies)
		for _, dependency := range dependencies {
			if err := visit(dependency); err != nil {
				return err
			}
		}
		state[name] = 2
		order = append(order, service)
		return nil
	}
	enableNames := make([]string, 0, len(declared))
	for _, service := range declared {
		if service.State == models.UbuntuProServiceEnabled {
			enableNames = append(enableNames, service.Name)
		}
	}
	sort.Strings(enableNames)
	for _, name := range enableNames {
		if err := visit(name); err != nil {
			return nil, err
		}
	}
	disableNames := make([]string, 0, len(declared))
	for _, service := range declared {
		if service.State == models.UbuntuProServiceDisabled && !preDisable[service.Name] {
			disableNames = append(disableNames, service.Name)
		}
	}
	sort.Sort(sort.Reverse(sort.StringSlice(disableNames)))
	for _, name := range disableNames {
		order = append(order, byName[name])
	}
	return order, nil
}

func (applicator *Applicator) Revert(ctx context.Context) error {
	if applicator.rollback == nil {
		return appErr.ErrNoOp
	}
	err := applicator.rollback.Rollback(ctx, func(payload []byte) error {
		if len(payload) == 0 || len(payload) > maxRollbackPayloadBytes {
			return errors.New("invalid Ubuntu Pro rollback snapshot size")
		}
		var snapshot rollbackSnapshot
		decoder := json.NewDecoder(bytes.NewReader(payload))
		decoder.DisallowUnknownFields()
		if err := decoder.Decode(&snapshot); err != nil || snapshot.Version != 1 {
			return errors.New("invalid Ubuntu Pro rollback snapshot")
		}
		if err := decoder.Decode(&struct{}{}); err != io.EOF || len(snapshot.Services) > len(models.UbuntuProServiceCatalog()) {
			return errors.New("invalid Ubuntu Pro rollback snapshot")
		}
		if snapshot.OriginallyAttached {
			return restorePersistedServices(applicator.api.WithContext(ctx), snapshot.Services)
		}
		if len(snapshot.Services) != 0 {
			return errors.New("invalid Ubuntu Pro rollback snapshot")
		}
		api := applicator.api.WithContext(ctx)
		status, err := api.IsAttached()
		if err != nil || len(status.WarningCodes) != 0 {
			return errors.New("Ubuntu Pro rollback attachment probe failed")
		}
		if !status.Attached {
			return nil
		}
		result, err := api.Detach()
		if err != nil || len(result.WarningCodes) != 0 {
			return errors.New("Ubuntu Pro rollback detach failed")
		}
		observed, err := api.IsAttached()
		if err != nil || observed.Attached || len(observed.WarningCodes) != 0 {
			return errors.New("Ubuntu Pro rollback detach check failed")
		}
		return nil
	})
	if errors.Is(err, os.ErrNotExist) {
		return appErr.ErrNoOp
	}
	return err
}

func restorePersistedServices(api *APIClient, prior []rollbackServiceState) error {
	if len(prior) == 0 {
		return nil
	}
	seen := make(map[string]bool, len(prior))
	for _, service := range prior {
		contract, cataloged := models.UbuntuProServiceContractFor(service.Name)
		if !cataloged || seen[service.Name] || contract.Recovery != models.UbuntuProRecoverBestEffort || (service.Variant != "" && !slices.Contains(contract.Variants, service.Variant)) {
			return errors.New("invalid Ubuntu Pro rollback service snapshot")
		}
		seen[service.Name] = true
	}
	currentResult, err := api.EnabledServices()
	if err != nil || len(currentResult.WarningCodes) != 0 {
		return errors.New("Ubuntu Pro rollback service probe failed")
	}
	current := make(map[string]EnabledService, len(currentResult.Services))
	for _, service := range currentResult.Services {
		current[service.Name] = service
	}
	for index := len(prior) - 1; index >= 0; index-- {
		service := prior[index]
		observed, enabled := current[service.Name]
		if service.Enabled {
			if enabled && observed.Variant == service.Variant {
				continue
			}
			result, err := api.Enable(service.Name, service.Variant, false)
			if err != nil || len(result.WarningCodes) != 0 || !slices.Equal(result.Enabled, []string{service.Name}) || len(result.Disabled) != 0 {
				return errors.New("Ubuntu Pro rollback service enable failed")
			}
			current[service.Name] = EnabledService{Name: service.Name, Variant: service.Variant}
		} else if enabled {
			result, err := api.Disable(service.Name, false)
			if err != nil || len(result.WarningCodes) != 0 || !slices.Equal(result.Disabled, []string{service.Name}) || len(result.Enabled) != 0 {
				return errors.New("Ubuntu Pro rollback service disable failed")
			}
			delete(current, service.Name)
		}
	}
	observedResult, err := api.EnabledServices()
	if err != nil || len(observedResult.WarningCodes) != 0 {
		return errors.New("Ubuntu Pro rollback service check failed")
	}
	observed := make(map[string]EnabledService, len(observedResult.Services))
	for _, service := range observedResult.Services {
		observed[service.Name] = service
	}
	for _, service := range prior {
		actual, enabled := observed[service.Name]
		if enabled != service.Enabled || (enabled && actual.Variant != service.Variant) {
			return errors.New("Ubuntu Pro rollback service check failed")
		}
	}
	return nil
}

func (applicator *Applicator) preflight() error {
	if err := applicator.resource.Validate(); err != nil {
		return err
	}
	if !applicator.facts.ExactUbuntu() || applicator.facts.Arch != types.X86 || applicator.facts.Package != types.Apt ||
		!slices.Contains([]string{"20.04", "22.04", "24.04", "26.04"}, applicator.facts.DistroVersion) {
		return fmt.Errorf("Ubuntu Pro provider is unsupported on this endpoint")
	}
	return nil
}
