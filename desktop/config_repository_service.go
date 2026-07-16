package main

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"sync"
	"unicode"

	"github.com/DavidHoenisch/remotr/internal/configcompose"
	"github.com/DavidHoenisch/remotr/internal/configrepo"
	"github.com/DavidHoenisch/remotr/internal/hubcatalog"
	"github.com/DavidHoenisch/remotr/internal/scaffold"
	"github.com/google/uuid"
	wailsruntime "github.com/wailsapp/wails/v2/pkg/runtime"
)

const (
	configArtifactLimit      = 200
	configArtifactBytesLimit = 2 << 20
	configRenderBytesLimit   = 8 << 20
	configHubEntryLimit      = 10_000
)

type ConfigDirectoryDialog func(context.Context) (string, error)
type ConfigRenderSaveDialog func(context.Context, string) (string, error)

type ConfigRepositoryServiceOptions struct {
	ChooseExisting   ConfigDirectoryDialog
	ChooseInitialize ConfigDirectoryDialog
	ChooseRenderSave ConfigRenderSaveDialog
	HubRoot          string
	CatalogPath      string
	RemoteCatalogURL string
	HTTPClient       *http.Client
}

type ConfigWorkingTreeView struct {
	ID            string `json:"id"`
	DirectoryName string `json:"directoryName"`
	Status        string `json:"status"`
}

type ConfigRepositoryInitRequest struct {
	Fleet             string `json:"fleet"`
	RemediationPolicy string `json:"remediationPolicy"`
}

type ConfigRepositoryInitResult struct {
	WorkingTree ConfigWorkingTreeView `json:"workingTree"`
	Fleet       string                `json:"fleet"`
	Status      string                `json:"status"`
}

type ConfigValidationFinding struct {
	Path    string `json:"path"`
	Message string `json:"message"`
}

type ConfigValidationDiagnosticView struct {
	Path    string `json:"path"`
	Code    string `json:"code"`
	Message string `json:"message"`
}

type ConfigValidationView struct {
	WorkingTreeID string                           `json:"workingTreeId"`
	Valid         bool                             `json:"valid"`
	OK            []string                         `json:"ok"`
	Issues        []ConfigValidationFinding        `json:"issues"`
	Diagnostics   []ConfigValidationDiagnosticView `json:"diagnostics"`
}

type ConfigFleetDiscoverRequest struct {
	WorkingTreeID string `json:"workingTreeId"`
	Fleet         string `json:"fleet"`
}

type ConfigFleetDiscoveryView struct {
	WorkingTreeID          string                           `json:"workingTreeId"`
	Fleet                  string                           `json:"fleet"`
	Manifest               string                           `json:"manifest"`
	Modules                []string                         `json:"modules"`
	Applications           []string                         `json:"applications"`
	Crons                  []string                         `json:"crons"`
	ResourceKinds          []string                         `json:"resourceKinds"`
	CapabilityRequirements []string                         `json:"capabilityRequirements"`
	Diagnostics            []ConfigValidationDiagnosticView `json:"diagnostics"`
}

type ConfigRenderRequest struct {
	WorkingTreeID string `json:"workingTreeId"`
	Scope         string `json:"scope"`
	TargetID      string `json:"targetId"`
}

type ConfigRenderedArtifactView struct {
	TargetType   string `json:"targetType"`
	TargetID     string `json:"targetId"`
	ArtifactType string `json:"artifactType"`
	Content      string `json:"content"`
	Digest       string `json:"digest"`
}

type ConfigRenderView struct {
	WorkingTreeID string                       `json:"workingTreeId"`
	Artifacts     []ConfigRenderedArtifactView `json:"artifacts"`
}

type ConfigRenderSaveRequest struct {
	WorkingTreeID string `json:"workingTreeId"`
	TargetType    string `json:"targetType"`
	TargetID      string `json:"targetId"`
	ArtifactType  string `json:"artifactType"`
	Digest        string `json:"digest"`
}

type ConfigRenderSaveResult struct {
	FileName string `json:"fileName"`
	Status   string `json:"status"`
}

type ConfigHubSnippetView struct {
	ID          string   `json:"id"`
	Title       string   `json:"title"`
	Description string   `json:"description"`
	Category    string   `json:"category"`
	Tags        []string `json:"tags"`
	Distros     []string `json:"distros"`
	Author      string   `json:"author"`
	Featured    bool     `json:"featured"`
}

