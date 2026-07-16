package main

import (
	"context"
	"errors"
	"io/fs"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"
	"sync"

	aibundle "github.com/DavidHoenisch/remotr/ai"
	"github.com/DavidHoenisch/remotr/internal/aisetup"
	"github.com/google/uuid"
	wailsruntime "github.com/wailsapp/wails/v2/pkg/runtime"
)

var aiReleaseVersionPattern = regexp.MustCompile(`^v?[0-9][0-9A-Za-z._-]{0,63}$`)

type AIProjectDirectoryDialog func(context.Context) (string, error)
type AIUpgradeFetcher func(context.Context, string) (aisetup.FetchedBundle, error)
type AIRuntimeLookup func(string) (string, error)

type AIIntegrationServiceOptions struct {
	ChooseProjectRoot AIProjectDirectoryDialog
	EmbeddedSource    fs.FS
	EmbeddedRoot      string
	EmbeddedVersion   string
	FetchUpgrade      AIUpgradeFetcher
	RuntimeLookup     AIRuntimeLookup
}

type AIProjectRootView struct {
	ID            string `json:"id"`
	DirectoryName string `json:"directoryName"`
	Status        string `json:"status"`
}

type AIIntegrationListRequest struct {
	Scope         string `json:"scope"`
	ProjectRootID string `json:"projectRootId"`
}

type AIIntegrationInstallRequest struct {
	Agent         string `json:"agent"`
	Scope         string `json:"scope"`
	ProjectRootID string `json:"projectRootId"`
	Replace       bool   `json:"replace"`
}

type AIIntegrationUpgradeRequest struct {
	Agent         string `json:"agent"`
	Scope         string `json:"scope"`
	ProjectRootID string `json:"projectRootId"`
	Version       string `json:"version"`
	Replace       bool   `json:"replace"`
}

type AIIntegrationView struct {
	Agent            string `json:"agent"`
	DisplayName      string `json:"displayName"`
	Scope            string `json:"scope"`
	Installed        bool   `json:"installed"`
	BundleVersion    string `json:"bundleVersion"`
	Source           string `json:"source"`
	SourceVersion    string `json:"sourceVersion"`
	RuntimeAvailable bool   `json:"runtimeAvailable"`
	RuntimeStatus    string `json:"runtimeStatus"`
	Guidance         string `json:"guidance"`
}

type AIIntegrationActionResult struct {
	Integration AIIntegrationView `json:"integration"`
	Status      string            `json:"status"`
}

type AIIntegrationService struct {
	mu                sync.Mutex
	chooseProjectRoot AIProjectDirectoryDialog
	embeddedSource    fs.FS
	embeddedRoot      string
	embeddedVersion   string
	fetchUpgrade      AIUpgradeFetcher
	runtimeLookup     AIRuntimeLookup
	projectRoots      map[string]string
	inflight          map[string]bool
}

type resolvedAIIntegrationTarget struct {
	target       aisetup.Target
	boundaryRoot string
}

func NewAIIntegrationService(options AIIntegrationServiceOptions) *AIIntegrationService {
	return &AIIntegrationService{
		chooseProjectRoot: options.ChooseProjectRoot,
		embeddedSource:    options.EmbeddedSource,
		embeddedRoot:      options.EmbeddedRoot,
		embeddedVersion:   options.EmbeddedVersion,
		fetchUpgrade:      options.FetchUpgrade,
		runtimeLookup:     options.RuntimeLookup,
		projectRoots:      map[string]string{},
		inflight:          map[string]bool{},
	}
}

func defaultAIIntegrationService(version string) *AIIntegrationService {
	return NewAIIntegrationService(AIIntegrationServiceOptions{
		ChooseProjectRoot: func(ctx context.Context) (string, error) {
			return wailsruntime.OpenDirectoryDialog(ctx, wailsruntime.OpenDialogOptions{Title: "Choose project directory for AI integration"})
		},
		EmbeddedSource:  aibundle.RemotrAgent,
		EmbeddedRoot:    aibundle.BundleRoot,
		EmbeddedVersion: version,
		FetchUpgrade: func(ctx context.Context, version string) (aisetup.FetchedBundle, error) {
			return aisetup.FetchFromGitHub(ctx, aisetup.FetchOptions{Version: version})
		},
		RuntimeLookup: exec.LookPath,
	})
}

