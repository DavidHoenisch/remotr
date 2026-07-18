package secrets

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"strconv"
	"strings"
	"time"

	"github.com/DavidHoenisch/remotr/internal/executor"
	"github.com/DavidHoenisch/remotr/internal/models"
	"github.com/DavidHoenisch/remotr/internal/secretref"
)

var (
	ErrVersionNotFound                 = errors.New("secret version not found")
	ErrVersionRevoked                  = errors.New("secret version revoked")
	ErrVersionActive                   = errors.New("active secret version cannot be deleted")
	ErrVersionReferenced               = errors.New("secret version is retained for rollback")
	ErrRecoveryAbandonmentUnauthorized = errors.New("secret recovery abandonment is unauthorized")
)

const (
	EndpointCopyRotationRequired = "rotation-or-removal-required"
	MaxOfflineRecoveryAge        = 24 * time.Hour
)

type UploadRequest struct {
	Name       string
	Fleet      string
	EndpointID string
	Material   []byte
	ActorID    string
}

type VersionMetadata struct {
	Name                 string           `json:"name"`
	Version              string           `json:"version"`
	Fingerprint          string           `json:"fingerprint"`
	Fleet                string           `json:"fleet,omitempty"`
	EndpointID           string           `json:"endpointId,omitempty"`
	Active               bool             `json:"active"`
	ActivationGeneration uint64           `json:"activationGeneration,omitempty"`
	CreatedAt            time.Time        `json:"createdAt"`
	CreatedBy            string           `json:"createdBy"`
	ActivatedAt          *time.Time       `json:"activatedAt,omitempty"`
	ActivatedBy          string           `json:"activatedBy,omitempty"`
	Revoked              bool             `json:"revoked"`
	RevokedAt            *time.Time       `json:"revokedAt,omitempty"`
	RevokedBy            string           `json:"revokedBy,omitempty"`
	ResolutionBlocked    bool             `json:"resolutionBlocked"`
	EndpointCopyStatus   string           `json:"endpointCopyStatus,omitempty"`
	Rollouts             []RolloutBinding `json:"rollouts,omitempty"`
}

type StoredVersion struct {
	Record               EncryptedRecord
	CreatedAt            time.Time
	CreatedBy            string
	Active               bool
	ActivationGeneration uint64
	ActivatedAt          *time.Time
	ActivatedBy          string
	RevokedAt            *time.Time
	RevokedBy            string
	Rollouts             []RolloutBinding
}

type VersionRepository interface {
	AllocateVersion(context.Context, string) (string, error)
	CreateVersion(context.Context, StoredVersion) error
	GetExactVersion(context.Context, string, string) (StoredVersion, error)
	GetActiveVersion(context.Context, string) (StoredVersion, error)
	ListVersions(context.Context, string) ([]StoredVersion, error)
	ActivationGeneration(context.Context, string) (uint64, error)
	ActivateVersion(context.Context, string, string, uint64, string, []RolloutBinding) (StoredVersion, error)
	RevokeVersion(context.Context, string, string, string) (StoredVersion, error)
	CreateRollbackReference(context.Context, StoredRollbackReference) error
	ListActiveRollbackReferences(context.Context, string, string, time.Time) ([]StoredRollbackReference, error)
	AbandonRollbackReferences(context.Context, string, string, string, time.Time) error
	DeleteVersion(context.Context, string, string, time.Time) error
}

type RollbackReferenceStatus string

const (
	RollbackReferenceArmed     RollbackReferenceStatus = "armed"
	RollbackReferenceCompleted RollbackReferenceStatus = "completed"
	RollbackReferenceAbandoned RollbackReferenceStatus = "abandoned"
)

