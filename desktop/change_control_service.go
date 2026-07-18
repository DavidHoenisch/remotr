package main

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"strings"
	"sync"
	"time"

	"github.com/DavidHoenisch/remotr/internal/admin"
)

const (
	changeAttemptLimit         = 100
	changeConcurrencyLimit     = 100
	changeJustificationLimit   = 1024
	changeExecutionWindowLimit = 32
	changeMaximumValidity      = 30 * 24 * time.Hour
)

type ChangeExecutionWindowInput struct {
	Weekdays        []int `json:"weekdays"`
	StartMinuteUTC  int   `json:"startMinuteUtc"`
	DurationMinutes int   `json:"durationMinutes"`
}

type ChangeAuthorizationRequest struct {
	ChangeRequestID  string                       `json:"changeRequestId"`
	Confirmation     string                       `json:"confirmation"`
	Justification    string                       `json:"justification"`
	ValidFrom        string                       `json:"validFrom"`
	ValidUntil       string                       `json:"validUntil"`
	AttemptLimit     int                          `json:"attemptLimit"`
	MaxConcurrency   int                          `json:"maxConcurrency"`
	ExecutionWindows []ChangeExecutionWindowInput `json:"executionWindows"`
}

type ChangeLifecycleRequest struct {
	ChangeRequestID string `json:"changeRequestId"`
	Confirmation    string `json:"confirmation"`
	Action          string `json:"action"`
}

type ChangeBaselinePromotionRequest struct {
	ChangeRequestID       string `json:"changeRequestId"`
	ResourceAddress       string `json:"resourceAddress"`
	Confirmation          string `json:"confirmation"`
	AcknowledgeExceptions bool   `json:"acknowledgeExceptions"`
}

type BaselineAdoptionRequest struct {
	PlanID       string `json:"planId"`
	Fleet        string `json:"fleet"`
	Confirmation string `json:"confirmation"`
}

type BaselineAdoptionPreview struct {
	PlanID            string   `json:"planId"`
	Fleet             string   `json:"fleet"`
	ReleaseRef        string   `json:"releaseRef"`
	ArtifactDigest    string   `json:"artifactDigest"`
	TargetCount       int      `json:"targetCount"`
	ResourceCount     int      `json:"resourceCount"`
	ResourceAddresses []string `json:"resourceAddresses"`
}

type RolloutAuthorizationView struct {
	ID               string                      `json:"id"`
	ChangeRequestID  string                      `json:"changeRequestId"`
	Fleet            string                      `json:"fleet"`
	ValidFrom        string                      `json:"validFrom"`
	ValidUntil       string                      `json:"validUntil"`
	AttemptLimit     int                         `json:"attemptLimit"`
	MaxConcurrency   int                         `json:"maxConcurrency"`
	ExecutionWindows []ChangeExecutionWindowView `json:"executionWindows"`
	AuthorizedBy     string                      `json:"authorizedBy"`
	AuthorizedAt     string                      `json:"authorizedAt"`
	Justification    string                      `json:"justification"`
}

type ChangeExecutionWindowView struct {
	Weekdays        []int `json:"weekdays"`
	StartMinuteUTC  int   `json:"startMinuteUtc"`
	DurationMinutes int64 `json:"durationMinutes"`
}

type BaselineAuthorizationView struct {
	ID              string `json:"id"`
	ChangeRequestID string `json:"changeRequestId"`
	Fleet           string `json:"fleet"`
	ResourceAddress string `json:"resourceAddress"`
	DesiredHash     string `json:"desiredHash"`
	Risk            string `json:"risk"`
	Provider        string `json:"provider"`
	AuthorizedBy    string `json:"authorizedBy"`
	AuthorizedAt    string `json:"authorizedAt"`
}

type ChangeActionResult struct {
	Action           string                     `json:"action"`
	ChangeRequest    ChangeRequestDetailView    `json:"changeRequest"`
	Authorization    *RolloutAuthorizationView  `json:"authorization,omitempty"`
	Baseline         *BaselineAuthorizationView `json:"baseline,omitempty"`
	AffectedEvidence []string                   `json:"affectedEvidence"`
}

type pendingBaselineAdoption struct {
	id    string
	fleet string
}

type ChangeControlService struct {
	mu              sync.Mutex
	inflight        map[string]struct{}
	pendingAdoption *pendingBaselineAdoption
	now             func() time.Time
}

