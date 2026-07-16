package main

import (
	"cmp"
	"context"
	"errors"
	"fmt"
	"os"
	"slices"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/DavidHoenisch/remotr/internal/admin"
	"github.com/DavidHoenisch/remotr/internal/secretref"
	"github.com/DavidHoenisch/remotr/internal/secrets"
	wailsruntime "github.com/wailsapp/wails/v2/pkg/runtime"
)

const secretVersionViewLimit = 5_000

type SecretUploadRequest struct {
	Name      string `json:"name"`
	ScopeType string `json:"scopeType"`
	ScopeID   string `json:"scopeId"`
}

type SecretLifecycleRequest struct {
	Name         string `json:"name"`
	Version      string `json:"version"`
	Confirmation string `json:"confirmation"`
}

type SecretRolloutView struct {
	Fleet           string `json:"fleet"`
	ResourceAddress string `json:"resourceAddress"`
	Purpose         string `json:"purpose"`
	Risk            string `json:"risk"`
	EffectiveHash   string `json:"effectiveHash"`
	ChangeRequestID string `json:"changeRequestId"`
}

// SecretVersionView deliberately contains only safe registry metadata. Secret
// material, ciphertext, and encryption keys never cross the desktop bridge.
type SecretVersionView struct {
	Name                 string              `json:"name"`
	Version              string              `json:"version"`
	Fingerprint          string              `json:"fingerprint"`
	ScopeType            string              `json:"scopeType"`
	ScopeID              string              `json:"scopeId"`
	Status               string              `json:"status"`
	ActivationGeneration uint64              `json:"activationGeneration"`
	CreatedAt            string              `json:"createdAt"`
	CreatedBy            string              `json:"createdBy"`
	ActivatedAt          string              `json:"activatedAt"`
	ActivatedBy          string              `json:"activatedBy"`
	RevokedAt            string              `json:"revokedAt"`
	RevokedBy            string              `json:"revokedBy"`
	ResolutionBlocked    bool                `json:"resolutionBlocked"`
	EndpointCopyStatus   string              `json:"endpointCopyStatus"`
	Rollouts             []SecretRolloutView `json:"rollouts"`
}

type SecretInputOpenDialog func(context.Context) (string, error)
type ProtectedSecretReader func(string, uint32) ([]byte, error)

type SecretService struct {
	mu          sync.Mutex
	openInput   SecretInputOpenDialog
	readInput   ProtectedSecretReader
	requiredUID uint32
	uploading   bool
	lifecycle   map[string]bool
}

func NewSecretService(dialog SecretInputOpenDialog, reader ProtectedSecretReader, requiredUID uint32) *SecretService {
	return &SecretService{
		openInput: dialog, readInput: reader, requiredUID: requiredUID,
		lifecycle: map[string]bool{},
	}
}

func defaultSecretService() *SecretService {
	return NewSecretService(func(ctx context.Context) (string, error) {
		return wailsruntime.OpenFileDialog(ctx, wailsruntime.OpenDialogOptions{
			Title: "Choose protected secret file",
		})
	}, readProtectedSecretFile, uint32(os.Getuid()))
}

func readProtectedSecretFile(path string, requiredUID uint32) ([]byte, error) {
	return secrets.ReadProtectedMaterialFile(path, requiredUID)
}

func (s *SecretService) UploadConnected(ctx context.Context, client *admin.Client, request SecretUploadRequest) (SecretVersionView, error) {
	if client == nil {
		return SecretVersionView{}, ErrSessionNotConnected
	}
	if err := validateSecretName(request.Name); err != nil {
		return SecretVersionView{}, err
	}
	if request.ScopeID == "" || strings.TrimSpace(request.ScopeID) != request.ScopeID {
		return SecretVersionView{}, secretValidationFailure("Select an existing Fleet or Endpoint scope before choosing protected input.")
	}
	if err := validateSecretScope(ctx, client, request.ScopeType, request.ScopeID); err != nil {
		return SecretVersionView{}, err
	}
	if s == nil || s.openInput == nil || s.readInput == nil {
		return SecretVersionView{}, errors.New("native protected secret input is unavailable")
	}

	s.mu.Lock()
	if s.uploading {
		s.mu.Unlock()
		return SecretVersionView{}, secretConflictFailure("A secret upload is already in progress.")
	}
	s.uploading = true
	s.mu.Unlock()
	defer func() {
		s.mu.Lock()
		s.uploading = false
		s.mu.Unlock()
	}()

	path, err := s.openInput(ctx)
	if err != nil {
		return SecretVersionView{}, fmt.Errorf("choose protected secret input: %w", err)
	}
	if path == "" {
		return SecretVersionView{}, nil
	}
	material, err := s.readInput(path, s.requiredUID)
	if err != nil {
		return SecretVersionView{}, err
	}
	defer zeroBytes(material)

	fleet, endpointID := "", ""
	if request.ScopeType == "fleet" {
		fleet = request.ScopeID
	} else {
		endpointID = request.ScopeID
	}
	metadata, err := client.UploadSecretVersionContext(ctx, request.Name, fleet, endpointID, material)
	if err != nil {
		return SecretVersionView{}, err
	}
	view, err := mapSecretVersionView(metadata)
	if err != nil {
		return SecretVersionView{}, err
	}
	if view.Name != request.Name || view.ScopeType != request.ScopeType || view.ScopeID != request.ScopeID || view.Status != "inactive" {
		return SecretVersionView{}, errors.New("server returned inconsistent uploaded secret metadata")
	}
	return view, nil
}

