package configrepo

import (
	"path/filepath"
	"testing"

	"github.com/DavidHoenisch/remotr/internal/models"
)

func TestValidateRepository_fleetManifestWithCrons(t *testing.T) {
	dir := t.TempDir()
	writeFleetModule(t, dir, "engineering", `configurations:
  - name: base
    packages:
      - name: nmap
        present: true
        packageManager: pacman
`)
	writeFile(t, filepath.Join(dir, "crons", "weekly.yaml"), `kind: crons
crons:
  - use: builtin/system-upgrade
    schedule: "0 0 * * 0"
`)
	manifest := `kind: manifest
modules:
  - modules/engineering-module.yaml
crons:
  - crons/weekly.yaml
`
	writeFile(t, filepath.Join(dir, "fleets", "engineering", "manifest.yaml"), manifest)

	res, err := ValidateRepository(dir)
	if err != nil {
		t.Fatal(err)
	}
	if len(res.Issues) != 0 {
		t.Fatalf("issues = %+v", res.Issues)
	}
}

func TestValidateCronState_rejectsInvalidSchedule(t *testing.T) {
	state := models.CronState{
		Crons: []models.CronJob{{
			Name:     "bad",
			Schedule: "not-a-cron",
			Commands: []models.CommandResource{{Name: "run", Apply: []string{"true"}}},
		}},
	}
	if err := ValidateCronState(state, "test"); err == nil {
		t.Fatal("expected error")
	}
}
