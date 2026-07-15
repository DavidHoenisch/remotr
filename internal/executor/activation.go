package executor

import (
	"context"
	"sort"
)

// Activator executes an ordered activation plan after resource Apply succeeds.
type Activator interface {
	Activate(context.Context, []ActivationSignal) error
}

// CollectActivations deduplicates successful Apply activation signals and
// orders them so daemon reload precedes service reload/restart. Signals of the
// same class retain resource dependency order from the input results.
func CollectActivations(results []ApplyResult) []ActivationSignal {
	seen := make(map[string]struct{})
	plan := make([]ActivationSignal, 0)
	for _, result := range results {
		for _, signal := range result.Activation {
			addActivation(seen, signal, &plan)
		}
		if result.RebootRequired == RebootRequired {
			addActivation(seen, ActivationSignal{Kind: ActivationRebootRequired}, &plan)
		}
	}
	sort.SliceStable(plan, func(i, j int) bool {
		return activationRank(plan[i].Kind) < activationRank(plan[j].Kind)
	})
	return plan
}

func addActivation(seen map[string]struct{}, signal ActivationSignal, plan *[]ActivationSignal) {
	key := string(signal.Kind) + "\x00" + signal.Target
	if _, exists := seen[key]; exists {
		return
	}
	seen[key] = struct{}{}
	*plan = append(*plan, signal)
}

func activationRank(kind ActivationKind) int {
	switch kind {
	case ActivationDaemonReload:
		return 0
	case ActivationTrustStoreRefresh:
		return 1
	case ActivationReload:
		return 2
	case ActivationTryRestart:
		return 3
	case ActivationRestart:
		return 4
	case ActivationLogoutRequired:
		return 5
	case ActivationNextBoot:
		return 6
	case ActivationRebootRequired:
		return 7
	default:
		return 99
	}
}
