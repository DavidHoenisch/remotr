package gitsync

import (
	"context"
	"fmt"
	"os"
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
	if len(entries) == 0 {
		return g.clone(ctx, repo)
	}
	return g.initFromRemote(ctx, repo)
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

func (g *GitSyncer) initFromRemote(ctx context.Context, repo string) error {
	if err := g.runGitInRepo(ctx, "init"); err != nil {
		return fmt.Errorf("git init: %w", err)
	}
	if err := g.ensureRemoteOrigin(ctx); err != nil {
		return err
	}
	branch := g.branch()
	if err := g.runGitInRepo(ctx, "fetch", "origin", branch); err != nil {
		return fmt.Errorf("git fetch: %w", err)
	}
	return g.syncWorkingTree(ctx)
}

func (g *GitSyncer) syncWorkingTree(ctx context.Context) error {
	branch := g.branch()
	ref := "origin/" + branch
	if err := g.runGitInRepo(ctx, "checkout", "-B", branch, ref); err != nil {
		return fmt.Errorf("git checkout: %w", err)
	}
	if err := g.runGitInRepo(ctx, "reset", "--hard", ref); err != nil {
		return fmt.Errorf("git reset: %w", err)
	}
	return nil
}
