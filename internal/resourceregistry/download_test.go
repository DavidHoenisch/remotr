package resourceregistry_test

import (
	"context"
	"crypto/sha256"
	"fmt"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"testing"

	"github.com/DavidHoenisch/remotr/internal/executor"
	"github.com/DavidHoenisch/remotr/internal/resourceregistry"
	"github.com/DavidHoenisch/remotr/internal/secrets"
	"github.com/DavidHoenisch/remotr/test/testsupport"
	"gopkg.in/yaml.v3"
)

// OS-AEC-097: an accepted authenticationRef must cross the composed agent
// provider factory and reach the HTTPS provider without exposing its value.
func TestRegistryDownloadProviderResolvesScopedAuthentication(t *testing.T) {
	content := []byte("authenticated registry download\n")
	canary := testsupport.SecretCanary("ubuntu-download-registry")
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, request *http.Request) {
		if got := request.Header.Get("Authorization"); got != "Bearer "+canary {
			t.Errorf("Authorization = %q, want resolved bearer token", got)
			http.Error(w, "unauthorized", http.StatusUnauthorized)
			return
		}
		_, _ = w.Write(content)
	}))
	t.Cleanup(server.Close)

	dest := filepath.Join(t.TempDir(), "authenticated-download")
	var node yaml.Node
	resourceYAML := fmt.Sprintf(`kind: download
name: authenticated-download
url: %s
dest: %s
checksum: sha256:%x
authenticationRef: remotr:download-tokens/qualification@active
redirectPolicy: same-origin
timeout: 5s
`, server.URL, dest, sha256.Sum256(content))
	if err := yaml.Unmarshal([]byte(resourceYAML), &node); err != nil {
		t.Fatal(err)
	}
	registry, err := resourceregistry.NewDefault()
	if err != nil {
		t.Fatal(err)
	}
	resource, err := registry.Decode(node.Content[0])
	if err != nil {
		t.Fatal(err)
	}
	if err := resource.Validate(); err != nil {
		t.Fatal(err)
	}
	resolver := &recordingDownloadResolver{material: []byte(canary)}
	handler, err := resource.NewProvider(resourceregistry.FactoryContext{
		SecretResolver: resolver, ArtifactDigest: "sha256:artifact", ResourceAddress: "m1-filesystem/authenticated-download",
	})
	if err != nil {
		t.Fatal(err)
	}

	result := executor.New().ApplyState(context.Background(), handler)
	if result.Status != executor.Changed || result.Err != nil {
		t.Fatalf("composed download Apply = %+v, want changed", result)
	}
	if resolver.request.Reference != "remotr:download-tokens/qualification@active" ||
		resolver.request.ArtifactDigest != "sha256:artifact" ||
		resolver.request.ResourceAddress != "m1-filesystem/authenticated-download" ||
		resolver.request.Purpose != "download-authentication" {
		t.Fatalf("secret resolution request = %+v", resolver.request)
	}
	if check := executor.Check(context.Background(), handler); check.Status != executor.Compliant {
		t.Fatalf("second Check = %+v, want compliant", check)
	}
}

type recordingDownloadResolver struct {
	material []byte
	request  secrets.ResolveRequest
}

func (r *recordingDownloadResolver) Resolve(_ context.Context, request secrets.ResolveRequest) (secrets.Resolved, error) {
	r.request = request
	return secrets.Resolved{Provider: secrets.ProviderRemotr, Material: append([]byte(nil), r.material...)}, nil
}
