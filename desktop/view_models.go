package main

type OperatorView struct {
	OperatorID string   `json:"operatorId"`
	Roles      []string `json:"roles"`
}

type WorkspaceView struct {
	Operator           OperatorView           `json:"operator"`
	Sections           WorkspaceSections      `json:"sections"`
	Endpoints          []EndpointRow          `json:"endpoints"`
	Fleets             []FleetSummary         `json:"fleets"`
	StateEvidence      []StateEvidence        `json:"stateEvidence"`
	ChangeRequests     []ChangeRequestSummary `json:"changeRequests"`
	Activity           []ActivityEvent        `json:"activity"`
	ActivityNextCursor string                 `json:"activityNextCursor"`
}

type WorkspaceSections struct {
	Fleets         SectionResult `json:"fleets"`
	Endpoints      SectionResult `json:"endpoints"`
	State          SectionResult `json:"state"`
	ChangeRequests SectionResult `json:"changeRequests"`
	Activity       SectionResult `json:"activity"`
}

type EndpointDetailView struct {
	Header             EndpointRow            `json:"header"`
	Sections           EndpointDetailSections `json:"sections"`
	State              StateEvidence          `json:"state"`
	StateTruncated     bool                   `json:"stateTruncated"`
	Schedules          []ScheduleEvidence     `json:"schedules"`
	SchedulesTruncated bool                   `json:"schedulesTruncated"`
	Firewall           []FirewallEvidence     `json:"firewall"`
	FirewallTruncated  bool                   `json:"firewallTruncated"`
	System             SystemEvidence         `json:"system"`
}

type EndpointDetailSections struct {
	Overview  SectionResult `json:"overview"`
	State     SectionResult `json:"state"`
	Schedules SectionResult `json:"schedules"`
	Firewall  SectionResult `json:"firewall"`
	System    SectionResult `json:"system"`
}

type ScheduleEvidence struct {
	Name             string `json:"name"`
	Schedule         string `json:"schedule"`
	Applicable       bool   `json:"applicable"`
	LastStatus       string `json:"lastStatus"`
	LastMessage      string `json:"lastMessage"`
	LastScheduledFor string `json:"lastScheduledFor"`
	LastCompletedAt  string `json:"lastCompletedAt"`
}

type FirewallEvidence struct {
	Timestamp string   `json:"timestamp"`
	RuleName  string   `json:"ruleName"`
	Action    string   `json:"action"`
	Protocol  string   `json:"protocol"`
	Ports     []int    `json:"ports"`
	Sources   []string `json:"sources"`
	Backend   string   `json:"backend"`
	WouldHave string   `json:"wouldHave"`
	Enforced  bool     `json:"enforced"`
}

type SystemEvidence struct {
	Hostname   string `json:"hostname"`
	OS         string `json:"os"`
	Kernel     string `json:"kernel"`
	CPU        string `json:"cpu"`
	CPUCores   string `json:"cpuCores"`
	Memory     string `json:"memory"`
	Digest     string `json:"digest"`
	ReportedAt string `json:"reportedAt"`
}

type SectionState string

const (
	SectionLoading     SectionState = "loading"
	SectionReady       SectionState = "ready"
	SectionEmpty       SectionState = "empty"
	SectionPartial     SectionState = "partial"
	SectionStale       SectionState = "stale"
	SectionUnavailable SectionState = "unavailable"
	SectionFailed      SectionState = "failed"
)

type ErrorKind string

const (
	ErrorAuthorization ErrorKind = "authorization"
	ErrorConnection    ErrorKind = "connection"
	ErrorValidation    ErrorKind = "validation"
	ErrorUnavailable   ErrorKind = "unavailable"
	ErrorUnexpected    ErrorKind = "unexpected"
)

type ClassifiedError struct {
	Kind     ErrorKind `json:"kind"`
	Message  string    `json:"message"`
	Guidance string    `json:"guidance"`
}

func (e *ClassifiedError) Error() string {
	return e.Message
}

type SnapshotTimestamps struct {
	LoadedAt   string  `json:"loadedAt"`
	ObservedAt *string `json:"observedAt"`
	FailedAt   *string `json:"failedAt"`
}

type SectionResult struct {
	State    SectionState       `json:"state"`
	Snapshot SnapshotTimestamps `json:"snapshot"`
	Error    *ClassifiedError   `json:"error"`
}

type ComplianceStatus string

