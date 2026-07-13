package services

import (
	"github.com/DavidHoenisch/remotr/internal/executor"
	"github.com/DavidHoenisch/remotr/internal/models"
)

// ActivationSignals translates shared post-change notifications into the
// engine's reusable service-action queue. Callers add these signals only after
// their own mutation succeeds.
func ActivationSignals(notifications []models.Notification) []executor.ActivationSignal {
	if len(notifications) == 0 {
		return nil
	}
	signals := make([]executor.ActivationSignal, 0, len(notifications))
	for _, notification := range notifications {
		signals = append(signals, executor.ActivationSignal{
			Kind: executor.ActivationKind(notification.Type), Target: notification.Target,
		})
	}
	return signals
}