func NewChangeControlService() *ChangeControlService {
	return &ChangeControlService{
		inflight: make(map[string]struct{}),
		now:      time.Now,
	}
}

func (s *ChangeControlService) Clear() {
	if s == nil {
		return
	}
	s.mu.Lock()
	s.pendingAdoption = nil
	s.mu.Unlock()
}

func (s *ChangeControlService) AuthorizeConnected(ctx context.Context, client *admin.Client, request ChangeAuthorizationRequest) (ChangeActionResult, error) {
	if client == nil {
		return ChangeActionResult{}, ErrSessionNotConnected
	}
	if err := validateConfirmedChangeRequest(request.ChangeRequestID, request.Confirmation); err != nil {
		return ChangeActionResult{}, err
	}
	justification := strings.TrimSpace(request.Justification)
	if justification == "" || len(justification) > changeJustificationLimit {
		return ChangeActionResult{}, changeControlValidationFailure("Enter a justification of 1 to 1,024 characters.")
	}
	release, err := s.begin("authorize", request.ChangeRequestID)
	if err != nil {
		return ChangeActionResult{}, err
	}
	defer release()

	change, err := loadExactChangeRequest(ctx, client, request.ChangeRequestID)
	if err != nil {
		return ChangeActionResult{}, err
	}
	spec, err := s.rolloutSpec(request, len(change.FrozenTargets))
	if err != nil {
		return ChangeActionResult{}, err
	}
	authorization, err := client.AuthorizeChangeRequestContext(ctx, request.ChangeRequestID, spec, justification)
	if err != nil {
		return ChangeActionResult{}, err
	}
	if authorization.ID != "" && (authorization.ChangeRequestID != request.ChangeRequestID || authorization.Fleet != change.Fleet) {
		return ChangeActionResult{}, errors.New("server returned authorization for a different Change request")
	}
	refreshed, err := loadExactChangeRequest(ctx, client, request.ChangeRequestID)
	if err != nil {
		return ChangeActionResult{}, err
	}
	action := "approval_recorded"
	if refreshed.AuthorizationState == "authorized" {
		action = "rollout_authorized"
	}
	result := changeActionResult(action, refreshed)
	if authorization.ID != "" {
		view := mapRolloutAuthorizationView(authorization)
		result.Authorization = &view
	}
	return result, nil
}

func (s *ChangeControlService) LifecycleConnected(ctx context.Context, client *admin.Client, request ChangeLifecycleRequest) (ChangeActionResult, error) {
	if client == nil {
		return ChangeActionResult{}, ErrSessionNotConnected
	}
	if err := validateConfirmedChangeRequest(request.ChangeRequestID, request.Confirmation); err != nil {
		return ChangeActionResult{}, err
	}
	switch request.Action {
	case "pause", "resume", "revoke":
	default:
		return ChangeActionResult{}, changeControlValidationFailure("Choose pause, resume, or revoke for the exact Change request.")
	}
	release, err := s.begin(request.Action, request.ChangeRequestID)
	if err != nil {
		return ChangeActionResult{}, err
	}
	defer release()

	before, err := loadExactChangeRequest(ctx, client, request.ChangeRequestID)
	if err != nil {
		return ChangeActionResult{}, err
	}
	changed, err := client.ChangeRequestLifecycleContext(ctx, request.ChangeRequestID, request.Action)
	if err != nil {
		return ChangeActionResult{}, err
	}
	if changed.ID != request.ChangeRequestID || changed.Fleet != before.Fleet {
		return ChangeActionResult{}, errors.New("server returned lifecycle evidence for a different Change request")
	}
	return changeActionResult(request.Action+"d", changed), nil
}