func (s *AIIntegrationService) ChooseProjectRoot(ctx context.Context) (AIProjectRootView, error) {
	if s == nil || s.chooseProjectRoot == nil {
		return AIProjectRootView{}, errors.New("native project selection is unavailable")
	}
	root, err := s.chooseProjectRoot(ctx)
	if err != nil {
		return AIProjectRootView{}, &ActionFailure{Kind: ActionUnexpected, Message: "The project directory could not be selected.", Guidance: "Choose an existing local project directory and try again.", Retryable: true}
	}
	if root == "" {
		return AIProjectRootView{Status: "canceled"}, nil
	}
	root, err = cleanAIIntegrationRoot(root)
	if err != nil {
		return AIProjectRootView{}, err
	}
	id := uuid.NewString()
	s.mu.Lock()
	s.projectRoots[id] = root
	s.mu.Unlock()
	return AIProjectRootView{ID: id, DirectoryName: filepath.Base(root), Status: "selected"}, nil
}

func (s *AIIntegrationService) List(request AIIntegrationListRequest) ([]AIIntegrationView, error) {
	scope, err := aisetup.ParseScope(request.Scope)
	if err != nil {
		return nil, aiIntegrationValidation("Choose user or project installation scope.")
	}
	views := make([]AIIntegrationView, 0, len(aisetup.SupportedAgents()))
	for _, agent := range aisetup.SupportedAgents() {
		resolved, resolveErr := s.resolveTarget(agent, scope, request.ProjectRootID)
		if resolveErr != nil {
			return nil, resolveErr
		}
		view, viewErr := s.view(resolved, agent, scope)
		if viewErr != nil {
			return nil, viewErr
		}
		views = append(views, view)
	}
	return views, nil
}

func (s *AIIntegrationService) Setup(request AIIntegrationInstallRequest) (AIIntegrationActionResult, error) {
	agent, scope, resolved, err := s.resolveRequest(request.Agent, request.Scope, request.ProjectRootID)
	if err != nil {
		return AIIntegrationActionResult{}, err
	}
	release, err := s.begin("setup\x00" + string(agent) + "\x00" + string(scope) + "\x00" + request.ProjectRootID)
	if err != nil {
		return AIIntegrationActionResult{}, err
	}
	defer release()
	installed, err := aisetup.InstalledWithin(resolved.boundaryRoot, resolved.target.InstallDir)
	if err != nil {
		return AIIntegrationActionResult{}, aiIntegrationFilesystemFailure()
	}
	if installed && !request.Replace {
		return AIIntegrationActionResult{}, aiIntegrationValidation("Confirm replacement of the existing installation before continuing.")
	}
	if s.embeddedSource == nil || strings.TrimSpace(s.embeddedRoot) == "" {
		return AIIntegrationActionResult{}, errors.New("embedded AI integration bundle is unavailable")
	}
	manifest, err := aisetup.Install(aisetup.InstallOptions{
		Target: resolved.target, Source: s.embeddedSource, SourceRoot: s.embeddedRoot,
		SourceLabel: "embedded", SourceVersion: s.embeddedVersion, Force: request.Replace, BoundaryRoot: resolved.boundaryRoot,
	})
	if err != nil {
		return AIIntegrationActionResult{}, aiIntegrationFilesystemFailure()
	}
	return AIIntegrationActionResult{Integration: s.manifestView(agent, scope, manifest), Status: "installed"}, nil
}