// RollbackReferenceMetadata is safe server-side recovery metadata. Reference
// and fingerprint identify the protected prior version without its bytes.
type RollbackReferenceMetadata struct {
	ID              string                  `json:"id"`
	Reference       string                  `json:"reference"`
	Fingerprint     string                  `json:"fingerprint"`
	ResourceAddress string                  `json:"resourceAddress"`
	ArtifactDigest  string                  `json:"artifactDigest"`
	Attempt         int                     `json:"attempt"`
	CreatedAt       time.Time               `json:"createdAt"`
	ExpiresAt       time.Time               `json:"expiresAt"`
	Status          RollbackReferenceStatus `json:"status"`
	AbandonedAt     *time.Time              `json:"abandonedAt,omitempty"`
	AbandonedBy     string                  `json:"abandonedBy,omitempty"`
}

// ClassifiedMetadata returns the only generic serialization shape for the
// server-side reference to a protected rollback version.
func (metadata RollbackReferenceMetadata) ClassifiedMetadata() (executor.SafeSummary, error) {
	fields := []executor.SafeField{
		{Path: "artifact_digest", Sensitivity: executor.SafeSensitiveMetadata, Projection: executor.SafeFingerprint, Text: metadata.ArtifactDigest},
		{Path: "attempt", Sensitivity: executor.SafeSensitiveMetadata, Projection: executor.SafeCount, Count: secretIntPointer(metadata.Attempt)},
		{Path: "created_at", Sensitivity: executor.SafeSensitiveMetadata, Projection: executor.SafeMetadata, Text: metadata.CreatedAt.UTC().Format(time.RFC3339Nano)},
		{Path: "expires_at", Sensitivity: executor.SafeSensitiveMetadata, Projection: executor.SafeMetadata, Text: metadata.ExpiresAt.UTC().Format(time.RFC3339Nano)},
		{Path: "fingerprint", Sensitivity: executor.SafeSensitiveMetadata, Projection: executor.SafeFingerprint, Text: metadata.Fingerprint},
		{Path: "id", Sensitivity: executor.SafeSensitiveMetadata, Projection: executor.SafeMetadata, Text: metadata.ID},
		{Path: "reference", Sensitivity: executor.SafeSecret, Projection: executor.SafeReference, Text: metadata.Reference},
		{Path: "resource_address", Sensitivity: executor.SafePublic, Projection: executor.SafeValue, Text: metadata.ResourceAddress},
		{Path: "status", Sensitivity: executor.SafePublic, Projection: executor.SafeValue, Text: string(metadata.Status)},
	}
	if metadata.AbandonedAt != nil {
		fields = append(fields, executor.SafeField{
			Path: "abandoned_at", Sensitivity: executor.SafeSensitiveMetadata, Projection: executor.SafeMetadata,
			Text: metadata.AbandonedAt.UTC().Format(time.RFC3339Nano),
		})
	}
	if metadata.AbandonedBy != "" {
		fields = append(fields, executor.SafeField{
			Path: "abandoned_by", Sensitivity: executor.SafeSensitiveMetadata, Projection: executor.SafeMetadata, Text: metadata.AbandonedBy,
		})
	}
	return executor.NewSafeSummary(fields)
}

// MarshalJSON prevents rollback-reference metadata from being serialized as
// an unclassified application struct.
func (metadata RollbackReferenceMetadata) MarshalJSON() ([]byte, error) {
	classified, err := metadata.ClassifiedMetadata()
	if err != nil {
		return nil, err
	}
	return json.Marshal(classified)
}

func secretIntPointer(value int) *int { return &value }

type StoredRollbackReference struct {
	RollbackReferenceMetadata
	Name    string
	Version string
}

type RollbackReferenceRequest struct {
	Name            string
	Version         string
	ResourceAddress string
	ArtifactDigest  string
	Attempt         int
	ExpiresAt       time.Time
}

type DeleteVersionRequest struct {
	Name            string
	Version         string
	ActorID         string
	AbandonRecovery bool
}

type RecoveryAbandonmentRequest struct {
	ActorID    string
	Name       string
	Version    string
	References []RollbackReferenceMetadata
}

type RecoveryAbandonmentAuthorizer interface {
	AuthorizeRecoveryAbandonment(context.Context, RecoveryAbandonmentRequest) bool
}

type RegistryOption func(*RegistryService)

