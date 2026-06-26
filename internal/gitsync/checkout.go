package gitsync

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
)

func (g *GitSyncer) ensureCheckout(ctx context.Context) error {
	if g.RemoteURL == "" {
		return nil
	}
	if isGitRepo(g.RepoPath) {
		return g.ensureRemoteOrigin(ctx)
	}

	repo, err := validateRepoPath(g.RepoPath)
	if err != nil {
		return err
	}
	if err := os.MkdirAll(repo, 0o755); err != nil {
		return fmt.Errorf("mkdir config repo: %w", err)
	}

	entries, err := os.ReadDir(repo)
	if err != nil {
		return fmt.Errorf("read config repo: %w", err)
	}
	if len(entries) > 0 {
		// Fly and similar images ship a bundled seed tree at REMOTR_CONFIG_REPO.
		// Remove it before cloning so untracked files do not block checkout.
		if err := g.clearRepoContents(repo); err != nil {
			return err
		}
	}
	return g.clone(ctx, repo)
}

func (g *GitSyncer) ensureRemoteOrigin(ctx context.Context) error {
	clean := cleanRemoteURL(g.RemoteURL)
	if clean == "" {
		return nil
	}
	if err := g.runGitInRepo(ctx, "remote", "get-url", "origin"); err != nil {
		if err := g.runGitInRepo(ctx, "remote", "add", "origin", clean); err != nil {
			return fmt.Errorf("git remote add: %w", err)
		}
		return nil
	}
	if err := g.runGitInRepo(ctx, "remote", "set-url", "origin", clean); err != nil {
		return fmt.Errorf("git remote set-url: %w", err)
	}
	return nil
}

func (g *GitSyncer) clone(ctx context.Context, repo string) error {
	branch := g.branch()
	clean := cleanRemoteURL(g.RemoteURL)
	args := []string{"clone", "--branch", branch, "--single-branch", clean, repo}
	if err := g.runGitGlobal(ctx, args...); err != nil {
		return fmt.Errorf("git clone: %w", err)
	}
	return nil
}

func (g *GitSyncer) clearRepoContents(repo string) error {
	entries, err := os.ReadDir(repo)
	if err != nil {
		return fmt.Errorf("read config repo: %w", err)
	}
	for _, entry := range entries {
		if entry.Name() == ".git" {
			continue
		}
		if err := os.RemoveAll(filepath.Join(repo, entry.Name())); err != nil {
			return fmt.Errorf("clear config repo: %w", err)
		}
	}
	return nil
}

func (g *GitSyncer) syncWorkingTree(ctx context.Context) error {
	branch := g.branch()
	ref := "origin/" + branch
	if err := g.runGitInRepo(ctx, "checkout", "-f", "-B", branch, ref); err != nil {
		return fmt.Errorf("git checkout: %w", err)
	}
	if err := g.runGitInRepo(ctx, "reset", "--hard", ref); err != nil {
		return fmt.Errorf("git reset: %w", err)
	}
	return nil
}