func (s *AIIntegrationService) Upgrade(ctx context.Context, request AIIntegrationUpgradeRequest) (AIIntegrationActionResult, error) {
	agent, scope, resolved, err := s.resolveRequest(request.Agent, request.Scope, request.ProjectRootID)
	if err != nil {
		return AIIntegrationActionResult{}, err
	}
	version := strings.TrimSpace(request.Version)
	if version != "" && !aiReleaseVersionPattern.MatchString(version) {
		return AIIntegrationActionResult{}, aiIntegrationValidation("Enter a release version such as v2.0.0, or leave it blank for the latest stable release.")
	}
	release, err := s.begin("upgrade\x00" + string(agent) + "\x00" + string(scope) + "\x00" + request.ProjectRootID)
	if err != nil {
		return AIIntegrationActionResult{}, err
	}
	defer release()
	installed, err := aisetup.InstalledWithin(resolved.boundaryRoot, resolved.target.InstallDir)
	if err != nil {
		return AIIntegrationActionResult{}, aiIntegrationFilesystemFailure()
	}
	if installed && !request.Replace {
		return AIIntegrationActionResult{}, aiIntegrationValidation("Confirm replacement of the existing installation before downloading an upgrade.")
	}
	if s.fetchUpgrade == nil {
		return AIIntegrationActionResult{}, errors.New("AI integration upgrade source is unavailable")
	}
	bundle, err := s.fetchUpgrade(ctx, version)
	if err != nil {
		return AIIntegrationActionResult{}, &ActionFailure{Kind: ActionConnection, Message: "The AI integration release could not be downloaded.", Guidance: "Keep the current installation and retry when the release source is available.", Retryable: true}
	}
	defer bundle.Close()
	manifest, err := aisetup.Install(aisetup.InstallOptions{
		Target: resolved.target, Source: bundle.FS(), SourceRoot: ".", SourceLabel: "github",
		SourceVersion: bundle.Tag, Force: request.Replace, BoundaryRoot: resolved.boundaryRoot,
	})
	if err != nil {
		return AIIntegrationActionResult{}, aiIntegrationFilesystemFailure()
	}
	return AIIntegrationActionResult{Integration: s.manifestView(agent, scope, manifest), Status: "upgraded"}, nil
}

func (s *AIIntegrationService) resolveRequest(rawAgent, rawScope, projectRootID string) (aisetup.Agent, aisetup.Scope, resolvedAIIntegrationTarget, error) {
	agent, err := aisetup.ParseAgent(rawAgent)
	if err != nil {
		return "", "", resolvedAIIntegrationTarget{}, aiIntegrationValidation("Choose Claude Code, Cursor, or Pi.")
	}
	scope, err := aisetup.ParseScope(rawScope)
	if err != nil {
		return "", "", resolvedAIIntegrationTarget{}, aiIntegrationValidation("Choose user or project installation scope.")
	}
	resolved, err := s.resolveTarget(agent, scope, projectRootID)
	return agent, scope, resolved, err
}

func (s *AIIntegrationService) resolveTarget(agent aisetup.Agent, scope aisetup.Scope, projectRootID string) (resolvedAIIntegrationTarget, error) {
	target, err := aisetup.ResolveTarget(agent, scope)
	if err != nil {
		return resolvedAIIntegrationTarget{}, aiIntegrationValidation("Choose a supported AI runtime and installation scope.")
	}
	if scope == aisetup.ScopeUser {
		home, homeErr := os.UserHomeDir()
		if homeErr != nil || !filepath.IsAbs(home) {
			return resolvedAIIntegrationTarget{}, &ActionFailure{Kind: ActionUnexpected, Message: "The user installation scope is unavailable.", Guidance: "Verify the desktop session has a valid home directory, then retry.", Retryable: true}
		}
		return resolvedAIIntegrationTarget{target: target, boundaryRoot: filepath.Clean(home)}, nil
	}
	root, err := s.projectRoot(projectRootID)
	if err != nil {
		return resolvedAIIntegrationTarget{}, err
	}
	target.InstallDir = filepath.Join(root, target.InstallDir)
	return resolvedAIIntegrationTarget{target: target, boundaryRoot: root}, nil
}

