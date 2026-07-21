package providermatrix

import (
	"strings"
	"testing"
)

func TestServerImagesIncludeEmbeddedProviderEvidence(t *testing.T) {
	t.Parallel()

	dockerignore := readRepositoryFile(t, ".dockerignore")
	for _, required := range []string{
		"!test/provider_matrix.go",
		"!test/provider-matrix.yaml",
	} {
		if !strings.Contains(dockerignore, required) {
			t.Errorf(".dockerignore does not expose required image input %q", required)
		}
	}

	for _, dockerfile := range []string{
		"docker/remotr-server/Dockerfile",
		"deploy/fly/Dockerfile",
	} {
		dockerfile := dockerfile
		t.Run(dockerfile, func(t *testing.T) {
			t.Parallel()

			contents := readRepositoryFile(t, strings.Split(dockerfile, "/")...)
			if !strings.Contains(contents, "COPY test/provider_matrix.go test/provider-matrix.yaml ./test/") {
				t.Errorf("%s omits the embedded provider evidence required by providermatrix.Default", dockerfile)
			}
		})
	}
}

func TestComposeArchAgentUsesQualifiedSnapshot(t *testing.T) {
	const qualifiedBase = "archlinux:latest@sha256:2b4d67033863d9f495dfd0f52ad8b451fae84adb71b4bdf63f69d10643df2403"

	providerDockerfile := readRepositoryFile(t, "test", "provider-matrix", "containers", "Dockerfile.arch-2026-07-06")
	if !strings.Contains(providerDockerfile, "FROM "+qualifiedBase) {
		t.Fatalf("qualified Arch fixture does not pin %q", qualifiedBase)
	}
	compose := readRepositoryFile(t, "compose", "docker-compose.yml")
	if !strings.Contains(compose, "BASE_IMAGE: "+qualifiedBase) {
		t.Fatalf("Compose Arch agent does not use qualified snapshot %q", qualifiedBase)
	}
}
