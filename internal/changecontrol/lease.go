package changecontrol

import (
	"fmt"
	"strconv"
	"time"

	"github.com/DavidHoenisch/remotr/internal/effectivehash"
	"github.com/DavidHoenisch/remotr/internal/models"
)

const defaultExecutionLeaseTTL = 5 * time.Minute

type PreflightReport struct {
	ChangeRequestID string            `json:"change_request_id,omitempty"`
	EndpointID      string            `json:"endpoint_id,omitempty"`
	Ready           bool              `json:"ready"`
	Reason          string            `json:"reason,omitempty"`
	ResourceHashes  map[string]string `json:"resource_hashes,omitempty"`
}

type ExecutionLease struct {
	ID                  string            `json:"id"`
	ChangeRequestID     string            `json:"change_request_id"`
	EndpointID          string            `json:"endpoint_id"`
	ResourceHashes      map[string]string `json:"resource_hashes"`
	HashContractVersion int               `json:"hash_contract_version,omitempty"`
	Attempt             int               `json:"attempt"`
	IssuedAt            time.Time         `json:"issued_at"`
	ExpiresAt           time.Time         `json:"expires_at"`
	Completed           bool              `json:"completed"`
	Risk                models.RiskClass  `json:"risk"`
	Progress            ProgressState     `json:"progress"`
	Evidence            RiskEvidence      `json:"evidence,omitempty"`
	UpdatedAt           time.Time         `json:"updated_at"`
}

func (r *Registry) IssueExecutionLease(changeRequestID string, preflight PreflightReport) (ExecutionLease, bool, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	request, ok := r.requests[changeRequestID]
	if !ok {
		return ExecutionLease{}, false, fmt.Errorf("change request %q not found", changeRequestID)
	}
	authorization, ok := r.rollouts[changeRequestID]
	now := r.now().UTC()
	if !ok || request.AuthorizationState != AuthorizationActive || now.Before(authorization.ValidFrom) || !now.Before(authorization.ValidUntil) || !windowActive(authorization.ExecutionWindows, now) || !preflight.Ready {
		return ExecutionLease{}, false, nil
	}
	if request.HashContractVersion == effectivehash.SchemaVersion && !equalHashes(preflight.ResourceHashes, authorization.ResourceHashes) {
		return ExecutionLease{}, false, fmt.Errorf("preflight resource hashes do not match canonical authorization")
	}
	var frozen *TargetEvidence
	for _, target := range authorization.FrozenTargets {
		if target.EndpointID == preflight.EndpointID {
			copy := target
			frozen = &copy
			break
		}
	}
	if frozen == nil || !frozen.Compatible || !frozen.PreflightReady {
		return ExecutionLease{}, false, nil
	}
	key := executionAttemptKey(changeRequestID, preflight.EndpointID)
	if r.attempts[key] >= authorization.AttemptLimit {
		return ExecutionLease{}, false, nil
	}
	active := 0
	for _, lease := range r.leases {
		if lease.ChangeRequestID == changeRequestID && !lease.Completed && now.Before(lease.ExpiresAt) {
			active++
		}
		if lease.ChangeRequestID == changeRequestID && lease.EndpointID == preflight.EndpointID && !lease.Completed && now.Before(lease.ExpiresAt) {
			return ExecutionLease{}, false, nil
		}
	}
	if active >= authorization.MaxConcurrency {
		return ExecutionLease{}, false, nil
	}
	previous := r.snapshotLocked()
	r.attempts[key]++
	lease := ExecutionLease{ID: r.newID(), ChangeRequestID: changeRequestID, EndpointID: preflight.EndpointID, ResourceHashes: cloneHashes(authorization.ResourceHashes), HashContractVersion: request.HashContractVersion, Attempt: r.attempts[key], IssuedAt: now, ExpiresAt: now.Add(defaultExecutionLeaseTTL), Risk: request.Risk, Progress: ProgressLeaseIssued, UpdatedAt: now}
	r.leases[lease.ID] = cloneLease(lease)
	if err := r.persistLocked(previous); err != nil {
		return ExecutionLease{}, false, err
	}
	return cloneLease(lease), true, nil
}

func executionAttemptKey(changeRequestID, endpointID string) string {
	return strconv.Itoa(len(changeRequestID)) + ":" + changeRequestID + endpointID
}

func (r *Registry) CompleteExecutionLease(leaseID string) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	lease, ok := r.leases[leaseID]
	if !ok {
		return fmt.Errorf("execution lease %q not found", leaseID)
	}
	previous := r.snapshotLocked()
	lease.Completed = true
	lease.UpdatedAt = r.now().UTC()
	r.leases[leaseID] = lease
	return r.persistLocked(previous)
}

func windowActive(windows []RecurringWindow, at time.Time) bool {
	if len(windows) == 0 {
		return true
	}
	for _, window := range windows {
		if window.contains(at) {
			return true
		}
	}
	return false
}

func cloneLease(input ExecutionLease) ExecutionLease {
	input.ResourceHashes = cloneHashes(input.ResourceHashes)
	return input
}
