package main

import (
	"context"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"sync"
	"time"

	"github.com/DavidHoenisch/remotr/internal/admin"
	"github.com/DavidHoenisch/remotr/internal/apppackages"
	wailsruntime "github.com/wailsapp/wails/v2/pkg/runtime"
)

const localPackageFileLimit = 10_000

const applicationPackageViewLimit = 5_000

type ApplicationPackageDirectoryDialog func(context.Context) (string, error)
type ApplicationPackageArchiveDialog func(context.Context) (string, error)
type ApplicationPackageSaveDialog func(context.Context, string) (string, error)

type ApplicationPackageDialogs struct {
	ChooseCreateParent ApplicationPackageDirectoryDialog
	ChooseSource       ApplicationPackageDirectoryDialog
	ChooseArchive      ApplicationPackageArchiveDialog
	ChooseBuildOutput  ApplicationPackageSaveDialog
}

type LocalPackageCreateRequest struct {
	DirectoryName string `json:"directoryName"`
	Name          string `json:"name"`
	Version       string `json:"version"`
	Mode          string `json:"mode"`
}

type LocalPackageView struct {
	Name         string `json:"name"`
	Version      string `json:"version"`
	Mode         string `json:"mode"`
	LocationName string `json:"locationName"`
}

type AppPackageArchiveView struct {
	Name      string `json:"name"`
	Version   string `json:"version"`
	Mode      string `json:"mode"`
	SHA256    string `json:"sha256"`
	SizeBytes int64  `json:"sizeBytes"`
	FileName  string `json:"fileName"`
	Source    string `json:"source"`
}

type AppPackageView struct {
	ID          string `json:"id"`
	Name        string `json:"name"`
	Version     string `json:"version"`
	ObjectKey   string `json:"objectKey"`
	SHA256      string `json:"sha256"`
	InstallMode string `json:"installMode"`
	CreatedAt   string `json:"createdAt"`
}

type AppPackagePublishRequest struct {
	Name         string `json:"name"`
	Version      string `json:"version"`
	SHA256       string `json:"sha256"`
	Confirmation string `json:"confirmation"`
}

type AppPackageDeleteRequest struct {
	Name         string `json:"name"`
	Version      string `json:"version"`
	DeleteObject bool   `json:"deleteObject"`
	Confirmation string `json:"confirmation"`
}

type AppPackageDeleteResult struct {
	Name    string `json:"name"`
	Version string `json:"version"`
	Scope   string `json:"scope"`
}

type ApplicationPackageService struct {
	mu         sync.Mutex
	dialogs    ApplicationPackageDialogs
	source     string
	archive    string
	selected   AppPackageArchiveView
	publishing bool
	deleting   map[string]bool
}

func NewApplicationPackageService(dialogs ApplicationPackageDialogs) *ApplicationPackageService {
	return &ApplicationPackageService{dialogs: dialogs, deleting: map[string]bool{}}
}

func defaultApplicationPackageDialogs() ApplicationPackageDialogs {
	return ApplicationPackageDialogs{
		ChooseCreateParent: func(ctx context.Context) (string, error) {
			return wailsruntime.OpenDirectoryDialog(ctx, wailsruntime.OpenDialogOptions{Title: "Choose package parent directory", CanCreateDirectories: true})
		},
		ChooseSource: func(ctx context.Context) (string, error) {
			return wailsruntime.OpenDirectoryDialog(ctx, wailsruntime.OpenDialogOptions{Title: "Choose package source directory"})
		},
		ChooseArchive: func(ctx context.Context) (string, error) {
			return wailsruntime.OpenFileDialog(ctx, wailsruntime.OpenDialogOptions{
				Title:   "Choose application package archive",
				Filters: []wailsruntime.FileFilter{{DisplayName: "Remotr package archive (*.zip)", Pattern: "*.zip"}},
			})
		},
		ChooseBuildOutput: func(ctx context.Context, suggestedName string) (string, error) {
			return wailsruntime.SaveFileDialog(ctx, wailsruntime.SaveDialogOptions{
				Title:                "Save application package archive",
				DefaultFilename:      suggestedName,
				CanCreateDirectories: true,
				Filters:              []wailsruntime.FileFilter{{DisplayName: "Remotr package archive (*.zip)", Pattern: "*.zip"}},
			})
		},
	}
}