func WithRecoveryAbandonmentAuthorizer(authorizer RecoveryAbandonmentAuthorizer) RegistryOption {
	return func(service *RegistryService) { service.abandonAuthorizer = authorizer }
}

type ActivationUse struct {
	Fleet           string
	ResourceAddress string
	Purpose         string
	Risk            models.RiskClass
	Provider        string
	ReleaseRef      string
	ArtifactDigest  string
	EndpointIDs     []string
	EffectiveHash   string
}

type ActivationRequest struct {
	Name    string
	Version string
	ActorID string
	Uses    []ActivationUse
}

type RevokeRequest struct {
	Name    string
	Version string
	ActorID string
}

type ActivationPlan struct {
	Name       string
	Version    string
	Generation uint64
	ActorID    string
	Uses       []ActivationUse
}

type RolloutBinding struct {
	Fleet           string           `json:"fleet"`
	ResourceAddress string           `json:"resourceAddress"`
	Purpose         string           `json:"purpose"`
	Risk            models.RiskClass `json:"risk"`
	EffectiveHash   string           `json:"effectiveHash"`
	ChangeRequestID string           `json:"changeRequestId,omitempty"`
}

type ActivationPlanner interface {
	CreateActivationRollouts(context.Context, ActivationPlan) ([]RolloutBinding, error)
}

type RolloutGate interface {
	RolloutActive(context.Context, string) bool
}

type RegistryService struct {
	repository        VersionRepository
	envelope          *Envelope
	planner           ActivationPlanner
	gate              RolloutGate
	abandonAuthorizer RecoveryAbandonmentAuthorizer
	now               func() time.Time
	random            io.Reader
}

func NewRegistryService(repository VersionRepository, envelope *Envelope, planner ActivationPlanner, gate RolloutGate, options ...RegistryOption) (*RegistryService, error) {
	if repository == nil || envelope == nil {
		return nil, fmt.Errorf("secret version repository and envelope are required")
	}
	service := &RegistryService{repository: repository, envelope: envelope, planner: planner, gate: gate, now: func() time.Time { return time.Now().UTC() }, random: rand.Reader}
	for _, option := range options {
		if option != nil {
			option(service)
		}
	}
	return service, nil
}

func (s *RegistryService) RetainRollbackReference(ctx context.Context, request RollbackReferenceRequest) (RollbackReferenceMetadata, error) {
	if _, err := secretref.ParseSelected("remotr:" + request.Name + "@" + request.Version); err != nil {
		return RollbackReferenceMetadata{}, fmt.Errorf("rollback secret reference: %w", err)
	}
	if _, err := parseVersion(request.Version); err != nil {
		return RollbackReferenceMetadata{}, err
	}
	if strings.TrimSpace(request.ResourceAddress) == "" || request.ResourceAddress != strings.TrimSpace(request.ResourceAddress) || !strings.Contains(request.ResourceAddress, "/") {
		return RollbackReferenceMetadata{}, fmt.Errorf("rollback resource address is required")
	}
	if strings.TrimSpace(request.ArtifactDigest) == "" || request.ArtifactDigest != strings.TrimSpace(request.ArtifactDigest) || len(request.ArtifactDigest) > 256 {
		return RollbackReferenceMetadata{}, fmt.Errorf("rollback artifact digest is invalid")
	}
	if request.Attempt <= 0 {
		return RollbackReferenceMetadata{}, fmt.Errorf("rollback attempt must be positive")
	}
	now := s.now().UTC()
	expiresAt := request.ExpiresAt.UTC()
	if !expiresAt.After(now) || expiresAt.After(now.Add(MaxOfflineRecoveryAge)) {
		return RollbackReferenceMetadata{}, fmt.Errorf("rollback reference expiry must be within %s", MaxOfflineRecoveryAge)
	}
	version, err := s.repository.GetExactVersion(ctx, request.Name, request.Version)
	if err != nil {
		return RollbackReferenceMetadata{}, err
	}
	if version.RevokedAt != nil {
		return RollbackReferenceMetadata{}, ErrVersionRevoked
	}
	idBytes := make([]byte, 16)
	if _, err := io.ReadFull(s.random, idBytes); err != nil {
		return RollbackReferenceMetadata{}, fmt.Errorf("create rollback reference identifier: %w", err)
	}
	metadata := RollbackReferenceMetadata{
		ID: "rollback-" + hex.EncodeToString(idBytes), Reference: "remotr:" + request.Name + "@" + request.Version,
		Fingerprint: version.Record.Fingerprint, ResourceAddress: request.ResourceAddress, ArtifactDigest: request.ArtifactDigest,
		Attempt: request.Attempt, CreatedAt: now, ExpiresAt: expiresAt, Status: RollbackReferenceArmed,
	}
	if err := s.repository.CreateRollbackReference(ctx, StoredRollbackReference{RollbackReferenceMetadata: metadata, Name: request.Name, Version: request.Version}); err != nil {
		return RollbackReferenceMetadata{}, err
	}
	return metadata, nil
}