func (s *ChangeControlService) PromoteBaselineConnected(ctx context.Context, client *admin.Client, request ChangeBaselinePromotionRequest) (ChangeActionResult, error) {
	if client == nil {
		return ChangeActionResult{}, ErrSessionNotConnected
	}
	if err := validateChangeRequestID(request.ChangeRequestID); err != nil {
		return ChangeActionResult{}, changeControlValidationFailure("Select one exact Change request before promoting a baseline.")
	}
	if request.ResourceAddress == "" || request.ResourceAddress != strings.TrimSpace(request.ResourceAddress) || request.Confirmation != request.ResourceAddress {
		return ChangeActionResult{}, changeControlValidationFailure("Type the exact case-sensitive resource address to confirm baseline promotion.")
	}
	release, err := s.begin("baseline-promote", request.ChangeRequestID)
	if err != nil {
		return ChangeActionResult{}, err
	}
	defer release()

	change, err := loadExactChangeRequest(ctx, client, request.ChangeRequestID)
	if err != nil {
		return ChangeActionResult{}, err
	}
	resourceFound := false
	for _, resource := range change.Resources {
		if resource.Address == request.ResourceAddress {
			resourceFound = resource.BaselineEligible
			break
		}
	}
	if !resourceFound {
		return ChangeActionResult{}, changeControlValidationFailure("Choose a baseline-eligible resource frozen into this Change request.")
	}
	verified, exceptions := baselineOutcomeCounts(change)
	if verified == 0 {
		return ChangeActionResult{}, changeControlValidationFailure("Baseline promotion requires at least one verified successful target outcome.")
	}
	if exceptions > 0 && !request.AcknowledgeExceptions {
		return ChangeActionResult{}, changeControlValidationFailure("Acknowledge every failed, blocked, or not-seen target before promotion.")
	}
	baseline, err := client.PromoteChangeBaselineContext(ctx, request.ChangeRequestID, request.ResourceAddress, request.AcknowledgeExceptions)
	if err != nil {
		return ChangeActionResult{}, err
	}
	if baseline.ID == "" || baseline.ChangeRequestID != request.ChangeRequestID || baseline.Fleet != change.Fleet || baseline.ResourceAddress != request.ResourceAddress {
		return ChangeActionResult{}, errors.New("server returned baseline authorization for a different resource")
	}
	refreshed, err := loadExactChangeRequest(ctx, client, request.ChangeRequestID)
	if err != nil {
		return ChangeActionResult{}, err
	}
	result := changeActionResult("baseline_promoted", refreshed)
	view := mapBaselineAuthorizationView(baseline)
	result.Baseline = &view
	return result, nil
}

func (s *ChangeControlService) ChooseBaselineAdoptionPlan(_ context.Context, fleet string) (BaselineAdoptionPreview, error) {
	if s == nil {
		return BaselineAdoptionPreview{}, errors.New("Change control is unavailable")
	}
	if err := validateChangeFleet(fleet); err != nil {
		return BaselineAdoptionPreview{}, err
	}
	planID, err := newBaselineAdoptionPlanID()
	if err != nil {
		return BaselineAdoptionPreview{}, errors.New("the baseline adoption request could not be protected for review")
	}
	preview := BaselineAdoptionPreview{PlanID: planID, Fleet: fleet}
	s.mu.Lock()
	s.pendingAdoption = &pendingBaselineAdoption{id: planID, fleet: fleet}
	s.mu.Unlock()
	return preview, nil
}

func (s *ChangeControlService) CreateBaselineAdoptionConnected(ctx context.Context, client *admin.Client, request BaselineAdoptionRequest) (ChangeActionResult, error) {
	if client == nil {
		return ChangeActionResult{}, ErrSessionNotConnected
	}
	if err := validateChangeFleet(request.Fleet); err != nil {
		return ChangeActionResult{}, err
	}
	if request.Confirmation != request.Fleet {
		return ChangeActionResult{}, changeControlValidationFailure("Type the exact case-sensitive Fleet name to confirm baseline adoption.")
	}
	if request.PlanID == "" || request.PlanID != strings.TrimSpace(request.PlanID) {
		return ChangeActionResult{}, changeControlValidationFailure("Prepare and confirm a baseline adoption request before submission.")
	}
	release, err := s.begin("baseline-adopt", request.Fleet)
	if err != nil {
		return ChangeActionResult{}, err
	}
	defer release()

	s.mu.Lock()
	pending := s.pendingAdoption
	if pending == nil || pending.id != request.PlanID || pending.fleet != request.Fleet {
		s.mu.Unlock()
		return ChangeActionResult{}, changeControlValidationFailure("The reviewed baseline adoption request no longer matches this Fleet.")
	}
	s.mu.Unlock()

	created, err := client.CreateBaselineAdoptionContext(ctx, request.Fleet)
	if err != nil {
		return ChangeActionResult{}, err
	}
	if created.ID == "" || created.Fleet != request.Fleet {
		return ChangeActionResult{}, errors.New("server returned baseline adoption evidence for a different Fleet")
	}
	s.mu.Lock()
	if s.pendingAdoption != nil && s.pendingAdoption.id == request.PlanID {
		s.pendingAdoption = nil
	}
	s.mu.Unlock()
	return changeActionResult("baseline_adoption_created", created), nil
}

