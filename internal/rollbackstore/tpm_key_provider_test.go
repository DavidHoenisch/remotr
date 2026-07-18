package rollbackstore_test

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"testing"

	"github.com/DavidHoenisch/remotr/internal/rollbackstore"
)

func TestTPM2ToolsKeyProviderCapabilityGate(t *testing.T) {
	probeFailure := errors.New("TPM transport unavailable")
	tests := []struct {
		name         string
		deviceExists bool
		missingTool  string
		runnerErr    error
		want         bool
		wantErr      error
		wantCommands []tpmCommand
	}{
		{name: "no TPM device"},
		{name: "required tool missing", deviceExists: true, missingTool: "tpm2_unseal"},
		{
			name: "supported", deviceExists: true, want: true,
			wantCommands: []tpmCommand{{name: "tpm2_getcap", args: []string{"-Q", "properties-fixed"}}},
		},
		{
			name: "advertised provider fails", deviceExists: true, runnerErr: probeFailure, wantErr: probeFailure,
			wantCommands: []tpmCommand{{name: "tpm2_getcap", args: []string{"-Q", "properties-fixed"}}},
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			runner := &recordingTPMRunner{failCommand: "tpm2_getcap", err: test.runnerErr}
			provider := rollbackstore.NewTPM2ToolsKeyProvider(rollbackstore.TPM2ToolsOptions{
				Runner: runner,
				DeviceExists: func(path string) bool {
					return test.deviceExists && (path == "/dev/tpmrm0" || path == "/dev/tpm0")
				},
				LookPath: func(name string) (string, error) {
					if name == test.missingTool {
						return "", os.ErrNotExist
					}
					return "/usr/bin/" + name, nil
				},
			})
			got, err := provider.Supported(context.Background())
			if got != test.want || !errors.Is(err, test.wantErr) {
				t.Fatalf("Supported() = %t, %v, want %t, %v", got, err, test.want, test.wantErr)
			}
			if !reflect.DeepEqual(runner.commands, test.wantCommands) {
				t.Fatalf("commands = %#v, want %#v", runner.commands, test.wantCommands)
			}
		})
	}
}

