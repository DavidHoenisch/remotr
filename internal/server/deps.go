package server

import (
	"context"
	"encoding/json"

	"github.com/DavidHoenisch/remotr/internal/audit"
	"github.com/DavidHoenisch/remotr/internal/diagnostics"
	"github.com/DavidHoenisch/remotr/internal/executor"
	"github.com/DavidHoenisch/remotr/internal/rbac"
	"github.com/DavidHoenisch/remotr/internal/registry"
)

// FleetSettings reads per-fleet server registry settings.
type FleetSettings interface {
	RemediationPolicy(ctx context.Context, fleet string) (string, error)
}

// SyncTelemetry persists agent-reported sync telemetry.
type SyncTelemetry interface {
	RecordEndpointCheckIn(ctx context.Context, endpointID, releaseRef, digest string) error
	UpsertEndpointLabels(ctx context.Context, endpointID string, labels map[string]string) error
	UpsertEndpointSystemInfo(ctx context.Context, endpointID, digest string, infoJSON []byte) error
	InsertDriftReport(ctx context.Context, endpointID, releaseRef, digest string, reportJSON []byte) error
	InsertApplyFailure(ctx context.Context, endpointID, releaseRef, resourceAddress, message string) error
	UpdateAgentUpgradeReport(ctx context.Context, endpointID, reportedVersion, phase, message string, clearDesired bool) error
	UpdateEndpointUsernames(ctx context.Context, endpointID string, usernames []string) error
	InsertFirewallAuditReport(ctx context.Context, endpointID, digest string, reportJSON []byte) error
}

// ReleaseRefSource resolves the global release ref for sync responses.
type ReleaseRefSource interface {
	ReleaseRef(ctx context.Context) string
}

// StateReports reads agent compliance evidence for admin queries.
type StateReports interface {
	GetEndpointStateReport(ctx context.Context, endpointID string) (registry.StateReport, bool, error)
	ListFleetStateReports(ctx context.Context, fleet string) (registry.FleetStateReport, error)
	GetEndpointFirewallAudit(ctx context.Context, endpointID string) (registry.FirewallAuditReport, bool, error)
}

// AuditLog persists and queries durable API audit events.
type AuditLog interface {
	RecordAuditEvent(ctx context.Context, event audit.Event) error
	ListAuditEvents(ctx context.Context, filter audit.ListFilter) (audit.Page, error)
	EnsureAuditExportPathKey(ctx context.Context) (string, error)
}

// DiagnosticsStore manages endpoint diagnostic collection jobs.
type DiagnosticsStore interface {
	CreateDiagnosticRequest(ctx context.Context, endpointID, requestedBy string, spec diagnostics.Spec) (diagnostics.Request, error)
	GetDiagnosticRequest(ctx context.Context, requestID string) (diagnostics.Request, bool, error)
	PendingDiagnosticForEndpoint(ctx context.Context, endpointID string) (diagnostics.Request, bool, error)
	MarkDiagnosticDispatched(ctx context.Context, requestID string) error
	MarkDiagnosticRunning(ctx context.Context, requestID string) error
	CompleteDiagnosticRequest(ctx context.Context, result diagnostics.ResultPayload) error
	ExpireDiagnosticRequests(ctx context.Context) error
	DeleteExpiredDiagnosticRequests(ctx context.Context) ([]diagnostics.Request, error)
}

// RBAC authorizes operator requests and manages roles and assignments.
type RBAC interface {
	EnsureBuiltInRoles(ctx context.Context) error
	Authorize(ctx context.Context, operatorID, method, path string) (bool, error)
	ListRBACRoles(ctx context.Context) ([]rbac.Role, error)
	GetRBACRole(ctx context.Context, name string) (rbac.Role, error)
	CreateRBACRole(ctx context.Context, name, description string) error
	DeleteRBACRole(ctx context.Context, name string) error
	AddRBACRule(ctx context.Context, roleName string, rule rbac.Rule) (rbac.Rule, error)
	RemoveRBACRule(ctx context.Context, roleName, ruleID string) error
	ListOperators(ctx context.Context) ([]registry.Operator, error)
	SetOperatorRoles(ctx context.Context, operatorID string, roles []string) error
	OperatorRoles(ctx context.Context, operatorID string) ([]string, error)
	RegisterOperator(ctx context.Context, operatorID, fingerprint string, roles []string) error
}

type driftReportPayload struct {
	Digest string          `json:"digest,omitempty"`
	Report json.RawMessage `json:"report,omitempty"`
}

type systemInfoPayload struct {
	Digest string          `json:"digest,omitempty"`
	Report json.RawMessage `json:"report,omitempty"`
}

type applyFailurePayload struct {
	ResourceAddress string              `json:"resourceAddress"`
	Message         string              `json:"message,omitempty"`
	Failure         *executor.SafeError `json:"failure,omitempty"`
}

type firewallAuditPayload struct {
	Digest string          `json:"digest,omitempty"`
	Report json.RawMessage `json:"report,omitempty"`
}