func (s *ChangeControlService) begin(action, target string) (func(), error) {
	key := action + "\x00" + target
	s.mu.Lock()
	if _, exists := s.inflight[key]; exists {
		s.mu.Unlock()
		return nil, &ActionFailure{
			Kind: ActionConflict, Message: "This Change-control action is already pending.",
			Guidance: "Wait for the current request to resolve before choosing an explicit retry.", Retryable: false,
		}
	}
	s.inflight[key] = struct{}{}
	s.mu.Unlock()
	return func() {
		s.mu.Lock()
		delete(s.inflight, key)
		s.mu.Unlock()
	}, nil
}

func (s *ChangeControlService) rolloutSpec(request ChangeAuthorizationRequest, targetCount int) (admin.RolloutSpec, error) {
	if targetCount <= 0 {
		return admin.RolloutSpec{}, changeControlValidationFailure("The Change request has no frozen targets to authorize.")
	}
	if request.AttemptLimit <= 0 || request.AttemptLimit > changeAttemptLimit {
		return admin.RolloutSpec{}, changeControlValidationFailure("Attempt limit must be between 1 and 100.")
	}
	if request.MaxConcurrency <= 0 || request.MaxConcurrency > targetCount || request.MaxConcurrency > changeConcurrencyLimit {
		return admin.RolloutSpec{}, changeControlValidationFailure("Maximum concurrency must be positive and no greater than the frozen target count or 100.")
	}
	now := time.Now().UTC()
	if s != nil && s.now != nil {
		now = s.now().UTC()
	}
	validFrom := now
	if request.ValidFrom != "" {
		parsed, err := time.Parse(time.RFC3339, request.ValidFrom)
		if err != nil {
			return admin.RolloutSpec{}, changeControlValidationFailure("Rollout start must be an RFC3339 timestamp.")
		}
		validFrom = parsed.UTC()
	}
	validUntil := validFrom.Add(changeMaximumValidity)
	if request.ValidUntil != "" {
		parsed, err := time.Parse(time.RFC3339, request.ValidUntil)
		if err != nil {
			return admin.RolloutSpec{}, changeControlValidationFailure("Rollout end must be an RFC3339 timestamp.")
		}
		validUntil = parsed.UTC()
	}
	if !validUntil.After(validFrom) || validUntil.Sub(validFrom) > changeMaximumValidity {
		return admin.RolloutSpec{}, changeControlValidationFailure("Rollout validity must end after it starts and span no more than 30 days.")
	}
	if len(request.ExecutionWindows) > changeExecutionWindowLimit {
		return admin.RolloutSpec{}, changeControlValidationFailure("Use no more than 32 execution windows.")
	}
	windows := make([]admin.RecurringWindow, 0, len(request.ExecutionWindows))
	for _, input := range request.ExecutionWindows {
		if input.StartMinuteUTC < 0 || input.StartMinuteUTC >= 24*60 || input.DurationMinutes <= 0 || input.DurationMinutes > 24*60 || len(input.Weekdays) == 0 || len(input.Weekdays) > 7 {
			return admin.RolloutSpec{}, changeControlValidationFailure("Each execution window needs weekdays, a valid UTC start minute, and a duration of at most 24 hours.")
		}
		seen := make(map[int]struct{}, len(input.Weekdays))
		weekdays := make([]time.Weekday, 0, len(input.Weekdays))
		for _, weekday := range input.Weekdays {
			if weekday < 0 || weekday > 6 {
				return admin.RolloutSpec{}, changeControlValidationFailure("Execution-window weekdays must use values 0 through 6.")
			}
			if _, duplicate := seen[weekday]; duplicate {
				return admin.RolloutSpec{}, changeControlValidationFailure("Execution-window weekdays must not repeat.")
			}
			seen[weekday] = struct{}{}
			weekdays = append(weekdays, time.Weekday(weekday))
		}
		windows = append(windows, admin.RecurringWindow{Weekdays: weekdays, StartMinuteUTC: input.StartMinuteUTC, Duration: time.Duration(input.DurationMinutes) * time.Minute})
	}
	return admin.RolloutSpec{
		ValidFrom: validFrom, ValidUntil: validUntil, AttemptLimit: request.AttemptLimit,
		MaxConcurrency: request.MaxConcurrency, ExecutionWindows: windows,
	}, nil
}

