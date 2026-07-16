package main

import (
	"bytes"
	"encoding/json"
	"encoding/xml"
	"image"
	_ "image/png"
	"io/fs"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
)

const desktopApplicationID = "io.github.davidhoenisch.remotr.desktop"

func TestLinuxApplicationPackagingIdentifiesRemotrDesktop(t *testing.T) {
	manifestData, err := os.ReadFile("build/icon-manifest.json")
	if err != nil {
		t.Errorf("read versioned icon manifest: %v", err)
	} else {
		var manifest struct {
			SchemaVersion int    `json:"schemaVersion"`
			IconSet       string `json:"iconSet"`
			ApplicationID string `json:"applicationId"`
			ProductName   string `json:"productName"`
			Icons         []struct {
				Path    string `json:"path"`
				Purpose string `json:"purpose"`
				Size    int    `json:"size"`
			} `json:"icons"`
		}
		if err := json.Unmarshal(manifestData, &manifest); err != nil {
			t.Errorf("parse versioned icon manifest: %v", err)
		} else {
			if manifest.SchemaVersion != 1 || manifest.IconSet != "remotr-v1" {
				t.Errorf("icon manifest version = %d/%q, want 1/remotr-v1", manifest.SchemaVersion, manifest.IconSet)
			}
			if manifest.ApplicationID != desktopApplicationID || manifest.ProductName != "Remotr Desktop" {
				t.Errorf("icon manifest identity = %q/%q, want %q/Remotr Desktop", manifest.ApplicationID, manifest.ProductName, desktopApplicationID)
			}
			wantIcons := []struct {
				path    string
				purpose string
				size    int
			}{
				{path: "icons/remotr-v1.png", purpose: "versioned-source", size: 1024},
				{path: "appicon.png", purpose: "wails-application", size: 1024},
				{path: "linux/icons/hicolor/256x256/apps/remotr-desktop.png", purpose: "linux-launcher", size: 256},
			}
			if len(manifest.Icons) != len(wantIcons) {
				t.Errorf("icon manifest entries = %d, want %d", len(manifest.Icons), len(wantIcons))
			} else {
				for i, want := range wantIcons {
					got := manifest.Icons[i]
					if got.Path != want.path || got.Purpose != want.purpose || got.Size != want.size {
						t.Errorf("icon manifest entry %d = %#v, want path %q, purpose %q, size %d", i, got, want.path, want.purpose, want.size)
					}
				}
			}
		}
	}

	versionedIcon := readPNG(t, "build/icons/remotr-v1.png", 1024)
	wailsIcon := readPNG(t, "build/appicon.png", 1024)
	readPNG(t, "build/linux/icons/hicolor/256x256/apps/remotr-desktop.png", 256)
	if versionedIcon != nil && wailsIcon != nil && !bytes.Equal(versionedIcon, wailsIcon) {
		t.Error("Wails appicon.png does not match the immutable remotr-v1 icon source")
	}

	app := newApplicationOptions()
	if app.Linux == nil {
		t.Error("Linux application options are absent")
	} else {
		if app.Linux.ProgramName != "remotr-desktop" {
			t.Errorf("Linux program name = %q, want remotr-desktop", app.Linux.ProgramName)
		}
		if wailsIcon != nil && !bytes.Equal(app.Linux.Icon, wailsIcon) {
			t.Error("native Linux window icon does not match build/appicon.png")
		}
	}

	desktopEntry := parseDesktopEntry(t, "build/linux/remotr-desktop.desktop")
	wantDesktopEntry := map[string]string{
		"Type":           "Application",
		"Version":        "1.0",
		"Name":           "Remotr Desktop",
		"GenericName":    "Linux Fleet Administration",
		"Comment":        "Native Linux fleet administration for Remotr",
		"Exec":           "remotr-desktop",
		"Icon":           "remotr-desktop",
		"Terminal":       "false",
		"Categories":     "System;",
		"StartupWMClass": "remotr-desktop",
		"StartupNotify":  "true",
	}
	for key, want := range wantDesktopEntry {
		if got := desktopEntry[key]; got != want {
			t.Errorf("desktop entry %s = %q, want %q", key, got, want)
		}
	}
	if strings.ContainsAny(desktopEntry["Exec"], "%\n\r") {
		t.Errorf("desktop entry Exec = %q, want the standalone executable without field codes", desktopEntry["Exec"])
	}

	metainfoData, err := os.ReadFile("build/linux/io.github.davidhoenisch.remotr.desktop.metainfo.xml")
	if err != nil {
		t.Errorf("read Linux AppStream metadata: %v", err)
	} else {
		var component struct {
			Type            string `xml:"type,attr"`
			ID              string `xml:"id"`
			MetadataLicense string `xml:"metadata_license"`
			ProjectLicense  string `xml:"project_license"`
			Name            string `xml:"name"`
			Summary         string `xml:"summary"`
			Launchable      struct {
				Type string `xml:"type,attr"`
				ID   string `xml:",chardata"`
			} `xml:"launchable"`
			Binary     string   `xml:"provides>binary"`
			Categories []string `xml:"categories>category"`
		}
		if err := xml.Unmarshal(metainfoData, &component); err != nil {
			t.Errorf("parse Linux AppStream metadata: %v", err)
		} else {
			if component.Type != "desktop-application" || component.ID != desktopApplicationID {
				t.Errorf("AppStream component identity = %q/%q, want desktop-application/%q", component.Type, component.ID, desktopApplicationID)
			}
			if component.Name != "Remotr Desktop" || component.Summary != "Native Linux fleet administration for Remotr" {
				t.Errorf("AppStream product identity = %q/%q", component.Name, component.Summary)
			}
			if component.MetadataLicense != "CC0-1.0" || component.ProjectLicense != "LicenseRef-proprietary" {
				t.Errorf("AppStream licenses = %q/%q, want CC0-1.0/LicenseRef-proprietary", component.MetadataLicense, component.ProjectLicense)
			}
			if component.Launchable.Type != "desktop-id" || strings.TrimSpace(component.Launchable.ID) != "remotr-desktop.desktop" || component.Binary != "remotr-desktop" {
				t.Errorf("AppStream launch identity = %#v/%q", component.Launchable, component.Binary)
			}
			if !slices.Equal(component.Categories, []string{"System"}) {
				t.Errorf("AppStream categories = %v, want [System]", component.Categories)
			}
		}
	}

	forbidden := []string{"darwin", "macos", "windows", ".icns", ".ico", ".dmg", ".pkg", ".exe", ".msi"}
	if err := filepath.WalkDir("build", func(path string, entry fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		lowerPath := strings.ToLower(filepath.ToSlash(path))
		for _, fragment := range forbidden {
			if strings.Contains(lowerPath, fragment) {
				t.Errorf("non-Linux desktop packaging asset exists: %s", path)
			}
		}
		return nil
	}); err != nil {
		t.Errorf("scan desktop build assets: %v", err)
	}
}

