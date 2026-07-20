package downloads_test

import (
	"context"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"sync/atomic"
	"testing"

	"github.com/DavidHoenisch/remotr/internal/applicators/downloads"
	"github.com/DavidHoenisch/remotr/internal/models"
	contract "github.com/DavidHoenisch/remotr/internal/providercontract"
	harness "github.com/DavidHoenisch/remotr/test/providercontract"
	"github.com/DavidHoenisch/remotr/test/testsupport"
)

// OS-AEC-097: the checksum-qualified download contract converges through its
// public provider seam and the second Check prevents another network request.
func TestApplicator_ConformsForVerifiedDownload(t *testing.T) {
	harness.RunConvergence(t, harness.Fixture{
		Compliant: func(t *testing.T) contract.Provider { return newDownloadContractProvider(t, true) },
		Drifted:   func(t *testing.T) contract.Provider { return newDownloadContractProvider(t, false) },
	})
}

// OS-AEC-097: authenticated transport keeps the token at the HTTP boundary,
// verifies the payload, and leaves a compliant destination after Apply.
func TestApplicator_AuthenticatedTransportConverges(t *testing.T) {
	content := []byte("authenticated payload\n")
	canary := testsupport.SecretCanary("ubuntu-download-transport")
	var requests atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, request *http.Request) {
		requests.Add(1)
		if got := request.Header.Get("Authorization"); got != "Bearer "+canary {
			t.Errorf("Authorization = %q, want bearer canary", got)
			http.Error(w, "unauthorized", http.StatusUnauthorized)
			return
		}
		_, _ = w.Write(content)
	}))
	t.Cleanup(server.Close)

	dest := filepath.Join(t.TempDir(), "authenticated")
	applicator := downloads.New(models.DownloadResource{
		Name: "authenticated", URL: server.URL, Dest: dest,
		Checksum: "sha256:" + sha256Hex(content), AuthenticationRef: "remotr:download-token@active",
		RedirectPolicy: "same-origin", Timeout: "5s", Mode: []int{0o600},
	}, nil)
	applicator.ResolveSecret = func(context.Context, string) (string, error) { return canary, nil }
	provider, err := contract.New(applicator)
	if err != nil {
		t.Fatal(err)
	}

	if check := provider.Check(context.Background()); check.Status != contract.Drifted {
		t.Fatalf("initial Check = %+v, want drifted", check)
	}
	if result := provider.Apply(context.Background()); result.Status != contract.Changed || result.Err != nil {
		t.Fatalf("Apply = %+v, want changed", result)
	}
	if check := provider.Check(context.Background()); check.Status != contract.Compliant {
		t.Fatalf("second Check = %+v, want compliant", check)
	}
	if result := provider.Apply(context.Background()); result.Status != contract.NoChange || result.Err != nil {
		t.Fatalf("second Apply = %+v, want no-change", result)
	}
	if got := requests.Load(); got != 1 {
		t.Fatalf("HTTP requests = %d, want one", got)
	}
}

// OS-AEC-097: redirect policy is enforced before an untrusted origin can
// receive the request or replace the active destination.
func TestApplicator_RedirectPolicyPreservesActiveDestination(t *testing.T) {
	content := []byte("redirected payload\n")
	for _, test := range []struct {
		policy      string
		wantStatus  contract.ApplyStatus
		wantTarget  int32
		wantContent string
	}{
		{policy: "none", wantStatus: contract.Failed, wantContent: "active\n"},
		{policy: "same-origin", wantStatus: contract.Failed, wantContent: "active\n"},
		{policy: "follow", wantStatus: contract.Changed, wantTarget: 1, wantContent: string(content)},
	} {
		t.Run(test.policy, func(t *testing.T) {
			var targetRequests atomic.Int32
			target := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
				targetRequests.Add(1)
				_, _ = w.Write(content)
			}))
			t.Cleanup(target.Close)
			source := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, request *http.Request) {
				http.Redirect(w, request, target.URL, http.StatusFound)
			}))
			t.Cleanup(source.Close)

			dir := t.TempDir()
			dest := filepath.Join(dir, "redirected")
			if err := os.WriteFile(dest, []byte("active\n"), 0o600); err != nil {
				t.Fatal(err)
			}
			provider, err := contract.New(downloads.New(models.DownloadResource{
				Name: "redirected", URL: source.URL, Dest: dest,
				Checksum: "sha256:" + sha256Hex(content), RedirectPolicy: test.policy, Timeout: "5s",
			}, nil))
			if err != nil {
				t.Fatal(err)
			}

			result := provider.Apply(context.Background())
			if result.Status != test.wantStatus || (test.wantStatus == contract.Failed) != (result.Err != nil) {
				t.Fatalf("Apply = %+v, want %s", result, test.wantStatus)
			}
			if got := targetRequests.Load(); got != test.wantTarget {
				t.Fatalf("target requests = %d, want %d", got, test.wantTarget)
			}
			if got, err := os.ReadFile(dest); err != nil || string(got) != test.wantContent {
				t.Fatalf("destination = %q, %v; want %q", got, err, test.wantContent)
			}
			entries, err := os.ReadDir(dir)
			if err != nil {
				t.Fatal(err)
			}
			if len(entries) != 1 || entries[0].Name() != "redirected" {
				t.Fatalf("redirect handling left staging artifacts: %v", entries)
			}
		})
	}
}

func newDownloadContractProvider(t *testing.T, installed bool) contract.Provider {
	t.Helper()
	content := []byte("verified download\n")
	var requests atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		requests.Add(1)
		_, _ = w.Write(content)
	}))
	t.Cleanup(server.Close)

	dest := filepath.Join(t.TempDir(), "verified-download")
	if installed {
		if err := os.WriteFile(dest, content, 0o640); err != nil {
			t.Fatal(err)
		}
	}
	applicator := downloads.New(models.DownloadResource{
		Name: "verified-download", URL: server.URL, Dest: dest,
		Checksum: "sha256:" + sha256Hex(content), RedirectPolicy: "follow", Timeout: "5s", Mode: []int{0o640},
	}, nil)
	provider, err := contract.New(applicator)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		want := int32(1)
		if installed {
			want = 0
		}
		if got := requests.Load(); got != want {
			t.Errorf("HTTP requests = %d, want %d", got, want)
		}
	})
	return provider
}
