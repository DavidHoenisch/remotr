package registry

import (
	"bytes"
	"encoding/json"
	"fmt"

	"github.com/DavidHoenisch/remotr/internal/executor"
)

// UnmarshalJSON accepts the version-7 classified summary shape while retaining
// the existing state-report model until the durable task-3.4 migration.
func (i *StateReportItem) UnmarshalJSON(data []byte) error {
	type itemAlias StateReportItem
	fields, remainder, err := splitSummaryFields(data, "desiredSummary", "observedSummary")
	if err != nil {
		return err
	}
	if err := json.Unmarshal(remainder, (*itemAlias)(i)); err != nil {
		return err
	}
	if i.DesiredSummary, err = decodeSafeSummaryText(fields["desiredSummary"]); err != nil {
		return fmt.Errorf("desiredSummary: %w", err)
	}
	if i.ObservedSummary, err = decodeSafeSummaryText(fields["observedSummary"]); err != nil {
		return fmt.Errorf("observedSummary: %w", err)
	}
	return nil
}

func (i *StateReportSubresult) UnmarshalJSON(data []byte) error {
	type subresultAlias StateReportSubresult
	fields, remainder, err := splitSummaryFields(data, "desiredSummary", "observedSummary")
	if err != nil {
		return err
	}
	if err := json.Unmarshal(remainder, (*subresultAlias)(i)); err != nil {
		return err
	}
	if i.DesiredSummary, err = decodeSafeSummaryText(fields["desiredSummary"]); err != nil {
		return fmt.Errorf("desiredSummary: %w", err)
	}
	if i.ObservedSummary, err = decodeSafeSummaryText(fields["observedSummary"]); err != nil {
		return fmt.Errorf("observedSummary: %w", err)
	}
	return nil
}

func (i *StateReportApplyItem) UnmarshalJSON(data []byte) error {
	type applyAlias StateReportApplyItem
	fields, remainder, err := splitSummaryFields(data, "desiredSummary", "observedSummary", "diagnostics")
	if err != nil {
		return err
	}
	if err := json.Unmarshal(remainder, (*applyAlias)(i)); err != nil {
		return err
	}
	if i.DesiredSummary, err = decodeSafeSummaryText(fields["desiredSummary"]); err != nil {
		return fmt.Errorf("desiredSummary: %w", err)
	}
	if i.ObservedSummary, err = decodeSafeSummaryText(fields["observedSummary"]); err != nil {
		return fmt.Errorf("observedSummary: %w", err)
	}
	if raw := fields["diagnostics"]; len(bytes.TrimSpace(raw)) != 0 && !bytes.Equal(bytes.TrimSpace(raw), []byte("null")) {
		var diagnostics []json.RawMessage
		if err := json.Unmarshal(raw, &diagnostics); err != nil {
			return fmt.Errorf("diagnostics: %w", err)
		}
		i.Diagnostics = make([]string, len(diagnostics))
		for index, diagnostic := range diagnostics {
			if i.Diagnostics[index], err = decodeSafeSummaryText(diagnostic); err != nil {
				return fmt.Errorf("diagnostics[%d]: %w", index, err)
			}
		}
	}
	return nil
}

func splitSummaryFields(data []byte, keys ...string) (map[string]json.RawMessage, []byte, error) {
	var object map[string]json.RawMessage
	if err := json.Unmarshal(data, &object); err != nil {
		return nil, nil, err
	}
	fields := make(map[string]json.RawMessage, len(keys))
	for _, key := range keys {
		fields[key] = object[key]
		delete(object, key)
	}
	remainder, err := json.Marshal(object)
	return fields, remainder, err
}

func decodeSafeSummaryText(raw json.RawMessage) (string, error) {
	trimmed := bytes.TrimSpace(raw)
	if len(trimmed) == 0 || bytes.Equal(trimmed, []byte("null")) {
		return "", nil
	}
	if trimmed[0] == '"' {
		var legacy string
		if err := json.Unmarshal(trimmed, &legacy); err != nil {
			return "", err
		}
		return legacy, nil
	}
	var summary executor.SafeSummary
	if err := json.Unmarshal(trimmed, &summary); err != nil {
		return "", err
	}
	return summary.String(), nil
}
