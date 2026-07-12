package acceptance

import (
	"bytes"
	"context"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
	"time"

	"github.com/cucumber/godog"
)

const acceptanceTimeout = 2 * time.Minute

var secretPattern = regexp.MustCompile(`(?i)(secret|token|password)=[^\s]+`)

// ScenarioSteps exposes Godog step registration without leaking the Godog
// dependency to the integration suites that exercise public Remotr workflows.
type ScenarioSteps struct {
	context *godog.ScenarioContext
}

// Step registers a Gherkin step implementation.
func (s *ScenarioSteps) Step(expression string, handler interface{}) {
	s.context.Step(expression, handler)
}

// Run executes one deterministic, sequential acceptance suite under go test.
// Features are passed explicitly in tests or discovered by a later tracer
// feature; all scenarios receive a bounded context and isolated hook lifecycle.
func Run(t *testing.T, features []godog.Feature, initialize func(*godog.ScenarioContext)) int {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), acceptanceTimeout)
	t.Cleanup(cancel)
	var output bytes.Buffer
	suite := godog.TestSuite{
		Name:                "remotr-acceptance",
		ScenarioInitializer: initialize,
		Options: &godog.Options{
			Strict:          true,
			NoColors:        true,
			Format:          "progress",
			Randomize:       1,
			Concurrency:     1,
			Tags:            os.Getenv("REMOTR_ACCEPTANCE_TAGS"),
			FeatureContents: features,
			DefaultContext:  ctx,
			TestingT:        t,
			Output:          &output,
		},
	}
	status := suite.Run()
	if status != 0 {
		t.Logf("redacted acceptance failure attachment:\n%s", redact(output.String()))
	}
	return status
}

// RunFeatureFiles reads named feature files and executes them through the
// isolated Godog wrapper. Callers only supply Remotr workflow steps.
func RunFeatureFiles(t *testing.T, paths []string, initialize func(*ScenarioSteps)) int {
	t.Helper()
	features := make([]godog.Feature, 0, len(paths))
	for _, path := range paths {
		contents, err := os.ReadFile(path)
		if err != nil {
			t.Fatalf("read acceptance feature %s: %v", path, err)
		}
		features = append(features, godog.Feature{
			Name:     filepath.Base(path),
			Contents: contents,
		})
	}
	if len(features) == 0 {
		t.Fatal("acceptance run requires at least one feature")
	}
	if initialize == nil {
		t.Fatal("acceptance run requires a scenario initializer")
	}

	status := Run(t, features, func(context *godog.ScenarioContext) {
		initialize(&ScenarioSteps{context: context})
	})
	if status != 0 {
		t.Fatalf("acceptance scenarios failed with status %d", status)
	}

	return status
}

func redact(value string) string {
	return secretPattern.ReplaceAllStringFunc(value, func(match string) string {
		key, _, _ := strings.Cut(match, "=")
		return key + "=[REDACTED]"
	})
}
