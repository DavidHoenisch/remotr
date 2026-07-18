package executor

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"sort"
	"strings"
	"unicode/utf8"
)

const maxSafeFieldTextBytes = 512

// SafeSensitivity is the classification carried by a value after it leaves a
// provider's raw execution boundary.
type SafeSensitivity string

const (
	SafePublic            SafeSensitivity = "public"
	SafeSensitiveMetadata SafeSensitivity = "sensitive-metadata"
	SafeSecret            SafeSensitivity = "secret"
)

// SafeProjection is the approved representation of one classified value.
type SafeProjection string

const (
	SafeValue       SafeProjection = "value"
	SafeMetadata    SafeProjection = "metadata"
	SafeReference   SafeProjection = "reference"
	SafeFingerprint SafeProjection = "fingerprint"
	SafePresence    SafeProjection = "presence"
	SafeCount       SafeProjection = "count"
)

// SafeField is one classified, sink-safe projection. Text is permitted only
// for public values and approved metadata/reference/fingerprint projections;
// secret bytes have no representable projection.
type SafeField struct {
	Path        string          `json:"path"`
	Sensitivity SafeSensitivity `json:"sensitivity"`
	Projection  SafeProjection  `json:"projection"`
	Text        string          `json:"text,omitempty"`
	Present     *bool           `json:"present,omitempty"`
	Count       *int            `json:"count,omitempty"`
}

// SafeSummary is the only desired/observed value shape admitted to generic
// reports and persistence. Its fields are validated and sorted on admission.
type SafeSummary struct {
	Fields []SafeField `json:"fields,omitempty"`
}

// NewSafeSummary validates and copies classified fields for sink admission.
func NewSafeSummary(fields []SafeField) (SafeSummary, error) {
	copyFields := append([]SafeField(nil), fields...)
	for i := range copyFields {
		if err := copyFields[i].validate(); err != nil {
			return SafeSummary{}, fmt.Errorf("safe field %d: %w", i+1, err)
		}
		copyFields[i].Text = truncateSafeText(copyFields[i].Text)
		if copyFields[i].Present != nil {
			value := *copyFields[i].Present
			copyFields[i].Present = &value
		}
		if copyFields[i].Count != nil {
			value := *copyFields[i].Count
			copyFields[i].Count = &value
		}
	}
	sort.SliceStable(copyFields, func(i, j int) bool {
		if copyFields[i].Path != copyFields[j].Path {
			return copyFields[i].Path < copyFields[j].Path
		}
		return copyFields[i].Text < copyFields[j].Text
	})
	return SafeSummary{Fields: copyFields}, nil
}

// Validate rejects hand-built summaries that bypassed NewSafeSummary.
func (s SafeSummary) Validate() error {
	for i, field := range s.Fields {
		if err := field.validate(); err != nil {
			return fmt.Errorf("safe field %d: %w", i+1, err)
		}
		if len(field.Text) > maxSafeFieldTextBytes || !utf8.ValidString(field.Text) {
			return fmt.Errorf("safe field %d has invalid text", i+1)
		}
	}
	return nil
}

// Clone returns an independently mutable copy.
func (s SafeSummary) Clone() SafeSummary {
	fields := append([]SafeField(nil), s.Fields...)
	for i := range fields {
		if fields[i].Present != nil {
			value := *fields[i].Present
			fields[i].Present = &value
		}
		if fields[i].Count != nil {
			value := *fields[i].Count
			fields[i].Count = &value
		}
	}
	return SafeSummary{Fields: fields}
}

// String provides a deterministic safe rendering for logs and legacy text
// consumers. It never has access to an unprojected provider value.
func (s SafeSummary) String() string {
	if len(s.Fields) == 0 {
		return ""
	}
	parts := make([]string, 0, len(s.Fields))
	for _, field := range s.Fields {
		switch field.Projection {
		case SafePresence:
			parts = append(parts, fmt.Sprintf("%s=%t", field.Path, field.Present != nil && *field.Present))
		case SafeCount:
			count := 0
			if field.Count != nil {
				count = *field.Count
			}
			parts = append(parts, fmt.Sprintf("%s=%d", field.Path, count))
		default:
			parts = append(parts, field.Path+"="+field.Text)
		}
	}
	return strings.Join(parts, ", ")
}

