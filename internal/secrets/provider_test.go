package secrets

import (
	"context"
	"os"
	"path/filepath"
	"testing"
)

func TestLocalFileProviderResolvesOnlyProtectedRootReadableMaterial(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "database-password")
	if err := os.WriteFile(path, []byte("secret-canary"), 0o600); err != nil {
		t.Fatal(err)
	}

	provider := NewLocalFileProvider(WithRequiredUID(uint32(os.Getuid())))
	resolved, err := provider.Resolve(context.Background(), ResolveRequest{
		Reference:       "local-file:" + path,
		ResourceAddress: "base/database",
		Purpose:         "database-password",
	})
	if err != nil {
		t.Fatal(err)
	}
	if string(resolved.Material) != "secret-canary" || resolved.Provider != ProviderLocalFile {
		t.Fatalf("resolved = %#v", resolved)
	}

	if err := os.Chmod(path, 0o640); err != nil {
		t.Fatal(err)
	}
	if _, err := provider.Resolve(context.Background(), ResolveRequest{Reference: "local-file:" + path, ResourceAddress: "base/database", Purpose: "database-password"}); err == nil {
		t.Fatal("group-readable secret was accepted")
	}

	link := filepath.Join(dir, "database-password-link")
	if err := os.Symlink(path, link); err != nil {
		t.Fatal(err)
	}
	if _, err := provider.Resolve(context.Background(), ResolveRequest{Reference: "local-file:" + link, ResourceAddress: "base/database", Purpose: "database-password"}); err == nil {
		t.Fatal("symlinked secret was accepted")
	}
}
