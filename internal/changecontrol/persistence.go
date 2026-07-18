package changecontrol

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
)

const persistedStateVersion = 1

var (
	ErrPersistence           = errors.New("change-control persistence unavailable")
	ErrInvalidPersistedState = errors.New("change-control persisted state is invalid")
)

type persistenceError struct {
	cause error
}

type invalidPersistedStateError struct {
	cause error
}

func (e invalidPersistedStateError) Error() string { return ErrInvalidPersistedState.Error() }
func (e invalidPersistedStateError) Unwrap() error { return e.cause }
func (e invalidPersistedStateError) Is(target error) bool {
	return target == ErrInvalidPersistedState
}

func (e persistenceError) Error() string { return ErrPersistence.Error() }
func (e persistenceError) Unwrap() error { return e.cause }
func (e persistenceError) Is(target error) bool {
	return target == ErrPersistence
}

func IsPersistenceError(err error) bool {
	return errors.Is(err, ErrPersistence)
}

// StateStore is the external persistence boundary for Change-control state.
// SaveChangeControlState must atomically compare expectedRevision and replace
// the payload, returning a new monotonically increasing revision.
type StateStore interface {
	LoadChangeControlState(context.Context) (payload []byte, revision int64, err error)
	SaveChangeControlState(context.Context, int64, []byte) (revision int64, err error)
}

type persistedState struct {
	Version            int                                 `json:"version"`
	Requests           map[string]ChangeRequest            `json:"requests"`
	Rollouts           map[string]RolloutAuthorization     `json:"rollouts"`
	Baselines          map[string]BaselineAuthorization    `json:"baselines"`
	Policy             ApprovalPolicy                      `json:"policy"`
	AutomaticPromotion map[string]AutomaticPromotionPolicy `json:"automatic_promotion"`
	Leases             map[string]ExecutionLease           `json:"leases"`
	Attempts           map[string]int                      `json:"attempts"`
	BreakGlass         map[string]BreakGlassAuthorization  `json:"break_glass"`
}

// NewPersistentRegistry restores Change-control state before returning. It
// fails closed when the stored payload cannot be decoded or validated.
func NewPersistentRegistry(ctx context.Context, store StateStore, options RegistryOptions) (*Registry, error) {
	if store == nil {
		return nil, fmt.Errorf("change-control state store is required")
	}
	if ctx == nil {
		ctx = context.Background()
	}
	payload, revision, err := store.LoadChangeControlState(ctx)
	if err != nil {
		return nil, persistenceError{cause: fmt.Errorf("load change-control state: %w", err)}
	}
	registry := NewRegistry(options)
	registry.storeContext = ctx
	registry.stateStore = store
	registry.stateRevision = revision
	if len(bytes.TrimSpace(payload)) == 0 {
		if revision != 0 {
			return nil, invalidPersistedStateError{cause: fmt.Errorf("revision %d has no payload", revision)}
		}
		return registry, nil
	}
	if revision <= 0 {
		return nil, invalidPersistedStateError{cause: fmt.Errorf("payload has non-positive revision %d", revision)}
	}
	state, err := decodePersistedState(payload)
	if err != nil {
		return nil, invalidPersistedStateError{cause: err}
	}
	registry.restoreLocked(state)
	return registry, nil
}

func decodePersistedState(payload []byte) (persistedState, error) {
	migrated, err := migrateLegacyPredictedEffects(payload)
	if err != nil {
		return persistedState{}, fmt.Errorf("migrate persisted state: %w", err)
	}
	decoder := json.NewDecoder(bytes.NewReader(migrated))
	decoder.DisallowUnknownFields()
	var state persistedState
	if err := decoder.Decode(&state); err != nil {
		return persistedState{}, fmt.Errorf("decode persisted state: %w", err)
	}
	if err := decoder.Decode(&struct{}{}); err != io.EOF {
		if err == nil {
			return persistedState{}, fmt.Errorf("decode persisted state: trailing JSON value")
		}
		return persistedState{}, fmt.Errorf("decode persisted state: %w", err)
	}
	state.normalize()
	if err := state.validate(); err != nil {
		return persistedState{}, fmt.Errorf("validate persisted state: %w", err)
	}
	return state, nil
}

func migrateLegacyPredictedEffects(payload []byte) ([]byte, error) {
	decoder := json.NewDecoder(bytes.NewReader(payload))
	decoder.UseNumber()
	var document map[string]any
	if err := decoder.Decode(&document); err != nil {
		return nil, err
	}
	if err := decoder.Decode(&struct{}{}); err != io.EOF {
		if err == nil {
			return nil, fmt.Errorf("trailing JSON value")
		}
		return nil, err
	}
	requests, _ := document["requests"].(map[string]any)
	for _, rawRequest := range requests {
		request, _ := rawRequest.(map[string]any)
		resources, _ := request["resources"].([]any)
		for _, rawResource := range resources {
			resource, _ := rawResource.(map[string]any)
			effects, _ := resource["predicted_effects"].([]any)
			for index, effect := range effects {
				if _, legacy := effect.(string); legacy {
					// Version-1 state stored caller-authored prose. Preserve
					// visibility of the plan without retaining or reclassifying
					// that prose. This migration is intentionally restricted to
					// the durable restore boundary; public JSON rejects strings.
					effects[index] = map[string]any{"code": EffectLegacyUnclassified}
				}
			}
		}
	}
	return json.Marshal(document)
}

