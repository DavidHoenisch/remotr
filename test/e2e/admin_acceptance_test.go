//go:build e2e

package e2e

import (
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/DavidHoenisch/remotr/internal/agent/sync"
	"github.com/DavidHoenisch/remotr/internal/tlsconfig"
	"github.com/DavidHoenisch/remotr/test/acceptance"
)

type adminTracerState struct {
	t             *testing.T
	baseURL       string
	caPath        string
	operatorState string
	bootstrap     string
	operatorReady bool
	bootstrapUsed bool
	enrollToken   string
	agentState    string
	agentName     string
	endpointID    string
	labels        map[string]string
	artifactReady bool
	appPackages   []appPackage
	endpoints     []endpointJSON
}

type appPackage struct {
	Name    string `json:"name"`
	Version string `json:"version"`
}

func runAdminWorkflowTracers(t *testing.T) {
	t.Helper()
	skipAdminIfUnavailable(t)

	state := &adminTracerState{
		t:             t,
		baseURL:       baseURL(),
		caPath:        envOr("REMOTR_E2E_CA", defaultCAPath()),
		operatorState: filepath.Join(t.TempDir(), "operator"),
		agentState:    filepath.Join(t.TempDir(), "agent"),
	}
	featureDir := filepath.Join(repoRoot(t), "test", "acceptance", "features")
	acceptance.RunFeatureFiles(t, []string{
		filepath.Join(featureDir, "operator_bootstrap.feature"),
		filepath.Join(featureDir, "enrollment_sync.feature"),
		filepath.Join(featureDir, "app_package.feature"),
		filepath.Join(featureDir, "endpoint_labels.feature"),
	}, state.registerSteps)
}

func (s *adminTracerState) registerSteps(steps *acceptance.ScenarioSteps) {
	steps.Step(`^an available one-time operator bootstrap token$`, s.loadBootstrapToken)
	steps.Step(`^the operator bootstraps credentials$`, s.bootstrapOperator)
	steps.Step(`^endpoint listing succeeds with those credentials$`, s.listEndpoints)
	steps.Step(`^the operator reuses the bootstrap token$`, s.reuseBootstrapToken)
	steps.Step(`^bootstrap is rejected$`, s.bootstrapRejected)
	steps.Step(`^an authenticated operator$`, s.ensureAuthenticatedOperator)
	steps.Step(`^the operator creates an enrollment token for "([^"]*)"$`, s.createEnrollmentToken)
	steps.Step(`^an agent enrolls using that token$`, s.enrollAgent)
	steps.Step(`^the agent stores credentials$`, s.agentCredentialsStored)
	steps.Step(`^the enrolled agent Syncs$`, s.syncEnrolledAgent)
	steps.Step(`^it receives an authenticated artifact$`, s.authenticatedArtifactDelivered)
	steps.Step(`^the operator lists application packages$`, s.listApplicationPackages)
	steps.Step(`^the seeded "([^"]*)" version "([^"]*)" package is visible$`, s.seededPackageVisible)
	steps.Step(`^an authenticated operator and enrolled agent "([^"]*)"$`, s.authenticatedOperatorAndAgent)
	steps.Step(`^the agent Syncs labels "([^"]*)"$`, s.syncLabels)
	steps.Step(`^endpoint listing shows those labels$`, s.endpointListingShowsLabels)
}

func (s *adminTracerState) loadBootstrapToken() error {
	s.bootstrap = waitBootstrapToken(s.t, 60*time.Second)
	if s.bootstrap == "" {
		return fmt.Errorf("bootstrap token unavailable; start a fresh Compose stack with make test-e2e")
	}
	return nil
}

func (s *adminTracerState) bootstrapOperator() error {
	if s.bootstrap == "" {
		return fmt.Errorf("bootstrap token was not loaded")
	}
	if _, err := s.operatorCommand("bootstrap", "--token", s.bootstrap); err != nil {
		return err
	}
	s.operatorReady = true
	return nil
}

func (s *adminTracerState) listEndpoints() error {
	out, err := s.operatorCommand("endpoint", "list", "--json")
	if err != nil {
		return err
	}
	if err := json.Unmarshal([]byte(out), &s.endpoints); err != nil {
		return fmt.Errorf("decode endpoint list: %w", err)
	}
	return nil
}

func (s *adminTracerState) reuseBootstrapToken() error {
	_, err := s.operatorCommand("bootstrap", "--token", s.bootstrap)
	if err == nil {
		return fmt.Errorf("reused bootstrap token was accepted")
	}
	if !strings.Contains(err.Error(), "401") || !strings.Contains(err.Error(), "invalid bootstrap token") {
		return fmt.Errorf("bootstrap reuse did not report token rejection: %w", err)
	}
	s.bootstrapUsed = true
	return nil
}

func (s *adminTracerState) bootstrapRejected() error {
	if !s.bootstrapUsed {
		return fmt.Errorf("bootstrap reuse was not attempted")
	}
	return nil
}

func (s *adminTracerState) ensureAuthenticatedOperator() error {
	if s.operatorReady {
		return nil
	}
	if err := s.loadBootstrapToken(); err != nil {
		return err
	}
	if err := s.bootstrapOperator(); err != nil {
		return err
	}
	return s.listEndpoints()
}

func (s *adminTracerState) createEnrollmentToken(fleet string) error {
	tokenPath := filepath.Join(s.t.TempDir(), "enrollment.token")
	if _, err := s.operatorCommand("enroll", "token", "create", "--fleet", fleet, "--out", tokenPath, "--quiet"); err != nil {
		return err
	}
	token, err := os.ReadFile(tokenPath)
	if err != nil {
		return fmt.Errorf("read enrollment token file: %w", err)
	}
	s.enrollToken = strings.TrimSpace(string(token))
	if s.enrollToken == "" {
		return fmt.Errorf("enrollment token file was empty")
	}
	return nil
}

