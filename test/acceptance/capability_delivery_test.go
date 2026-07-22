package acceptance

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
	"path/filepath"
	"testing"
	"time"

	"github.com/DavidHoenisch/remotr/internal/agent/facts"
	"github.com/DavidHoenisch/remotr/internal/capabilitydoc"
	"github.com/DavidHoenisch/remotr/internal/configrepo"
	"github.com/DavidHoenisch/remotr/internal/registry"
	"github.com/DavidHoenisch/remotr/internal/server"
	"github.com/DavidHoenisch/remotr/internal/types"
)

func TestCapabilityDeliveryWorkflow(t *testing.T) {
	state := &capabilityDeliveryAcceptanceState{endpointID: "62000000-0000-0000-0000-000000000006"}
	RunFeatureFiles(t, []string{filepath.Join("features", "capability_delivery.feature")}, func(steps *ScenarioSteps) {
		steps.Step(`^the representative mixed-target Ubuntu Pro repository$`, state.selectRepository)
		steps.Step(`^the operator validates it and the server accepts the Git snapshot$`, state.validateAndSyncRepository)
		steps.Step(`^Ubuntu delivery is not rejected for the Arch package branch$`, state.assertRepositoryAccepted)
		steps.Step(`^a legacy Ubuntu endpoint is explicitly targeted for the approved current agent$`, state.registerLegacyEndpoint)
		steps.Step(`^the legacy endpoint performs authenticated Sync$`, state.legacySync)
		steps.Step(`^artifact delivery is blocked and the approved agent upgrade is returned$`, state.assertBlockedUpgrade)
		steps.Step(`^the upgraded endpoint reports its current observed capabilities$`, state.currentSync)
		steps.Step(`^the complete Ubuntu artifact is offered without Pacman requirements$`, state.assertOffer)
		steps.Step(`^the endpoint acknowledges the exact offered digest$`, state.acknowledge)
		steps.Step(`^fleet delivery state reports that digest active$`, state.assertActive)
	})
}

type capabilityDeliveryAcceptanceState struct {
	endpointID string
	repo       string
	reg        *registry.Memory
	server     *server.Server
	validation configrepo.ValidationResult
	legacy     capabilityDeliverySyncResponse
	offer      capabilityDeliverySyncResponse
	ack        capabilityDeliverySyncResponse
	document   capabilitydoc.Document
}

type capabilityDeliverySyncResponse struct {
	Unchanged         bool            `json:"unchanged"`
	ReleaseRef        string          `json:"releaseRef"`
	Digest            string          `json:"digest"`
	ArtifactYAML      []byte          `json:"artifactYaml"`
	CapabilityBlocked json.RawMessage `json:"capabilityBlocked"`
	AgentUpgrade      *struct {
		Version string `json:"version"`
	} `json:"agentUpgrade"`
}

func (s *capabilityDeliveryAcceptanceState) selectRepository() error {
	s.repo = filepath.Join(repositoryRoot(), "test", "config-repos", "capability-delivery-blockers")
	return nil
}

func (s *capabilityDeliveryAcceptanceState) validateAndSyncRepository() error {
	result, err := configrepo.ValidateRepository(s.repo)
	if err != nil {
		return err
	}
	s.validation = result
	if len(result.Issues) != 0 {
		return fmt.Errorf("local validation issues: %+v", result.Issues)
	}
	s.reg = registry.NewMemory()
	s.server = server.New(server.Config{ConfigRepoPath: s.repo, ReleaseRef: "git-release-qualified", Registry: s.reg, Admin: s.reg})
	return nil
}

func (s *capabilityDeliveryAcceptanceState) assertRepositoryAccepted() error {
	if s.server == nil || len(s.validation.OK) == 0 {
		return fmt.Errorf("validated Git snapshot was not accepted")
	}
	return nil
}

func (s *capabilityDeliveryAcceptanceState) registerLegacyEndpoint() error {
	now := time.Now().UTC()
	return s.reg.RegisterEndpoint(registry.Endpoint{
		ID: s.endpointID, Fleet: "engineering", ReportedAgentVersion: "v0.5.1", DesiredAgentVersion: "v0.6.8",
		LastCheckIn: &registry.CheckInSummary{ReleaseRef: "previous-release", Digest: "previous-digest", At: now},
	})
}