func (s *persistedState) normalize() {
	if s.Requests == nil {
		s.Requests = make(map[string]ChangeRequest)
	}
	if s.Rollouts == nil {
		s.Rollouts = make(map[string]RolloutAuthorization)
	}
	if s.Baselines == nil {
		s.Baselines = make(map[string]BaselineAuthorization)
	}
	if s.AutomaticPromotion == nil {
		s.AutomaticPromotion = make(map[string]AutomaticPromotionPolicy)
	}
	if s.Leases == nil {
		s.Leases = make(map[string]ExecutionLease)
	}
	if s.Attempts == nil {
		s.Attempts = make(map[string]int)
	}
	if s.BreakGlass == nil {
		s.BreakGlass = make(map[string]BreakGlassAuthorization)
	}
	s.Policy = cloneApprovalPolicy(s.Policy)
}

func (s persistedState) validate() error {
	if s.Version != persistedStateVersion {
		return fmt.Errorf("unsupported version %d", s.Version)
	}
	for key, request := range s.Requests {
		if key == "" || request.ID != key {
			return fmt.Errorf("change request key %q does not match id %q", key, request.ID)
		}
		if request.AuthorizationState == AuthorizationActive {
			if _, ok := s.Rollouts[key]; !ok {
				return fmt.Errorf("authorized change request %q has no rollout", key)
			}
		}
	}
	for key, rollout := range s.Rollouts {
		if rollout.ChangeRequestID != key {
			return fmt.Errorf("rollout key %q does not match change request %q", key, rollout.ChangeRequestID)
		}
		if _, ok := s.Requests[key]; !ok {
			return fmt.Errorf("rollout %q references missing change request", key)
		}
	}
	for key, baseline := range s.Baselines {
		if key != baselineKey(baseline.Fleet, baseline.ResourceAddress) {
			return fmt.Errorf("baseline key %q does not match authorization", key)
		}
		if _, ok := s.Requests[baseline.ChangeRequestID]; !ok {
			return fmt.Errorf("baseline %q references missing change request", key)
		}
	}
	for key, lease := range s.Leases {
		if lease.ID != key {
			return fmt.Errorf("lease key %q does not match id %q", key, lease.ID)
		}
		if _, ok := s.Requests[lease.ChangeRequestID]; !ok {
			return fmt.Errorf("lease %q references missing change request", key)
		}
	}
	for key, attempts := range s.Attempts {
		if key == "" || attempts < 0 {
			return fmt.Errorf("invalid attempt accounting for %q", key)
		}
	}
	for key, authorization := range s.BreakGlass {
		if authorization.ID != key {
			return fmt.Errorf("break-glass key %q does not match id %q", key, authorization.ID)
		}
	}
	return nil
}

func (r *Registry) snapshotLocked() persistedState {
	state := persistedState{
		Version:            persistedStateVersion,
		Requests:           make(map[string]ChangeRequest, len(r.requests)),
		Rollouts:           make(map[string]RolloutAuthorization, len(r.rollouts)),
		Baselines:          make(map[string]BaselineAuthorization, len(r.baselines)),
		Policy:             cloneApprovalPolicy(r.policy),
		AutomaticPromotion: make(map[string]AutomaticPromotionPolicy, len(r.automaticPromotion)),
		Leases:             make(map[string]ExecutionLease, len(r.leases)),
		Attempts:           make(map[string]int, len(r.attempts)),
		BreakGlass:         make(map[string]BreakGlassAuthorization, len(r.breakGlass)),
	}
	for key, request := range r.requests {
		state.Requests[key] = cloneRequest(request)
	}
	for key, rollout := range r.rollouts {
		state.Rollouts[key] = cloneRollout(rollout)
	}
	for key, baseline := range r.baselines {
		state.Baselines[key] = cloneBaseline(baseline)
	}
	for fleet, policy := range r.automaticPromotion {
		policy.CanaryStages = append([]int(nil), policy.CanaryStages...)
		state.AutomaticPromotion[fleet] = policy
	}
	for key, lease := range r.leases {
		state.Leases[key] = cloneLease(lease)
	}
	for key, attempts := range r.attempts {
		state.Attempts[key] = attempts
	}
	for key, authorization := range r.breakGlass {
		state.BreakGlass[key] = cloneBreakGlass(authorization)
	}
	return state
}

func (r *Registry) restoreLocked(state persistedState) {
	state.normalize()
	r.requests = state.Requests
	r.rollouts = state.Rollouts
	r.baselines = state.Baselines
	r.policy = state.Policy
	r.automaticPromotion = state.AutomaticPromotion
	r.leases = state.Leases
	r.attempts = state.Attempts
	r.breakGlass = state.BreakGlass
}

func (r *Registry) persistLocked(previous persistedState) error {
	if r.stateStore == nil {
		return nil
	}
	payload, err := json.Marshal(r.snapshotLocked())
	if err != nil {
		r.restoreLocked(previous)
		return persistenceError{cause: fmt.Errorf("encode change-control state: %w", err)}
	}
	revision, err := r.stateStore.SaveChangeControlState(r.storeContext, r.stateRevision, payload)
	if err != nil {
		r.restoreLocked(previous)
		return persistenceError{cause: err}
	}
	if revision <= r.stateRevision {
		r.restoreLocked(previous)
		return persistenceError{cause: fmt.Errorf("revision did not advance")}
	}
	r.stateRevision = revision
	return nil
}