func (s *RegistryService) DeleteVersion(ctx context.Context, request DeleteVersionRequest) error {
	if strings.TrimSpace(request.ActorID) == "" {
		return fmt.Errorf("secret deletion actor is required")
	}
	if _, err := parseVersion(request.Version); err != nil {
		return err
	}
	stored, err := s.repository.GetExactVersion(ctx, request.Name, request.Version)
	if err != nil {
		return err
	}
	if stored.Active {
		return ErrVersionActive
	}
	now := s.now().UTC()
	references, err := s.repository.ListActiveRollbackReferences(ctx, request.Name, request.Version, now)
	if err != nil {
		return err
	}
	if len(references) > 0 {
		if !request.AbandonRecovery {
			return ErrVersionReferenced
		}
		metadata := make([]RollbackReferenceMetadata, len(references))
		for i := range references {
			metadata[i] = references[i].RollbackReferenceMetadata
		}
		if s.abandonAuthorizer == nil || !s.abandonAuthorizer.AuthorizeRecoveryAbandonment(ctx, RecoveryAbandonmentRequest{ActorID: request.ActorID, Name: request.Name, Version: request.Version, References: metadata}) {
			return ErrRecoveryAbandonmentUnauthorized
		}
		if err := s.repository.AbandonRollbackReferences(ctx, request.Name, request.Version, request.ActorID, now); err != nil {
			return err
		}
	}
	return s.repository.DeleteVersion(ctx, request.Name, request.Version, now)
}

func (s *RegistryService) Upload(ctx context.Context, request UploadRequest) (VersionMetadata, error) {
	if _, err := secretref.ParseSelected("remotr:" + request.Name + "@active"); err != nil {
		return VersionMetadata{}, fmt.Errorf("secret name: %w", err)
	}
	if (strings.TrimSpace(request.Fleet) == "") == (strings.TrimSpace(request.EndpointID) == "") {
		return VersionMetadata{}, fmt.Errorf("exactly one secret fleet or endpoint scope is required")
	}
	if strings.TrimSpace(request.ActorID) == "" {
		return VersionMetadata{}, fmt.Errorf("secret upload actor is required")
	}
	version, err := s.repository.AllocateVersion(ctx, request.Name)
	if err != nil {
		return VersionMetadata{}, err
	}
	record, err := s.envelope.EncryptContext(ctx, ScopeMetadata{Name: request.Name, Version: version, Fleet: request.Fleet, EndpointID: request.EndpointID}, request.Material)
	if err != nil {
		return VersionMetadata{}, err
	}
	stored := StoredVersion{Record: record, CreatedAt: s.now(), CreatedBy: request.ActorID}
	if err := s.repository.CreateVersion(ctx, stored); err != nil {
		return VersionMetadata{}, err
	}
	return metadataFromStored(stored), nil
}

func (s *RegistryService) ListMetadata(ctx context.Context, name string) ([]VersionMetadata, error) {
	versions, err := s.repository.ListVersions(ctx, name)
	if err != nil {
		return nil, err
	}
	out := make([]VersionMetadata, 0, len(versions))
	for _, version := range versions {
		out = append(out, metadataFromStored(version))
	}
	return out, nil
}