func (s *ApplicationPackageService) CreateLocal(ctx context.Context, request LocalPackageCreateRequest) (LocalPackageView, error) {
	if s == nil || s.dialogs.ChooseCreateParent == nil {
		return LocalPackageView{}, errors.New("native package directory dialog is unavailable")
	}
	directoryName, err := validateLocalPackageDirectoryName(request.DirectoryName)
	if err != nil {
		return LocalPackageView{}, err
	}
	parent, err := s.dialogs.ChooseCreateParent(ctx)
	if err != nil {
		return LocalPackageView{}, fmt.Errorf("choose package parent directory: %w", err)
	}
	if parent == "" {
		return LocalPackageView{}, nil
	}
	parentInfo, err := os.Stat(parent)
	if err != nil || !parentInfo.IsDir() {
		return LocalPackageView{}, errors.New("selected package parent is not a directory")
	}
	destination := filepath.Join(filepath.Clean(parent), directoryName)
	manifest, err := apppackages.CreateScaffold(apppackages.ScaffoldOptions{
		Dir: destination, Name: request.Name, Version: request.Version, Mode: request.Mode,
	})
	if err != nil {
		return LocalPackageView{}, fmt.Errorf("create package source: %w", err)
	}
	s.mu.Lock()
	s.source = destination
	s.archive = ""
	s.selected = AppPackageArchiveView{}
	s.mu.Unlock()
	return localPackageView(manifest, directoryName), nil
}

func (s *ApplicationPackageService) ChooseSource(ctx context.Context) (LocalPackageView, error) {
	if s == nil || s.dialogs.ChooseSource == nil {
		return LocalPackageView{}, errors.New("native package source dialog is unavailable")
	}
	directory, err := s.dialogs.ChooseSource(ctx)
	if err != nil {
		return LocalPackageView{}, fmt.Errorf("choose package source directory: %w", err)
	}
	if directory == "" {
		return LocalPackageView{}, nil
	}
	if err := validateLocalPackageSourceTree(directory); err != nil {
		return LocalPackageView{}, err
	}
	summary, err := apppackages.ValidateDir(directory)
	if err != nil {
		return LocalPackageView{}, fmt.Errorf("validate package source: %w", err)
	}
	directory = filepath.Clean(directory)
	s.mu.Lock()
	s.source = directory
	s.archive = ""
	s.selected = AppPackageArchiveView{}
	s.mu.Unlock()
	return localPackageView(summary.Manifest, filepath.Base(directory)), nil
}

func (s *ApplicationPackageService) Build(ctx context.Context) (AppPackageArchiveView, error) {
	if s == nil || s.dialogs.ChooseBuildOutput == nil {
		return AppPackageArchiveView{}, errors.New("native package save dialog is unavailable")
	}
	s.mu.Lock()
	source := s.source
	s.mu.Unlock()
	if source == "" {
		return AppPackageArchiveView{}, errors.New("choose a package source directory before building")
	}
	if err := validateLocalPackageSourceTree(source); err != nil {
		return AppPackageArchiveView{}, err
	}
	data, summary, err := apppackages.BuildZipFromDir(source)
	if err != nil {
		return AppPackageArchiveView{}, fmt.Errorf("build package archive: %w", err)
	}
	destination, err := s.dialogs.ChooseBuildOutput(ctx, apppackages.DefaultZipFilename(summary.Manifest))
	if err != nil {
		return AppPackageArchiveView{}, fmt.Errorf("choose package archive destination: %w", err)
	}
	if destination == "" {
		return AppPackageArchiveView{}, nil
	}
	if !strings.EqualFold(filepath.Ext(destination), ".zip") {
		return AppPackageArchiveView{}, errors.New("package archive destination must use the .zip extension")
	}
	if err := writePackageArchiveAtomic(destination, data); err != nil {
		return AppPackageArchiveView{}, err
	}
	destination = filepath.Clean(destination)
	view := archiveView(summary, filepath.Base(destination), "built")
	s.mu.Lock()
	s.archive = destination
	s.selected = view
	s.mu.Unlock()
	return view, nil
}

