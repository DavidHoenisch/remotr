package main

import (
	"context"
	"errors"
	"fmt"
	"path/filepath"
	"slices"
	"strings"
	"sync"
	"time"

	"github.com/DavidHoenisch/remotr/internal/admin"
	"github.com/DavidHoenisch/remotr/internal/deploytoken"
	wailsruntime "github.com/wailsapp/wails/v2/pkg/runtime"
)

const deploymentTokenViewLimit = 5000

type DeploymentTokenCreateRequest struct {
	Label      string `json:"label"`
	Fleet      string `json:"fleet"`
	TTLSeconds int64  `json:"ttlSeconds"`
}

type DeploymentTokenRevokeRequest struct {
	Label        string `json:"label"`
	Confirmation string `json:"confirmation"`
}

type DeploymentTokenSaveDialogRequest struct {
	SuggestedName string
}

type DeploymentTokenSaveDialog func(context.Context, DeploymentTokenSaveDialogRequest) (string, error)

type DeploymentTokenService struct {
	mu                sync.Mutex
	token             []byte
	label             string
	creating          bool
	chooseDestination DeploymentTokenSaveDialog
	now               func() time.Time
}

func NewDeploymentTokenService(dialog DeploymentTokenSaveDialog) *DeploymentTokenService {
	return &DeploymentTokenService{
		chooseDestination: dialog,
		now:               time.Now,
	}
}

func defaultDeploymentTokenSaveDialog(ctx context.Context, request DeploymentTokenSaveDialogRequest) (string, error) {
	return wailsruntime.SaveFileDialog(ctx, wailsruntime.SaveDialogOptions{
		Title:                "Save deployment token",
		DefaultFilename:      request.SuggestedName,
		CanCreateDirectories: true,
		Filters: []wailsruntime.FileFilter{{
			DisplayName: "Remotr deployment token (*.token)",
			Pattern:     "*.token",
		}},
	})
}

func (s *DeploymentTokenService) ListConnected(ctx context.Context, client *admin.Client) ([]DeploymentTokenView, error) {
	if client == nil {
		return nil, ErrSessionNotConnected
	}
	tokens, err := client.ListDeploymentTokensContext(ctx)
	if err != nil {
		return nil, err
	}
	if len(tokens) > deploymentTokenViewLimit {
		return nil, errors.New("deployment token inventory exceeds the supported limit")
	}
	views := make([]DeploymentTokenView, 0, len(tokens))
	now := s.currentTime()
	for _, token := range tokens {
		view, mapErr := mapDeploymentTokenView(token, now)
		if mapErr != nil {
			return nil, mapErr
		}
		views = append(views, view)
	}
	slices.SortFunc(views, func(left, right DeploymentTokenView) int {
		return strings.Compare(left.Label, right.Label)
	})
	return views, nil
}

func (s *DeploymentTokenService) LoadConnected(ctx context.Context, client *admin.Client, label string) (DeploymentTokenView, error) {
	if client == nil {
		return DeploymentTokenView{}, ErrSessionNotConnected
	}
	if err := validateDeploymentTokenLabel(label); err != nil {
		return DeploymentTokenView{}, err
	}
	token, err := client.GetDeploymentTokenContext(ctx, label)
	if err != nil {
		return DeploymentTokenView{}, err
	}
	if token.Label != label {
		return DeploymentTokenView{}, errors.New("server returned a different deployment token identity")
	}
	return mapDeploymentTokenView(token, s.currentTime())
}

func (s *DeploymentTokenService) CreateConnected(ctx context.Context, client *admin.Client, request DeploymentTokenCreateRequest) (DeploymentTokenCreateResult, error) {
	if client == nil {
		return DeploymentTokenCreateResult{}, ErrSessionNotConnected
	}
	if err := validateDeploymentTokenLabel(request.Label); err != nil {
		return DeploymentTokenCreateResult{}, err
	}
	if request.Fleet == "" || strings.TrimSpace(request.Fleet) != request.Fleet {
		return DeploymentTokenCreateResult{}, deploymentTokenValidationFailure("Select an existing Fleet before creating a deployment token.")
	}
	if request.TTLSeconds <= 0 || request.TTLSeconds > maxEnrollmentTTLSeconds {
		return DeploymentTokenCreateResult{}, deploymentTokenValidationFailure("Enter a positive token lifetime supported by the server.")
	}

	s.mu.Lock()
	if s.creating || len(s.token) > 0 {
		s.mu.Unlock()
		return DeploymentTokenCreateResult{}, &ActionFailure{
			Kind:      ActionConflict,
			Message:   "A one-time deployment token is already active.",
			Guidance:  "Copy, save, or clear the current token before creating another.",
			Retryable: false,
		}
	}
	s.creating = true
	s.mu.Unlock()
	defer func() {
		s.mu.Lock()
		s.creating = false
		s.mu.Unlock()
	}()

	fleets, err := client.ListFleetsContext(ctx)
	if err != nil {
		return DeploymentTokenCreateResult{}, err
	}
	if !slices.Contains(fleets, request.Fleet) {
		return DeploymentTokenCreateResult{}, deploymentTokenValidationFailure("Select an existing Fleet from the current workspace.")
	}
	response, err := client.CreateDeploymentTokenContext(ctx, request.Label, request.Fleet, time.Duration(request.TTLSeconds)*time.Second)
	if err != nil {
		return DeploymentTokenCreateResult{}, err
	}
	if response.Token == "" || response.Label != request.Label || response.Fleet != request.Fleet || response.ExpiresAt.IsZero() {
		return DeploymentTokenCreateResult{}, errors.New("server returned incomplete deployment token metadata")
	}
	if cause := context.Cause(ctx); cause != nil {
		return DeploymentTokenCreateResult{}, cause
	}

	s.mu.Lock()
	zeroBytes(s.token)
	s.token = []byte(response.Token)
	s.label = response.Label
	s.mu.Unlock()
	return DeploymentTokenCreateResult{
		Token: response.Token,
		Metadata: DeploymentTokenView{
			Label:     response.Label,
			Fleet:     response.Fleet,
			Status:    deploymentTokenStatusFromTimes(nil, response.ExpiresAt, s.currentTime()),
			ExpiresAt: formatTimestamp(response.ExpiresAt),
		},
	}, nil
}

