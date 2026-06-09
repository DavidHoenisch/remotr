package audit

import "time"

const (
	ActorOperator   = "operator"
	ActorEndpoint   = "endpoint"
	ActorAnonymous  = "anonymous"
	ActorSystem     = "system"

	ActionAPIRequest              = "api.request"
	ActionAdminBootstrap          = "admin.bootstrap"
	ActionAdminEnrollTokenCreate  = "admin.enroll_token.create"
	ActionAdminDeploymentCreate   = "admin.deployment_token.create"
	ActionAdminDeploymentRevoke   = "admin.deployment_token.revoke"
	ActionAdminEndpointDelete     = "admin.endpoint.delete"
	ActionAdminEndpointUpgrade    = "admin.endpoint.agent_upgrade"
	ActionAdminFleetUpgrade       = "admin.fleet.agent_upgrade"
	ActionAdminGitSync            = "admin.git_sync"
	ActionAdminOperatorCreate     = "admin.operator_credential.create"
	ActionAuthzDenied             = "authz.denied"
	ActionRBACRoleCreate          = "rbac.role.create"
	ActionRBACRoleDelete          = "rbac.role.delete"
	ActionRBACRuleCreate          = "rbac.rule.create"
	ActionRBACRuleDelete          = "rbac.rule.delete"
	ActionRBACOperatorRolesSet    = "rbac.operator.roles.set"
	ActionAgentEnroll             = "agent.enroll"
	ActionAgentSync               = "agent.sync"
	ActionWebhookGit              = "webhook.git"
)

// Event is a durable audit record for API activity.
type Event struct {
	ID               string
	OccurredAt       time.Time
	RequestID        string
	ActorType        string
	ActorID          string
	ActorFingerprint string
	Action           string
	Method           string
	Path             string
	StatusCode       int
	ResourceType     string
	ResourceID       string
	ClientIP         string
	Details          map[string]any
}

// ListFilter selects audit events for admin review or SIEM export.
type ListFilter struct {
	Since     time.Time
	Until     time.Time
	Action    string
	ActorType string
	Limit     int
	Cursor    string
}

// Page is a paginated audit event result set.
type Page struct {
	Events     []Event
	NextCursor string
}
