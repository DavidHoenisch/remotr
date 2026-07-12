package acceptance

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"github.com/cucumber/godog"
)

func TestConfigurationAuthoringFeature(t *testing.T) {
	feature, err := os.ReadFile("features/config_authoring.feature")
	if err != nil {
		t.Fatal(err)
	}
	state := &configAuthoringState{}
	status := Run(t, []godog.Feature{{Name: "config_authoring.feature", Contents: feature}}, func(ctx *godog.ScenarioContext) {
		ctx.Before(func(context.Context, *godog.Scenario) (context.Context, error) {
			*state = configAuthoringState{}
			return context.Background(), nil
		})
		ctx.Step(`^an invalid configuration repository$`, state.invalidRepository)
		ctx.Step(`^the Compose configuration repository$`, func() error { state.repo = filepath.Join(repositoryRoot(), "compose", "config-repo"); return nil })
		ctx.Step(`^a canonical configuration with an unknown resource field$`, state.canonicalUnknownFieldRepository)
		ctx.Step(`^a canonical configuration with a cross-kind duplicate name$`, state.canonicalCrossKindDuplicateRepository)
		ctx.Step(`^the operator validates the repository$`, state.validate)
		ctx.Step(`^validation is rejected$`, func() error {
			if state.err == nil {
				return os.ErrInvalid
			}
			return nil
		})
		ctx.Step(`^the operator renders fleet "([^"]*)" twice$`, state.renderTwice)
		ctx.Step(`^both rendered artifacts are identical$`, func() error {
			if state.err != nil || state.first != state.second {
				return os.ErrInvalid
			}
			return nil
		})
		ctx.Step(`^validation identifies resource "([^"]*)" and field "([^"]*)"$`, state.validationIdentifies)
		ctx.Step(`^validation rejects ambiguous resource "([^"]*)"$`, state.validationRejectsAmbiguousResource)
	})
	if status != 0 {
		t.Fatalf("acceptance status = %d", status)
	}
}

type configAuthoringState struct {
	repo, first, second string
	output              string
	err                 error
}

func (s *configAuthoringState) invalidRepository() error {
	dir, err := os.MkdirTemp("", "remotr-invalid-config-")
	if err != nil {
		return err
	}
	s.repo = dir
	return os.WriteFile(filepath.Join(dir, "invalid.yaml"), []byte("kind: invalid\n"), 0o600)
}
func (s *configAuthoringState) validate() error {
	s.output, s.err = runRemotr("config", "validate", s.repo)
	return nil
}

func (s *configAuthoringState) canonicalUnknownFieldRepository() error {
	dir, err := os.MkdirTemp("", "remotr-canonical-config-")
	if err != nil {
		return err
	}
	s.repo = dir
	module := `kind: module
schemaVersion: 1
configurations:
  - name: base
    resources:
      - kind: package
        name: curl
        present: true
        presnt: false
`
	manifest := "kind: manifest\nmodules:\n  - modules/base.yaml\n"
	for path, content := range map[string]string{
		filepath.Join(dir, "modules", "base.yaml"):                  module,
		filepath.Join(dir, "fleets", "test-fleet", "manifest.yaml"): manifest,
	} {
		if err := os.MkdirAll(filepath.Dir(path), 0o750); err != nil {
			return err
		}
		if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
			return err
		}
	}
	return nil
}

func (s *configAuthoringState) validationIdentifies(address, field string) error {
	if s.err == nil || !strings.Contains(s.output, address) || !strings.Contains(s.output, field) {
		return fmt.Errorf("validation output %q, error %v; want address %q and field %q", s.output, s.err, address, field)
	}
	return nil
}

func (s *configAuthoringState) canonicalCrossKindDuplicateRepository() error {
	dir, err := os.MkdirTemp("", "remotr-duplicate-config-")
	if err != nil {
		return err
	}
	s.repo = dir
	module := `kind: module
schemaVersion: 1
configurations:
  - name: base
    resources:
      - kind: package
        name: shared
        present: true
      - kind: file
        name: shared
        path: /tmp/shared
        content: managed
`
	manifest := "kind: manifest\nmodules:\n  - modules/base.yaml\n"
	for path, content := range map[string]string{
		filepath.Join(dir, "modules", "base.yaml"):                  module,
		filepath.Join(dir, "fleets", "test-fleet", "manifest.yaml"): manifest,
	} {
		if err := os.MkdirAll(filepath.Dir(path), 0o750); err != nil {
			return err
		}
		if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
			return err
		}
	}
	return nil
}

func (s *configAuthoringState) validationRejectsAmbiguousResource(address string) error {
	if s.err == nil || !strings.Contains(s.output, address) || !strings.Contains(s.output, "duplicate") {
		return fmt.Errorf("validation output %q, error %v; want duplicate resource %q", s.output, s.err, address)
	}
	return nil
}
func (s *configAuthoringState) renderTwice(fleet string) error {
	s.first, s.err = runRemotr("config", "render", "--fleet", fleet, s.repo)
	if s.err != nil {
		return nil
	}
	s.second, s.err = runRemotr("config", "render", "--fleet", fleet, s.repo)
	return nil
}
func runRemotr(args ...string) (string, error) {
	command := exec.Command("go", append([]string{"run", "-mod=vendor", "./cmd/remotr"}, args...)...)
	command.Dir = repositoryRoot()
	out, err := command.CombinedOutput()
	return string(out), err
}
func repositoryRoot() string {
	_, file, _, _ := runtime.Caller(0)
	return filepath.Clean(filepath.Join(filepath.Dir(file), "..", ".."))
}
