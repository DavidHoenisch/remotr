package main

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"runtime"
	"slices"
	"strings"

	"github.com/DavidHoenisch/remotr/internal/agentversion"
	"github.com/DavidHoenisch/remotr/internal/cliupgrade"
	opconfig "github.com/DavidHoenisch/remotr/internal/operator/config"
	opcreds "github.com/DavidHoenisch/remotr/internal/operator/credentials"
)

const remotrDocumentationURL = "https://davidhoenisch.github.io/remotr"

type SetupReleaseCheck func(context.Context) (string, error)

type SetupMaintenanceOptions struct {
	ApplicationVersion  string
	DesktopProfilesPath string
	GOARCH              string
	GOOS                string
	OperatorConfigPath  string
	ReleaseCheck        SetupReleaseCheck
}

type SetupApplicationView struct {
	Name         string `json:"name"`
	Version      string `json:"version"`
	Platform     string `json:"platform"`
	Architecture string `json:"architecture"`
}

type SetupMaintenanceView struct {
	Application         SetupApplicationView `json:"application"`
	StandardConfigPath  string               `json:"standardConfigPath"`
	DesktopProfilesPath string               `json:"desktopProfilesPath"`
	Profiles            []ConnectionProfile  `json:"profiles"`
}

type DesktopDoctorCheck struct {
	Name     string `json:"name"`
	Status   string `json:"status"`
	Detail   string `json:"detail"`
	Guidance string `json:"guidance"`
}

type DesktopDoctorReport struct {
	ProfileName string               `json:"profileName"`
	Healthy     bool                 `json:"healthy"`
	OperatorID  string               `json:"operatorId"`
	Roles       []string             `json:"roles"`
	Checks      []DesktopDoctorCheck `json:"checks"`
}

type DesktopUpdateStatus struct {
	CurrentVersion   string `json:"currentVersion"`
	LatestVersion    string `json:"latestVersion"`
	UpdateAvailable  bool   `json:"updateAvailable"`
	InstallSupported bool   `json:"installSupported"`
	Platform         string `json:"platform"`
	Guidance         string `json:"guidance"`
}

type SetupMaintenanceService struct {
	applicationVersion  string
	desktopProfilesPath string
	goarch              string
	goos                string
	operatorConfigPath  string
	releaseCheck        SetupReleaseCheck
}

func NewSetupMaintenanceService(options SetupMaintenanceOptions) *SetupMaintenanceService {
	return &SetupMaintenanceService{
		applicationVersion:  strings.TrimSpace(options.ApplicationVersion),
		desktopProfilesPath: filepath.Clean(options.DesktopProfilesPath),
		goarch:              strings.TrimSpace(options.GOARCH),
		goos:                strings.TrimSpace(options.GOOS),
		operatorConfigPath:  filepath.Clean(options.OperatorConfigPath),
		releaseCheck:        options.ReleaseCheck,
	}
}

func defaultSetupMaintenanceService(version string) *SetupMaintenanceService {
	return NewSetupMaintenanceService(SetupMaintenanceOptions{
		ApplicationVersion:  version,
		DesktopProfilesPath: defaultDesktopProfilesPath(),
		GOARCH:              runtime.GOARCH,
		GOOS:                runtime.GOOS,
		OperatorConfigPath:  opconfig.DefaultPath(),
		ReleaseCheck: func(ctx context.Context) (string, error) {
			return cliupgrade.LatestStableRelease(ctx, nil, "")
		},
	})
}

func (s *SetupMaintenanceService) Load(profiles []ConnectionProfile) (SetupMaintenanceView, error) {
	if s == nil || !filepath.IsAbs(s.operatorConfigPath) || !filepath.IsAbs(s.desktopProfilesPath) {
		return SetupMaintenanceView{}, errors.New("setup paths are unavailable")
	}
	views := slices.Clone(profiles)
	for index := range views {
		views[index] = normalizeProfile(views[index])
		if err := validateProfile(views[index]); err != nil {
			return SetupMaintenanceView{}, errors.New("stored connection profile is invalid")
		}
	}
	slices.SortFunc(views, func(left, right ConnectionProfile) int { return strings.Compare(left.Name, right.Name) })
	return SetupMaintenanceView{
		Application: SetupApplicationView{
			Name:         "Remotr Desktop",
			Version:      displayDesktopVersion(s.applicationVersion),
			Platform:     s.goos,
			Architecture: s.goarch,
		},
		StandardConfigPath:  s.operatorConfigPath,
		DesktopProfilesPath: s.desktopProfilesPath,
		Profiles:            views,
	}, nil
}

