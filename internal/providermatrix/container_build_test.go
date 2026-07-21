package providermatrix

import (
	"strings"
	"testing"
)

func TestServerImagesIncludeEmbeddedProviderEvidence(t *testing.T) {
	t.Parallel()

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
