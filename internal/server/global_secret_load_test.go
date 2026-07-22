package server

import (
	"bytes"
	"context"
	"crypto/tls"
	"crypto/x509"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"net/url"
	"sync"
	"sync/atomic"
	"testing"

	"github.com/DavidHoenisch/remotr/internal/models"
	"github.com/DavidHoenisch/remotr/internal/registry"
	"github.com/DavidHoenisch/remotr/internal/secrets"
)

const (
	globalSecretLoadFleetCount    = 8
	globalSecretLoadEndpointCount = 400
	globalSecretLoadConcurrency   = 64
)

// OS-LSM-064/067/068: a global activation may overlap authenticated endpoint
// resolution without exposing an uncommitted version or multiplying activation
// planning by endpoint count. The channel boundary makes the overlap exact and
// deterministic; this scenario contains no wall-clock sleeps.
func TestAuthenticatedGlobalSecretActivationResolutionLoadHarness(t *testing.T) {
	const (
		reference = "remotr:ubuntu-pro/load-shared@active"
		canary    = "global-load-canary"
	)
	repoDir := t.TempDir()
	artifactStore := &OnDemandArtifactResolver{RepoRoot: repoDir}
	digests := make(map[string]string, globalSecretLoadFleetCount)
	for fleetIndex := range globalSecretLoadFleetCount {
		fleet := fmt.Sprintf("fleet-%02d", fleetIndex)
		writeTestFleetDesired(t, repoDir, fleet, `schemaVersion: 1
configurations:
  - name: subscriptions
    resources:
      - kind: ubuntuPro
        name: primary
        lifecycle: attached
        tokenRef: `+reference+`
`)
		_, digest, err := resolveFleetDesiredArtifact(t.Context(), artifactStore, repoDir, fleet, "release-load")
		if err != nil {
			t.Fatal(err)
		}
		digests[fleet] = digest
	}

	planner := &blockingGlobalLoadPlanner{entered: make(chan struct{}), release: make(chan struct{})}
	repository := &countingVersionRepository{VersionRepository: secrets.NewMemoryVersionRepository()}
	keyring, err := secrets.NewKeyring("load-kek", map[string][]byte{"load-kek": bytes.Repeat([]byte{0x7b}, 32)})
	if err != nil {
		t.Fatal(err)
	}
	envelope, err := secrets.NewEnvelope(keyring)
	if err != nil {
		t.Fatal(err)
	}
	service, err := secrets.NewRegistryService(repository, envelope, planner, nil)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := service.Upload(t.Context(), secrets.UploadRequest{
		Name: "ubuntu-pro/load-shared", Scope: secrets.ScopeGlobal, Material: []byte(canary), ActorID: "load-operator",
	}); err != nil {
		t.Fatal(err)
	}

	uses := make([]secrets.ActivationUse, 0, globalSecretLoadFleetCount)
	for fleetIndex := range globalSecretLoadFleetCount {
		fleet := fmt.Sprintf("fleet-%02d", fleetIndex)
		use := secrets.ActivationUse{
			Fleet: fleet, ResourceAddress: "subscriptions/primary", Purpose: "ubuntu-pro-token", Risk: models.RiskNormal,
			ReleaseRef: "release-load", ArtifactDigest: digests[fleet],
		}
		for endpointIndex := fleetIndex; endpointIndex < globalSecretLoadEndpointCount; endpointIndex += globalSecretLoadFleetCount {
			use.EndpointIDs = append(use.EndpointIDs, globalSecretLoadEndpointID(endpointIndex))
		}
		uses = append(uses, use)
	}

	reg := registry.NewMemory()
	for endpointIndex := range globalSecretLoadEndpointCount {
		fleet := fmt.Sprintf("fleet-%02d", endpointIndex%globalSecretLoadFleetCount)
		if err := reg.RegisterEndpoint(registry.Endpoint{ID: globalSecretLoadEndpointID(endpointIndex), Fleet: fleet}); err != nil {
			t.Fatal(err)
		}
	}
	srv := New(Config{
		ConfigRepoPath: repoDir, ArtifactStore: artifactStore, ReleaseRef: "release-load", Registry: reg, Secrets: service,
	})

	activationResult := make(chan error, 1)
	go func() {
		_, activateErr := service.Activate(context.Background(), secrets.ActivationRequest{
			Name: "ubuntu-pro/load-shared", Version: "1", ActorID: "load-operator", Uses: uses,
		})
		activationResult <- activateErr
	}()
	<-planner.entered

	before := runGlobalSecretResolutionLoadWave(t, srv.Handler(), digests, reference, canary)
	if before.successes != 0 || before.denials != globalSecretLoadEndpointCount || before.other != 0 {
		t.Fatalf("pre-commit load wave = %+v", before)
	}
	close(planner.release)
	if err := <-activationResult; err != nil {
		t.Fatalf("activate global secret: %v", err)
	}

	after := runGlobalSecretResolutionLoadWave(t, srv.Handler(), digests, reference, canary)
	if after.successes != globalSecretLoadEndpointCount || after.denials != 0 || after.other != 0 {
		t.Fatalf("post-commit load wave = %+v", after)
	}
	if planner.calls.Load() != 1 || planner.useCount.Load() != globalSecretLoadFleetCount {
		t.Fatalf("activation fan-out calls=%d uses=%d, want one plan with one use per fleet", planner.calls.Load(), planner.useCount.Load())
	}
	if repository.activeReads.Load() != 2*globalSecretLoadEndpointCount {
		t.Fatalf("active-version reads=%d, want one bounded read per authenticated request", repository.activeReads.Load())
	}
	metadata, err := service.ListMetadata(t.Context(), "ubuntu-pro/load-shared")
	if err != nil || len(metadata) != 1 || len(metadata[0].Rollouts) != globalSecretLoadFleetCount {
		t.Fatalf("global activation metadata=%#v err=%v", metadata, err)
	}
}