func (s *SetupMaintenanceService) Doctor(ctx context.Context, profile ConnectionProfile) (DesktopDoctorReport, error) {
	profile = normalizeProfile(profile)
	report := DesktopDoctorReport{ProfileName: profile.Name, Healthy: true, Roles: []string{}}
	appendCheck := func(check DesktopDoctorCheck) {
		report.Checks = append(report.Checks, check)
		if check.Status != "ok" {
			report.Healthy = false
		}
	}

	if err := validateProfile(profile); err != nil {
		appendCheck(DesktopDoctorCheck{Name: "Connection profile", Status: "fail", Detail: "The profile contains invalid connection references.", Guidance: err.Error()})
		return report, nil
	}
	appendCheck(DesktopDoctorCheck{Name: "Connection profile", Status: "ok", Detail: "Profile references are valid."})

	if _, err := os.Stat(s.operatorConfigPath); err != nil {
		appendCheck(DesktopDoctorCheck{Name: "Operator configuration", Status: "warn", Detail: "The standard Operator configuration is not present.", Guidance: "Save a named desktop profile or initialize the standard Operator configuration."})
	} else {
		appendCheck(DesktopDoctorCheck{Name: "Operator configuration", Status: "ok", Detail: s.operatorConfigPath})
	}

	if !opcreds.Present(profile.StateDir) {
		appendCheck(DesktopDoctorCheck{Name: "Operator credentials", Status: "fail", Detail: "A complete protected Operator credential is not present.", Guidance: "Bootstrap the first Operator or select a profile with an existing credential."})
		return report, nil
	}
	appendCheck(DesktopDoctorCheck{Name: "Operator credentials", Status: "ok", Detail: "A complete protected credential layout is present."})

	caPath := profile.CAPath
	if caPath == "" {
		layout, err := opcreds.Layout(profile.StateDir)
		if err == nil {
			caPath = layout.CA
		}
	}
	if caPath == "" {
		appendCheck(DesktopDoctorCheck{Name: "Server trust", Status: "fail", Detail: "No CA certificate reference is available.", Guidance: "Select an absolute CA certificate path or restore the protected credential layout."})
		return report, nil
	}
	if _, err := os.Stat(caPath); err != nil {
		appendCheck(DesktopDoctorCheck{Name: "Server trust", Status: "fail", Detail: "The configured CA certificate is unavailable.", Guidance: "Verify the profile's CA certificate reference."})
		return report, nil
	}
	appendCheck(DesktopDoctorCheck{Name: "Server trust", Status: "ok", Detail: "The CA certificate reference is readable."})

	connection, err := NewConnectionService().Connect(ctx, profile)
	if err != nil {
		if cause := context.Cause(ctx); cause != nil {
			return DesktopDoctorReport{}, cause
		}
		check := DesktopDoctorCheck{Name: "Authenticated connection", Status: "fail", Detail: "The Operator identity could not be verified.", Guidance: "Review the profile and try again."}
		var classified *ConnectionFailure
		if errors.As(err, &classified) {
			check.Detail = classified.Message
			check.Guidance = classified.Guidance
		}
		appendCheck(check)
		return report, nil
	}
	report.OperatorID = connection.OperatorID
	report.Roles = slices.Clone(connection.Roles)
	appendCheck(DesktopDoctorCheck{Name: "Authenticated connection", Status: "ok", Detail: "Verified Operator identity " + connection.OperatorID + "."})
	return report, nil
}

func (s *SetupMaintenanceService) CheckUpdate(ctx context.Context) (DesktopUpdateStatus, error) {
	if s == nil || s.releaseCheck == nil {
		return DesktopUpdateStatus{}, errors.New("desktop update checks are unavailable")
	}
	latest, err := s.releaseCheck(ctx)
	if err != nil {
		return DesktopUpdateStatus{}, errors.New("the Remotr Desktop update check could not be completed")
	}
	latest, err = agentversion.Normalize(latest)
	if err != nil {
		return DesktopUpdateStatus{}, errors.New("the release service returned an invalid version")
	}
	current := displayDesktopVersion(s.applicationVersion)
	available := false
	if current != "dev" {
		order, compareErr := agentversion.Compare(current, latest)
		if compareErr != nil {
			return DesktopUpdateStatus{}, errors.New("the embedded desktop version is invalid")
		}
		available = order < 0
	}
	guidance := "This development build does not install release packages in place."
	if s.goos == "linux" {
		guidance = "Install a verified Linux release package when one is published for this build."
	}
	return DesktopUpdateStatus{
		CurrentVersion: current, LatestVersion: latest, UpdateAvailable: available,
		InstallSupported: false, Platform: s.goos + "/" + s.goarch, Guidance: guidance,
	}, nil
}

func displayDesktopVersion(version string) string {
	version = strings.TrimSpace(version)
	if version == "" {
		return "dev"
	}
	return version
}
