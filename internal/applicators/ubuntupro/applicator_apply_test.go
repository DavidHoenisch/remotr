package ubuntupro

import (
	"context"
	"fmt"
	"strings"
	"testing"

	"github.com/DavidHoenisch/remotr/internal/models"
)

type attachmentLifecycleRunner struct {
	statusOutputs [][]byte
	attachOutput  []byte
	attachErr     error
	readCalls     int
	inputCalls    int
}

func (runner *attachmentLifecycleRunner) Run(name string, args ...string) ([]byte, []byte, error) {
	return runner.run(name, args...)
}

func (runner *attachmentLifecycleRunner) RunContext(_ context.Context, name string, args ...string) ([]byte, []byte, error) {
	return runner.run(name, args...)
}

func (runner *attachmentLifecycleRunner) run(name string, args ...string) ([]byte, []byte, error) {
	runner.readCalls++
	if name != proExecutable || len(args) != 2 || args[0] != "api" || args[1] != isAttachedEndpoint || len(runner.statusOutputs) == 0 {
		return nil, nil, fmt.Errorf("unexpected read process %s %v", name, args)
	}
	output := runner.statusOutputs[0]
	runner.statusOutputs = runner.statusOutputs[1:]
	return append([]byte(nil), output...), nil, nil
}

func (runner *attachmentLifecycleRunner) RunInput(name string, input []byte, args ...string) ([]byte, []byte, error) {
	return runner.runInput(name, input, args...)
}

func (runner *attachmentLifecycleRunner) RunInputContext(_ context.Context, name string, input []byte, args ...string) ([]byte, []byte, error) {
	return runner.runInput(name, input, args...)
}

func (runner *attachmentLifecycleRunner) runInput(name string, _ []byte, args ...string) ([]byte, []byte, error) {
	runner.inputCalls++
	if name != proExecutable || len(args) != 4 || args[0] != "api" || args[1] != fullTokenAttachEndpoint || args[2] != "--data" || args[3] != "-" {
		return nil, nil, fmt.Errorf("unexpected input process %s %v", name, args)
	}
	return append([]byte(nil), runner.attachOutput...), nil, runner.attachErr
}

func attachedResource() models.UbuntuProResource {
	return models.UbuntuProResource{
		ResourceMeta: models.ResourceMeta{Lifecycle: models.UbuntuProAttached},
		Name:         "primary-subscription", TokenRef: "remotr:ubuntu-pro/production@active",
	}
}

func attachSuccessEnvelope() []byte {
	return []byte(`{"_schema_version":"v1","data":{"attributes":{"enabled":[],"reboot_required":false},"meta":{"environment_vars":[]},"type":"FullTokenAttachResult"},"errors":[],"result":"success","version":"32.3ubuntu0","warnings":[]}`)
}

// OS-UPM-014 and OS-UPM-024: successful mutation is not convergence until a
// health-aware public second Check passes.
func TestApplicatorApplyRejectsPostAttachWarning(t *testing.T) {
	const warningCanary = "ubuntu-pro-post-attach-localized-warning-canary"
	warningStatus := strings.Replace(
		string(attachmentEnvelope(true)),
		`"warnings":[]`,
		`"warnings":[{"code":"contract-warning","msg":"`+warningCanary+`","meta":{}}]`,
		1,
	)
	runner := &attachmentLifecycleRunner{
		statusOutputs: [][]byte{attachmentEnvelope(false), []byte(warningStatus)},
		attachOutput:  attachSuccessEnvelope(),
	}
	resolverCalls := 0
	applicator := New(attachedResource(), exactUbuntuFacts(), runner, func(context.Context, string) ([]byte, error) {
		resolverCalls++
		return []byte("post-attach-warning-token-canary"), nil
	})
	err := applicator.Apply(context.Background())
	if err == nil {
		t.Fatal("Apply() accepted an unhealthy post-attach Check")
	}
	if strings.Contains(err.Error(), warningCanary) {
		t.Fatalf("Apply() exposed localized warning: %v", err)
	}
	if runner.readCalls != 2 || runner.inputCalls != 1 || resolverCalls != 1 {
		t.Fatalf("Apply() calls = read:%d input:%d resolve:%d", runner.readCalls, runner.inputCalls, resolverCalls)
	}
}