func (s *RegistryService) GetMetadata(ctx context.Context, name, version string) (VersionMetadata, error) {
	stored, err := s.repository.GetExactVersion(ctx, name, version)
	if err != nil {
		return VersionMetadata{}, err
	}
	return metadataFromStored(stored), nil
}

func (s *RegistryService) Activate(ctx context.Context, request ActivationRequest) (VersionMetadata, error) {
	if strings.TrimSpace(request.ActorID) == "" {
		return VersionMetadata{}, fmt.Errorf("secret activation actor is required")
	}
	if _, err := parseVersion(request.Version); err != nil {
		return VersionMetadata{}, err
	}
	stored, err := s.repository.GetExactVersion(ctx, request.Name, request.Version)
	if err != nil {
		return VersionMetadata{}, err
	}
	if stored.RevokedAt != nil {
		return VersionMetadata{}, ErrVersionRevoked
	}
	currentGeneration, err := s.repository.ActivationGeneration(ctx, request.Name)
	if err != nil {
		return VersionMetadata{}, err
	}
	generation := currentGeneration + 1
	uses := make([]ActivationUse, len(request.Uses))
	copy(uses, request.Uses)
	requiresPlanner := false
	for i := range uses {
		hash, err := EffectiveReferenceHash(ProviderRemotr, request.Name, request.Version, generation, uses[i].Purpose)
		if err != nil {
			return VersionMetadata{}, err
		}
		uses[i].EffectiveHash = hash
		if uses[i].Risk.RequiresPreflight() {
			requiresPlanner = true
		}
	}
	var rollouts []RolloutBinding
	if len(uses) > 0 {
		if requiresPlanner && s.planner == nil {
			return VersionMetadata{}, fmt.Errorf("high-risk secret activation requires a rollout planner")
		}
		if s.planner != nil {
			rollouts, err = s.planner.CreateActivationRollouts(ctx, ActivationPlan{Name: request.Name, Version: request.Version, Generation: generation, ActorID: request.ActorID, Uses: uses})
			if err != nil {
				return VersionMetadata{}, err
			}
		} else {
			rollouts = make([]RolloutBinding, 0, len(uses))
			for _, use := range uses {
				rollouts = append(rollouts, RolloutBinding{Fleet: use.Fleet, ResourceAddress: use.ResourceAddress, Purpose: use.Purpose, Risk: use.Risk, EffectiveHash: use.EffectiveHash})
			}
		}
		if err := validateActivationRollouts(uses, rollouts); err != nil {
			return VersionMetadata{}, err
		}
	}
	activated, err := s.repository.ActivateVersion(ctx, request.Name, request.Version, generation, request.ActorID, rollouts)
	if err != nil {
		return VersionMetadata{}, err
	}
	return metadataFromStored(activated), nil
}

func (s *RegistryService) Revoke(ctx context.Context, request RevokeRequest) (VersionMetadata, error) {
	if strings.TrimSpace(request.ActorID) == "" {
		return VersionMetadata{}, fmt.Errorf("secret revocation actor is required")
	}
	if _, err := parseVersion(request.Version); err != nil {
		return VersionMetadata{}, err
	}
	revoked, err := s.repository.RevokeVersion(ctx, request.Name, request.Version, request.ActorID)
	if err != nil {
		return VersionMetadata{}, err
	}
	return metadataFromStored(revoked), nil
}

