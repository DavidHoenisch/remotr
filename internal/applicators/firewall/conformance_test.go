package firewall

import (
	"context"
	"testing"

	"github.com/DavidHoenisch/remotr/internal/executor"
	"github.com/DavidHoenisch/remotr/internal/models"
)

// Audit mode intentionally cannot satisfy enforcement conformance: its public
// contract is a persistent structured plan until an operator enables enforcement.
func TestApplicator_AuditContractProducesStructuredPlan(t *testing.T) {
	audit := true
	a := New(models.FirewallResource{Name: "contract-firewall-rule", Audit: &audit, Action: "allow", Ports: []int{443}}, nil)
	check := a.Check(context.Background())
	if check.Status != executor.Drifted || check.ReasonCode != "audit_plan" {
		t.Fatalf("check = %+v", check)
	}
	if plan, ok := check.Actual.(Plan); !ok || plan.Enforced {
		t.Fatalf("plan = %#v", check.Actual)
	}
}