func TestTPM2ToolsKeyProviderSealsAndReloadsWithoutArgumentLeakage(t *testing.T) {
	ctx := context.Background()
	root := t.TempDir()
	key := bytes.Repeat([]byte{0xa7}, 32)
	random := append(append([]byte(nil), key...), bytes.Repeat([]byte{0xb8}, 16)...)
	runner := &recordingTPMRunner{unsealed: key}
	provider := rollbackstore.NewTPM2ToolsKeyProvider(rollbackstore.TPM2ToolsOptions{
		Runner: runner, Random: bytes.NewReader(random),
	})

	created, err := provider.LoadOrCreate(ctx, root)
	if err != nil {
		t.Fatal(err)
	}
	if created.Protection != rollbackstore.ProtectionTPMSealed || created.ID == "" || !bytes.Equal(created.Key, key) {
		t.Fatalf("created TPM material = id %q class %q bytes %x", created.ID, created.Protection, created.Key)
	}
	if len(runner.commands) != 3 {
		t.Fatalf("create command count = %d, want 3: %#v", len(runner.commands), runner.commands)
	}
	primary := commandFlag(t, runner.commands[0].args, "-c")
	public := commandFlag(t, runner.commands[1].args, "-u")
	private := commandFlag(t, runner.commands[1].args, "-r")
	wantCreate := []tpmCommand{
		{name: "tpm2_createprimary", args: []string{"-Q", "-C", "o", "-G", "rsa", "-g", "sha256", "-c", primary}},
		{name: "tpm2_create", args: []string{"-Q", "-C", primary, "-g", "sha256", "-i", "-", "-u", public, "-r", private}, stdin: key},
		{name: "tpm2_flushcontext", args: []string{"-Q", primary}},
	}
	if !reflect.DeepEqual(runner.commands, wantCreate) {
		t.Fatalf("create commands = %#v, want %#v", runner.commands, wantCreate)
	}
	for _, command := range runner.commands {
		for _, arg := range command.args {
			if bytes.Contains([]byte(arg), key) {
				t.Fatalf("TPM argv exposed key material: %#v", command)
			}
		}
	}
	raw, err := os.ReadFile(filepath.Join(root, "rollback.tpm-key"))
	if err != nil {
		t.Fatal(err)
	}
	if bytes.Contains(raw, key) {
		t.Fatalf("sealed-key file contains plaintext key: %x", raw)
	}

	runner.commands = nil
	reloaded, err := provider.LoadOrCreate(ctx, root)
	if err != nil {
		t.Fatal(err)
	}
	if reloaded.ID != created.ID || reloaded.Protection != rollbackstore.ProtectionTPMSealed || !bytes.Equal(reloaded.Key, key) {
		t.Fatalf("reloaded TPM material = id %q class %q bytes %x", reloaded.ID, reloaded.Protection, reloaded.Key)
	}
	if len(runner.commands) != 5 {
		t.Fatalf("load command count = %d, want 5: %#v", len(runner.commands), runner.commands)
	}
	primary = commandFlag(t, runner.commands[0].args, "-c")
	public = commandFlag(t, runner.commands[1].args, "-u")
	private = commandFlag(t, runner.commands[1].args, "-r")
	sealed := commandFlag(t, runner.commands[1].args, "-c")
	wantLoad := []tpmCommand{
		{name: "tpm2_createprimary", args: []string{"-Q", "-C", "o", "-G", "rsa", "-g", "sha256", "-c", primary}},
		{name: "tpm2_load", args: []string{"-Q", "-C", primary, "-u", public, "-r", private, "-c", sealed}},
		{name: "tpm2_unseal", args: []string{"-Q", "-c", sealed}},
		{name: "tpm2_flushcontext", args: []string{"-Q", sealed}},
		{name: "tpm2_flushcontext", args: []string{"-Q", primary}},
	}
	if !reflect.DeepEqual(runner.commands, wantLoad) {
		t.Fatalf("load commands = %#v, want %#v", runner.commands, wantLoad)
	}
}