func loadExactChangeRequest(ctx context.Context, client *admin.Client, id string) (admin.ChangeRequest, error) {
	if err := validateChangeRequestID(id); err != nil {
		return admin.ChangeRequest{}, changeControlValidationFailure("Select one exact Change request before continuing.")
	}
	change, err := client.GetChangeRequestContext(ctx, id)
	if err != nil {
		return admin.ChangeRequest{}, err
	}
	if change.ID != id {
		return admin.ChangeRequest{}, errors.New("server returned evidence for a different Change request")
	}
	return change, nil
}

func validateConfirmedChangeRequest(id, confirmation string) error {
	if err := validateChangeRequestID(id); err != nil {
		return changeControlValidationFailure("Select one exact Change request before continuing.")
	}
	if confirmation != id {
		return changeControlValidationFailure("Type the exact case-sensitive Change request ID to confirm this action.")
	}
	return nil
}

func validateChangeFleet(fleet string) error {
	if fleet == "" || fleet != strings.TrimSpace(fleet) || len(fleet) > 255 {
		return changeControlValidationFailure("Select one exact Fleet before reviewing baseline adoption.")
	}
	return nil
}

func baselineOutcomeCounts(change admin.ChangeRequest) (verified, exceptions int) {
	for _, target := range change.FrozenTargets {
		outcome, found := change.Outcomes[target.EndpointID]
		if found && outcome.State == "verified_successful" {
			verified++
			continue
		}
		exceptions++
	}
	return verified, exceptions
}

func changeActionResult(action string, change admin.ChangeRequest) ChangeActionResult {
	view := mapChangeRequestDetail(change)
	view.ReadOnly = false
	return ChangeActionResult{
		Action: action, ChangeRequest: view,
		AffectedEvidence: []string{"change_request", "activity"},
	}
}

func mapRolloutAuthorizationView(authorization admin.RolloutAuthorization) RolloutAuthorizationView {
	windows := make([]ChangeExecutionWindowView, 0, len(authorization.ExecutionWindows))
	for _, window := range authorization.ExecutionWindows {
		weekdays := make([]int, 0, len(window.Weekdays))
		for _, weekday := range window.Weekdays {
			weekdays = append(weekdays, int(weekday))
		}
		windows = append(windows, ChangeExecutionWindowView{
			Weekdays: weekdays, StartMinuteUTC: window.StartMinuteUTC,
			DurationMinutes: int64(window.Duration / time.Minute),
		})
	}
	return RolloutAuthorizationView{
		ID: authorization.ID, ChangeRequestID: authorization.ChangeRequestID, Fleet: authorization.Fleet,
		ValidFrom: formatTimestamp(authorization.ValidFrom), ValidUntil: formatTimestamp(authorization.ValidUntil),
		AttemptLimit: authorization.AttemptLimit, MaxConcurrency: authorization.MaxConcurrency,
		ExecutionWindows: windows,
		AuthorizedBy:     authorization.AuthorizedBy, AuthorizedAt: formatTimestamp(authorization.AuthorizedAt),
		Justification: authorization.Justification,
	}
}

func mapBaselineAuthorizationView(baseline admin.BaselineAuthorization) BaselineAuthorizationView {
	return BaselineAuthorizationView{
		ID: baseline.ID, ChangeRequestID: baseline.ChangeRequestID, Fleet: baseline.Fleet,
		ResourceAddress: baseline.ResourceAddress, DesiredHash: baseline.DesiredHash,
		Risk: string(baseline.Risk), Provider: baseline.Provider, AuthorizedBy: baseline.AuthorizedBy,
		AuthorizedAt: formatTimestamp(baseline.AuthorizedAt),
	}
}

func newBaselineAdoptionPlanID() (string, error) {
	buffer := make([]byte, 16)
	if _, err := rand.Read(buffer); err != nil {
		return "", err
	}
	return hex.EncodeToString(buffer), nil
}

func changeControlValidationFailure(guidance string) error {
	return &ActionFailure{
		Kind: ActionValidation, Message: "The Change-control request is invalid.",
		Guidance: guidance, Retryable: false,
	}
}