func (s *ApplicationPackageService) ChooseArchive(ctx context.Context) (AppPackageArchiveView, error) {
	if s == nil || s.dialogs.ChooseArchive == nil {
		return AppPackageArchiveView{}, errors.New("native package archive dialog is unavailable")
	}
	path, err := s.dialogs.ChooseArchive(ctx)
	if err != nil {
		return AppPackageArchiveView{}, fmt.Errorf("choose package archive: %w", err)
	}
	if path == "" {
		return AppPackageArchiveView{}, nil
	}
	data, err := readPackageArchive(path)
	if err != nil {
		return AppPackageArchiveView{}, err
	}
	summary, err := apppackages.ValidateZip(data)
	if err != nil {
		return AppPackageArchiveView{}, fmt.Errorf("validate package archive: %w", err)
	}
	path = filepath.Clean(path)
	view := archiveView(summary, filepath.Base(path), "selected")
	s.mu.Lock()
	s.archive = path
	s.selected = view
	s.mu.Unlock()
	return view, nil
}

func (s *ApplicationPackageService) ListConnected(ctx context.Context, client *admin.Client, prefix string) ([]AppPackageView, error) {
	if client == nil {
		return nil, ErrSessionNotConnected
	}
	prefix = strings.TrimSpace(prefix)
	if len(prefix) > 128 || strings.ContainsAny(prefix, "\r\n\x00") {
		return nil, errors.New("application package name filter is invalid")
	}
	packages, err := client.ListAppPackagesContext(ctx, prefix)
	if err != nil {
		return nil, err
	}
	if len(packages) > applicationPackageViewLimit {
		return nil, errors.New("application package catalog exceeds the supported limit")
	}
	views := make([]AppPackageView, 0, len(packages))
	for _, record := range packages {
		view, mapErr := mapAppPackageView(record)
		if mapErr != nil {
			return nil, mapErr
		}
		views = append(views, view)
	}
	slices.SortFunc(views, func(left, right AppPackageView) int {
		if order := strings.Compare(left.Name, right.Name); order != 0 {
			return order
		}
		return strings.Compare(left.Version, right.Version)
	})
	return views, nil
}

func (s *ApplicationPackageService) LoadConnected(ctx context.Context, client *admin.Client, name, version string) (AppPackageView, error) {
	if client == nil {
		return AppPackageView{}, ErrSessionNotConnected
	}
	if err := validateAppPackageIdentity(name, version); err != nil {
		return AppPackageView{}, err
	}
	record, err := client.GetAppPackageContext(ctx, name, version)
	if err != nil {
		return AppPackageView{}, err
	}
	if record.Name != name || record.Version != version {
		return AppPackageView{}, errors.New("server returned a different application package identity")
	}
	return mapAppPackageView(record)
}

func (s *ApplicationPackageService) PublishConnected(ctx context.Context, client *admin.Client, request AppPackagePublishRequest) (AppPackageView, error) {
	if client == nil {
		return AppPackageView{}, ErrSessionNotConnected
	}
	if err := validateAppPackageIdentity(request.Name, request.Version); err != nil {
		return AppPackageView{}, err
	}
	if request.Confirmation != request.Name+"@"+request.Version {
		return AppPackageView{}, errors.New("type the exact case-sensitive package identity to confirm publication")
	}
	if !validSHA256(request.SHA256) {
		return AppPackageView{}, errors.New("selected package integrity digest is invalid")
	}
	s.mu.Lock()
	if s.publishing {
		s.mu.Unlock()
		return AppPackageView{}, errors.New("an application package publication is already in progress")
	}
	archive := s.archive
	selected := s.selected
	if archive == "" || selected.SHA256 == "" {
		s.mu.Unlock()
		return AppPackageView{}, errors.New("choose and validate an application package archive before publishing")
	}
	s.publishing = true
	s.mu.Unlock()
	defer func() {
		s.mu.Lock()
		s.publishing = false
		s.mu.Unlock()
	}()

	data, err := readPackageArchive(archive)
	if err != nil {
		return AppPackageView{}, err
	}
	summary, err := apppackages.ValidateZip(data)
	if err != nil {
		return AppPackageView{}, fmt.Errorf("revalidate package archive: %w", err)
	}
	if summary.Manifest.Name != request.Name || summary.Manifest.Version != request.Version || summary.SHA256 != request.SHA256 ||
		selected.Name != request.Name || selected.Version != request.Version || selected.SHA256 != request.SHA256 || selected.SizeBytes != summary.Size {
		return AppPackageView{}, errors.New("selected package changed after validation; choose and validate it again")
	}
	record, err := client.UploadAppPackageContext(ctx, data, "")
	if err != nil {
		return AppPackageView{}, err
	}
	if record.Name != request.Name || record.Version != request.Version || record.SHA256 != request.SHA256 {
		return AppPackageView{}, errors.New("server returned different application package integrity metadata")
	}
	return mapAppPackageView(record)
}

