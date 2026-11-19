// Package queryscheduler computes due scheduled osquery queries for an endpoint
// and deterministic server-side row diffs for mode: diff queries.
package queryscheduler

import (
	"encoding/json"
	"sort"
	"strings"
	"time"

	"github.com/google/uuid"

	"github.com/DavidHoenisch/remotr/internal/models"
	"github.com/DavidHoenisch/remotr/internal/schedule"
	"github.com/DavidHoenisch/remotr/internal/types"
)

const runningTimeout = 30 * time.Minute

// LastRun is the persisted latest execution state for one endpoint query.
type LastRun struct {
	QueryName    string
	RunID        string
	ScheduledFor time.Time
	Status       string
	StartedAt    time.Time
	CompletedAt  time.Time
	UpdatedAt    time.Time
}

// DueQuery is a scheduled query the server wants the endpoint to run.
type DueQuery struct {
	RunID        string
	ScheduledFor time.Time
	Query        models.QuerySpec
}

// RowDiff holds canonicalized added and removed rows between two snapshots.
type RowDiff struct {
	Added   []map[string]string `json:"added_rows"`
	Removed []map[string]string `json:"removed_rows"`
}

// ListDue returns scheduled queries eligible for the endpoint that are due at now.
func ListDue(now time.Time, queries []models.QuerySpec, lastRuns map[string]LastRun, distro types.Distro, arch types.Architecture) []DueQuery {
	out := make([]DueQuery, 0, len(queries))
	for _, q := range queries {
		if !matchesTarget(q, distro, arch) {
			continue
		}
		if due, ok := dueQuery(now, q, lastRuns[q.Name]); ok {
			out = append(out, due)
		}
	}
	return out
}

func matchesTarget(q models.QuerySpec, distro types.Distro, arch types.Architecture) bool {
	if len(q.TargetDistros) > 0 && distro != "" {
		matched := false
		for _, d := range q.TargetDistros {
			if strings.EqualFold(string(d), string(distro)) {
				matched = true
				break
			}
		}
		if !matched {
			return false
		}
	}
	if len(q.TargetArch) > 0 && arch != "" {
		matched := false
		for _, a := range q.TargetArch {
			if strings.EqualFold(string(a), string(arch)) {
				matched = true
				break
			}
		}
		if !matched {
			return false
		}
	}
	return true
}

func dueQuery(now time.Time, q models.QuerySpec, last LastRun) (DueQuery, bool) {
	if last.Status == "running" {
		started := last.StartedAt
		if started.IsZero() {
			started = last.UpdatedAt
		}
		if !started.IsZero() && now.Sub(started) < runningTimeout {
			return DueQuery{}, false
		}
	}
	loc, err := schedule.LocationForCron(q.Timezone)
	if err != nil {
		return DueQuery{}, false
	}
	anchor := time.Time{}
	if !last.ScheduledFor.IsZero() && (last.Status == "success" || last.Status == "failed" || last.Status == "unsupported") {
		anchor = last.ScheduledFor
	}
	slot, ok := schedule.LastDue(q.Schedule, loc, now, anchor)
	if !ok {
		return DueQuery{}, false
	}
	if last.ScheduledFor.Equal(slot) && (last.Status == "success" || last.Status == "unsupported") {
		return DueQuery{}, false
	}
	return DueQuery{
		RunID:        uuid.NewString(),
		ScheduledFor: slot,
		Query:        q,
	}, true
}

// ComputeRowDiff computes deterministic added and removed rows between prev and curr.
func ComputeRowDiff(prev, curr []map[string]string) RowDiff {
	prevCounts := map[string]int{}
	prevSample := map[string]map[string]string{}
	for _, row := range prev {
		key := canonicalRowKey(row)
		prevCounts[key]++
		if _, ok := prevSample[key]; !ok {
			prevSample[key] = cloneRow(row)
		}
	}

	currCounts := map[string]int{}
	currSample := map[string]map[string]string{}
	for _, row := range curr {
		key := canonicalRowKey(row)
		currCounts[key]++
		if _, ok := currSample[key]; !ok {
			currSample[key] = cloneRow(row)
		}
	}

	var addedKeys, removedKeys []string
	for key, c := range currCounts {
		if delta := c - prevCounts[key]; delta > 0 {
			for i := 0; i < delta; i++ {
				addedKeys = append(addedKeys, key)
			}
		}
	}
	for key, p := range prevCounts {
		if delta := p - currCounts[key]; delta > 0 {
			for i := 0; i < delta; i++ {
				removedKeys = append(removedKeys, key)
			}
		}
	}
	sort.Strings(addedKeys)
	sort.Strings(removedKeys)

	diff := RowDiff{
		Added:   make([]map[string]string, 0, len(addedKeys)),
		Removed: make([]map[string]string, 0, len(removedKeys)),
	}
	for _, key := range addedKeys {
		diff.Added = append(diff.Added, cloneRow(currSample[key]))
	}
	for _, key := range removedKeys {
		diff.Removed = append(diff.Removed, cloneRow(prevSample[key]))
	}
	return diff
}

// CanonicalRowKey returns a deterministic JSON encoding of a row with sorted keys.
func CanonicalRowKey(row map[string]string) string {
	return canonicalRowKey(row)
}

func canonicalRowKey(row map[string]string) string {
	// encoding/json marshals map[string]string with keys sorted lexicographically.
	b, err := json.Marshal(row)
	if err != nil {
		return "{}"
	}
	return string(b)
}

func cloneRow(row map[string]string) map[string]string {
	out := make(map[string]string, len(row))
	for k, v := range row {
		out[k] = v
	}
	return out
}