type ConfigHubImportRequest struct {
	WorkingTreeID string `json:"workingTreeId"`
	EntryID       string `json:"entryId"`
	OutPath       string `json:"outPath"`
}

type ConfigHubImportResult struct {
	EntryID string `json:"entryId"`
	OutPath string `json:"outPath"`
	Status  string `json:"status"`
}

type ConfigRepositoryService struct {
	mu               sync.Mutex
	chooseExisting   ConfigDirectoryDialog
	chooseInitialize ConfigDirectoryDialog
	chooseRenderSave ConfigRenderSaveDialog
	hubOptions       hubcatalog.ImportOptions
	workingTrees     map[string]string
	inflight         map[string]bool
}

func NewConfigRepositoryService(options ConfigRepositoryServiceOptions) *ConfigRepositoryService {
	return &ConfigRepositoryService{
		chooseExisting:   options.ChooseExisting,
		chooseInitialize: options.ChooseInitialize,
		chooseRenderSave: options.ChooseRenderSave,
		hubOptions: hubcatalog.ImportOptions{
			HubRoot: options.HubRoot, CatalogPath: options.CatalogPath,
			RemoteCatalogURL: options.RemoteCatalogURL, HTTPClient: options.HTTPClient,
		},
		workingTrees: map[string]string{}, inflight: map[string]bool{},
	}
}

func defaultConfigRepositoryService() *ConfigRepositoryService {
	return NewConfigRepositoryService(ConfigRepositoryServiceOptions{
		ChooseExisting: func(ctx context.Context) (string, error) {
			return wailsruntime.OpenDirectoryDialog(ctx, wailsruntime.OpenDialogOptions{Title: "Choose Configuration repository working tree"})
		},
		ChooseInitialize: func(ctx context.Context) (string, error) {
			return wailsruntime.OpenDirectoryDialog(ctx, wailsruntime.OpenDialogOptions{Title: "Choose empty directory for Configuration repository"})
		},
		ChooseRenderSave: func(ctx context.Context, suggestedName string) (string, error) {
			return wailsruntime.SaveFileDialog(ctx, wailsruntime.SaveDialogOptions{Title: "Save rendered Configuration artifact", DefaultFilename: suggestedName, Filters: []wailsruntime.FileFilter{{DisplayName: "YAML", Pattern: "*.yaml;*.yml"}}})
		},
	})
}

func (s *ConfigRepositoryService) Choose(ctx context.Context) (ConfigWorkingTreeView, error) {
	if s == nil || s.chooseExisting == nil {
		return ConfigWorkingTreeView{}, errors.New("native working-tree selection is unavailable")
	}
	root, err := s.chooseExisting(ctx)
	if err != nil {
		return ConfigWorkingTreeView{}, fmt.Errorf("choose Configuration repository: %w", err)
	}
	if root == "" {
		return ConfigWorkingTreeView{Status: "canceled"}, nil
	}
	return s.selectRoot(root)
}

func (s *ConfigRepositoryService) Initialize(ctx context.Context, request ConfigRepositoryInitRequest) (ConfigRepositoryInitResult, error) {
	if s == nil || s.chooseInitialize == nil {
		return ConfigRepositoryInitResult{}, errors.New("native initialization directory selection is unavailable")
	}
	if err := configrepo.ValidateFleetName(request.Fleet); err != nil {
		return ConfigRepositoryInitResult{}, configRepositoryValidation("Enter a valid initial Fleet name.")
	}
	if request.RemediationPolicy != "auto" && request.RemediationPolicy != "report" {
		return ConfigRepositoryInitResult{}, configRepositoryValidation("Choose auto or report remediation policy.")
	}
	release, err := s.begin("initialize")
	if err != nil {
		return ConfigRepositoryInitResult{}, err
	}
	defer release()
	root, err := s.chooseInitialize(ctx)
	if err != nil {
		return ConfigRepositoryInitResult{}, fmt.Errorf("choose initialization directory: %w", err)
	}
	if root == "" {
		return ConfigRepositoryInitResult{Status: "canceled"}, nil
	}
	root, err = cleanAbsoluteDirectory(root)
	if err != nil {
		return ConfigRepositoryInitResult{}, err
	}
	result, err := scaffold.Init(ctx, scaffold.Options{Dir: root, Fleet: request.Fleet, RemediationPolicy: request.RemediationPolicy})
	if err != nil {
		return ConfigRepositoryInitResult{}, err
	}
	view, err := s.selectRoot(result.Dir)
	if err != nil {
		return ConfigRepositoryInitResult{}, err
	}
	return ConfigRepositoryInitResult{WorkingTree: view, Fleet: result.Fleet, Status: "initialized"}, nil
}

