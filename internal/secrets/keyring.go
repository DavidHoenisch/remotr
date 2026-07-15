package secrets

import (
	"bytes"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"strings"

	"golang.org/x/sys/unix"
)

const maxKeyringBytes = 1 << 20

// Keyring keeps one active KEK and any decrypt-only historical KEKs. It is
// loaded from deployment-managed material outside Postgres.
type Keyring struct {
	active string
	keys   map[string][]byte
}

func NewKeyring(active string, keys map[string][]byte) (*Keyring, error) {
	if strings.TrimSpace(active) == "" || strings.TrimSpace(active) != active || len(active) > 128 || strings.ContainsAny(active, "\x00\r\n") {
		return nil, fmt.Errorf("active KEK identifier is invalid")
	}
	copyKeys := make(map[string][]byte, len(keys))
	for id, key := range keys {
		if strings.TrimSpace(id) == "" || strings.TrimSpace(id) != id || len(id) > 128 || strings.ContainsAny(id, "\x00\r\n") {
			return nil, fmt.Errorf("KEK identifier is invalid")
		}
		if len(key) != 32 {
			return nil, fmt.Errorf("KEK %q must be 32 bytes", id)
		}
		copyKeys[id] = append([]byte(nil), key...)
	}
	if _, ok := copyKeys[active]; !ok {
		return nil, fmt.Errorf("active KEK %q is absent from external keyring", active)
	}
	return &Keyring{active: active, keys: copyKeys}, nil
}

func (k *Keyring) ActiveID() string { return k.active }

func (k *Keyring) Has(id string) bool {
	_, ok := k.keys[id]
	return ok
}

func (k *Keyring) activeKey() (string, []byte, error) {
	if k == nil {
		return "", nil, fmt.Errorf("external KEK keyring is required")
	}
	key, ok := k.keys[k.active]
	if !ok || len(key) != 32 {
		return "", nil, fmt.Errorf("active external KEK is unavailable")
	}
	return k.active, key, nil
}

func (k *Keyring) key(id string) ([]byte, bool) {
	if k == nil {
		return nil, false
	}
	key, ok := k.keys[id]
	return key, ok
}

type keyringDocument struct {
	Active string            `json:"active"`
	Keys   map[string]string `json:"keys"`
}

func LoadKeyringJSON(data []byte) (*Keyring, error) {
	if len(data) == 0 || len(data) > maxKeyringBytes {
		return nil, fmt.Errorf("external KEK keyring is empty or exceeds %d bytes", maxKeyringBytes)
	}
	var document keyringDocument
	decoder := json.NewDecoder(bytes.NewReader(data))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&document); err != nil {
		return nil, fmt.Errorf("decode external KEK keyring: %w", err)
	}
	if err := ensureJSONEOF(decoder); err != nil {
		return nil, err
	}
	keys := make(map[string][]byte, len(document.Keys))
	for id, encoded := range document.Keys {
		key, err := base64.StdEncoding.Strict().DecodeString(encoded)
		if err != nil {
			return nil, fmt.Errorf("decode external KEK %q: invalid base64", id)
		}
		keys[id] = key
	}
	return NewKeyring(document.Active, keys)
}

type KeyringFileOption func(*keyringFileOptions)

type keyringFileOptions struct {
	requiredUID uint32
}

func WithKeyringRequiredUID(uid uint32) KeyringFileOption {
	return func(options *keyringFileOptions) { options.requiredUID = uid }
}

func LoadKeyringFile(path string, options ...KeyringFileOption) (*Keyring, error) {
	settings := keyringFileOptions{requiredUID: 0}
	for _, option := range options {
		option(&settings)
	}
	if path == "" {
		return nil, fmt.Errorf("external KEK keyring path is required")
	}
	fd, err := unix.Open(path, unix.O_RDONLY|unix.O_CLOEXEC|unix.O_NOFOLLOW, 0)
	if err != nil {
		return nil, fmt.Errorf("open external KEK keyring: %w", err)
	}
	file := os.NewFile(uintptr(fd), path)
	if file == nil {
		_ = unix.Close(fd)
		return nil, fmt.Errorf("open external KEK keyring: invalid file descriptor")
	}
	defer file.Close()

	var stat unix.Stat_t
	if err := unix.Fstat(fd, &stat); err != nil {
		return nil, fmt.Errorf("inspect external KEK keyring: %w", err)
	}
	if stat.Mode&unix.S_IFMT != unix.S_IFREG {
		return nil, fmt.Errorf("external KEK keyring must be a regular file")
	}
	if stat.Uid != settings.requiredUID {
		return nil, fmt.Errorf("external KEK keyring must be owned by uid %d", settings.requiredUID)
	}
	if stat.Mode&0o077 != 0 {
		return nil, fmt.Errorf("external KEK keyring must not be accessible by group or other")
	}
	if stat.Size <= 0 || stat.Size > maxKeyringBytes {
		return nil, fmt.Errorf("external KEK keyring is empty or exceeds %d bytes", maxKeyringBytes)
	}
	data, err := io.ReadAll(io.LimitReader(file, maxKeyringBytes+1))
	if err != nil {
		return nil, fmt.Errorf("read external KEK keyring: %w", err)
	}
	return LoadKeyringJSON(data)
}

func ensureJSONEOF(decoder *json.Decoder) error {
	var extra any
	if err := decoder.Decode(&extra); err != io.EOF {
		if err == nil {
			return fmt.Errorf("external KEK keyring contains trailing JSON")
		}
		return fmt.Errorf("decode external KEK keyring: %w", err)
	}
	return nil
}
