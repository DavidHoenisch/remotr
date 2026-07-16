package acceptance

import (
	"strings"
	"testing"
)

func FuzzRedactDoesNotLeakRecognizedSecret(f *testing.F) {
	f.Add("token=canary password=another normal=value", "canary")

	f.Fuzz(func(t *testing.T, value, canary string) {
		if len(value)+len(canary) > 1<<16 || canary == "" || strings.ContainsAny(canary, " \t\n\f\r") {
			return
		}
		redacted := redact("token=" + canary + " " + value)
		if !strings.HasPrefix(redacted, "token=[REDACTED] ") {
			t.Fatalf("redaction prefix = %q, want recognized token replacement", redacted)
		}
	})
}