func (s *AIIntegrationService) view(resolved resolvedAIIntegrationTarget, agent aisetup.Agent, scope aisetup.Scope) (AIIntegrationView, error) {
	installed, err := aisetup.InstalledWithin(resolved.boundaryRoot, resolved.target.InstallDir)
	if err != nil {
		return AIIntegrationView{}, aiIntegrationFilesystemFailure()
	}
	view := s.runtimeView(agent, scope)
	view.Installed = installed
	if installed {
		manifest, manifestErr := aisetup.ReadManifestWithin(resolved.boundaryRoot, resolved.target.InstallDir)
		if manifestErr == nil {
			view.BundleVersion = boundedConfigText(manifest.BundleVersion, 128)
			view.Source = boundedConfigText(manifest.Source, 128)
			view.SourceVersion = boundedConfigText(manifest.SourceVersion, 128)
		}
	}
	return view, nil
}

func (s *AIIntegrationService) manifestView(agent aisetup.Agent, scope aisetup.Scope, manifest aisetup.InstallManifest) AIIntegrationView {
	view := s.runtimeView(agent, scope)
	view.Installed = true
	view.BundleVersion = boundedConfigText(manifest.BundleVersion, 128)
	view.Source = boundedConfigText(manifest.Source, 128)
	view.SourceVersion = boundedConfigText(manifest.SourceVersion, 128)
	return view
}

func (s *AIIntegrationService) runtimeView(agent aisetup.Agent, scope aisetup.Scope) AIIntegrationView {
	view := AIIntegrationView{Agent: string(agent), DisplayName: aiIntegrationDisplayName(agent), Scope: string(scope), RuntimeStatus: "not_found"}
	if s.runtimeLookup != nil {
		if _, err := s.runtimeLookup(string(agent)); err == nil {
			view.RuntimeAvailable = true
			view.RuntimeStatus = "available"
			return view
		}
	}
	view.Guidance = "Install " + view.DisplayName + ", then return here. The skill can be prepared before the runtime is available."
	return view
}

func (s *AIIntegrationService) projectRoot(id string) (string, error) {
	if s == nil || id == "" {
		return "", aiIntegrationValidation("Choose the project directory for this installation scope first.")
	}
	s.mu.Lock()
	root, ok := s.projectRoots[id]
	s.mu.Unlock()
	if !ok {
		return "", aiIntegrationValidation("Choose the project directory again before continuing.")
	}
	return root, nil
}

func (s *AIIntegrationService) begin(key string) (func(), error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.inflight[key] {
		return nil, &ActionFailure{Kind: ActionConflict, Message: "The AI integration action is already in progress.", Guidance: "Wait for it to finish before retrying.", Retryable: false}
	}
	s.inflight[key] = true
	return func() {
		s.mu.Lock()
		delete(s.inflight, key)
		s.mu.Unlock()
	}, nil
}

func (s *AIIntegrationService) Clear() {
	if s == nil {
		return
	}
	s.mu.Lock()
	clear(s.projectRoots)
	clear(s.inflight)
	s.mu.Unlock()
}

func cleanAIIntegrationRoot(root string) (string, error) {
	if !filepath.IsAbs(root) || filepath.Clean(root) != root {
		return "", aiIntegrationValidation("Choose an existing local project directory.")
	}
	info, err := os.Stat(root)
	if err != nil || !info.IsDir() {
		return "", aiIntegrationValidation("Choose an existing local project directory.")
	}
	return root, nil
}

func aiIntegrationDisplayName(agent aisetup.Agent) string {
	switch agent {
	case aisetup.AgentClaude:
		return "Claude Code"
	case aisetup.AgentCursor:
		return "Cursor"
	case aisetup.AgentPi:
		return "Pi"
	default:
		return "AI runtime"
	}
}

func aiIntegrationValidation(guidance string) error {
	return &ActionFailure{Kind: ActionValidation, Message: "The AI integration request is invalid.", Guidance: guidance, Retryable: false}
}

func aiIntegrationFilesystemFailure() error {
	return &ActionFailure{Kind: ActionValidation, Message: "The selected AI integration scope could not be changed safely.", Guidance: "Choose a scope without paths or symlinks that escape its root, then retry.", Retryable: false}
}
