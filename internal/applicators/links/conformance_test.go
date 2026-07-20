package links_test

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/DavidHoenisch/remotr/internal/applicators/links"
	"github.com/DavidHoenisch/remotr/internal/models"
	contract "github.com/DavidHoenisch/remotr/internal/providercontract"
	harness "github.com/DavidHoenisch/remotr/test/providercontract"
)

func TestSymbolicLinkProviderContract(t *testing.T) {
	harness.RunConvergence(t, harness.Fixture{
		Compliant: func(t *testing.T) contract.Provider {
			return symbolicContractProvider(t, "release-b", models.LifecyclePresent)
		},
		Drifted: func(t *testing.T) contract.Provider {
			return symbolicContractProvider(t, "release-a", models.LifecyclePresent)
		},
	})
	harness.RunAbsence(t, harness.AbsenceFixture{
		Absent: func(t *testing.T) contract.Provider { return symbolicContractProvider(t, "", models.LifecycleAbsent) },
		Present: func(t *testing.T) contract.Provider {
			return symbolicContractProvider(t, "release-a", models.LifecycleAbsent)
		},
	})
}

func TestHardLinkProviderContract(t *testing.T) {
	harness.RunConvergence(t, harness.Fixture{
		Compliant: func(t *testing.T) contract.Provider { return hardContractProvider(t, true) },
		Drifted:   func(t *testing.T) contract.Provider { return hardContractProvider(t, false) },
	})
}

func symbolicContractProvider(t *testing.T, installedTarget string, lifecycle models.Lifecycle) contract.Provider {
	t.Helper()
	path := filepath.Join(t.TempDir(), "current")
	if installedTarget != "" {
		if err := os.Symlink(installedTarget, path); err != nil {
			t.Fatal(err)
		}
	}
	provider, err := contract.New(links.New(models.LinkResource{
		Name: "current", Path: path, Target: "release-b", LinkType: models.LinkTypeSymbolic,
		AllowTypeReplacement: true, ResourceMeta: models.ResourceMeta{Lifecycle: lifecycle},
	}))
	if err != nil {
		t.Fatal(err)
	}
	return provider
}

func hardContractProvider(t *testing.T, compliant bool) contract.Provider {
	t.Helper()
	dir := t.TempDir()
	source := filepath.Join(dir, "source")
	path := filepath.Join(dir, "linked")
	if err := os.WriteFile(source, []byte("source\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if compliant {
		if err := os.Link(source, path); err != nil {
			t.Fatal(err)
		}
	} else if err := os.WriteFile(path, []byte("different inode\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	provider, err := contract.New(links.New(models.LinkResource{
		Name: "linked", Path: path, Target: source, LinkType: models.LinkTypeHard,
		AllowTypeReplacement: true, ResourceMeta: models.ResourceMeta{Lifecycle: models.LifecyclePresent},
	}))
	if err != nil {
		t.Fatal(err)
	}
	return provider
}
