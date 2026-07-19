package performance_test

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/DavidHoenisch/remotr/internal/performance"
	"github.com/DavidHoenisch/remotr/test/testsupport"
)

func TestDiagnosticsExposeOnlyBoundedRuntimeMetrics(t *testing.T) {
	recorder := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodGet, "/debug/remotr/metrics", nil)
	performance.NewDiagnosticsHandler().ServeHTTP(recorder, request)

	if recorder.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", recorder.Code)
	}
	var got map[string]any
	if err := json.Unmarshal(recorder.Body.Bytes(), &got); err != nil {
		t.Fatal(err)
	}
	for _, key := range []string{"heapAllocBytes", "heapObjects", "goroutines", "gcCycles"} {
		if _, ok := got[key]; !ok {
			t.Errorf("runtime metrics omitted %q: %s", key, recorder.Body.String())
		}
	}
	for _, forbidden := range []string{"environment", "headers", "requests", "credentials", "payloads"} {
		if _, ok := got[forbidden]; ok {
			t.Errorf("runtime metrics exposed %q", forbidden)
		}
	}
}

func TestSanitizeDiagnosticIsBoundedAndRemovesCredentialCanaries(t *testing.T) {
	canary := testsupport.SecretCanary("performance-profile")
	input := []byte("hotFunction\nAuthorization: Bearer " + canary + "\npostgres://operator:" + canary + "@database/remotr\n" + string(bytes.Repeat([]byte("x"), 256)))
	got := performance.SanitizeDiagnostic(input, 128)
	if len(got) > 128 {
		t.Fatalf("sanitized diagnostic bytes=%d, want <=128", len(got))
	}
	if bytes.Contains(got, []byte(canary)) || bytes.Contains(got, []byte("Bearer")) || bytes.Contains(got, []byte("operator:")) {
		t.Fatalf("sanitized diagnostic leaked credentials: %q", got)
	}
	if !bytes.Contains(got, []byte("hotFunction")) {
		t.Fatalf("sanitized diagnostic lost safe function evidence: %q", got)
	}
}

func TestDiagnosticsListenAddressMustBeLoopback(t *testing.T) {
	for _, address := range []string{"127.0.0.1:6060", "localhost:6060", "[::1]:6060"} {
		if err := performance.ValidateDiagnosticsAddress(address); err != nil {
			t.Errorf("ValidateDiagnosticsAddress(%q): %v", address, err)
		}
	}
	for _, address := range []string{":6060", "0.0.0.0:6060", "192.0.2.10:6060", "127.0.0.1"} {
		if err := performance.ValidateDiagnosticsAddress(address); err == nil {
			t.Errorf("ValidateDiagnosticsAddress(%q) accepted a non-loopback or malformed address", address)
		}
	}
}

func TestDiagnosticsExposeStandardProfilesOnTheControlledMux(t *testing.T) {
	recorder := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodGet, "/debug/pprof/goroutine?debug=1", nil)
	performance.NewDiagnosticsHandler().ServeHTTP(recorder, request)

	if recorder.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", recorder.Code)
	}
	if recorder.Body.Len() == 0 {
		t.Fatal("goroutine profile is empty")
	}
}
