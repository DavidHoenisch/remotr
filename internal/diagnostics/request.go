package diagnostics

import "time"

// Request is a diagnostic collection job tracked by the server.
type Request struct {
	ID           string
	EndpointID   string
	RequestedBy  string
	Status       string
	Spec         Spec
	S3Key        string
	SHA256       string
	SizeBytes    int64
	ErrorMessage string
	CreatedAt    time.Time
	DispatchedAt *time.Time
	CompletedAt  *time.Time
	ExpiresAt    time.Time
}

// CollectionInstruction is delivered to the agent on sync.
type CollectionInstruction struct {
	RequestID  string    `json:"requestId"`
	Collectors []string  `json:"collectors"`
	Since      time.Time `json:"since"`
	Until      time.Time `json:"until"`
}

// ResultPayload is agent-reported diagnostic completion telemetry.
type ResultPayload struct {
	RequestID string `json:"requestId"`
	Status    string `json:"status"`
	SHA256    string `json:"sha256,omitempty"`
	SizeBytes int64  `json:"sizeBytes,omitempty"`
	Message   string `json:"message,omitempty"`
}
