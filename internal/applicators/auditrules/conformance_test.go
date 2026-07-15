package auditrules_test

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/DavidHoenisch/remotr/internal/applicators/auditrules"
	"github.com/DavidHoenisch/remotr/internal/models"
	contract "github.com/DavidHoenisch/remotr/internal/providercontract"
	harness "github.com/DavidHoenisch/remotr/test/providercontract"
)

func TestApplicatorProviderContract(t *testing.T) {
	harness.RunConvergence(t, harness.Fixture{
		Compliant: func(t *testing.T) contract.Provider { return auditContractProvider(t, true) },
		Drifted:   func(t *testing.T) contract.Provider { return auditContractProvider(t, false) },
	})
}

func auditContractProvider(t *testing.T, compliant bool) contract.Provider {
	t.Helper()
	rules := []string{"-w /etc/passwd -p wa -k identity"}
	runner := &auditRunner{desired: rules}
	provider := auditrules.New(models.AuditRulesResource{Name: "identity", Rules: rules}, runner)
	provider.RulesDir = t.TempDir()
	if compliant {
		runner.loaded = append([]string(nil), rules...)
		if err := os.WriteFile(filepath.Join(provider.RulesDir, "remotr-identity.rules"), []byte(rules[0]+"\n"), 0o640); err != nil {
			t.Fatal(err)
		}
	}
	adapted, err := contract.New(provider)
	if err != nil {
		t.Fatal(err)
	}
	return adapted
}
