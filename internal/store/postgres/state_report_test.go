package postgres

import "testing"

func TestParseDriftReportJSON(t *testing.T) {
	t.Run("compliant", func(t *testing.T) {
		parsed, err := parseDriftReportJSON([]byte(`{"inCompliance":true,"items":[]}`))
		if err != nil {
			t.Fatal(err)
		}
		if !parsed.InCompliance || len(parsed.Items) != 0 {
			t.Fatalf("parsed = %+v", parsed)
		}
	})

	t.Run("drift", func(t *testing.T) {
		parsed, err := parseDriftReportJSON([]byte(`{"inCompliance":false,"items":[{"address":"cfg/a","name":"a","description":"drift"}]}`))
		if err != nil {
			t.Fatal(err)
		}
		if parsed.InCompliance || len(parsed.Items) != 1 || parsed.Items[0].Address != "cfg/a" {
			t.Fatalf("parsed = %+v", parsed)
		}
	})
}