func (s *SecretService) ListConnected(ctx context.Context, client *admin.Client, name string) ([]SecretVersionView, error) {
	if client == nil {
		return nil, ErrSessionNotConnected
	}
	if err := validateSecretName(name); err != nil {
		return nil, err
	}
	metadata, err := client.ListSecretVersionsContext(ctx, name)
	if err != nil {
		return nil, err
	}
	if len(metadata) > secretVersionViewLimit {
		return nil, errors.New("secret version inventory exceeds the supported limit")
	}
	views := make([]SecretVersionView, 0, len(metadata))
	for _, version := range metadata {
		view, mapErr := mapSecretVersionView(version)
		if mapErr != nil {
			return nil, mapErr
		}
		if view.Name != name {
			return nil, errors.New("server returned a different secret identity")
		}
		views = append(views, view)
	}
	slices.SortFunc(views, func(left, right SecretVersionView) int {
		leftVersion, _ := strconv.ParseUint(left.Version, 10, 64)
		rightVersion, _ := strconv.ParseUint(right.Version, 10, 64)
		return cmp.Compare(leftVersion, rightVersion)
	})
	return views, nil
}

func (s *SecretService) ActivateConnected(ctx context.Context, client *admin.Client, request SecretLifecycleRequest) (SecretVersionView, error) {
	return s.lifecycleConnected(ctx, client, "activate", request)
}

func (s *SecretService) RevokeConnected(ctx context.Context, client *admin.Client, request SecretLifecycleRequest) (SecretVersionView, error) {
	return s.lifecycleConnected(ctx, client, "revoke", request)
}

func (s *SecretService) lifecycleConnected(ctx context.Context, client *admin.Client, action string, request SecretLifecycleRequest) (SecretVersionView, error) {
	if client == nil {
		return SecretVersionView{}, ErrSessionNotConnected
	}
	if err := validateSecretVersion(request.Name, request.Version); err != nil {
		return SecretVersionView{}, err
	}
	wantConfirmation := request.Name + "@" + request.Version + " " + strings.ToUpper(action)
	if request.Confirmation != wantConfirmation {
		return SecretVersionView{}, secretValidationFailure("Type " + wantConfirmation + " exactly to confirm this secret lifecycle action.")
	}
	key := action + "\x00" + request.Name + "\x00" + request.Version
	s.mu.Lock()
	if s.lifecycle[key] {
		s.mu.Unlock()
		return SecretVersionView{}, secretConflictFailure("This secret lifecycle action is already in progress.")
	}
	s.lifecycle[key] = true
	s.mu.Unlock()
	defer func() {
		s.mu.Lock()
		delete(s.lifecycle, key)
		s.mu.Unlock()
	}()

	var metadata admin.SecretVersionMetadata
	var err error
	if action == "activate" {
		metadata, err = client.ActivateSecretVersionContext(ctx, request.Name, request.Version)
	} else {
		metadata, err = client.RevokeSecretVersionContext(ctx, request.Name, request.Version)
	}
	if err != nil {
		return SecretVersionView{}, err
	}
	view, err := mapSecretVersionView(metadata)
	if err != nil {
		return SecretVersionView{}, err
	}
	if view.Name != request.Name || view.Version != request.Version {
		return SecretVersionView{}, errors.New("server returned a different secret version identity")
	}
	if action == "activate" && !metadata.Active {
		return SecretVersionView{}, errors.New("server returned inactive metadata after secret activation")
	}
	if action == "revoke" && !metadata.Revoked {
		return SecretVersionView{}, errors.New("server returned non-revoked metadata after secret revocation")
	}
	return view, nil
}

func (s *SecretService) Clear() {
	if s == nil {
		return
	}
	s.mu.Lock()
	s.uploading = false
	clear(s.lifecycle)
	s.mu.Unlock()
}

