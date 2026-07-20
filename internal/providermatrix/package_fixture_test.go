package providermatrix

import (
	"bufio"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

type corePackageFixtureMetadata struct {
	SourceDateEpoch     int64    `json:"sourceDateEpoch"`
	SigningFingerprint  string   `json:"signingFingerprint"`
	MismatchFingerprint string   `json:"mismatchFingerprint"`
	APTVersions         []string `json:"aptVersions"`
	PacmanVersions      []string `json:"pacmanVersions"`
	AURVersion          string   `json:"aurVersion"`
	RequiredArtifacts   []string `json:"requiredArtifacts"`
}

func TestCorePackageFixtureManifestIsImmutableAndComplete(t *testing.T) {
	root := filepath.Join("..", "..", "test", "provider-matrix", "fixtures", "core-packages")
	data, err := os.ReadFile(filepath.Join(root, "METADATA.json"))
	if err != nil {
		t.Fatal(err)
	}
	var metadata corePackageFixtureMetadata
	if err := json.Unmarshal(data, &metadata); err != nil {
		t.Fatal(err)
	}
	if metadata.SourceDateEpoch != 1784419200 ||
		metadata.SigningFingerprint != "8DDFCCB89FC8A63796554F956177FE96142F67AB" ||
		metadata.MismatchFingerprint != "F9E2B9F7F04D8BB33EC7FB3431DD6980551A87F1" ||
		!equalStrings(metadata.APTVersions, []string{"1.0.0-1", "2.0.0-1"}) ||
		!equalStrings(metadata.PacmanVersions, []string{"1.0.0-1", "2.0.0-1"}) ||
		metadata.AURVersion != "1.0.0-1" {
		t.Fatalf("fixture metadata = %+v", metadata)
	}

	wantArtifacts := []string{
		"apt/dists/stable/InRelease",
		"apt/dists/stable/Release.gpg",
		"apt/dists/stable/Release.mismatch.gpg",
		"apt/pool/main/r/remotr-fixture/remotr-fixture_1.0.0-1_amd64.deb",
		"apt/pool/main/r/remotr-fixture/remotr-fixture_2.0.0-1_amd64.deb",
		"aur/remotr-aur-fixture/PKGBUILD",
		"native-config/apt/unrelated.sources",
		"native-config/pacman/unrelated.conf",
		"pacman/v1/remotr-fixture-1.0.0-1-x86_64.pkg.tar.zst",
		"pacman/v1/remotr-fixture.db.sig",
		"pacman/v1/remotr-fixture.db.mismatch.sig",
		"pacman/v2/remotr-fixture-2.0.0-1-x86_64.pkg.tar.zst",
		"pacman/v2/remotr-fixture.db.sig",
		"pacman/v2/remotr-fixture.db.mismatch.sig",
	}
	if !equalStrings(metadata.RequiredArtifacts, wantArtifacts) {
		t.Fatalf("required artifacts = %#v, want %#v", metadata.RequiredArtifacts, wantArtifacts)
	}

	checksums := readChecksumManifest(t, filepath.Join(root, "SHA256SUMS"))
	for _, relative := range append([]string{"METADATA.json"}, metadata.RequiredArtifacts...) {
		path := filepath.Join(root, filepath.FromSlash(relative))
		content, err := os.ReadFile(path)
		if err != nil {
			t.Fatalf("read required fixture %s: %v", relative, err)
		}
		digest := sha256.Sum256(content)
		if got := hex.EncodeToString(digest[:]); checksums["./"+relative] != got {
			t.Fatalf("checksum for %s = %q, want %q", relative, got, checksums["./"+relative])
		}
	}
}

func readChecksumManifest(t *testing.T, path string) map[string]string {
	t.Helper()
	file, err := os.Open(path)
	if err != nil {
		t.Fatal(err)
	}
	defer file.Close()
	checksums := map[string]string{}
	scanner := bufio.NewScanner(file)
	for scanner.Scan() {
		digest, relative, ok := strings.Cut(scanner.Text(), "  ")
		if !ok || len(digest) != sha256.Size*2 || relative == "" {
			t.Fatalf("malformed checksum line %q", scanner.Text())
		}
		checksums[relative] = digest
	}
	if err := scanner.Err(); err != nil {
		t.Fatal(err)
	}
	return checksums
}

func equalStrings(got, want []string) bool {
	if len(got) != len(want) {
		return false
	}
	for index := range want {
		if got[index] != want[index] {
			return false
		}
	}
	return true
}
