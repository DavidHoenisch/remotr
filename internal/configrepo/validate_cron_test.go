package configrepo

import (
	"os"
	"path/filepath"
	"testing"
)

func TestValidateRepository_fleetCronsWithBuiltin(t *testing.T) {
	dir := t.TempDir()
	fleetDir := filepath.Join(dir, "fleets", "engineering")
	if err := os.MkdirAll(fleetDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(fleetDir, "desired.yaml"), []byte("configurations:\n  - name: base\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	crons := `crons:
  - use: builtin/system-upgrade
    schedule: "0 0 * * 0"
`
	if err := os.WriteFile(filepath.Join(fleetDir, "crons.yaml"), []byte(crons), 0o644); err != nil {
		t.Fatal(err)
	}

	res, err := ValidateRepository(dir)
	if err != nil {
		t.Fatal(err)
	}
	if len(res.Issues) != 0 {
		t.Fatalf("issues = %+v", res.Issues)
	}
	wantOK := map[string]bool{
		filepath.Join("fleets", "engineering", "desired.yaml"): true,
		filepath.Join("fleets", "engineering", "crons.yaml"):   true,
	}
	for _, okPath := range res.OK {
		if !wantOK[okPath] && okPath != "remotr.yaml" {
			// remotr.yaml may or may not exist
		}
		delete(wantOK, okPath)
	}
	for path := range wantOK {
		if path == filepath.Join("fleets", "engineering", "desired.yaml") ||
			path == filepath.Join("fleets", "engineering", "crons.yaml") {
			t.Fatalf("missing ok entry for %s", path)
		}
	}
}

func TestValidateRepository_rejectsInvalidCronSchedule(t *testing.T) {
	dir := t.TempDir()
	fleetDir := filepath.Join(dir, "fleets", "engineering")
	if err := os.MkdirAll(fleetDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(fleetDir, "desired.yaml"), []byte("configurations:\n  - name: base\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	crons := `crons:
  - name: bad
    schedule: "not-a-cron"
    commands:
      - name: run
        apply: [true]
`
	if err := os.WriteFile(filepath.Join(fleetDir, "crons.yaml"), []byte(crons), 0o644); err != nil {
		t.Fatal(err)
	}

	res, err := ValidateRepository(dir)
	if err != nil {
		t.Fatal(err)
	}
	if len(res.Issues) != 1 {
		t.Fatalf("issues = %+v", res.Issues)
	}
}
