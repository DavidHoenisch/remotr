package models

import (
	"strings"
	"testing"

	"github.com/DavidHoenisch/remotr/internal/types"
)

func TestParseCronState_parsesCustomCron(t *testing.T) {
	input := `crons:
  - name: nightly-backup
    schedule: "0 3 * * *"
    targetDistros: [Debian]
    commands:
      - name: backup
        apply: [/usr/local/bin/backup.sh]
`
	got, err := ParseCronState(strings.NewReader(input))
	if err != nil {
		t.Fatal(err)
	}
	if len(got.Crons) != 1 {
		t.Fatalf("crons = %d", len(got.Crons))
	}
	job := got.Crons[0]
	if job.Name != "nightly-backup" {
		t.Fatalf("name = %q", job.Name)
	}
	if job.Schedule != "0 3 * * *" {
		t.Fatalf("schedule = %q", job.Schedule)
	}
	if len(job.Commands) != 1 {
		t.Fatalf("commands = %d", len(job.Commands))
	}
}

func TestCronJob_HasResources(t *testing.T) {
	job := CronJob{Name: "x", TargetDistros: []types.Distro{types.Debian}}
	if job.HasResources() {
		t.Fatal("expected no resources")
	}
	job.Commands = []CommandResource{{Name: "run", Apply: []string{"true"}}}
	if !job.HasResources() {
		t.Fatal("expected resources")
	}
}

func TestCronJob_ToConfiguration(t *testing.T) {
	job := CronJob{
		Name:          "pkg-refresh",
		TargetDistros: []types.Distro{types.Arch},
		Commands:      []CommandResource{{Name: "sync", Apply: []string{"pacman", "-Sy"}}},
	}
	cfg := job.ToConfiguration()
	if cfg.Name != job.Name {
		t.Fatalf("name = %q", cfg.Name)
	}
	if len(cfg.Commands) != 1 {
		t.Fatalf("commands = %d", len(cfg.Commands))
	}
}
