package acceptance

import (
	"testing"

	"github.com/cucumber/godog"
)

func TestRunnerExecutesFeatureUnderGoTest(t *testing.T) {
	feature := godog.Feature{Name: "passing.feature", Contents: []byte("Feature: runner\n  Scenario: deterministic pass\n    Given an acceptance step passes\n")}
	status := Run(t, []godog.Feature{feature}, func(ctx *godog.ScenarioContext) {
		ctx.Step(`^an acceptance step passes$`, func() error { return nil })
	})
	if status != 0 {
		t.Fatalf("acceptance status = %d", status)
	}
}

func TestRedactFailureAttachment(t *testing.T) {
	got := redact("token=super-secret password=hunter2 normal=value")
	if got != "token=[REDACTED] password=[REDACTED] normal=value" {
		t.Fatalf("redaction = %q", got)
	}
}
