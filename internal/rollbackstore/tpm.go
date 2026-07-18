package rollbackstore

import (
	"bytes"
	"context"
	"crypto/rand"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
)

const (
	tpmKeyFilename = "rollback.tpm-key"
	tpmKeyVersion  = 1
	maxTPMBlobSize = 64 << 10
)

var requiredTPM2Tools = []string{
	"tpm2_getcap",
	"tpm2_createprimary",
	"tpm2_create",
	"tpm2_load",
	"tpm2_unseal",
	"tpm2_flushcontext",
}

// TPMCommandRunner is the process boundary used by the concrete TPM2-tools
// provider. Sealing input is passed through stdin and must not appear in argv.
type TPMCommandRunner interface {
	Run(context.Context, string, []string, []byte) ([]byte, error)
}

// TPM2ToolsOptions exposes only external boundaries needed for deterministic
// provider-contract tests. Nil fields select the production OS implementations.
type TPM2ToolsOptions struct {
	Runner       TPMCommandRunner
	LookPath     func(string) (string, error)
	DeviceExists func(string) bool
	Random       io.Reader
}

// TPM2ToolsKeyProvider seals one AES-256 rollback key to the TPM owner
// hierarchy using the supported tpm2-tools process interface.
type TPM2ToolsKeyProvider struct {
	runner       TPMCommandRunner
	lookPath     func(string) (string, error)
	deviceExists func(string) bool
	random       io.Reader
}

func NewTPM2ToolsKeyProvider(options TPM2ToolsOptions) *TPM2ToolsKeyProvider {
	if options.Runner == nil {
		options.Runner = osTPMCommandRunner{}
	}
	if options.LookPath == nil {
		options.LookPath = exec.LookPath
	}
	if options.DeviceExists == nil {
		options.DeviceExists = func(path string) bool {
			_, err := os.Stat(path)
			return err == nil
		}
	}
	if options.Random == nil {
		options.Random = rand.Reader
	}
	return &TPM2ToolsKeyProvider{
		runner: options.Runner, lookPath: options.LookPath,
		deviceExists: options.DeviceExists, random: options.Random,
	}
}

// Supported reports false only when the device or required provider tools are
// absent. A present provider that fails its live capability probe returns an
// error so selection cannot silently downgrade to a root file.
func (p *TPM2ToolsKeyProvider) Supported(ctx context.Context) (bool, error) {
	if p == nil {
		return false, errors.New("TPM2-tools provider is nil")
	}
	if !p.deviceExists("/dev/tpmrm0") && !p.deviceExists("/dev/tpm0") {
		return false, nil
	}
	for _, name := range requiredTPM2Tools {
		if _, err := p.lookPath(name); err != nil {
			return false, nil
		}
	}
	if _, err := p.runner.Run(ctx, "tpm2_getcap", []string{"-Q", "properties-fixed"}, nil); err != nil {
		return false, fmt.Errorf("probe TPM2-tools provider: %w", err)
	}
	return true, nil
}

type tpmKeyFile struct {
	Version int    `json:"version"`
	ID      string `json:"id"`
	Public  []byte `json:"public"`
	Private []byte `json:"private"`
}

func (p *TPM2ToolsKeyProvider) LoadOrCreate(ctx context.Context, root string) (KeyMaterial, error) {
	if p == nil || p.runner == nil || p.random == nil {
		return KeyMaterial{}, errors.New("TPM2-tools key provider is incomplete")
	}
	path := filepath.Join(root, tpmKeyFilename)
	raw, err := os.ReadFile(path)
	if err == nil {
		return p.load(ctx, root, raw)
	}
	if !errors.Is(err, os.ErrNotExist) {
		return KeyMaterial{}, err
	}
	return p.create(ctx, root, path)
}

func (p *TPM2ToolsKeyProvider) create(ctx context.Context, root, path string) (KeyMaterial, error) {
	temporary, err := os.MkdirTemp(root, ".rollback-tpm-")
	if err != nil {
		return KeyMaterial{}, err
	}
	defer os.RemoveAll(temporary)
	primary := filepath.Join(temporary, "primary.ctx")
	if err := p.createPrimary(ctx, primary); err != nil {
		return KeyMaterial{}, err
	}
	defer p.flush(ctx, primary)

	key := make([]byte, 32)
	if _, err := io.ReadFull(p.random, key); err != nil {
		return KeyMaterial{}, err
	}
	id, err := newKeyID("tpm-v1", p.random)
	if err != nil {
		clear(key)
		return KeyMaterial{}, err
	}
	publicPath := filepath.Join(temporary, "sealed.pub")
	privatePath := filepath.Join(temporary, "sealed.priv")
	args := []string{
		"-Q", "-C", primary, "-g", "sha256", "-i", "-",
		"-u", publicPath, "-r", privatePath,
	}
	if _, err := p.runner.Run(ctx, "tpm2_create", args, key); err != nil {
		clear(key)
		return KeyMaterial{}, fmt.Errorf("seal rollback key with TPM: %w", err)
	}
	public, err := readBoundedTPMBlob(publicPath)
	if err != nil {
		clear(key)
		return KeyMaterial{}, err
	}
	private, err := readBoundedTPMBlob(privatePath)
	if err != nil {
		clear(key)
		return KeyMaterial{}, err
	}
	stored := tpmKeyFile{Version: tpmKeyVersion, ID: id, Public: public, Private: private}
	if err := writeJSONDurable(path, stored); err != nil {
		clear(key)
		return KeyMaterial{}, err
	}
	return KeyMaterial{ID: id, Key: key, Protection: ProtectionTPMSealed}, nil
}