func (s *ApplicationPackageService) DeleteConnected(ctx context.Context, client *admin.Client, request AppPackageDeleteRequest) (AppPackageDeleteResult, error) {
	if client == nil {
		return AppPackageDeleteResult{}, ErrSessionNotConnected
	}
	if err := validateAppPackageIdentity(request.Name, request.Version); err != nil {
		return AppPackageDeleteResult{}, err
	}
	scopePhrase := " CATALOG ONLY"
	scope := "catalog_only"
	if request.DeleteObject {
		scopePhrase = " DELETE OBJECT"
		scope = "catalog_and_object"
	}
	if request.Confirmation != request.Name+"@"+request.Version+scopePhrase {
		return AppPackageDeleteResult{}, errors.New("type the exact case-sensitive package identity and deletion scope to confirm")
	}
	key := request.Name + "@" + request.Version
	s.mu.Lock()
	if s.deleting[key] {
		s.mu.Unlock()
		return AppPackageDeleteResult{}, errors.New("this application package deletion is already in progress")
	}
	s.deleting[key] = true
	s.mu.Unlock()
	defer func() {
		s.mu.Lock()
		delete(s.deleting, key)
		s.mu.Unlock()
	}()
	if err := client.DeleteAppPackageContext(ctx, request.Name, request.Version, request.DeleteObject); err != nil {
		return AppPackageDeleteResult{}, err
	}
	return AppPackageDeleteResult{Name: request.Name, Version: request.Version, Scope: scope}, nil
}

func (s *ApplicationPackageService) Clear() {
	if s == nil {
		return
	}
	s.mu.Lock()
	s.source = ""
	s.archive = ""
	s.selected = AppPackageArchiveView{}
	s.mu.Unlock()
}

func mapAppPackageView(record admin.AppPackage) (AppPackageView, error) {
	if record.ID == "" || record.CreatedAt.IsZero() || record.Name != record.Manifest.Name || record.Version != record.Manifest.Version ||
		!validSHA256(record.SHA256) || strings.TrimSpace(record.S3Key) == "" || len(record.S3Key) > 1024 {
		return AppPackageView{}, errors.New("server returned invalid application package metadata")
	}
	if err := apppackages.ValidateManifest(record.Manifest); err != nil {
		return AppPackageView{}, errors.New("server returned invalid application package manifest")
	}
	return AppPackageView{
		ID: record.ID, Name: record.Name, Version: record.Version, ObjectKey: record.S3Key,
		SHA256: record.SHA256, InstallMode: record.Manifest.Install.Mode, CreatedAt: record.CreatedAt.UTC().Format(time.RFC3339),
	}, nil
}

func validateAppPackageIdentity(name, version string) error {
	if strings.TrimSpace(name) != name || strings.TrimSpace(version) != version {
		return errors.New("application package identity must not contain surrounding whitespace")
	}
	if err := apppackages.ValidateNameVersion(name, version); err != nil {
		return fmt.Errorf("invalid application package identity: %w", err)
	}
	return nil
}

func validSHA256(digest string) bool {
	if len(digest) != 64 || strings.ToLower(digest) != digest {
		return false
	}
	decoded, err := hex.DecodeString(digest)
	return err == nil && len(decoded) == 32
}

func localPackageView(manifest apppackages.Manifest, location string) LocalPackageView {
	return LocalPackageView{Name: manifest.Name, Version: manifest.Version, Mode: manifest.Install.Mode, LocationName: location}
}