type globalSecretLoadWave struct {
	successes int64
	denials   int64
	other     int64
}

func runGlobalSecretResolutionLoadWave(t *testing.T, handler http.Handler, digests map[string]string, reference, canary string) globalSecretLoadWave {
	t.Helper()
	var result globalSecretLoadWave
	sem := make(chan struct{}, globalSecretLoadConcurrency)
	var group sync.WaitGroup
	for endpointIndex := range globalSecretLoadEndpointCount {
		endpointIndex := endpointIndex
		group.Add(1)
		go func() {
			defer group.Done()
			sem <- struct{}{}
			defer func() { <-sem }()
			fleet := fmt.Sprintf("fleet-%02d", endpointIndex%globalSecretLoadFleetCount)
			body, _ := json.Marshal(map[string]string{
				"reference": reference, "artifactDigest": digests[fleet],
				"resourceAddress": "subscriptions/primary", "purpose": "ubuntu-pro-token",
			})
			uri, _ := url.Parse("urn:remotr:endpoint:" + globalSecretLoadEndpointID(endpointIndex))
			req := httptest.NewRequest(http.MethodPost, "/v1/secrets/resolve", bytes.NewReader(body))
			req.TLS = &tls.ConnectionState{PeerCertificates: []*x509.Certificate{{URIs: []*url.URL{uri}}}}
			rec := httptest.NewRecorder()
			handler.ServeHTTP(rec, req)
			switch rec.Code {
			case http.StatusOK:
				var resolved secrets.Resolved
				if json.Unmarshal(rec.Body.Bytes(), &resolved) != nil || string(resolved.Material) != canary {
					atomic.AddInt64(&result.other, 1)
					return
				}
				atomic.AddInt64(&result.successes, 1)
			case http.StatusForbidden:
				atomic.AddInt64(&result.denials, 1)
			default:
				atomic.AddInt64(&result.other, 1)
			}
		}()
	}
	group.Wait()
	return result
}

func globalSecretLoadEndpointID(index int) string {
	return fmt.Sprintf("00000000-0000-0000-0000-%012d", index+1)
}

type blockingGlobalLoadPlanner struct {
	entered   chan struct{}
	release   chan struct{}
	enterOnce sync.Once
	calls     atomic.Int64
	useCount  atomic.Int64
}

func (p *blockingGlobalLoadPlanner) CreateActivationRollouts(ctx context.Context, plan secrets.ActivationPlan) ([]secrets.RolloutBinding, error) {
	p.calls.Add(1)
	p.useCount.Add(int64(len(plan.Uses)))
	p.enterOnce.Do(func() { close(p.entered) })
	select {
	case <-ctx.Done():
		return nil, ctx.Err()
	case <-p.release:
	}
	bindings := make([]secrets.RolloutBinding, len(plan.Uses))
	for index, use := range plan.Uses {
		bindings[index] = secrets.RolloutBinding{
			Fleet: use.Fleet, ResourceAddress: use.ResourceAddress, Purpose: use.Purpose, Risk: use.Risk,
			EffectiveHash: "sha256:aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa",
		}
	}
	return bindings, nil
}

type countingVersionRepository struct {
	secrets.VersionRepository
	activeReads atomic.Int64
}

func (r *countingVersionRepository) GetActiveVersion(ctx context.Context, name string) (secrets.StoredVersion, error) {
	r.activeReads.Add(1)
	return r.VersionRepository.GetActiveVersion(ctx, name)
}