func (s *ConfigRepositoryService) Validate(id string) (ConfigValidationView, error) {
	root, err := s.root(id)
	if err != nil {
		return ConfigValidationView{}, err
	}
	result, err := configrepo.ValidateRepository(root)
	if err != nil {
		return ConfigValidationView{}, err
	}
	if has, hasErr := configcompose.HasManifests(root); hasErr != nil {
		return ConfigValidationView{}, hasErr
	} else if has {
		composition, compositionErr := configcompose.ValidateComposition(root)
		if compositionErr != nil {
			return ConfigValidationView{}, compositionErr
		}
		for _, issue := range composition.Issues {
			result.Issues = append(result.Issues, configrepo.ValidationIssue{Path: issue.Path, Message: issue.Message})
		}
	}
	view := ConfigValidationView{WorkingTreeID: id, Valid: len(result.Issues) == 0, OK: []string{}, Issues: []ConfigValidationFinding{}, Diagnostics: []ConfigValidationDiagnosticView{}}
	for _, path := range result.OK {
		relative, mapErr := configViewPath(root, path)
		if mapErr != nil {
			return ConfigValidationView{}, mapErr
		}
		view.OK = append(view.OK, relative)
	}
	for _, issue := range result.Issues {
		relative, mapErr := configViewPath(root, issue.Path)
		if mapErr != nil {
			return ConfigValidationView{}, mapErr
		}
		view.Issues = append(view.Issues, ConfigValidationFinding{Path: relative, Message: boundedConfigText(issue.Message, 4_096)})
	}
	for _, diagnostic := range result.Diagnostics {
		relative, mapErr := configViewPath(root, diagnostic.Path)
		if mapErr != nil {
			return ConfigValidationView{}, mapErr
		}
		view.Diagnostics = append(view.Diagnostics, ConfigValidationDiagnosticView{Path: relative, Code: string(diagnostic.Code), Message: boundedConfigText(diagnostic.Message, 4_096)})
	}
	slices.Sort(view.OK)
	return view, nil
}

func (s *ConfigRepositoryService) Discover(request ConfigFleetDiscoverRequest) (ConfigFleetDiscoveryView, error) {
	root, err := s.root(request.WorkingTreeID)
	if err != nil {
		return ConfigFleetDiscoveryView{}, err
	}
	if err := configrepo.ValidateFleetName(request.Fleet); err != nil {
		return ConfigFleetDiscoveryView{}, configRepositoryValidation("Enter a valid Fleet name from this working tree.")
	}
	summary, err := configcompose.DiscoverFleet(root, request.Fleet)
	if err != nil {
		return ConfigFleetDiscoveryView{}, err
	}
	view := ConfigFleetDiscoveryView{
		WorkingTreeID: request.WorkingTreeID, Fleet: summary.Fleet, Manifest: summary.Manifest,
		Modules: slices.Clone(summary.Modules), Applications: slices.Clone(summary.Applications), Crons: slices.Clone(summary.Crons),
		ResourceKinds: []string{}, CapabilityRequirements: slices.Clone(summary.CapabilityRequirements), Diagnostics: []ConfigValidationDiagnosticView{},
	}
	for _, kind := range summary.ResourceKinds {
		view.ResourceKinds = append(view.ResourceKinds, string(kind))
	}
	for _, diagnostic := range summary.Diagnostics {
		view.Diagnostics = append(view.Diagnostics, ConfigValidationDiagnosticView{Code: string(diagnostic.Code), Message: boundedConfigText(diagnostic.Message, 4_096)})
	}
	return view, nil
}

