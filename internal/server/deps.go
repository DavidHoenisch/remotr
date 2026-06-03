package server

import (
	"context"
	"encoding/json"
)

// FleetSettings reads per-fleet server registry settings.
type FleetSettings interface {
	RemediationPolicy(ctx context.Context, fleet string) (string, error)
}

// SyncTelemetry persists agent-reported sync telemetry.
type SyncTelemetry interface {
	RecordEndpointCheckIn(ctx context.Context, endpointID, releaseRef, digest string) error
	UpsertEndpointLabels(ctx context.Context, endpointID string, labels map[string]string) error
	InsertDriftReport(ctx context.Context, endpointID, releaseRef, digest string, reportJSON []byte) error
	InsertApplyFailure(ctx context.Context, endpointID, releaseRef, resourceAddress, message string) error
	UpdateAgentUpgradeReport(ctx context.Context, endpointID, reportedVersion, phase, message string, clearDesired bool) error
}

// ReleaseRefSource resolves the global release ref for sync responses.
type ReleaseRefSource interface {
	ReleaseRef(ctx context.Context) string
}

type driftReportPayload struct {
	Digest string          `json:"digest,omitempty"`
	Report json.RawMessage `json:"report,omitempty"`
}

type applyFailurePayload struct {
	ResourceAddress string `json:"resourceAddress"`
	Message         string `json:"message"`
}