func (s *RegistryService) Resolve(ctx context.Context, request ResolveRequest) (Resolved, error) {
	reference, err := secretref.ParseSelected(request.Reference)
	if err != nil || reference.Provider != ProviderRemotr {
		return Resolved{}, ErrInvalidReference
	}
	var stored StoredVersion
	if reference.FollowsActive() {
		stored, err = s.repository.GetActiveVersion(ctx, reference.Name)
	} else {
		stored, err = s.repository.GetExactVersion(ctx, reference.Name, reference.Selector)
	}
	if err != nil {
		return Resolved{}, err
	}
	if stored.RevokedAt != nil {
		return Resolved{}, ErrVersionRevoked
	}
	if stored.Record.Scope.Fleet != "" && stored.Record.Scope.Fleet != request.Fleet {
		return Resolved{}, ErrUnauthorized
	}
	if stored.Record.Scope.EndpointID != "" && stored.Record.Scope.EndpointID != request.EndpointID {
		return Resolved{}, ErrUnauthorized
	}
	if reference.FollowsActive() {
		for _, rollout := range stored.Rollouts {
			if rollout.Fleet != request.Fleet || rollout.ResourceAddress != request.ResourceAddress || rollout.Purpose != request.Purpose || rollout.ChangeRequestID == "" {
				continue
			}
			if s.gate == nil || !s.gate.RolloutActive(ctx, rollout.ChangeRequestID) {
				return Resolved{}, ErrUnauthorized
			}
		}
	}
	material, err := s.envelope.DecryptContext(ctx, stored.Record)
	if err != nil {
		return Resolved{}, err
	}
	return Resolved{
		Provider: ProviderRemotr, Version: stored.Record.Scope.Version,
		ActivationGeneration: stored.ActivationGeneration,
		Fingerprint:          stored.Record.Fingerprint, Material: material,
	}, nil
}

func EffectiveReferenceHash(provider, name, version string, activationGeneration uint64, purpose string) (string, error) {
	if provider == "" || name == "" || version == "" || purpose == "" {
		return "", fmt.Errorf("effective secret hash fields are required")
	}
	canonical, err := json.Marshal(struct {
		Provider             string `json:"provider"`
		Name                 string `json:"name"`
		Version              string `json:"version"`
		ActivationGeneration uint64 `json:"activationGeneration"`
		Purpose              string `json:"purpose"`
	}{provider, name, version, activationGeneration, purpose})
	if err != nil {
		return "", err
	}
	sum := sha256.Sum256(canonical)
	return "sha256:" + hex.EncodeToString(sum[:]), nil
}

func validateActivationRollouts(uses []ActivationUse, rollouts []RolloutBinding) error {
	for _, use := range uses {
		found := false
		for _, rollout := range rollouts {
			if rollout.Fleet == use.Fleet && rollout.ResourceAddress == use.ResourceAddress && rollout.Purpose == use.Purpose && rollout.EffectiveHash == use.EffectiveHash {
				found = true
				if use.Risk.RequiresPreflight() && rollout.ChangeRequestID == "" {
					return fmt.Errorf("high-risk activation rollout lacks a Change request")
				}
				break
			}
		}
		if !found {
			return fmt.Errorf("activation rollout omitted resource %q", use.ResourceAddress)
		}
	}
	return nil
}

func metadataFromStored(stored StoredVersion) VersionMetadata {
	metadata := VersionMetadata{
		Name: stored.Record.Scope.Name, Version: stored.Record.Scope.Version, Fingerprint: stored.Record.Fingerprint,
		Fleet: stored.Record.Scope.Fleet, EndpointID: stored.Record.Scope.EndpointID,
		Active: stored.Active, ActivationGeneration: stored.ActivationGeneration,
		CreatedAt: stored.CreatedAt, CreatedBy: stored.CreatedBy, ActivatedAt: stored.ActivatedAt, ActivatedBy: stored.ActivatedBy,
		Revoked: stored.RevokedAt != nil, RevokedAt: stored.RevokedAt, RevokedBy: stored.RevokedBy,
		ResolutionBlocked: stored.RevokedAt != nil, Rollouts: append([]RolloutBinding(nil), stored.Rollouts...),
	}
	if metadata.Revoked {
		metadata.EndpointCopyStatus = EndpointCopyRotationRequired
	}
	return metadata
}

func parseVersion(value string) (int64, error) {
	version, err := strconv.ParseInt(value, 10, 64)
	if err != nil || version <= 0 || strconv.FormatInt(version, 10) != value {
		return 0, fmt.Errorf("invalid secret version")
	}
	return version, nil
}