func (s *capabilityDeliveryAcceptanceState) legacySync() error {
	document, err := (capabilitydoc.Document{
		DocumentVersion: 1, ArtifactSchemaVersions: []int{1}, AgentVersion: "v0.5.1",
		Capabilities: []capabilitydoc.Capability{{ID: "provider:package/apt", Revision: "1"}},
		Facts:        []capabilitydoc.Fact{{Key: "architecture", Value: "x86"}, {Key: "package", Value: "apt"}},
	}).WithCanonicalDigest()
	if err != nil {
		return err
	}
	s.legacy, err = s.sync(map[string]any{
		"agentVersion": "v0.5.1", "capabilityDocument": document,
		"lastReleaseRef": "previous-release", "lastDigest": "previous-digest",
	})
	return err
}

func (s *capabilityDeliveryAcceptanceState) assertBlockedUpgrade() error {
	if len(s.legacy.CapabilityBlocked) == 0 || s.legacy.AgentUpgrade == nil || s.legacy.AgentUpgrade.Version != "v0.6.8" || len(s.legacy.ArtifactYAML) != 0 {
		return fmt.Errorf("legacy Sync did not return blocked upgrade: %+v", s.legacy)
	}
	return nil
}

func (s *capabilityDeliveryAcceptanceState) currentSync() error {
	if err := s.reg.UpdateAgentUpgradeReport(context.Background(), s.endpointID, "v0.6.8", "completed", "", true); err != nil {
		return err
	}
	generator, err := capabilitydoc.NewDefaultGenerator([]int{1})
	if err != nil {
		return err
	}
	document, err := generator.Generate(facts.Facts{
		Distro: types.Ubuntu, DistroVersion: "26.04", Arch: types.X86,
		OSID: "ubuntu", OSReleaseSourceCount: 2, OSReleaseConsistent: true, DistroVendor: "Ubuntu",
		Init: facts.InitSystemd, Package: types.Apt,
		UniversalPackage: []types.PackageManager{types.Flatpak},
		Browser:          []facts.BrowserBackend{facts.BrowserChromium},
	}, "v0.6.8")
	if err != nil {
		return err
	}
	s.document = document
	s.offer, err = s.sync(map[string]any{
		"agentVersion": "v0.6.8", "capabilityDocument": document,
		"lastReleaseRef": "previous-release", "lastDigest": "previous-digest",
	})
	return err
}

func (s *capabilityDeliveryAcceptanceState) assertOffer() error {
	if len(s.offer.CapabilityBlocked) != 0 || s.offer.Digest == "" || len(s.offer.ArtifactYAML) == 0 {
		return fmt.Errorf("qualified Sync did not offer artifact: %+v", s.offer)
	}
	if !bytes.Contains(s.offer.ArtifactYAML, []byte("kind: ubuntuPro")) || !bytes.Contains(s.offer.ArtifactYAML, []byte("name: pacman-example")) {
		return fmt.Errorf("offered artifact is incomplete")
	}
	return nil
}

func (s *capabilityDeliveryAcceptanceState) acknowledge() error {
	var err error
	s.ack, err = s.sync(map[string]any{
		"agentVersion": "v0.6.8", "capabilityDocument": s.document,
		"lastReleaseRef": s.offer.ReleaseRef, "lastDigest": s.offer.Digest,
	})
	return err
}

func (s *capabilityDeliveryAcceptanceState) assertActive() error {
	if !s.ack.Unchanged {
		return fmt.Errorf("exact acknowledgment was not unchanged: %+v", s.ack)
	}
	state, ok, err := s.reg.GetEndpointDeliveryState(context.Background(), s.endpointID)
	if err != nil || !ok || state.ActiveDigest != s.offer.Digest || state.ActiveReleaseRef != s.offer.ReleaseRef {
		return fmt.Errorf("active delivery state = %+v ok=%t err=%v", state, ok, err)
	}
	return nil
}

func (s *capabilityDeliveryAcceptanceState) sync(body any) (capabilityDeliverySyncResponse, error) {
	raw, err := json.Marshal(body)
	if err != nil {
		return capabilityDeliverySyncResponse{}, err
	}
	identity, _ := url.Parse("urn:remotr:endpoint:" + s.endpointID)
	request := httptest.NewRequest(http.MethodPost, "/v1/sync", bytes.NewReader(raw))
	request.TLS = &tls.ConnectionState{PeerCertificates: []*x509.Certificate{{URIs: []*url.URL{identity}}}}
	recorder := httptest.NewRecorder()
	s.server.Handler().ServeHTTP(recorder, request)
	if recorder.Code != http.StatusOK {
		return capabilityDeliverySyncResponse{}, fmt.Errorf("Sync status %d: %s", recorder.Code, recorder.Body.String())
	}
	var response capabilityDeliverySyncResponse
	if err := json.Unmarshal(recorder.Body.Bytes(), &response); err != nil {
		return capabilityDeliverySyncResponse{}, err
	}
	return response, nil
}
