package main

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/url"
	"os"
	"path/filepath"
	"slices"
	"strings"

	opconfig "github.com/DavidHoenisch/remotr/internal/operator/config"
)

type ConnectionProfile struct {
	Name         string `json:"name"`
	ServerURL    string `json:"serverUrl"`
	StateDir     string `json:"stateDir"`
	CAPath       string `json:"caPath"`
	DefaultFleet string `json:"defaultFleet"`
}

type ProfileValidationError struct {
	Fields map[string]string
}

func (e *ProfileValidationError) Error() string {
	fields := make([]string, 0, len(e.Fields))
	for field := range e.Fields {
		fields = append(fields, field)
	}
	slices.Sort(fields)

	parts := make([]string, 0, len(fields))
	for _, field := range fields {
		parts = append(parts, field+": "+e.Fields[field])
	}
	return "invalid connection profile: " + strings.Join(parts, "; ")
}

type ProfileService struct {
	settingsPath       string
	operatorConfigPath string
}

type profileDocument struct {
	Profiles []ConnectionProfile `json:"profiles"`
}

func NewProfileService(settingsPath, operatorConfigPath string) *ProfileService {
	return &ProfileService{
		settingsPath:       settingsPath,
		operatorConfigPath: operatorConfigPath,
	}
}

func (s *ProfileService) LoadProfiles() ([]ConnectionProfile, error) {
	document, exists, err := s.loadDocument()
	if err != nil {
		return nil, err
	}
	if exists && len(document.Profiles) > 0 {
		return slices.Clone(document.Profiles), nil
	}

	profile, exists, err := s.loadDefaultProfile()
	if err != nil {
		return nil, err
	}
	if !exists {
		return []ConnectionProfile{}, nil
	}
	return []ConnectionProfile{profile}, nil
}

func (s *ProfileService) SaveProfile(profile ConnectionProfile) error {
	profile = normalizeProfile(profile)
	if err := validateProfile(profile); err != nil {
		return err
	}

	document, _, err := s.loadDocument()
	if err != nil {
		return err
	}
	replaced := false
	for index := range document.Profiles {
		if document.Profiles[index].Name == profile.Name {
			document.Profiles[index] = profile
			replaced = true
			break
		}
	}
	if !replaced {
		document.Profiles = append(document.Profiles, profile)
	}
	slices.SortFunc(document.Profiles, func(left, right ConnectionProfile) int {
		return strings.Compare(left.Name, right.Name)
	})

	return s.saveDocument(document)
}

func (s *ProfileService) loadDocument() (profileDocument, bool, error) {
	file, err := os.Open(s.settingsPath)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return profileDocument{}, false, nil
		}
		return profileDocument{}, false, fmt.Errorf("open desktop profile settings: %w", err)
	}
	defer file.Close()
	info, err := file.Stat()
	if err != nil {
		return profileDocument{}, false, fmt.Errorf("inspect desktop profile settings: %w", err)
	}
	if info.Size() > 1<<20 {
		return profileDocument{}, false, errors.New("desktop profile settings exceed 1 MiB")
	}

	decoder := json.NewDecoder(io.LimitReader(file, 1<<20))
	decoder.DisallowUnknownFields()
	var document profileDocument
	if err := decoder.Decode(&document); err != nil {
		return profileDocument{}, false, fmt.Errorf("decode desktop profile settings: %w", err)
	}
	if err := requireJSONEOF(decoder); err != nil {
		return profileDocument{}, false, fmt.Errorf("decode desktop profile settings: %w", err)
	}
	for index := range document.Profiles {
		document.Profiles[index] = normalizeProfile(document.Profiles[index])
		if err := validateProfile(document.Profiles[index]); err != nil {
			return profileDocument{}, false, fmt.Errorf("validate stored profile %q: %w", document.Profiles[index].Name, err)
		}
	}
	return document, true, nil
}