const (
	ComplianceCompliant   ComplianceStatus = "compliant"
	ComplianceDrifted     ComplianceStatus = "drifted"
	ComplianceUnsupported ComplianceStatus = "unsupported"
	ComplianceCheckFailed ComplianceStatus = "check_failed"
	ComplianceDeferred    ComplianceStatus = "deferred"
	ComplianceApplyFailed ComplianceStatus = "apply_failed"
	ComplianceNotReported ComplianceStatus = "not_reported"
)

type FreshnessStatus string

const (
	FreshnessRecent        FreshnessStatus = "recent"
	FreshnessStale         FreshnessStatus = "stale"
	FreshnessNeverReported FreshnessStatus = "never_reported"
)

type LabelView struct {
	Key   string `json:"key"`
	Value string `json:"value"`
}

type EndpointRow struct {
	EndpointID           string           `json:"endpointId"`
	Fleet                string           `json:"fleet"`
	Usernames            []string         `json:"usernames"`
	Compliance           ComplianceStatus `json:"compliance"`
	Freshness            FreshnessStatus  `json:"freshness"`
	DesiredAgentVersion  string           `json:"desiredAgentVersion"`
	ReportedAgentVersion string           `json:"reportedAgentVersion"`
	ReleaseRef           string           `json:"releaseRef"`
	Labels               []LabelView      `json:"labels"`
	EvidenceAt           *string          `json:"evidenceAt"`
}

type StatusCount struct {
	Status string `json:"status"`
	Count  int    `json:"count"`
}

type FleetSummary struct {
	Fleet         string        `json:"fleet"`
	EndpointCount int           `json:"endpointCount"`
	Compliance    []StatusCount `json:"compliance"`
	Freshness     []StatusCount `json:"freshness"`
	AgentVersions []StatusCount `json:"agentVersions"`
}

type StateEvidence struct {
	EndpointID string              `json:"endpointId"`
	ReleaseRef string              `json:"releaseRef"`
	Digest     string              `json:"digest"`
	Status     ComplianceStatus    `json:"status"`
	ReportedAt string              `json:"reportedAt"`
	Items      []StateEvidenceItem `json:"items"`
}

type StateEvidenceItem struct {
	Address             string                   `json:"address"`
	Name                string                   `json:"name"`
	Description         string                   `json:"description"`
	Provider            string                   `json:"provider"`
	Status              ComplianceStatus         `json:"status"`
	ReasonCode          string                   `json:"reasonCode"`
	DesiredSummary      string                   `json:"desiredSummary"`
	ObservedSummary     string                   `json:"observedSummary"`
	Subresults          []StateEvidenceSubresult `json:"subresults"`
	SubresultsTruncated bool                     `json:"subresultsTruncated"`
}

type StateEvidenceSubresult struct {
	Target          string           `json:"target"`
	Status          ComplianceStatus `json:"status"`
	ReasonCode      string           `json:"reasonCode"`
	DesiredSummary  string           `json:"desiredSummary"`
	ObservedSummary string           `json:"observedSummary"`
}

type ChangeRequestSummary struct {
	ChangeRequestID   string `json:"changeRequestId"`
	Fleet             string `json:"fleet"`
	ReleaseRef        string `json:"releaseRef"`
	Risk              string `json:"risk"`
	Lifecycle         string `json:"lifecycle"`
	TargetCount       int    `json:"targetCount"`
	RequiredApprovals int    `json:"requiredApprovals"`
	ApprovalCount     int    `json:"approvalCount"`
	UpdatedAt         string `json:"updatedAt"`
}

type ActivityEvent struct {
	EventID      string           `json:"eventId"`
	OccurredAt   string           `json:"occurredAt"`
	Actor        string           `json:"actor"`
	Action       string           `json:"action"`
	ResourceType string           `json:"resourceType"`
	ResourceID   string           `json:"resourceId"`
	Status       string           `json:"status"`
	RequestID    string           `json:"requestId"`
	Details      []ActivityDetail `json:"details"`
}

type ActivityDetail struct {
	Key   string `json:"key"`
	Value string `json:"value"`
}

type ActionStatus string

const (
	ActionAccepted ActionStatus = "accepted"
)

type ActionResult struct {
	Action     string       `json:"action"`
	Target     string       `json:"target"`
	Status     ActionStatus `json:"status"`
	Message    string       `json:"message"`
	RequestID  string       `json:"requestId"`
	AcceptedAt string       `json:"acceptedAt"`
}
