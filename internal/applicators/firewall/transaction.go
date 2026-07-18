package firewall

import (
	"context"
	"crypto/sha256"
	"encoding/json"
	"errors"
	"fmt"

	"github.com/DavidHoenisch/remotr/internal/agent/networkstate"
	appErr "github.com/DavidHoenisch/remotr/internal/errors"
	"github.com/DavidHoenisch/remotr/internal/executor"
)

func (a *Applicator) prepareTransaction(ctx context.Context, selected backend) (*networkstate.Store, error) {
	store, intent, err := a.transactionIntent(ctx, selected)
	if err != nil {
		return nil, err
	}
	if _, err := store.Prepare(ctx, intent); err != nil {
		return nil, err
	}
	return store, nil
}

func (a *Applicator) PreflightRollback(ctx context.Context) error {
	if a.Resource.IsAudit() {
		return nil
	}
	if a.controlPlan.Host == "" {
		if err := a.Preflight(ctx); err != nil {
			return err
		}
	}
	selected, err := a.resolveBackend()
	if err != nil {
		return err
	}
	store, intent, err := a.transactionIntent(ctx, selected)
	if err != nil {
		return err
	}
	return store.Preflight(ctx, intent)
}

func (a *Applicator) transactionIntent(ctx context.Context, selected backend) (*networkstate.Store, networkstate.Intent, error) {
	if selected.name() != "nftables" {
		return nil, networkstate.Intent{}, fmt.Errorf("firewall backend %q does not provide transactional timed rollback; keep it in audit mode", selected.name())
	}
	snapshot, _, err := a.Exec.Run("nft", "list", "ruleset")
	if err != nil {
		return nil, networkstate.Intent{}, fmt.Errorf("snapshot nftables ruleset: %w", err)
	}
	if len(snapshot) == 0 {
		return nil, networkstate.Intent{}, fmt.Errorf("snapshot nftables ruleset: empty output")
	}
	snapshot = append([]byte("flush ruleset\n"), snapshot...)
	store, err := networkstate.New(networkstate.Options{Root: a.StateDir, Runner: a.Exec, Now: a.now})
	if err != nil {
		return nil, networkstate.Intent{}, err
	}
	current, err := store.Status()
	if err != nil {
		return nil, networkstate.Intent{}, err
	}
	attempt := 1
	if current.Intent != nil {
		attempt = current.Intent.Attempt + 1
	}
	resourceJSON, err := json.Marshal(a.Resource)
	if err != nil {
		return nil, networkstate.Intent{}, err
	}
	resourceSum := sha256.Sum256(resourceJSON)
	planJSON, err := json.Marshal(a.controlPlan)
	if err != nil {
		return nil, networkstate.Intent{}, err
	}
	planSum := sha256.Sum256(planJSON)
	timeout := a.controlPlan.RollbackTimeout
	now := a.now()
	idSum := sha256.Sum256([]byte(fmt.Sprintf("%x:%d:%d", resourceSum, attempt, now.UnixNano())))
	intent := networkstate.Intent{
		ID: fmt.Sprintf("%x", idSum[:16]), Address: "firewall/" + a.Resource.Name,
		ArtifactDigest: fmt.Sprintf("sha256:%x", resourceSum), Attempt: attempt,
		Backend: selected.name(), Deadline: now.Add(timeout), Snapshot: snapshot,
		PlanHash: fmt.Sprintf("sha256:%x", planSum),
	}
	return store, intent, nil
}

func (a *Applicator) armRollbackWatchdog(store *networkstate.Store) {
	if store == nil || a.AfterFunc == nil {
		return
	}
	delay := a.controlPlan.RollbackTimeout
	a.AfterFunc(delay, func() {
		_, _ = store.Reconcile(context.Background())
	})
}

// ApplyResult advertises the transactional rollback class once the protected
// snapshot has been durably armed.
func (a *Applicator) ApplyResult(ctx context.Context) executor.ApplyResult {
	err := a.Apply(ctx)
	if a.Resource.IsAudit() {
		if err == nil {
			return executor.ApplyResult{Status: executor.Changed, RollbackClass: executor.RollbackNone, RebootRequired: executor.RebootNotRequired}
		}
		if errors.Is(err, appErr.ErrStateAlreadyMet) {
			return executor.ApplyResult{Status: executor.NoChange, RollbackClass: executor.RollbackNone, RebootRequired: executor.RebootNotRequired}
		}
		return executor.ApplyResult{
			Status: executor.Failed, RollbackClass: executor.RollbackNone, RebootRequired: executor.RebootNotRequired, Err: err,
			Rollback: &executor.RollbackResult{Status: executor.NoRollback},
		}
	}
	if err == nil {
		return executor.ApplyResult{Status: executor.Changed, RollbackClass: executor.RollbackTransactional, RebootRequired: executor.RebootNotRequired}
	}
	if errors.Is(err, appErr.ErrStateAlreadyMet) {
		return executor.ApplyResult{Status: executor.NoChange, RollbackClass: executor.RollbackTransactional, RebootRequired: executor.RebootNotRequired}
	}
	if errors.Is(err, networkstate.ErrAwaitingAcknowledgement) {
		return executor.ApplyResult{
			Status: executor.ApplyDeferred, RollbackClass: executor.RollbackTransactional, RebootRequired: executor.RebootNotRequired,
			DeferredWork: &executor.DeferredWork{ReasonCode: executor.ReasonDeferred, Summary: "another connectivity transaction is awaiting authenticated acknowledgement"},
		}
	}
	result := executor.ApplyResult{Status: executor.Failed, RollbackClass: executor.RollbackTransactional, RebootRequired: executor.RebootNotRequired, Err: err}
	if a.StateDir != "" {
		store, storeErr := networkstate.New(networkstate.Options{Root: a.StateDir, Runner: a.Exec, Now: a.now})
		if storeErr == nil {
			status, statusErr := store.Status()
			if statusErr == nil && status.Intent != nil && status.Intent.Phase == networkstate.PhaseRolledBack {
				result.Rollback = &executor.RollbackResult{Status: executor.Reverted}
			}
		}
	}
	if result.Rollback == nil {
		result.Rollback = &executor.RollbackResult{Status: executor.NoRollback}
	}
	return result
}
