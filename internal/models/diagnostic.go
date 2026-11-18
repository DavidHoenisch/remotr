package models

// DiagnosticCode is a stable machine-readable configuration diagnostic.
type DiagnosticCode string

const (
	DiagnosticLegacySchema DiagnosticCode = "legacy_schema_0"
	// DiagnosticUnsatisfiablePreflight marks a resource whose effective risk
	// requires a preflight that its provider does not implement, so the agent
	// can never apply it.
	DiagnosticUnsatisfiablePreflight DiagnosticCode = "unsatisfiable_preflight"
)

// Diagnostic is a non-fatal authoring or migration notice.
type Diagnostic struct {
	Code    DiagnosticCode `json:"code" yaml:"code"`
	Message string         `json:"message" yaml:"message"`
}
