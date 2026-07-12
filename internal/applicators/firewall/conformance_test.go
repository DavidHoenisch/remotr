package firewall

import (
	"context"
	"path/filepath"
	"testing"

	"github.com/DavidHoenisch/remotr/internal/models"
	contract "github.com/DavidHoenisch/remotr/internal/providercontract"
	harness "github.com/DavidHoenisch/remotr/test/providercontract"
)

func TestApplicator_ConformsForAuditRule(t *testing.T) {
	harness.RunConvergence(t, harness.Fixture{
		Compliant: func(t *testing.T) contract.Provider { return newAuditContractProvider(t, true) },
		Drifted:   func(t *testing.T) contract.Provider { return newAuditContractProvider(t, false) },
	})
}

func newAuditContractProvider(t *testing.T, recorded bool) contract.Provider {
	t.Helper()
	audit := true
	applicator := New(models.FirewallResource{
		Name:   "contract-firewall-rule",
		Audit:  &audit,
		Action: "allow",
		Ports:  []int{443},
	}, nil)
	applicator.AuditPath = filepath.Join(t.TempDir(), "firewall-audit.log")
	if recorded {
		if err := applicator.Apply(context.Background()); err != nil {
			t.Fatal(err)
		}
	}
	provider, err := contract.New(applicator)
	if err != nil {
		t.Fatal(err)
	}
	return provider
}