func (s *DeploymentTokenService) RevokeConnected(ctx context.Context, client *admin.Client, request DeploymentTokenRevokeRequest) (DeploymentTokenView, error) {
	if client == nil {
		return DeploymentTokenView{}, ErrSessionNotConnected
	}
	if err := validateDeploymentTokenLabel(request.Label); err != nil {
		return DeploymentTokenView{}, err
	}
	if request.Confirmation != request.Label {
		return DeploymentTokenView{}, deploymentTokenValidationFailure("Type the exact case-sensitive deployment token label to confirm revocation.")
	}
	if err := client.RevokeDeploymentTokenContext(ctx, request.Label); err != nil {
		return DeploymentTokenView{}, err
	}
	token, err := client.GetDeploymentTokenContext(ctx, request.Label)
	if err != nil {
		return DeploymentTokenView{}, err
	}
	if token.Label != request.Label || token.RevokedAt == nil {
		return DeploymentTokenView{}, errors.New("server did not confirm deployment token revocation")
	}
	return mapDeploymentTokenView(token, s.currentTime())
}

func (s *DeploymentTokenService) Copy(ctx context.Context, writer ClipboardWriter) error {
	if writer == nil {
		return errors.New("native clipboard is unavailable")
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if len(s.token) == 0 {
		return errors.New("the deployment token has been cleared")
	}
	if err := writer(ctx, string(s.token)); err != nil {
		return errors.New("native clipboard copy failed")
	}
	return nil
}

func (s *DeploymentTokenService) Save(ctx context.Context, label string) (DeploymentTokenSaveResult, error) {
	if err := validateDeploymentTokenLabel(label); err != nil {
		return DeploymentTokenSaveResult{}, err
	}
	if s == nil || s.chooseDestination == nil {
		return DeploymentTokenSaveResult{}, errors.New("native deployment token save dialog is unavailable")
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if len(s.token) == 0 {
		return DeploymentTokenSaveResult{}, errors.New("the deployment token has been cleared")
	}
	if label != s.label {
		return DeploymentTokenSaveResult{}, deploymentTokenValidationFailure("Save only the exact deployment token currently shown.")
	}
	destination, err := s.chooseDestination(ctx, DeploymentTokenSaveDialogRequest{SuggestedName: label + ".token"})
	if err != nil {
		return DeploymentTokenSaveResult{}, fmt.Errorf("choose deployment token destination: %w", err)
	}
	if destination == "" {
		return DeploymentTokenSaveResult{Status: "canceled"}, nil
	}
	destination = filepath.Clean(destination)
	if filepath.Base(destination) == "." || filepath.Base(destination) == string(filepath.Separator) {
		return DeploymentTokenSaveResult{}, errors.New("deployment token destination must name a file")
	}
	data := append(slices.Clone(s.token), '\n')
	defer zeroBytes(data)
	if err := writeReadExportAtomic(destination, data); err != nil {
		return DeploymentTokenSaveResult{}, fmt.Errorf("save protected deployment token: %w", err)
	}
	return DeploymentTokenSaveResult{Status: "saved", Path: destination, SizeBytes: int64(len(data))}, nil
}

func (s *DeploymentTokenService) Clear() {
	if s == nil {
		return
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	zeroBytes(s.token)
	s.token = nil
	s.label = ""
}

func (s *DeploymentTokenService) currentTime() time.Time {
	if s != nil && s.now != nil {
		return s.now().UTC()
	}
	return time.Now().UTC()
}

func mapDeploymentTokenView(token admin.DeploymentToken, now time.Time) (DeploymentTokenView, error) {
	if token.ID == "" || token.Label == "" || token.Fleet == "" || token.CreatedAt.IsZero() || token.ExpiresAt.IsZero() {
		return DeploymentTokenView{}, errors.New("server returned incomplete deployment token metadata")
	}
	return DeploymentTokenView{
		ID:         token.ID,
		Label:      token.Label,
		Fleet:      token.Fleet,
		Status:     deploymentTokenStatusFromTimes(token.RevokedAt, token.ExpiresAt, now),
		CreatedAt:  formatTimestamp(token.CreatedAt),
		ExpiresAt:  formatTimestamp(token.ExpiresAt),
		RevokedAt:  formatOptionalReadExportTime(token.RevokedAt),
		LastUsedAt: formatOptionalReadExportTime(token.LastUsedAt),
	}, nil
}

func deploymentTokenStatusFromTimes(revokedAt *time.Time, expiresAt, now time.Time) string {
	if revokedAt != nil {
		return "revoked"
	}
	if !expiresAt.After(now) {
		return "expired"
	}
	return "active"
}

func validateDeploymentTokenLabel(label string) error {
	if strings.TrimSpace(label) != label || deploytoken.ValidateLabel(label) != nil {
		return deploymentTokenValidationFailure("Use a non-empty label containing only letters, numbers, hyphens, or underscores.")
	}
	return nil
}

func deploymentTokenValidationFailure(guidance string) error {
	return &ActionFailure{
		Kind:      ActionValidation,
		Message:   "The deployment token request is invalid.",
		Guidance:  guidance,
		Retryable: false,
	}
}