func (s *ProfileService) loadDefaultProfile() (ConnectionProfile, bool, error) {
	configPath := strings.TrimSpace(s.operatorConfigPath)
	if configPath == "" {
		configPath = opconfig.DefaultPath()
	}
	if _, err := os.Stat(configPath); err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return ConnectionProfile{}, false, nil
		}
		return ConnectionProfile{}, false, fmt.Errorf("inspect standard Operator configuration: %w", err)
	}

	settings, err := opconfig.Resolve(configPath, "", "", "", "")
	if err != nil {
		return ConnectionProfile{}, false, fmt.Errorf("resolve standard Operator configuration: %w", err)
	}
	profile := normalizeProfile(ConnectionProfile{
		Name:         "Default",
		ServerURL:    settings.ServerURL,
		StateDir:     settings.StateDir,
		CAPath:       settings.CA,
		DefaultFleet: settings.Fleet,
	})
	if err := validateProfile(profile); err != nil {
		return ConnectionProfile{}, false, fmt.Errorf("validate standard Operator configuration: %w", err)
	}
	return profile, true, nil
}

func (s *ProfileService) saveDocument(document profileDocument) error {
	directory := filepath.Dir(s.settingsPath)
	if err := os.MkdirAll(directory, 0o700); err != nil {
		return fmt.Errorf("create desktop settings directory: %w", err)
	}

	data, err := json.MarshalIndent(document, "", "  ")
	if err != nil {
		return fmt.Errorf("encode desktop profile settings: %w", err)
	}
	data = append(data, '\n')

	temporary, err := os.CreateTemp(directory, ".profiles-*.tmp")
	if err != nil {
		return fmt.Errorf("create temporary desktop settings: %w", err)
	}
	temporaryPath := temporary.Name()
	removeTemporary := true
	defer func() {
		if removeTemporary {
			_ = os.Remove(temporaryPath)
		}
	}()

	if err := temporary.Chmod(0o600); err != nil {
		_ = temporary.Close()
		return fmt.Errorf("protect temporary desktop settings: %w", err)
	}
	if _, err := temporary.Write(data); err != nil {
		_ = temporary.Close()
		return fmt.Errorf("write temporary desktop settings: %w", err)
	}
	if err := temporary.Sync(); err != nil {
		_ = temporary.Close()
		return fmt.Errorf("sync temporary desktop settings: %w", err)
	}
	if err := temporary.Close(); err != nil {
		return fmt.Errorf("close temporary desktop settings: %w", err)
	}
	if err := os.Rename(temporaryPath, s.settingsPath); err != nil {
		return fmt.Errorf("replace desktop profile settings: %w", err)
	}
	removeTemporary = false

	directoryHandle, err := os.Open(directory)
	if err != nil {
		return fmt.Errorf("open desktop settings directory: %w", err)
	}
	defer directoryHandle.Close()
	if err := directoryHandle.Sync(); err != nil {
		return fmt.Errorf("sync desktop settings directory: %w", err)
	}
	return nil
}

func normalizeProfile(profile ConnectionProfile) ConnectionProfile {
	profile.Name = strings.TrimSpace(profile.Name)
	profile.ServerURL = strings.TrimSpace(profile.ServerURL)
	profile.StateDir = strings.TrimSpace(profile.StateDir)
	profile.CAPath = strings.TrimSpace(profile.CAPath)
	profile.DefaultFleet = strings.TrimSpace(profile.DefaultFleet)
	return profile
}

func validateProfile(profile ConnectionProfile) error {
	fields := map[string]string{}
	if profile.Name == "" {
		fields["name"] = "Enter a profile name."
	}
	parsedServerURL, err := url.Parse(profile.ServerURL)
	if profile.ServerURL == "" {
		fields["serverUrl"] = "Enter the Remotr server URL."
	} else if err != nil || parsedServerURL.Scheme != "https" || parsedServerURL.Host == "" || parsedServerURL.User != nil || parsedServerURL.RawQuery != "" || parsedServerURL.Fragment != "" || (parsedServerURL.Path != "" && parsedServerURL.Path != "/") {
		fields["serverUrl"] = "Use an absolute HTTPS Remotr server URL."
	}
	if !filepath.IsAbs(profile.StateDir) {
		fields["stateDir"] = "Use an absolute Operator state directory."
	}
	if profile.CAPath != "" && !filepath.IsAbs(profile.CAPath) {
		fields["caPath"] = "Use an absolute CA certificate path or leave it empty."
	}
	if len(fields) > 0 {
		return &ProfileValidationError{Fields: fields}
	}
	return nil
}

func requireJSONEOF(decoder *json.Decoder) error {
	var extra any
	if err := decoder.Decode(&extra); !errors.Is(err, io.EOF) {
		if err == nil {
			return errors.New("multiple JSON values")
		}
		return err
	}
	return nil
}