// MarshalJSON validates even hand-built values before a generic sink can
// serialize them.
func (s SafeSummary) MarshalJSON() ([]byte, error) {
	if err := s.Validate(); err != nil {
		return nil, err
	}
	type wire SafeSummary
	return json.Marshal(wire(s))
}

// UnmarshalJSON validates classified wire values before admitting them to a
// consumer. Legacy unclassified strings are intentionally not accepted here.
func (s *SafeSummary) UnmarshalJSON(data []byte) error {
	type wire SafeSummary
	var decoded wire
	if err := json.Unmarshal(data, &decoded); err != nil {
		return err
	}
	normalized, err := NewSafeSummary(decoded.Fields)
	if err != nil {
		return err
	}
	*s = normalized
	return nil
}

func (f SafeField) validate() error {
	if strings.TrimSpace(f.Path) == "" {
		return errors.New("path is required")
	}
	valid := false
	switch f.Sensitivity {
	case SafePublic:
		valid = f.Projection == SafeValue
	case SafeSensitiveMetadata:
		valid = f.Projection == SafeMetadata || f.Projection == SafeFingerprint || f.Projection == SafePresence || f.Projection == SafeCount
	case SafeSecret:
		valid = f.Projection == SafeReference || f.Projection == SafePresence || f.Projection == SafeCount
	}
	if !valid {
		return fmt.Errorf("invalid sensitivity/projection %q/%q", f.Sensitivity, f.Projection)
	}
	switch f.Projection {
	case SafePresence:
		if f.Present == nil || f.Text != "" || f.Count != nil {
			return errors.New("presence projection has the wrong value shape")
		}
	case SafeCount:
		if f.Count == nil || *f.Count < 0 || f.Text != "" || f.Present != nil {
			return errors.New("count projection has the wrong value shape")
		}
	default:
		if f.Present != nil || f.Count != nil || !utf8.ValidString(f.Text) {
			return errors.New("text projection has the wrong value shape")
		}
	}
	return nil
}

func truncateSafeText(value string) string {
	if len(value) <= maxSafeFieldTextBytes && utf8.ValidString(value) {
		return value
	}
	for len(value) > maxSafeFieldTextBytes || !utf8.ValidString(value) {
		_, size := utf8.DecodeLastRuneInString(value)
		if size == 0 {
			return ""
		}
		value = value[:len(value)-size]
	}
	return value
}

// SafeError is the closed provider-error representation admitted to generic
// agent infrastructure. Raw error text and wrapped causes are deliberately not
// retained.
type SafeError struct {
	ReasonCode ReasonCode  `json:"reasonCode"`
	Operation  string      `json:"operation"`
	Canceled   bool        `json:"canceled,omitempty"`
	Details    SafeSummary `json:"details,omitempty"`
}

// NewSafeError converts an error at the provider boundary without retaining
// its message or cause.
func NewSafeError(reason ReasonCode, operation string, err error) SafeError {
	return SafeError{
		ReasonCode: reason,
		Operation:  operation,
		Canceled:   errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded),
	}
}

// NewSafeErrorWithDetails attaches only an already-classified diagnostic.
func NewSafeErrorWithDetails(reason ReasonCode, operation string, err error, details SafeSummary) SafeError {
	safe := NewSafeError(reason, operation, err)
	safe.Details = details.Clone()
	return safe
}

func (e SafeError) Error() string {
	details := e.Details.String()
	if details != "" {
		details = ": " + details
	}
	if e.Canceled {
		return fmt.Sprintf("%s canceled (%s)%s", e.Operation, e.ReasonCode, details)
	}
	return fmt.Sprintf("%s failed (%s)%s", e.Operation, e.ReasonCode, details)
}

// Is preserves cancellation control flow without exposing the discarded raw
// provider error through Unwrap.
func (e SafeError) Is(target error) bool {
	return e.Canceled && (target == context.Canceled || target == context.DeadlineExceeded)
}