func TestStoreReportsSelectedTPMClassWithoutKeyMaterial(t *testing.T) {
	key := bytes.Repeat([]byte{0xc9}, 32)
	store, err := rollbackstore.New(rollbackstore.Options{
		Root: t.TempDir(),
		KeyProvider: &rollbackstore.CapabilityKeyProvider{
			Capability: fixedTPMCapability{supported: true},
			TPM: &recordingKeyProvider{material: rollbackstore.KeyMaterial{
				ID: "tpm-v1-safe-identity", Key: key, Protection: rollbackstore.ProtectionTPMSealed,
			}},
			Root: &recordingKeyProvider{err: errors.New("root fallback must not be called")},
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	report := store.Protection()
	if report.Class != rollbackstore.ProtectionTPMSealed || report.KeyID != "tpm-v1-safe-identity" || report.ReducedProtection || report.Limitation != "" {
		t.Fatalf("TPM report = %+v", report)
	}
	encoded, err := json.Marshal(report)
	if err != nil {
		t.Fatal(err)
	}
	if bytes.Contains(encoded, key) {
		t.Fatalf("TPM report exposed key material: %s", encoded)
	}
}

func TestTPM2ToolsKeyProviderRotationRetainsSealedHistory(t *testing.T) {
	ctx := context.Background()
	root := t.TempDir()
	firstKey := bytes.Repeat([]byte{0xd1}, 32)
	firstRandom := append(append([]byte(nil), firstKey...), bytes.Repeat([]byte{0xd2}, 16)...)
	runner := &recordingTPMRunner{unsealed: firstKey}
	provider := rollbackstore.NewTPM2ToolsKeyProvider(rollbackstore.TPM2ToolsOptions{
		Runner: runner, Random: bytes.NewReader(firstRandom),
	})
	first, err := provider.LoadOrCreate(ctx, root)
	if err != nil {
		t.Fatal(err)
	}

	secondKey := bytes.Repeat([]byte{0xe1}, 32)
	secondRandom := append(append([]byte(nil), secondKey...), bytes.Repeat([]byte{0xe2}, 16)...)
	rotating := rollbackstore.NewTPM2ToolsKeyProvider(rollbackstore.TPM2ToolsOptions{
		Runner: runner, Random: bytes.NewReader(secondRandom),
	})
	runner.commands = nil
	second, err := rotating.Rotate(ctx, root)
	if err != nil {
		t.Fatal(err)
	}
	if second.ID == first.ID || !bytes.Equal(second.Key, secondKey) {
		t.Fatalf("rotated TPM key = id %q bytes %x", second.ID, second.Key)
	}
	if len(runner.commands) != 3 || runner.commands[1].name != "tpm2_create" ||
		!bytes.Equal(runner.commands[1].stdin, secondKey) {
		t.Fatalf("rotation commands = %#v, want one exact stdin sealing sequence", runner.commands)
	}

	runner.unsealed = firstKey
	historical, err := rotating.LoadByID(ctx, root, first.ID)
	if err != nil {
		t.Fatal(err)
	}
	if historical.ID != first.ID || !bytes.Equal(historical.Key, firstKey) {
		t.Fatalf("historical TPM key = id %q bytes %x", historical.ID, historical.Key)
	}
	runner.unsealed = secondKey
	active, err := rotating.LoadOrCreate(ctx, root)
	if err != nil {
		t.Fatal(err)
	}
	if active.ID != second.ID || !bytes.Equal(active.Key, secondKey) {
		t.Fatalf("active TPM key = id %q bytes %x", active.ID, active.Key)
	}
	raw, err := os.ReadFile(filepath.Join(root, "rollback.tpm-key"))
	if err != nil {
		t.Fatal(err)
	}
	var keyring struct {
		Version  int    `json:"version"`
		ActiveID string `json:"activeKeyId"`
		Keys     []struct {
			ID string `json:"id"`
		} `json:"keys"`
	}
	if err := json.Unmarshal(raw, &keyring); err != nil || keyring.Version != 2 ||
		keyring.ActiveID != second.ID || len(keyring.Keys) != 2 {
		t.Fatalf("rotated TPM keyring = %+v, %v", keyring, err)
	}
	if bytes.Contains(raw, firstKey) || bytes.Contains(raw, secondKey) {
		t.Fatalf("TPM keyring exposed plaintext key material: %x", raw)
	}
}

type tpmCommand struct {
	name  string
	args  []string
	stdin []byte
}

type recordingTPMRunner struct {
	commands    []tpmCommand
	unsealed    []byte
	failCommand string
	err         error
}

func (r *recordingTPMRunner) Run(_ context.Context, name string, args []string, stdin []byte) ([]byte, error) {
	r.commands = append(r.commands, tpmCommand{
		name: name, args: append([]string(nil), args...), stdin: append([]byte(nil), stdin...),
	})
	if name == r.failCommand && r.err != nil {
		return nil, r.err
	}
	switch name {
	case "tpm2_createprimary":
		return nil, os.WriteFile(flagValue(args, "-c"), []byte("primary-context"), 0o600)
	case "tpm2_create":
		if len(stdin) != 32 {
			return nil, fmt.Errorf("sealed input bytes = %d, want 32", len(stdin))
		}
		if err := os.WriteFile(flagValue(args, "-u"), []byte("sealed-public"), 0o600); err != nil {
			return nil, err
		}
		return nil, os.WriteFile(flagValue(args, "-r"), []byte("sealed-private"), 0o600)
	case "tpm2_load":
		return nil, os.WriteFile(flagValue(args, "-c"), []byte("sealed-context"), 0o600)
	case "tpm2_unseal":
		return append([]byte(nil), r.unsealed...), nil
	default:
		return nil, nil
	}
}

func commandFlag(t *testing.T, args []string, flag string) string {
	t.Helper()
	value := flagValue(args, flag)
	if value == "" {
		t.Fatalf("flag %q missing from argv %#v", flag, args)
	}
	return value
}

func flagValue(args []string, flag string) string {
	for index := 0; index+1 < len(args); index++ {
		if args[index] == flag {
			return args[index+1]
		}
	}
	return ""
}
