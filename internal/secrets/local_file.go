package secrets

import (
	"context"
	"fmt"
	"io"
	"os"

	"golang.org/x/sys/unix"
)

// LocalFileOption changes a local-file provider safety requirement.
type LocalFileOption func(*LocalFileProvider)

// WithRequiredUID is primarily useful at the OS test boundary. Production
// callers use the default root UID.
func WithRequiredUID(uid uint32) LocalFileOption {
	return func(provider *LocalFileProvider) { provider.requiredUID = uid }
}

// LocalFileProvider resolves independently provisioned root-owned files
// without copying their values into desired state or the server registry.
type LocalFileProvider struct {
	requiredUID uint32
}

func NewLocalFileProvider(options ...LocalFileOption) *LocalFileProvider {
	provider := &LocalFileProvider{requiredUID: 0}
	for _, option := range options {
		option(provider)
	}
	return provider
}

func (p *LocalFileProvider) Resolve(_ context.Context, request ResolveRequest) (Resolved, error) {
	provider, path, err := ParseReference(request.Reference)
	if err != nil {
		return Resolved{}, err
	}
	if provider != ProviderLocalFile {
		return Resolved{}, fmt.Errorf("local-file provider cannot resolve %q", provider)
	}
	if err := ValidateRequest(request); err != nil {
		return Resolved{}, err
	}

	fd, err := unix.Open(path, unix.O_RDONLY|unix.O_CLOEXEC|unix.O_NOFOLLOW, 0)
	if err != nil {
		return Resolved{}, fmt.Errorf("open protected secret: %w", err)
	}
	file := os.NewFile(uintptr(fd), path)
	if file == nil {
		_ = unix.Close(fd)
		return Resolved{}, fmt.Errorf("open protected secret: invalid file descriptor")
	}
	defer file.Close()

	var stat unix.Stat_t
	if err := unix.Fstat(fd, &stat); err != nil {
		return Resolved{}, fmt.Errorf("inspect protected secret: %w", err)
	}
	if stat.Mode&unix.S_IFMT != unix.S_IFREG {
		return Resolved{}, fmt.Errorf("protected secret must be a regular file")
	}
	if stat.Uid != p.requiredUID {
		return Resolved{}, fmt.Errorf("protected secret must be owned by uid %d", p.requiredUID)
	}
	if stat.Mode&0o077 != 0 {
		return Resolved{}, fmt.Errorf("protected secret must not be accessible by group or other")
	}
	if stat.Size < 0 || stat.Size > MaxMaterialBytes {
		return Resolved{}, fmt.Errorf("protected secret exceeds %d bytes", MaxMaterialBytes)
	}
	material, err := io.ReadAll(io.LimitReader(file, MaxMaterialBytes+1))
	if err != nil {
		return Resolved{}, fmt.Errorf("read protected secret: %w", err)
	}
	if len(material) == 0 || len(material) > MaxMaterialBytes {
		return Resolved{}, fmt.Errorf("protected secret is empty or exceeds %d bytes", MaxMaterialBytes)
	}
	return Resolved{Provider: ProviderLocalFile, Material: material}, nil
}
