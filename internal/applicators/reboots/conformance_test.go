package reboots_test

import (
	"context"
	"testing"
	"time"

	"github.com/DavidHoenisch/remotr/internal/agent/rebootstate"
	"github.com/DavidHoenisch/remotr/internal/applicators/reboots"
	contract "github.com/DavidHoenisch/remotr/internal/providercontract"
)

func TestApplicatorProviderContract(t *testing.T) {
	now := time.Date(2026, 7, 12, 2, 30, 0, 0, time.UTC)
	store := rebootstate.New(t.TempDir())
	if _, err := store.Record([]rebootstate.Source{{Address: "base/packages/kernel"}}); err != nil {
		t.Fatal(err)
	}
	resource := validResource()
	resource.Delay = "0s"
	probes := &mutableRebootProbes{rebootProbes: rebootProbes{bootID: "boot-1", acPower: true}}
	provider, err := contract.New(reboots.New(resource, store, probes, func() time.Time { return now }))
	if err != nil {
		t.Fatal(err)
	}
	if check := provider.Check(context.Background()); check.Status != contract.Drifted {
		t.Fatalf("initial Check() = %+v", check)
	}
	apply := provider.Apply(context.Background())
	if apply.Status != contract.ApplyDeferred || apply.DeferredWork == nil || apply.DeferredWork.ReasonCode != "pre_reboot_ack" {
		t.Fatalf("Apply() = %+v", apply)
	}
	if _, err := store.Acknowledge(resource.Generation, now, "boot-1"); err != nil {
		t.Fatal(err)
	}
	probes.bootID = "boot-2"
	if _, err := store.Reconcile("boot-2", now.Add(time.Minute)); err != nil {
		t.Fatal(err)
	}
	if check := provider.Check(context.Background()); check.Status != contract.Compliant {
		t.Fatalf("second Check() = %+v", check)
	}
	if rollback := provider.Rollback(context.Background()); rollback.Status != contract.NoRollback {
		t.Fatalf("Rollback() = %+v", rollback)
	}
}

type mutableRebootProbes struct{ rebootProbes }

func (p *mutableRebootProbes) BootID(context.Context) (string, error) { return p.bootID, nil }
