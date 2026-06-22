package croncatalog

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/DavidHoenisch/remotr/internal/models"
)

func TestResolve_builtinSystemUpgrade(t *testing.T) {
	state := models.CronState{
		Crons: []models.CronJob{{
			Use:      "builtin/system-upgrade",
			Schedule: "0 2 * * 0",
		}},
	}
	got, err := Resolve("", state)
	if err != nil {
		t.Fatal(err)
	}
	if len(got.Crons) != 2 {
		t.Fatalf("crons = %d", len(got.Crons))
	}
	for _, job := range got.Crons {
		if job.Schedule != "0 2 * * 0" {
			t.Fatalf("job %q schedule = %q", job.Name, job.Schedule)
		}
		if job.Use != "" {
			t.Fatalf("job %q still has use", job.Name)
		}
		if !job.HasResources() {
			t.Fatalf("job %q has no resources", job.Name)
		}
	}
}

func TestResolve_builtinSystemUpgradeDebianOnly(t *testing.T) {
	state := models.CronState{
		Crons: []models.CronJob{{
			Use: "builtin/system-upgrade-debian",
		}},
	}
	got, err := Resolve("", state)
	if err != nil {
		t.Fatal(err)
	}
	if len(got.Crons) != 1 {
		t.Fatalf("crons = %d", len(got.Crons))
	}
	if got.Crons[0].Name != "system-upgrade-debian" {
		t.Fatalf("name = %q", got.Crons[0].Name)
	}
}

func TestResolve_builtinClamavScan(t *testing.T) {
	state := models.CronState{
		Crons: []models.CronJob{{
			Use:      "builtin/clamav-scan",
			Schedule: "0 4 * * *",
		}},
	}
	got, err := Resolve("", state)
	if err != nil {
		t.Fatal(err)
	}
	if len(got.Crons) != 2 {
		t.Fatalf("crons = %d", len(got.Crons))
	}
	for _, job := range got.Crons {
		if job.Schedule != "0 4 * * *" {
			t.Fatalf("job %q schedule = %q", job.Name, job.Schedule)
		}
		if len(job.Commands) != 2 {
			t.Fatalf("job %q commands = %d", job.Name, len(job.Commands))
		}
		if job.Commands[1].DependsOn == nil {
			t.Fatalf("job %q scan missing dependsOn", job.Name)
		}
	}
}

func TestResolve_builtinClamavScanDebianOnly(t *testing.T) {
	state := models.CronState{
		Crons: []models.CronJob{{
			Use: "builtin/clamav-scan-debian",
		}},
	}
	got, err := Resolve("", state)
	if err != nil {
		t.Fatal(err)
	}
	if len(got.Crons) != 1 {
		t.Fatalf("crons = %d", len(got.Crons))
	}
	if got.Crons[0].Name != "clamav-scan-debian" {
		t.Fatalf("name = %q", got.Crons[0].Name)
	}
}

func TestResolve_repoTemplate(t *testing.T) {
	dir := t.TempDir()
	relDir := filepath.Join(dir, "crons", "builtin")
	if err := os.MkdirAll(relDir, 0o755); err != nil {
		t.Fatal(err)
	}
	yaml := `crons:
  - name: custom-job
    schedule: "0 4 * * *"
    commands:
      - name: run
        apply: [echo, ok]
`
	if err := os.WriteFile(filepath.Join(relDir, "custom.yaml"), []byte(yaml), 0o644); err != nil {
		t.Fatal(err)
	}
	state := models.CronState{
		Crons: []models.CronJob{{
			Use: "crons/builtin/custom",
		}},
	}
	got, err := Resolve(dir, state)
	if err != nil {
		t.Fatal(err)
	}
	if len(got.Crons) != 1 || got.Crons[0].Name != "custom-job" {
		t.Fatalf("got = %+v", got.Crons)
	}
}
