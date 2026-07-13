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

	t.Run("reboot required", func(t *testing.T) {
		parsed, err := parseDriftReportJSON([]byte(`{"schemaVersion":4,"inCompliance":true,"items":[],"rebootRequired":{"required":true,"sources":[{"address":"cfg/kernel","provider":"apt"}]}}`))
		if err != nil {
			t.Fatal(err)
		}
		if parsed.RebootRequired == nil || !parsed.RebootRequired.Required || len(parsed.RebootRequired.Sources) != 1 || parsed.RebootRequired.Sources[0].Address != "cfg/kernel" {
			t.Fatalf("parsed = %+v", parsed)
		}
	})
}