func (s *adminTracerState) enrollAgent() error {
	if s.enrollToken == "" {
		return fmt.Errorf("enrollment token was not created")
	}
	command := exec.Command("go", "run", "-mod=vendor", "./cmd/remotr-agent", "enroll",
		"--token", s.enrollToken,
		"--server-url", s.baseURL,
		"--ca", s.caPath,
		"--state-dir", s.agentState,
		"--endpoint-id", "godog-agent",
		"--no-sync",
	)
	command.Dir = repoRoot(s.t)
	if output, err := command.CombinedOutput(); err != nil {
		return commandFailure("remotr-agent enroll", output, err)
	}
	return nil
}

func (s *adminTracerState) agentCredentialsStored() error {
	for _, name := range []string{"state.json", "agent.crt", "agent.key", "ca.crt"} {
		info, err := os.Stat(filepath.Join(s.agentState, name))
		if err != nil {
			return fmt.Errorf("stored agent credential %s: %w", name, err)
		}
		if info.Size() == 0 {
			return fmt.Errorf("stored agent credential %s is empty", name)
		}
	}
	return nil
}

func (s *adminTracerState) syncEnrolledAgent() error {
	tlsConfig, err := tlsconfig.ClientTLSConfig(
		filepath.Join(s.agentState, "agent.crt"),
		filepath.Join(s.agentState, "agent.key"),
		filepath.Join(s.agentState, "ca.crt"),
	)
	if err != nil {
		return fmt.Errorf("load stored agent credentials: %w", err)
	}
	response, err := sync.NewClient(s.baseURL, tlsConfig).Sync(qualifiedPackageSyncRequest(s.t, "debian"))
	if err != nil {
		return fmt.Errorf("authenticated Sync: %w", err)
	}
	if response.Unchanged || len(response.ArtifactYAML) == 0 {
		return fmt.Errorf("initial Sync did not deliver an artifact")
	}
	if !strings.Contains(string(response.ArtifactYAML), "configurations:") {
		return fmt.Errorf("authenticated artifact has no configurations section")
	}
	s.artifactReady = true
	return nil
}

func (s *adminTracerState) authenticatedArtifactDelivered() error {
	if !s.artifactReady {
		return fmt.Errorf("authenticated artifact was not delivered")
	}
	return nil
}

func (s *adminTracerState) listApplicationPackages() error {
	out, err := s.operatorCommand("app", "list", "--json")
	if err != nil {
		return err
	}
	if err := json.Unmarshal([]byte(out), &s.appPackages); err != nil {
		return fmt.Errorf("decode application package list: %w", err)
	}
	return nil
}

func (s *adminTracerState) seededPackageVisible(name, version string) error {
	for _, item := range s.appPackages {
		if item.Name == name && item.Version == version {
			return nil
		}
	}
	return fmt.Errorf("seeded package %s@%s was not listed", name, version)
}

func (s *adminTracerState) authenticatedOperatorAndAgent(agent string) error {
	if err := s.ensureAuthenticatedOperator(); err != nil {
		return err
	}
	deadline := time.Now().Add(90 * time.Second)
	for time.Now().Before(deadline) {
		tlsConfig, endpointID, err := enrolledAgentTLS(agent)
		if err == nil {
			s.agentName = agent
			s.endpointID = endpointID
			if tlsConfig == nil || endpointID == "" {
				return fmt.Errorf("agent %q has incomplete enrollment state", agent)
			}
			return nil
		}
		time.Sleep(500 * time.Millisecond)
	}
	return fmt.Errorf("agent %q did not finish enrollment", agent)
}

func (s *adminTracerState) syncLabels(serialized string) error {
	labels, err := parseLabels(serialized)
	if err != nil {
		return err
	}
	tlsConfig, _, err := enrolledAgentTLS(s.agentName)
	if err != nil {
		return err
	}
	request := qualifiedPackageSyncRequest(s.t, s.agentName)
	request.Labels = labels
	if _, err := sync.NewClient(s.baseURL, tlsConfig).Sync(request); err != nil {
		return fmt.Errorf("Sync labels: %w", err)
	}
	s.labels = labels
	return nil
}

func (s *adminTracerState) endpointListingShowsLabels() error {
	if err := s.listEndpoints(); err != nil {
		return err
	}
	for _, endpoint := range s.endpoints {
		if endpoint.ID != s.endpointID {
			continue
		}
		if endpoint.Labels["site"] == s.labels["site"] && endpoint.Labels["role"] == s.labels["role"] {
			return nil
		}
		return fmt.Errorf("endpoint labels = %+v", endpoint.Labels)
	}
	return fmt.Errorf("endpoint %s was not listed", s.endpointID)
}

func (s *adminTracerState) operatorCommand(args ...string) (string, error) {
	command := remotrCommand(s.t, s.baseURL, s.caPath, s.operatorState, args...)
	output, err := command.CombinedOutput()
	if err != nil {
		return "", commandFailure("remotr "+strings.Join(args[:min(2, len(args))], " "), output, err)
	}
	return string(output), nil
}

func commandFailure(action string, output []byte, err error) error {
	return fmt.Errorf("%s: %w: %s", action, err, strings.TrimSpace(string(output)))
}

func parseLabels(serialized string) (map[string]string, error) {
	labels := make(map[string]string)
	for _, pair := range strings.Split(serialized, ",") {
		key, value, ok := strings.Cut(pair, "=")
		if !ok || key == "" || value == "" {
			return nil, fmt.Errorf("invalid label %q", pair)
		}
		labels[key] = value
	}
	return labels, nil
}
