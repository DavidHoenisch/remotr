package accountlimits_test

import (
	"context"
	"encoding/hex"
	"os"
	"path/filepath"
	"testing"

	"github.com/DavidHoenisch/remotr/internal/applicators/accountlimits"
	"github.com/DavidHoenisch/remotr/internal/executor"
	"github.com/DavidHoenisch/remotr/internal/models"
)

func TestApplicatorValidatesCompleteNativeLimitsSyntax(t *testing.T) {
	tests := []struct {
		name    string
		content []byte
		valid   bool
	}{
		{
			name: "documented native forms",
			content: []byte("# preserved native policy\n" +
				"@1000:2000 soft nofile 2048\n" +
				"%:123 - maxlogins 2\n" +
				"* - nonewprivs 1\n" +
				"* - rttime unlimited\n" +
				"root - chroot /srv/jail\n" +
				"disabled -\n"),
			valid: true,
		},
		{name: "NUL byte", content: []byte("root soft nofile 1024\x00\n")},
		{name: "missing field", content: []byte("root soft nofile\n")},
		{name: "unknown type", content: []byte("root advisory nofile 1024\n")},
		{name: "unknown item", content: []byte("root soft unknown 1024\n")},
		{name: "invalid nonewprivs", content: []byte("root - nonewprivs 2\n")},
		{name: "invalid numeric value", content: []byte("root soft nofile many\n")},
		{name: "malformed range", content: []byte("1000:invalid soft nofile 1024\n")},
		{name: "overflowing range", content: []byte("1000:4294967296 soft nofile 1024\n")},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			dir := t.TempDir()
			if err := os.WriteFile(filepath.Join(dir, "80-unmanaged.conf"), test.content, 0o644); err != nil {
				t.Fatal(err)
			}
			provider := accountlimits.New(models.AccountLimitResource{
				Name: "build", Entries: []models.AccountLimitEntry{
					{Domain: "@build", Type: models.AccountLimitSoft, Item: "nofile", Value: "4096"},
				},
			})
			provider.LimitsDir = dir

			result := provider.ApplyResult(context.Background())
			if test.valid {
				if result.Status != executor.Changed || result.Err != nil {
					t.Fatalf("ApplyResult() = %+v, want changed", result)
				}
				return
			}
			if result.Status != executor.Failed || result.Err == nil {
				t.Fatalf("ApplyResult() = %+v, want failed validation", result)
			}
			if _, err := os.Stat(filepath.Join(dir, "90-remotr-build.conf")); !os.IsNotExist(err) {
				t.Fatalf("failed validation created managed fragment: %v", err)
			}
		})
	}
}

func FuzzApplicatorAcceptsBoundedCommentedConfiguration(f *testing.F) {
	f.Add([]byte("operator note"))
	f.Add([]byte{0, '\n', '#', 0xff})
	f.Fuzz(func(t *testing.T, arbitrary []byte) {
		if len(arbitrary) > 1024 {
			// test-exception: EXC-034
			t.Skip()
		}
		dir := t.TempDir()
		comment := []byte("# " + hex.EncodeToString(arbitrary) + "\nroot soft nofile 2048\n")
		if err := os.WriteFile(filepath.Join(dir, "80-unmanaged.conf"), comment, 0o644); err != nil {
			t.Fatal(err)
		}
		provider := accountlimits.New(models.AccountLimitResource{
			Name: "build", Entries: []models.AccountLimitEntry{
				{Domain: "@build", Type: models.AccountLimitSoft, Item: "nofile", Value: "4096"},
			},
		})
		provider.LimitsDir = dir
		if result := provider.ApplyResult(context.Background()); result.Status != executor.Changed || result.Err != nil {
			t.Fatalf("comment-safe ApplyResult() = %+v", result)
		}
	})
}