func (p *TPM2ToolsKeyProvider) load(ctx context.Context, root string, raw []byte) (KeyMaterial, error) {
	var stored tpmKeyFile
	if err := decodeStrictJSON(raw, &stored); err != nil {
		return KeyMaterial{}, errors.New("sealed TPM rollback key file is malformed")
	}
	if stored.Version != tpmKeyVersion || !validKeyID(stored.ID) ||
		len(stored.Public) == 0 || len(stored.Public) > maxTPMBlobSize ||
		len(stored.Private) == 0 || len(stored.Private) > maxTPMBlobSize {
		return KeyMaterial{}, errors.New("sealed TPM rollback key file has an unsupported version or invalid blob")
	}
	temporary, err := os.MkdirTemp(root, ".rollback-tpm-")
	if err != nil {
		return KeyMaterial{}, err
	}
	defer os.RemoveAll(temporary)
	publicPath := filepath.Join(temporary, "sealed.pub")
	privatePath := filepath.Join(temporary, "sealed.priv")
	if err := os.WriteFile(publicPath, stored.Public, 0o600); err != nil {
		return KeyMaterial{}, err
	}
	if err := os.WriteFile(privatePath, stored.Private, 0o600); err != nil {
		return KeyMaterial{}, err
	}
	primary := filepath.Join(temporary, "primary.ctx")
	if err := p.createPrimary(ctx, primary); err != nil {
		return KeyMaterial{}, err
	}
	defer p.flush(ctx, primary)
	sealed := filepath.Join(temporary, "sealed.ctx")
	loadArgs := []string{
		"-Q", "-C", primary, "-u", publicPath, "-r", privatePath, "-c", sealed,
	}
	if _, err := p.runner.Run(ctx, "tpm2_load", loadArgs, nil); err != nil {
		return KeyMaterial{}, fmt.Errorf("load sealed TPM rollback key: %w", err)
	}
	defer p.flush(ctx, sealed)
	unsealed, err := p.runner.Run(ctx, "tpm2_unseal", []string{"-Q", "-c", sealed}, nil)
	if err != nil {
		return KeyMaterial{}, fmt.Errorf("unseal TPM rollback key: %w", err)
	}
	if len(unsealed) != 32 {
		clear(unsealed)
		return KeyMaterial{}, errors.New("TPM returned an invalid rollback key length")
	}
	key := append([]byte(nil), unsealed...)
	clear(unsealed)
	return KeyMaterial{ID: stored.ID, Key: key, Protection: ProtectionTPMSealed}, nil
}

func (p *TPM2ToolsKeyProvider) createPrimary(ctx context.Context, path string) error {
	args := []string{"-Q", "-C", "o", "-G", "rsa", "-g", "sha256", "-c", path}
	if _, err := p.runner.Run(ctx, "tpm2_createprimary", args, nil); err != nil {
		return fmt.Errorf("create TPM owner primary: %w", err)
	}
	return nil
}

func (p *TPM2ToolsKeyProvider) flush(ctx context.Context, path string) {
	_, _ = p.runner.Run(ctx, "tpm2_flushcontext", []string{"-Q", path}, nil)
}

func readBoundedTPMBlob(path string) ([]byte, error) {
	info, err := os.Stat(path)
	if err != nil {
		return nil, err
	}
	if info.Size() <= 0 || info.Size() > maxTPMBlobSize {
		return nil, errors.New("TPM sealed-key blob has an invalid size")
	}
	return os.ReadFile(path)
}

type osTPMCommandRunner struct{}

func (osTPMCommandRunner) Run(ctx context.Context, name string, args []string, stdin []byte) ([]byte, error) {
	command := exec.CommandContext(ctx, name, args...) // #nosec G204 -- fixed provider command names and argv
	if stdin != nil {
		command.Stdin = bytes.NewReader(stdin)
	}
	output, err := command.Output()
	if err != nil {
		return nil, fmt.Errorf("%s failed: %w", name, err)
	}
	return output, nil
}