func (s *ConfigRepositoryService) Render(request ConfigRenderRequest) (ConfigRenderView, error) {
	root, err := s.root(request.WorkingTreeID)
	if err != nil {
		return ConfigRenderView{}, err
	}
	artifacts, err := renderConfigArtifacts(root, request.Scope, request.TargetID)
	if err != nil {
		return ConfigRenderView{}, err
	}
	views, err := mapConfigArtifacts(artifacts)
	if err != nil {
		return ConfigRenderView{}, err
	}
	return ConfigRenderView{WorkingTreeID: request.WorkingTreeID, Artifacts: views}, nil
}

func (s *ConfigRepositoryService) SaveRender(ctx context.Context, request ConfigRenderSaveRequest) (ConfigRenderSaveResult, error) {
	root, err := s.root(request.WorkingTreeID)
	if err != nil {
		return ConfigRenderSaveResult{}, err
	}
	if s.chooseRenderSave == nil {
		return ConfigRenderSaveResult{}, errors.New("native render save is unavailable")
	}
	artifacts, err := renderConfigArtifacts(root, request.TargetType, request.TargetID)
	if err != nil {
		return ConfigRenderSaveResult{}, err
	}
	index := slices.IndexFunc(artifacts, func(artifact configcompose.RenderedArtifact) bool {
		return artifact.TargetType == request.TargetType && artifact.TargetID == request.TargetID && artifact.ArtifactType == request.ArtifactType && artifact.Digest == request.Digest
	})
	if index < 0 {
		return ConfigRenderSaveResult{}, configRepositoryValidation("Render the exact current artifact again before saving it.")
	}
	suggested := request.TargetType + "-" + request.TargetID + "-" + request.ArtifactType + ".yaml"
	destination, err := s.chooseRenderSave(ctx, suggested)
	if err != nil {
		return ConfigRenderSaveResult{}, fmt.Errorf("choose render destination: %w", err)
	}
	if destination == "" {
		return ConfigRenderSaveResult{Status: "canceled"}, nil
	}
	if !filepath.IsAbs(destination) || filepath.Clean(destination) != destination {
		return ConfigRenderSaveResult{}, errors.New("render destination must be clean and absolute")
	}
	if err := atomicWriteConfigRender(destination, artifacts[index].YAML); err != nil {
		return ConfigRenderSaveResult{}, err
	}
	return ConfigRenderSaveResult{FileName: filepath.Base(destination), Status: "saved"}, nil
}

func (s *ConfigRepositoryService) ListHub(ctx context.Context, id string) ([]ConfigHubSnippetView, error) {
	if _, err := s.root(id); err != nil {
		return nil, err
	}
	catalog, _, err := hubcatalog.ResolveCatalog(ctx, s.hubOptions)
	if err != nil {
		return nil, err
	}
	if len(catalog.Entries) > configHubEntryLimit {
		return nil, errors.New("Hub catalog exceeds the supported entry limit")
	}
	views := make([]ConfigHubSnippetView, 0, len(catalog.Entries))
	for _, entry := range catalog.Entries {
		if err := validateHubEntryView(entry); err != nil {
			return nil, err
		}
		views = append(views, ConfigHubSnippetView{
			ID: entry.ID, Title: entry.Title, Description: entry.Description, Category: entry.Category,
			Tags: slices.Clone(entry.Tags), Distros: slices.Clone(entry.Distros), Author: entry.Author, Featured: entry.Featured,
		})
	}
	slices.SortFunc(views, func(left, right ConfigHubSnippetView) int { return strings.Compare(left.ID, right.ID) })
	return views, nil
}

func (s *ConfigRepositoryService) ImportHub(ctx context.Context, request ConfigHubImportRequest) (ConfigHubImportResult, error) {
	root, err := s.root(request.WorkingTreeID)
	if err != nil {
		return ConfigHubImportResult{}, err
	}
	outPath, err := validateHubOutputPath(request.OutPath)
	if err != nil {
		return ConfigHubImportResult{}, err
	}
	catalog, _, err := hubcatalog.ResolveCatalog(ctx, s.hubOptions)
	if err != nil {
		return ConfigHubImportResult{}, err
	}
	entry, err := hubcatalog.FindEntry(catalog, request.EntryID)
	if err != nil || validateHubEntryView(entry) != nil {
		return ConfigHubImportResult{}, configRepositoryValidation("Select an exact current Hub catalog entry.")
	}
	release, err := s.begin("hub-import\x00" + request.WorkingTreeID + "\x00" + outPath)
	if err != nil {
		return ConfigHubImportResult{}, err
	}
	defer release()
	options := s.hubOptions
	options.RepoRoot = root
	options.EntryID = entry.ID
	options.OutPath = outPath
	result, err := hubcatalog.ImportSnippet(ctx, options)
	if err != nil {
		return ConfigHubImportResult{}, err
	}
	return ConfigHubImportResult{EntryID: result.EntryID, OutPath: result.OutPath, Status: "imported"}, nil
}

