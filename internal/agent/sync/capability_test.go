package sync

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/DavidHoenisch/remotr/internal/capabilitydoc"
)

func TestClientSendsCapabilityDocumentWithoutBearerCredential(t *testing.T) {
	document, err := (capabilitydoc.Document{
		DocumentVersion:        1,
		ArtifactSchemaVersions: []int{0, 1},
		Capabilities:           []capabilitydoc.Capability{{ID: "resource:package", Revision: "package-v1"}},
		Facts:                  []capabilitydoc.Fact{{Key: "architecture", Value: "x86"}},
		AgentVersion:           "v1.2.3",
	}).WithCanonicalDigest()
	if err != nil {
		t.Fatal(err)
	}
	server := httptest.NewTLSServer(http.HandlerFunc(func(response http.ResponseWriter, request *http.Request) {
		if request.Header.Get("Authorization") != "" {
			t.Fatalf("unexpected bearer authorization header")
		}
		var body struct {
			CapabilityDocument *capabilitydoc.Document `json:"capabilityDocument"`
		}
		if err := json.NewDecoder(request.Body).Decode(&body); err != nil {
			t.Fatal(err)
		}
		if body.CapabilityDocument == nil || body.CapabilityDocument.Digest != document.Digest {
			t.Fatalf("capability document = %+v", body.CapabilityDocument)
		}
		response.Header().Set("Content-Type", "application/json")
		_, _ = response.Write([]byte(`{"unchanged":true}`))
	}))
	defer server.Close()

	client := &Client{BaseURL: server.URL, HTTPClient: server.Client()}
	if _, err := client.Sync(Request{AgentVersion: "v1.2.3", CapabilityDocument: &document}); err != nil {
		t.Fatal(err)
	}
}
