package cliupgrade

import (
	"fmt"
	"runtime"
	"strings"
)

func currentPlatform() (goos, goarch string, err error) {
	goos = runtime.GOOS
	goarch = runtime.GOARCH
	if goos == "windows" && goarch == "arm64" {
		return "", "", fmt.Errorf("windows/arm64 releases are not published")
	}
	switch goarch {
	case "amd64", "arm64":
	default:
		return "", "", fmt.Errorf("unsupported architecture: %s", goarch)
	}
	return goos, goarch, nil
}

func assetFileName(version, goos, goarch string) string {
	v := strings.TrimPrefix(strings.TrimSpace(version), "v")
	if goos == "windows" {
		return fmt.Sprintf("remotr_%s_%s_%s.zip", v, goos, goarch)
	}
	return fmt.Sprintf("remotr_%s_%s_%s.tar.gz", v, goos, goarch)
}

func binaryName(goos string) string {
	if goos == "windows" {
		return "remotr.exe"
	}
	return "remotr"
}

func downloadURL(repo, tag, goos, goarch string) string {
	asset := assetFileName(tag, goos, goarch)
	return fmt.Sprintf("https://github.com/%s/releases/download/%s/%s", repo, tag, asset)
}

func checksumURL(repo, tag string) string {
	return fmt.Sprintf("https://github.com/%s/releases/download/%s/remotr_checksums.txt", repo, tag)
}
