package schedule

import (
	"fmt"
	"strconv"
	"strings"
)

// ValidateCronExpression checks a standard 5-field cron expression
// (minute hour day-of-month month day-of-week).
func ValidateCronExpression(expr string) error {
	expr = strings.TrimSpace(expr)
	if expr == "" {
		return fmt.Errorf("schedule is required")
	}
	fields := strings.Fields(expr)
	if len(fields) != 5 {
		return fmt.Errorf("schedule must have 5 fields (minute hour dom month dow), got %d", len(fields))
	}
	specs := []fieldSpec{
		{name: "minute", min: 0, max: 59},
		{name: "hour", min: 0, max: 23},
		{name: "day-of-month", min: 1, max: 31},
		{name: "month", min: 1, max: 12},
		{name: "day-of-week", min: 0, max: 7},
	}
	for i, spec := range specs {
		if err := validateField(fields[i], spec); err != nil {
			return fmt.Errorf("schedule %s: %w", spec.name, err)
		}
	}
	return nil
}

type fieldSpec struct {
	name string
	min  int
	max  int
}

func validateField(raw string, spec fieldSpec) error {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return fmt.Errorf("empty field")
	}
	if raw == "*" {
		return nil
	}
	for _, part := range strings.Split(raw, ",") {
		part = strings.TrimSpace(part)
		if part == "" {
			return fmt.Errorf("empty list item")
		}
		if err := validatePart(part, spec); err != nil {
			return err
		}
	}
	return nil
}

func validatePart(part string, spec fieldSpec) error {
	step := 1
	if i := strings.Index(part, "/"); i >= 0 {
		stepPart := strings.TrimSpace(part[i+1:])
		if stepPart == "" {
			return fmt.Errorf("invalid step in %q", part)
		}
		n, err := strconv.Atoi(stepPart)
		if err != nil || n <= 0 {
			return fmt.Errorf("invalid step in %q", part)
		}
		step = n
		part = strings.TrimSpace(part[:i])
	}

	if part == "" || part == "*" {
		return nil
	}

	if strings.Contains(part, "-") {
		lo, hi, ok := strings.Cut(part, "-")
		if !ok {
			return fmt.Errorf("invalid range %q", part)
		}
		start, err := parseBound(lo, spec)
		if err != nil {
			return err
		}
		end, err := parseBound(hi, spec)
		if err != nil {
			return err
		}
		if start > end {
			return fmt.Errorf("invalid range %q", part)
		}
		_ = step
		return nil
	}

	n, err := parseBound(part, spec)
	if err != nil {
		return err
	}
	_ = n
	_ = step
	return nil
}

func parseBound(raw string, spec fieldSpec) (int, error) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return 0, fmt.Errorf("empty value")
	}
	n, err := strconv.Atoi(raw)
	if err != nil {
		return 0, fmt.Errorf("invalid value %q", raw)
	}
	if spec.name == "day-of-week" && n == 7 {
		n = 0
	}
	if n < spec.min || n > spec.max {
		return 0, fmt.Errorf("value %d out of range for %s (%d-%d)", n, spec.name, spec.min, spec.max)
	}
	return n, nil
}
