package agentinstall

import "testing"

func Test_expandVersion(t *testing.T) {
	got := expandVersion("elastic-agent-${version}-linux", "9.3.0")
	if got != "elastic-agent-9.3.0-linux" {
		t.Fatalf("got %q", got)
	}
}
