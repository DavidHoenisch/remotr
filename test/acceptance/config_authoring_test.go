package acceptance

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
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
	})
	if status != 0 {
		t.Fatalf("acceptance status = %d", status)
	}
}

type configAuthoringState struct {
	repo, first, second string
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
	_, s.err = runRemotr("config", "validate", s.repo)
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