func archiveView(summary apppackages.ZipSummary, fileName, source string) AppPackageArchiveView {
	return AppPackageArchiveView{
		Name: summary.Manifest.Name, Version: summary.Manifest.Version, Mode: summary.Manifest.Install.Mode,
		SHA256: summary.SHA256, SizeBytes: summary.Size, FileName: fileName, Source: source,
	}
}

func validateLocalPackageDirectoryName(name string) (string, error) {
	name = strings.TrimSpace(name)
	if name == "" || name == "." || name == ".." || filepath.Base(name) != name || strings.ContainsAny(name, `/\\`) || len(name) > 128 {
		return "", errors.New("package directory name must be one safe path component")
	}
	return name, nil
}

func validateLocalPackageSourceTree(root string) error {
	root = filepath.Clean(root)
	var total int64
	files := 0
	err := filepath.WalkDir(root, func(path string, entry os.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if entry.Type()&os.ModeSymlink != 0 {
			return fmt.Errorf("package source contains a symbolic link: %s", filepath.Base(path))
		}
		if entry.IsDir() {
			return nil
		}
		info, err := entry.Info()
		if err != nil {
			return err
		}
		if !info.Mode().IsRegular() {
			return fmt.Errorf("package source contains a non-regular file: %s", filepath.Base(path))
		}
		files++
		if files > localPackageFileLimit {
			return errors.New("package source contains too many files")
		}
		total += info.Size()
		if total > apppackages.MaxPackageZipBytes {
			return errors.New("package source exceeds the supported size limit")
		}
		return nil
	})
	if err != nil {
		return fmt.Errorf("inspect package source: %w", err)
	}
	return nil
}

func readPackageArchive(path string) ([]byte, error) {
	if !strings.EqualFold(filepath.Ext(path), ".zip") {
		return nil, errors.New("selected package archive must use the .zip extension")
	}
	linkInfo, err := os.Lstat(path)
	if err != nil {
		return nil, fmt.Errorf("inspect package archive: %w", err)
	}
	if linkInfo.Mode()&os.ModeSymlink != 0 || !linkInfo.Mode().IsRegular() || linkInfo.Size() <= 0 || linkInfo.Size() > apppackages.MaxPackageZipBytes {
		return nil, errors.New("selected package archive must be a non-empty regular file within the supported size limit")
	}
	file, err := os.Open(path) // #nosec G304 -- path is selected through the native file dialog
	if err != nil {
		return nil, fmt.Errorf("open package archive: %w", err)
	}
	defer file.Close()
	data, err := io.ReadAll(io.LimitReader(file, apppackages.MaxPackageZipBytes+1))
	if err != nil {
		return nil, fmt.Errorf("read package archive: %w", err)
	}
	if len(data) == 0 || int64(len(data)) > apppackages.MaxPackageZipBytes {
		return nil, errors.New("selected package archive exceeds the supported size limit")
	}
	return data, nil
}

func writePackageArchiveAtomic(destination string, data []byte) error {
	destination = filepath.Clean(destination)
	directory := filepath.Dir(destination)
	if err := os.MkdirAll(directory, 0o755); err != nil {
		return fmt.Errorf("create package archive directory: %w", err)
	}
	temporary, err := os.CreateTemp(directory, ".remotr-package-*.tmp")
	if err != nil {
		return fmt.Errorf("create temporary package archive: %w", err)
	}
	temporaryPath := temporary.Name()
	committed := false
	defer func() {
		_ = temporary.Close()
		if !committed {
			_ = os.Remove(temporaryPath)
		}
	}()
	if err := temporary.Chmod(0o644); err != nil {
		return fmt.Errorf("set package archive permissions: %w", err)
	}
	if _, err := temporary.Write(data); err != nil {
		return fmt.Errorf("write package archive: %w", err)
	}
	if err := temporary.Sync(); err != nil {
		return fmt.Errorf("sync package archive: %w", err)
	}
	if err := temporary.Close(); err != nil {
		return fmt.Errorf("close package archive: %w", err)
	}
	if err := os.Rename(temporaryPath, destination); err != nil {
		return fmt.Errorf("place package archive: %w", err)
	}
	if err := syncDiagnosticDirectory(directory); err != nil {
		_ = os.Remove(destination)
		return err
	}
	committed = true
	return nil
}