func readPNG(t *testing.T, path string, wantSize int) []byte {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Errorf("read icon %s: %v", path, err)
		return nil
	}
	decoded, format, err := image.Decode(bytes.NewReader(data))
	if err != nil {
		t.Errorf("decode icon %s: %v", path, err)
		return data
	}
	if format != "png" || decoded.Bounds().Dx() != wantSize || decoded.Bounds().Dy() != wantSize {
		t.Errorf("icon %s = %s %dx%d, want PNG %dx%d", path, format, decoded.Bounds().Dx(), decoded.Bounds().Dy(), wantSize, wantSize)
	}
	_, _, _, cornerAlpha := decoded.At(decoded.Bounds().Min.X, decoded.Bounds().Min.Y).RGBA()
	if cornerAlpha != 0 {
		t.Errorf("icon %s corner alpha = %d, want transparent", path, cornerAlpha)
	}
	_, _, _, centerAlpha := decoded.At(decoded.Bounds().Min.X+decoded.Bounds().Dx()/2, decoded.Bounds().Min.Y+decoded.Bounds().Dy()/2).RGBA()
	if centerAlpha == 0 {
		t.Errorf("icon %s center is transparent, want visible Remotr mark", path)
	}
	return data
}

func parseDesktopEntry(t *testing.T, path string) map[string]string {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Errorf("read Linux desktop entry: %v", err)
		return map[string]string{}
	}
	result := map[string]string{}
	inDesktopEntry := false
	for line := range strings.SplitSeq(string(data), "\n") {
		line = strings.TrimSpace(line)
		if line == "[Desktop Entry]" {
			inDesktopEntry = true
			continue
		}
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		if strings.HasPrefix(line, "[") {
			inDesktopEntry = false
			continue
		}
		if !inDesktopEntry {
			continue
		}
		key, value, ok := strings.Cut(line, "=")
		if ok {
			result[key] = value
		}
	}
	return result
}
