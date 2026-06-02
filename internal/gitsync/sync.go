package gitsync

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync/atomic"
	"time"
)

const webhookHeader = "X-Remotr-Git-Webhook-Secret"

// ReleaseRefStore persists the global release ref.
type ReleaseRefStore interface {
	GetReleaseRef(ctx context.Context) (string, error)
	SetReleaseRef(ctx context.Context, ref string) error
}

// GitSyncer advances release ref from a configuration repository Git checkout.
type GitSyncer struct {
	RepoPath      string
	RemoteURL     string
	Branch        string
	Token         string
	Username      string
	PollInterval  time.Duration
	WebhookSecret string
	Store         ReleaseRefStore
	StaticRef     string

	current atomic.Value // string fallback when Store is nil
}

// Handler serves POST webhook requests that trigger an immediate sync.
func (g *GitSyncer) Handler() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
			return
		}
		if g.WebhookSecret != "" {
			if r.Header.Get(webhookHeader) != g.WebhookSecret {
				http.Error(w, "unauthorized", http.StatusUnauthorized)
				return
			}
		}
		if err := g.Sync(r.Context()); err != nil {
			slog.Error("git sync webhook", "err", err)
			http.Error(w, "sync failed", http.StatusInternalServerError)
			return
		}
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("ok"))
	}
}

// StartPoll runs periodic git fetch + HEAD resolution until ctx is cancelled.
func (g *GitSyncer) StartPoll(ctx context.Context) {
	if g.PollInterval <= 0 {
		return
	}
	go func() {
		ticker := time.NewTicker(g.PollInterval)
		defer ticker.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
				if err := g.Sync(ctx); err != nil {
					slog.Warn("git sync poll", "err", err)
				}
			}
		}
	}()
}

// Sync resolves HEAD and persists the release ref when it changed.
func (g *GitSyncer) Sync(ctx context.Context) error {
	ref, err := g.resolveHEAD(ctx)
	if err != nil {
		return err
	}
	if ref == "" {
		return errors.New("empty release ref")
	}
	if g.Store != nil {
		prev, err := g.Store.GetReleaseRef(ctx)
		if err != nil {
			return err
		}
		if prev == ref {
			return nil
		}
		if err := g.Store.SetReleaseRef(ctx, ref); err != nil {
			return err
		}
		slog.Info("release ref advanced", "ref", ref)
		return nil
	}
	g.current.Store(ref)
	slog.Info("release ref advanced", "ref", ref)
	return nil
}

// ReleaseRef implements server.ReleaseRefSource.
func (g *GitSyncer) ReleaseRef(ctx context.Context) string {
	return g.Current(ctx)
}

// Current returns the latest release ref from the store or memory fallback.
func (g *GitSyncer) Current(ctx context.Context) string {
	if g.Store != nil {
		ref, err := g.Store.GetReleaseRef(ctx)
		if err == nil && ref != "" {
			return ref
		}
	}
	if v := g.current.Load(); v != nil {
		if ref, ok := v.(string); ok && ref != "" {
			return ref
		}
	}
	return g.StaticRef
}

func (g *GitSyncer) resolveHEAD(ctx context.Context) (string, error) {
	if g.RemoteURL != "" {
		if err := g.ensureCheckout(ctx); err != nil {
			return "", err
		}
	}
	if !isGitRepo(g.RepoPath) {
		if g.StaticRef != "" {
			return g.StaticRef, nil
		}
		return "", fmt.Errorf("config repo is not a git repository")
	}
	if g.RemoteURL != "" {
		if err := g.runGitInRepo(ctx, "fetch", "origin", g.branch()); err != nil {
			return "", fmt.Errorf("git fetch: %w", err)
		}
		if err := g.syncWorkingTree(ctx); err != nil {
			return "", err
		}
	}
	out, err := g.runGitOutput(ctx, "rev-parse", "HEAD")
	if err != nil {
		return "", fmt.Errorf("git rev-parse HEAD: %w", err)
	}
	return strings.TrimSpace(string(out)), nil
}

func (g *GitSyncer) branch() string {
	if g.Branch != "" {
		return g.Branch
	}
	return "main"
}

func (g *GitSyncer) runGitInRepo(ctx context.Context, args ...string) error {
	_, err := g.runGitOutputInRepo(ctx, args...)
	return err
}

func (g *GitSyncer) runGitGlobal(ctx context.Context, args ...string) error {
	_, err := g.runGitOutputGlobal(ctx, args...)
	return err
}

func (g *GitSyncer) runGitOutput(ctx context.Context, args ...string) ([]byte, error) {
	return g.runGitOutputInRepo(ctx, args...)
}

func (g *GitSyncer) runGitOutputInRepo(ctx context.Context, args ...string) ([]byte, error) {
	repo, err := validateRepoPath(g.RepoPath)
	if err != nil {
		return nil, err
	}
	return g.runGitCommand(ctx, repo, args...)
}

func (g *GitSyncer) runGitOutputGlobal(ctx context.Context, args ...string) ([]byte, error) {
	return g.runGitCommand(ctx, "", args...)
}

func (g *GitSyncer) runGitCommand(ctx context.Context, repo string, args ...string) ([]byte, error) {
	cmdArgs := append([]string{}, g.authConfigArgs()...)
	if repo != "" {
		cmdArgs = append(cmdArgs, "-C", repo)
	}
	cmdArgs = append(cmdArgs, args...)
	cmd := exec.CommandContext(ctx, "git", cmdArgs...) // #nosec G204 G702 -- fixed git subcommands, validated repo path
	cmd.Env = append(os.Environ(), "GIT_TERMINAL_PROMPT=0")
	var stderr bytes.Buffer
	cmd.Stderr = &stderr
	out, err := cmd.Output()
	if err != nil {
		msg := strings.TrimSpace(stderr.String())
		if msg != "" {
			return nil, fmt.Errorf("%w: %s", err, msg)
		}
		return nil, err
	}
	return out, nil
}

func validateRepoPath(path string) (string, error) {
	clean := filepath.Clean(strings.TrimSpace(path))
	if clean == "" {
		return "", fmt.Errorf("repo path is required")
	}
	if !filepath.IsAbs(clean) {
		return "", fmt.Errorf("repo path must be absolute")
	}
	if strings.Contains(clean, "..") {
		return "", fmt.Errorf("invalid repo path")
	}
	return clean, nil
}

func isGitRepo(repoPath string) bool {
	_, err := os.Stat(filepath.Join(repoPath, ".git"))
	return err == nil
}
