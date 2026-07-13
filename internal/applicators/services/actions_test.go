package services_test

import (
	"slices"
	"testing"

	"github.com/DavidHoenisch/remotr/internal/applicators/services"
	"github.com/DavidHoenisch/remotr/internal/executor"
	"github.com/DavidHoenisch/remotr/internal/models"
)

func TestActivationSignalsMapsReusableServiceActions(t *testing.T) {
	notifications := []models.Notification{
		{Type: models.NotificationDaemonReload},
		{Type: models.NotificationReload, Target: "auditd.service"},
		{Type: models.NotificationTryRestart, Target: "collector.service"},
		{Type: models.NotificationRestart, Target: "telemetry.service"},
	}
	want := []executor.ActivationSignal{
		{Kind: executor.ActivationDaemonReload},
		{Kind: executor.ActivationReload, Target: "auditd.service"},
		{Kind: executor.ActivationTryRestart, Target: "collector.service"},
		{Kind: executor.ActivationRestart, Target: "telemetry.service"},
	}
	if got := services.ActivationSignals(notifications); !slices.Equal(got, want) {
		t.Fatalf("ActivationSignals() = %+v, want %+v", got, want)
	}
}
