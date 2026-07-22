package server

import (
	"fmt"
	"strings"
	"testing"

	"github.com/DavidHoenisch/remotr/internal/models"
)

var benchmarkGlobalConsumerCount int

func BenchmarkGlobalSecretConsumerDiscovery(b *testing.B) {
	var document strings.Builder
	document.WriteString("schemaVersion: 1\nconfigurations:\n  - name: subscriptions\n    resources:\n")
	for resource := range 1000 {
		_, _ = fmt.Fprintf(&document, "      - kind: ubuntuPro\n        name: subscription-%04d\n        lifecycle: attached\n        tokenRef: remotr:benchmark/global-%02d@active\n", resource, resource%10)
	}
	state, err := models.ParseState(strings.NewReader(document.String()))
	if err != nil {
		b.Fatal(err)
	}
	b.ReportAllocs()
	b.ResetTimer()
	for range b.N {
		index := activationUseIndexFromState(state, "fleet-001", "release-1", "sha256:artifact", []string{"endpoint-1", "endpoint-2"})
		benchmarkGlobalConsumerCount = len(index["remotr:benchmark/global-01@active"])
	}
}
