package admin

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestDemoTransport_roundTrip(t *testing.T) {
	dir := t.TempDir()
	fixture := demoFixture{
		Status: 200,
		Body: mustJSON(t, CreateEnrollTokenResponse{
			Token:     "demo-enroll-token",
			Fleet:     "engineering",
			ExpiresAt: time.Date(2026, 6, 1, 12, 0, 0, 0, time.UTC),
		}),
	}
	raw, err := json.Marshal(fixture)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "POST_v1_admin_enroll-tokens.json"), raw, 0o644); err != nil {
		t.Fatal(err)
	}

	t.Setenv("REMOTR_DEMO", "1")
	t.Setenv("REMOTR_DEMO_FIXTURES", dir)

	client, err := NewClient("https://demo.remotr.example:8443", t.TempDir(), nil)
	if err != nil {
		t.Fatal(err)
	}
	resp, err := client.CreateEnrollToken("engineering", time.Hour)
	if err != nil {
		t.Fatal(err)
	}
	if resp.Token != "demo-enroll-token" {
		t.Fatalf("token = %q", resp.Token)
	}
}

func TestFixtureKey(t *testing.T) {
	key, err := fixtureKey("GET", "/v1/admin/endpoints/laptop-01")
	if err != nil {
		t.Fatal(err)
	}
	if key != "GET_v1_admin_endpoints_laptop-01" {
		t.Fatalf("key = %q", key)
	}
}

func mustJSON(t *testing.T, v any) json.RawMessage {
	t.Helper()
	b, err := json.Marshal(v)
	if err != nil {
		t.Fatal(err)
	}
	return b
}
