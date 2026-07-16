package main

import (
	"context"
	"fmt"
	"time"

	"github.com/DavidHoenisch/remotr/internal/admin"
)

type GitSyncService struct {
	now func() time.Time
}

func NewGitSyncService() *GitSyncService {
	return &GitSyncService{now: time.Now}
}

func (s *GitSyncService) RequestConnected(ctx context.Context, client *admin.Client, profileName string) (GitSyncResult, error) {
	if client == nil {
		return GitSyncResult{}, ErrSessionNotConnected
	}
	if err := client.TriggerGitSyncContext(ctx); err != nil {
		return GitSyncResult{}, err
	}
	return GitSyncResult{
		Status:           "accepted",
		Action:           "git_sync",
		Target:           "config-repo",
		ProfileName:      profileName,
		Summary:          fmt.Sprintf("Server accepted Git sync for the %s profile.", profileName),
		AcceptedAt:       s.now().UTC().Format(time.RFC3339),
		AffectedEvidence: []string{"release_ref", "activity"},
	}, nil
}
