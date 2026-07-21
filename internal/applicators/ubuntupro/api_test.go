package ubuntupro

import (
	"bytes"
	"fmt"
	"slices"
	"strings"
	"testing"
)

type apiBoundaryRunner struct {
	name       string
	args       []string
	input      []byte
	inputCalls int
	runCalls   int
	stdout     []byte
}

func (r *apiBoundaryRunner) Run(name string, args ...string) ([]byte, []byte, error) {
	r.runCalls++
	return nil, nil, fmt.Errorf("ordinary Run must not be used: %s %v", name, args)
}

func (r *apiBoundaryRunner) RunInput(name string, input []byte, args ...string) ([]byte, []byte, error) {
	r.inputCalls++
	r.name = name
	r.args = append([]string(nil), args...)
	r.input = append([]byte(nil), input...)
	return append([]byte(nil), r.stdout...), nil, nil
}

// OS-UPM-010, OS-UPM-037, OS-UPM-039, OS-LPC-019, and OS-LPC-020: Canonical's
// v32 full-token endpoint receives a typed JSON object through protected stdin
// and no token-bearing or legacy command-line representation exists.
func TestAPIClientFullTokenAttachUsesExactProtectedProcessBoundary(t *testing.T) {
	const tokenCanary = "ubuntu-pro-process-boundary-token-canary"
	runner := &apiBoundaryRunner{stdout: []byte(`{
  "_schema_version":"v1",
  "data":{"attributes":{"enabled":[],"reboot_required":false},"meta":{"environment_vars":[]},"type":"FullTokenAttachResult"},
  "errors":[],"result":"success","version":"32.3ubuntu0","warnings":[]
}`)}
	token := []byte(tokenCanary)
	result, err := NewAPIClient(runner).FullTokenAttach(token)
	if err != nil {
		t.Fatal(err)
	}
	if runner.inputCalls != 1 || runner.runCalls != 0 {
		t.Fatalf("RunInput calls = %d, ordinary Run calls = %d", runner.inputCalls, runner.runCalls)
	}
	wantArgs := []string{"api", "u.pro.attach.token.full_token_attach.v1", "--data", "-"}
	if runner.name != "/usr/bin/pro" || !slices.Equal(runner.args, wantArgs) {
		t.Fatalf("process boundary = %q %q, want /usr/bin/pro %q", runner.name, runner.args, wantArgs)
	}
	wantInput := []byte(`{"token":"` + tokenCanary + `","auto_enable_services":false}`)
	if !bytes.Equal(runner.input, wantInput) {
		t.Fatalf("protected stdin = %s, want exact typed request %s", runner.input, wantInput)
	}
	argv := runner.name + " " + strings.Join(runner.args, " ")
	for _, forbidden := range []string{tokenCanary, "--args", " attach ", " enable ", " disable ", " status ", "sh -c"} {
		if strings.Contains(argv, forbidden) {
			t.Fatalf("unsafe process boundary contains %q: %s", forbidden, argv)
		}
	}
	for index, value := range token {
		if value != 0 {
			t.Fatalf("caller token byte %d was not cleared", index)
		}
	}
	if len(result.Enabled) != 0 || result.RebootRequired || result.ClientVersion != "32.3ubuntu0" {
		t.Fatalf("attach result = %#v", result)
	}
}
