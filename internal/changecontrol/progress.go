package changecontrol

import (
	"fmt"

	"github.com/DavidHoenisch/remotr/internal/models"
)

type ProgressState string

const (
	ProgressLeaseIssued            ProgressState = "lease-issued"
	ProgressPrepared               ProgressState = "prepared"
	ProgressApplying               ProgressState = "applying"
	ProgressVerifying              ProgressState = "verifying"
	ProgressAcknowledged           ProgressState = "acknowledged"
	ProgressRolledBack             ProgressState = "rolled-back"
	ProgressFailed                 ProgressState = "failed"
	ProgressAcknowledgementTimeout ProgressState = "acknowledgement-timeout"
)

type RiskEvidence struct {
	WatchdogArmed          bool   `json:"watchdog_armed,omitempty"`
	AuthenticatedSync      bool   `json:"authenticated_sync,omitempty"`
	TechnicalValidation    bool   `json:"technical_validation,omitempty"`
	RequireHumanCanary     bool   `json:"require_human_canary,omitempty"`
	HumanCanaryVerified    bool   `json:"human_canary_verified,omitempty"`
	PriorBootID            string `json:"prior_boot_id,omitempty"`
	CurrentBootID          string `json:"current_boot_id,omitempty"`
	StableDeviceIdentity   string `json:"stable_device_identity,omitempty"`
	PostconditionsVerified bool   `json:"postconditions_verified,omitempty"`
	RollbackClass          string `json:"rollback_class,omitempty"`
}

type ProgressUpdate struct {
	State    ProgressState `json:"state"`
	Evidence RiskEvidence  `json:"evidence,omitempty"`
	Reason   string        `json:"reason,omitempty"`
}

func (r *Registry) UpdateExecutionProgress(leaseID string, update ProgressUpdate) (ExecutionLease, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	lease, ok := r.leases[leaseID]
	if !ok {
		return ExecutionLease{}, fmt.Errorf("execution lease %q not found", leaseID)
	}
	if !validProgress(update.State) {
		return ExecutionLease{}, fmt.Errorf("invalid execution progress %q", update.State)
	}
	if update.State == ProgressAcknowledged {
		if err := validateAcknowledgement(lease.Risk, update.Evidence); err != nil {
			return ExecutionLease{}, err
		}
	}
	previous := r.snapshotLocked()
	lease.Progress = update.State
	lease.Evidence = update.Evidence
	lease.UpdatedAt = r.now().UTC()
	lease.Completed = terminalProgress(update.State)
	r.leases[leaseID] = lease
	if err := r.persistLocked(previous); err != nil {
		return ExecutionLease{}, err
	}
	return cloneLease(lease), nil
}

func validProgress(state ProgressState) bool {
	switch state {
	case ProgressLeaseIssued, ProgressPrepared, ProgressApplying, ProgressVerifying, ProgressAcknowledged, ProgressRolledBack, ProgressFailed, ProgressAcknowledgementTimeout:
		return true
	default:
		return false
	}
}

func terminalProgress(state ProgressState) bool {
	return state == ProgressAcknowledged || state == ProgressRolledBack || state == ProgressFailed || state == ProgressAcknowledgementTimeout
}

func validateAcknowledgement(risk models.RiskClass, evidence RiskEvidence) error {
	valid := true
	switch risk {
	case models.RiskConnectivity:
		valid = evidence.WatchdogArmed && evidence.AuthenticatedSync
	case models.RiskAccess:
		valid = evidence.TechnicalValidation && (!evidence.RequireHumanCanary || evidence.HumanCanaryVerified)
	case models.RiskBoot:
		valid = evidence.PriorBootID != "" && evidence.CurrentBootID != "" && evidence.PriorBootID != evidence.CurrentBootID
	case models.RiskDestructive:
		valid = evidence.StableDeviceIdentity != "" && evidence.PostconditionsVerified && evidence.RollbackClass == "none"
	}
	if !valid {
		return fmt.Errorf("acknowledgement evidence is incomplete for risk %q", risk)
	}
	return nil
}