func (s *ConfigRepositoryService) selectRoot(root string) (ConfigWorkingTreeView, error) {
	clean, err := cleanAbsoluteDirectory(root)
	if err != nil {
		return ConfigWorkingTreeView{}, err
	}
	id := uuid.NewString()
	s.mu.Lock()
	s.workingTrees[id] = clean
	s.mu.Unlock()
	return ConfigWorkingTreeView{ID: id, DirectoryName: filepath.Base(clean), Status: "selected"}, nil
}

func (s *ConfigRepositoryService) root(id string) (string, error) {
	if s == nil || id == "" {
		return "", configRepositoryValidation("Choose a Configuration repository working tree first.")
	}
	s.mu.Lock()
	root, ok := s.workingTrees[id]
	s.mu.Unlock()
	if !ok {
		return "", configRepositoryValidation("Choose the Configuration repository working tree again.")
	}
	return root, nil
}

func (s *ConfigRepositoryService) begin(key string) (func(), error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.inflight[key] {
		return nil, &ActionFailure{Kind: ActionConflict, Message: "The local Configuration operation is already in progress.", Guidance: "Wait for it to finish before retrying.", Retryable: false}
	}
	s.inflight[key] = true
	return func() {
		s.mu.Lock()
		delete(s.inflight, key)
		s.mu.Unlock()
	}, nil
}

func (s *ConfigRepositoryService) Clear() {
	if s == nil {
		return
	}
	s.mu.Lock()
	clear(s.workingTrees)
	clear(s.inflight)
	s.mu.Unlock()
}

func renderConfigArtifacts(root, scope, target string) ([]configcompose.RenderedArtifact, error) {
	scope = strings.TrimSpace(scope)
	target = strings.TrimSpace(target)
	switch scope {
	case "all":
		if target != "" {
			return nil, configRepositoryValidation("All-target rendering does not accept a target ID.")
		}
		return configcompose.RenderAll(root)
	case "fleet":
		desired, crons, desiredDigest, cronsDigest, err := configcompose.RenderFleet(root, target)
		return singleTargetArtifacts("fleet", target, desired, crons, desiredDigest, cronsDigest, err)
	case "endpoint":
		desired, crons, desiredDigest, cronsDigest, err := configcompose.RenderEndpoint(root, target)
		return singleTargetArtifacts("endpoint", target, desired, crons, desiredDigest, cronsDigest, err)
	default:
		return nil, configRepositoryValidation("Choose all, Fleet, or Endpoint render scope.")
	}
}

func singleTargetArtifacts(targetType, targetID string, desired, crons []byte, desiredDigest, cronsDigest string, err error) ([]configcompose.RenderedArtifact, error) {
	if err != nil {
		return nil, err
	}
	artifacts := []configcompose.RenderedArtifact{{TargetType: targetType, TargetID: targetID, ArtifactType: "desired", YAML: desired, Digest: desiredDigest}}
	if len(crons) > 0 {
		artifacts = append(artifacts, configcompose.RenderedArtifact{TargetType: targetType, TargetID: targetID, ArtifactType: "crons", YAML: crons, Digest: cronsDigest})
	}
	return artifacts, nil
}

