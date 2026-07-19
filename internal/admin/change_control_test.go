package admin

import (
	"io"
	"net/http"
	"net/http/httptest"
	"testing"
)

// OS-AEC-086: the Operator client identifies only the Fleet; authoritative
// plan facts are derived by the server.
func TestCreateBaselineAdoptionSendsEmptyServerDerivedRequest(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(response http.ResponseWriter, request *http.Request) {
		if request.Method != http.MethodPost || request.URL.EscapedPath() != "/v1/admin/fleets/engineering%2Fedge/baseline-adoptions" {
			t.Fatalf("request = %s %s", request.Method, request.URL.EscapedPath())
		}
		body, err := io.ReadAll(request.Body)
		if err != nil {
			t.Fatal(err)
		}
		if string(body) != `{}` {
			t.Fatalf("body = %s, want empty server-derived request", body)
		}
		response.Header().Set("Content-Type", "application/json")
		_, _ = response.Write([]byte(`{"id":"request-1","fleet":"engineering/edge"}`))
	}))
	t.Cleanup(server.Close)

	client := &Client{BaseURL: server.URL, HTTPClient: server.Client()}
	request, err := client.CreateBaselineAdoption("engineering/edge")
	if err != nil {
		t.Fatal(err)
	}
	if request.ID != "request-1" || request.Fleet != "engineering/edge" {
		t.Fatalf("response = %+v", request)
	}
}

func TestRegenerateChangeRequestSendsEmptyServerDerivedRequest(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(response http.ResponseWriter, request *http.Request) {
		if request.Method != http.MethodPost || request.URL.EscapedPath() != "/v1/admin/change-requests/legacy%2Frequest/regenerate" {
			t.Fatalf("request = %s %s", request.Method, request.URL.EscapedPath())
		}
		body, err := io.ReadAll(request.Body)
		if err != nil {
			t.Fatal(err)
		}
		if string(body) != `{}` {
			t.Fatalf("body = %s, want empty server-derived request", body)
		}
		response.Header().Set("Content-Type", "application/json")
		_, _ = response.Write([]byte(`{"legacy_request":{"id":"legacy/request"},"replacement_request":{"id":"replacement"}}`))
	}))
	t.Cleanup(server.Close)

	client := &Client{BaseURL: server.URL, HTTPClient: server.Client()}
	result, err := client.RegenerateChangeRequest("legacy/request")
	if err != nil {
		t.Fatal(err)
	}
	if result.LegacyRequest.ID != "legacy/request" || result.ReplacementRequest.ID != "replacement" {
		t.Fatalf("response = %+v", result)
	}
}
