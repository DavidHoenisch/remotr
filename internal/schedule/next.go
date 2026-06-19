package schedule

import (
	"strconv"
	"strings"
	"time"
)

const maxDueScan = 366 * 24 * time.Hour

// Matches reports whether t satisfies a 5-field cron expression in t's location.
func Matches(expr string, t time.Time) bool {
	fields := strings.Fields(strings.TrimSpace(expr))
	if len(fields) != 5 {
		return false
	}
	return fieldMatches(fields[0], t.Minute(), 0, 59) &&
		fieldMatches(fields[1], int(t.Hour()), 0, 23) &&
		fieldMatches(fields[2], t.Day(), 1, 31) &&
		fieldMatches(fields[3], int(t.Month()), 1, 12) &&
		dowMatches(fields[4], t.Weekday())
}

// LastDue returns the latest scheduled minute slot at or before now that is strictly
// after after (or any slot <= now when after is zero).
func LastDue(expr string, loc *time.Location, now, after time.Time) (time.Time, bool) {
	if loc == nil {
		loc = time.UTC
	}
	now = now.In(loc).Truncate(time.Minute)
	after = after.In(loc).Truncate(time.Minute)

	var start time.Time
	switch {
	case after.IsZero():
		start = now.Add(-maxDueScan)
	default:
		start = after.Add(time.Minute)
	}
	if start.After(now) {
		return time.Time{}, false
	}

	var last time.Time
	for cur := start; !cur.After(now); cur = cur.Add(time.Minute) {
		if Matches(expr, cur) {
			last = cur
		}
		if cur.Sub(start) > maxDueScan {
			break
		}
	}
	if last.IsZero() {
		return time.Time{}, false
	}
	return last, true
}

func fieldMatches(raw string, value, min, max int) bool {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return false
	}
	for _, part := range strings.Split(raw, ",") {
		if partMatches(strings.TrimSpace(part), value, min, max) {
			return true
		}
	}
	return false
}

func partMatches(part string, value, min, max int) bool {
	step := 1
	if i := strings.Index(part, "/"); i >= 0 {
		stepPart := strings.TrimSpace(part[i+1:])
		n, err := strconv.Atoi(stepPart)
		if err != nil || n <= 0 {
			return false
		}
		step = n
		part = strings.TrimSpace(part[:i])
	}

	var values []int
	switch {
	case part == "" || part == "*":
		for v := min; v <= max; v++ {
			values = append(values, v)
		}
	case strings.Contains(part, "-"):
		lo, hi, ok := strings.Cut(part, "-")
		if !ok {
			return false
		}
		start, err := parseIntField(lo, min, max)
		if err != nil {
			return false
		}
		end, err := parseIntField(hi, min, max)
		if err != nil {
			return false
		}
		if start > end {
			return false
		}
		for v := start; v <= end; v++ {
			values = append(values, v)
		}
	default:
		n, err := parseIntField(part, min, max)
		if err != nil {
			return false
		}
		values = []int{n}
	}

	for _, v := range values {
		if v == value {
			if step <= 1 {
				return true
			}
			if (v-min)%step == 0 {
				return true
			}
		}
	}
	return false
}

func dowMatches(raw string, weekday time.Weekday) bool {
	return fieldMatchesDOW(raw, int(weekday))
}

func fieldMatchesDOW(raw string, weekday int) bool {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return false
	}
	for _, part := range strings.Split(raw, ",") {
		if dowPartMatches(strings.TrimSpace(part), weekday) {
			return true
		}
	}
	return false
}

func dowPartMatches(part string, weekday int) bool {
	step := 1
	if i := strings.Index(part, "/"); i >= 0 {
		stepPart := strings.TrimSpace(part[i+1:])
		n, err := strconv.Atoi(stepPart)
		if err != nil || n <= 0 {
			return false
		}
		step = n
		part = strings.TrimSpace(part[:i])
	}

	matchValue := func(v int) bool {
		if v == 7 {
			v = 0
		}
		if v != weekday {
			return false
		}
		if step <= 1 {
			return true
		}
		return v%step == 0
	}

	switch {
	case part == "" || part == "*":
		if step <= 1 {
			return true
		}
		return weekday%step == 0
	case strings.Contains(part, "-"):
		lo, hi, ok := strings.Cut(part, "-")
		if !ok {
			return false
		}
		start, err := parseDOWBound(lo)
		if err != nil {
			return false
		}
		end, err := parseDOWBound(hi)
		if err != nil {
			return false
		}
		if start <= end {
			for v := start; v <= end; v++ {
				if matchValue(v) {
					return true
				}
			}
			return false
		}
		for v := start; v <= 6; v++ {
			if matchValue(v) {
				return true
			}
		}
		for v := 0; v <= end; v++ {
			if matchValue(v) {
				return true
			}
		}
		return false
	default:
		v, err := parseDOWBound(part)
		if err != nil {
			return false
		}
		return matchValue(v)
	}
}

func parseDOWBound(raw string) (int, error) {
	n, err := strconv.Atoi(strings.TrimSpace(raw))
	if err != nil {
		return 0, err
	}
	if n == 7 {
		n = 0
	}
	if n < 0 || n > 6 {
		return 0, strconv.ErrRange
	}
	return n, nil
}

func parseIntField(raw string, min, max int) (int, error) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return 0, strconv.ErrSyntax
	}
	n, err := strconv.Atoi(raw)
	if err != nil {
		return 0, err
	}
	if n < min || n > max {
		return 0, strconv.ErrRange
	}
	return n, nil
}

// LocationForCron resolves the timezone for a cron job (default UTC).
func LocationForCron(timezone string) (*time.Location, error) {
	tz := strings.TrimSpace(timezone)
	if tz == "" || tz == "UTC" {
		return time.UTC, nil
	}
	return time.LoadLocation(tz)
}