func mapConfigArtifacts(artifacts []configcompose.RenderedArtifact) ([]ConfigRenderedArtifactView, error) {
	if len(artifacts) == 0 || len(artifacts) > configArtifactLimit {
		return nil, errors.New("rendered artifact count is outside the supported limit")
	}
	total := 0
	views := make([]ConfigRenderedArtifactView, 0, len(artifacts))
	for _, artifact := range artifacts {
		if len(artifact.YAML) == 0 || len(artifact.YAML) > configArtifactBytesLimit {
			return nil, errors.New("rendered artifact exceeds the supported size")
		}
		total += len(artifact.YAML)
		if total > configRenderBytesLimit || len(artifact.Digest) != 64 {
			return nil, errors.New("rendered artifact response is invalid")
		}
		views = append(views, ConfigRenderedArtifactView{TargetType: artifact.TargetType, TargetID: artifact.TargetID, ArtifactType: artifact.ArtifactType, Content: string(artifact.YAML), Digest: artifact.Digest})
	}
	return views, nil
}

func validateHubOutputPath(value string) (string, error) {
	value = strings.TrimSpace(filepath.ToSlash(value))
	clean := filepath.ToSlash(filepath.Clean(filepath.FromSlash(value)))
	if value == "" || filepath.IsAbs(value) || clean == "." || clean == ".." || strings.HasPrefix(clean, "../") || (!strings.HasSuffix(strings.ToLower(clean), ".yaml") && !strings.HasSuffix(strings.ToLower(clean), ".yml")) {
		return "", configRepositoryValidation("Use a repository-relative YAML output path that stays inside the selected working tree.")
	}
	return clean, nil
}

func validateHubEntryView(entry hubcatalog.Entry) error {
	for _, value := range append([]string{entry.ID, entry.Title, entry.Description, entry.Category, entry.Author}, append(slices.Clone(entry.Tags), entry.Distros...)...) {
		if strings.TrimSpace(value) != value || len(value) > 4_096 || strings.IndexFunc(value, unicode.IsControl) >= 0 {
			return errors.New("Hub catalog returned invalid entry metadata")
		}
	}
	if entry.ID == "" || entry.Title == "" {
		return errors.New("Hub catalog returned incomplete entry metadata")
	}
	return nil
}

func cleanAbsoluteDirectory(root string) (string, error) {
	if !filepath.IsAbs(root) || filepath.Clean(root) != root {
		return "", errors.New("native Configuration repository path must be clean and absolute")
	}
	info, err := os.Stat(root)
	if err != nil {
		return "", fmt.Errorf("inspect Configuration repository: %w", err)
	}
	if !info.IsDir() {
		return "", errors.New("selected Configuration repository path is not a directory")
	}
	return root, nil
}

func configViewPath(root, path string) (string, error) {
	if !filepath.IsAbs(path) {
		clean := filepath.ToSlash(filepath.Clean(path))
		if clean == ".." || strings.HasPrefix(clean, "../") {
			return "", errors.New("Configuration result path escaped the selected working tree")
		}
		return clean, nil
	}
	relative, err := filepath.Rel(root, path)
	if err != nil || relative == ".." || strings.HasPrefix(relative, ".."+string(filepath.Separator)) {
		return "", errors.New("Configuration result path escaped the selected working tree")
	}
	if relative == "." {
		return ".", nil
	}
	return filepath.ToSlash(relative), nil
}

func boundedConfigText(value string, limit int) string {
	if len(value) <= limit {
		return value
	}
	return value[:limit]
}

func atomicWriteConfigRender(destination string, body []byte) error {
	directory := filepath.Dir(destination)
	temporary, err := os.CreateTemp(directory, ".remotr-render-*.tmp")
	if err != nil {
		return fmt.Errorf("create temporary render: %w", err)
	}
	temporaryPath := temporary.Name()
	defer os.Remove(temporaryPath)
	if err := temporary.Chmod(0o644); err != nil {
		_ = temporary.Close()
		return err
	}
	if _, err := temporary.Write(body); err != nil {
		_ = temporary.Close()
		return err
	}
	if err := temporary.Sync(); err != nil {
		_ = temporary.Close()
		return err
	}
	if err := temporary.Close(); err != nil {
		return err
	}
	if err := os.Rename(temporaryPath, destination); err != nil {
		return fmt.Errorf("install rendered artifact: %w", err)
	}
	parent, err := os.Open(directory)
	if err != nil {
		return err
	}
	defer parent.Close()
	return parent.Sync()
}

func configRepositoryValidation(guidance string) error {
	return &ActionFailure{Kind: ActionValidation, Message: "The local Configuration operation is invalid.", Guidance: guidance, Retryable: false}
}
