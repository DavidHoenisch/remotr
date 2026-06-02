package server

import (
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"os"
	"strings"
	"sync"

	"github.com/DavidHoenisch/remotr/internal/registry"
)

// Bootstrap holds the one-time operator bootstrap token until exchanged.
type Bootstrap struct {
	mu    sync.Mutex
	token string
	file  string
}

// NewBootstrap returns bootstrap state backed by the given token file path.
func NewBootstrap(file string) *Bootstrap {
	return &Bootstrap{file: file}
}

// MaybeInit generates and persists a bootstrap token when no operators are registered.
func (b *Bootstrap) MaybeInit(admin registry.Admin) error {
	if admin == nil {
		return nil
	}
	if admin.HasOperators() {
		return nil
	}
	if b.file != "" {
		if raw, err := os.ReadFile(b.file); err == nil {
			token := strings.TrimSpace(string(raw))
			if token != "" {
				b.mu.Lock()
				b.token = token
				b.mu.Unlock()
				return nil
			}
		}
	}
	return b.generate()
}

func (b *Bootstrap) generate() error {
	raw := make([]byte, 32)
	if _, err := rand.Read(raw); err != nil {
		return fmt.Errorf("bootstrap token: %w", err)
	}
	token := hex.EncodeToString(raw)

	b.mu.Lock()
	defer b.mu.Unlock()
	b.token = token

	if b.file != "" {
		if err := os.MkdirAll(dirOf(b.file), 0o700); err != nil {
			return fmt.Errorf("bootstrap token dir: %w", err)
		}
		if err := os.WriteFile(b.file, []byte(token+"\n"), 0o600); err != nil {
			return fmt.Errorf("write bootstrap token file: %w", err)
		}
	}

	fmt.Println("Remotr operator bootstrap token (one-time, exchange via: remotr bootstrap):")
	fmt.Println(token)
	return nil
}

// Valid reports whether token matches the active bootstrap secret.
func (b *Bootstrap) Valid(token string) bool {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.token != "" && token == b.token
}

// Invalidate clears the in-memory token and removes the bootstrap file.
func (b *Bootstrap) Invalidate() {
	b.mu.Lock()
	defer b.mu.Unlock()
	b.token = ""
	if b.file != "" {
		_ = os.Remove(b.file)
	}
}

func dirOf(path string) string {
	for i := len(path) - 1; i >= 0; i-- {
		if path[i] == '/' {
			if i == 0 {
				return "/"
			}
			return path[:i]
		}
	}
	return "."
}
