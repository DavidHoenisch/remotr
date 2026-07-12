// Package acceptance is the only Remotr package that imports Godog.
package acceptance

import (
	"testing"

	"github.com/cucumber/godog"
)

func TestGodogDependencyIsIsolated(t *testing.T) {
	if (godog.Options{}).Format != "" {
		t.Fatal("unexpected Godog default options")
	}
}
