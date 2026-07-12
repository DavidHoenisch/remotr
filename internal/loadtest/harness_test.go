package loadtest

import (
	"errors"
	"testing"
	"time"
)

func TestEndpointIDIsUniqueAndValid(t *testing.T) {
	seen := map[string]bool{}
	for i := range 400 {
		id := EndpointID("run-abc", i)
		if seen[id] {
			t.Fatalf("duplicate endpoint id %q", id)
		}
		if len(id) < 4 || len(id) > 63 {
			t.Fatalf("endpoint id %q has invalid length", id)
		}
		seen[id] = true
	}
}

func TestSummarizeRecordsLatencyAndErrors(t *testing.T) {
	summary := Summarize([]Sample{
		{Latency: 10 * time.Millisecond, ResponseBytes: 100},
		{Latency: 20 * time.Millisecond, ResponseBytes: 200},
		{Latency: 30 * time.Millisecond, Err: errors.New("sync failed")},
	})
	if summary.Requests != 3 || summary.Successes != 2 || summary.Errors != 1 {
		t.Fatalf("summary = %+v", summary)
	}
	if summary.P95 != 30*time.Millisecond || summary.ResponseBytes != 300 {
		t.Fatalf("summary = %+v", summary)
	}
}