func validateSecretScope(ctx context.Context, client *admin.Client, scopeType, scopeID string) error {
	switch scopeType {
	case "fleet":
		fleets, err := client.ListFleetsContext(ctx)
		if err != nil {
			return err
		}
		if !slices.Contains(fleets, scopeID) {
			return secretValidationFailure("Select an existing Fleet from the current workspace.")
		}
	case "endpoint":
		endpoints, err := client.ListEndpointsContext(ctx)
		if err != nil {
			return err
		}
		if !slices.ContainsFunc(endpoints, func(endpoint admin.Endpoint) bool { return endpoint.ID == scopeID }) {
			return secretValidationFailure("Select an existing Endpoint from the current workspace.")
		}
	default:
		return secretValidationFailure("Choose either Fleet or Endpoint as the secret scope.")
	}
	return nil
}

func validateSecretName(name string) error {
	if _, err := secretref.ParseSelected("remotr:" + name + "@active"); err != nil {
		return secretValidationFailure("Enter a valid case-sensitive Remotr secret name.")
	}
	return nil
}

func validateSecretVersion(name, version string) error {
	if _, err := secretref.ParseSelected("remotr:" + name + "@" + version); err != nil {
		return secretValidationFailure("Enter a valid secret name and positive canonical version.")
	}
	return nil
}

func mapSecretVersionView(metadata admin.SecretVersionMetadata) (SecretVersionView, error) {
	if err := validateSecretVersion(metadata.Name, metadata.Version); err != nil {
		return SecretVersionView{}, errors.New("server returned invalid secret identity metadata")
	}
	if !validPrefixedSHA256(metadata.Fingerprint) || metadata.CreatedAt.IsZero() || strings.TrimSpace(metadata.CreatedBy) == "" {
		return SecretVersionView{}, errors.New("server returned incomplete secret version metadata")
	}
	scopeType, scopeID := "", ""
	if (metadata.Fleet == "") == (metadata.EndpointID == "") {
		return SecretVersionView{}, errors.New("server returned invalid secret scope metadata")
	}
	if metadata.Fleet != "" {
		scopeType, scopeID = "fleet", metadata.Fleet
	} else {
		scopeType, scopeID = "endpoint", metadata.EndpointID
	}
	status := "inactive"
	if metadata.Revoked {
		status = "revoked"
	} else if metadata.Active && len(metadata.Rollouts) > 0 {
		status = "activation_planned"
	} else if metadata.Active {
		status = "active"
	}
	rollouts := make([]SecretRolloutView, 0, len(metadata.Rollouts))
	for _, rollout := range metadata.Rollouts {
		if rollout.Fleet == "" || rollout.ResourceAddress == "" || rollout.Purpose == "" || !rollout.Risk.Valid() || !validPrefixedSHA256(rollout.EffectiveHash) {
			return SecretVersionView{}, errors.New("server returned invalid secret rollout metadata")
		}
		if rollout.Risk.RequiresPreflight() && rollout.ChangeRequestID == "" {
			return SecretVersionView{}, errors.New("server omitted required secret rollout change control metadata")
		}
		rollouts = append(rollouts, SecretRolloutView{
			Fleet: rollout.Fleet, ResourceAddress: rollout.ResourceAddress, Purpose: rollout.Purpose,
			Risk: string(rollout.Risk), EffectiveHash: rollout.EffectiveHash, ChangeRequestID: rollout.ChangeRequestID,
		})
	}
	if metadata.Active && metadata.ActivationGeneration == 0 {
		return SecretVersionView{}, errors.New("server returned invalid secret activation generation")
	}
	if metadata.Revoked && metadata.RevokedAt == nil {
		return SecretVersionView{}, errors.New("server returned incomplete secret revocation metadata")
	}
	return SecretVersionView{
		Name: metadata.Name, Version: metadata.Version, Fingerprint: metadata.Fingerprint,
		ScopeType: scopeType, ScopeID: scopeID, Status: status,
		ActivationGeneration: metadata.ActivationGeneration,
		CreatedAt:            formatTimestamp(metadata.CreatedAt), CreatedBy: metadata.CreatedBy,
		ActivatedAt: formatOptionalSecretTimestamp(metadata.ActivatedAt), ActivatedBy: metadata.ActivatedBy,
		RevokedAt: formatOptionalSecretTimestamp(metadata.RevokedAt), RevokedBy: metadata.RevokedBy,
		ResolutionBlocked: metadata.ResolutionBlocked, EndpointCopyStatus: metadata.EndpointCopyStatus,
		Rollouts: rollouts,
	}, nil
}

func formatOptionalSecretTimestamp(value *time.Time) string {
	if value == nil {
		return ""
	}
	return formatTimestamp(*value)
}

func validPrefixedSHA256(digest string) bool {
	value, ok := strings.CutPrefix(digest, "sha256:")
	return ok && validSHA256(value)
}

func secretValidationFailure(guidance string) error {
	return &ActionFailure{Kind: ActionValidation, Message: "The secret action input is invalid.", Guidance: guidance, Retryable: false}
}

func secretConflictFailure(guidance string) error {
	return &ActionFailure{Kind: ActionConflict, Message: "The secret action is already in progress.", Guidance: guidance, Retryable: false}
}
