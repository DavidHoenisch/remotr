package server

import (
	"bytes"
	"crypto/tls"
	"crypto/x509"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/DavidHoenisch/remotr/internal/documenthash"
	"github.com/DavidHoenisch/remotr/internal/identity"
	"github.com/DavidHoenisch/remotr/internal/pki"
	"github.com/DavidHoenisch/remotr/internal/registry"
)

func primeEndpointDecision(cache *unchangedSyncCache, endpointID, fleet string) {
	hash := "sha256:aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"
	request := syncRequest{
		LastReleaseRef: "release", LastDigest: "digest",
		documentHashes: &documenthash.Summary{Version: 1, Documents: map[string]string{documenthash.Capability: hash}},
	}
	response := syncResponse{
		Unchanged: true, ReleaseRef: "release", Digest: "digest",
		AcceptedDocumentHashes: &documenthash.Summary{Version: 1, Documents: cloneHashes(request.documentHashes.Documents)},
	}
	snapshot := cache.authoritySnapshot(endpointID, fleet)
	cache.putWithSnapshot(endpointID, fleet, "fingerprint", request, response, time.Unix(0, 0), snapshot)
}

type observingEndpointAdmin struct {
	*mockAdmin
	server *Server
}

func (a *observingEndpointAdmin) assertInvalidated(endpointID string) {
	if _, present := a.server.fastPath.entries[endpointID]; present {
		panic("endpoint entry survived mutation begin")
	}
	if snapshot := a.server.fastPath.authoritySnapshot(endpointID, "engineering"); snapshot.stable {
		panic("endpoint authority remained stable during mutation")
	}
}

func (a *observingEndpointAdmin) DeleteEndpoint(id string) (bool, error) {
	a.assertInvalidated(id)
	return a.mockAdmin.DeleteEndpoint(id)
}

func (a *observingEndpointAdmin) SetEndpointLabel(id, key, value string) (map[string]string, error) {
	a.assertInvalidated(id)
	return a.mockAdmin.SetEndpointLabel(id, key, value)
}

func (a *observingEndpointAdmin) ReassignEndpoint(id, fleet string) (bool, error) {
	a.assertInvalidated(id)
	for index := range a.endpoints {
		if a.endpoints[index].ID == id {
			a.endpoints[index].Fleet = fleet
			return true, nil
		}
	}
	return false, nil
}

// OS-USF-006: endpoint registration, revocation, and targeting mutations evict
// before persistence while an unrelated endpoint remains eligible.
func TestEndpointPublicMutationsInvalidateBeforePersistence(t *testing.T) {
	endpointID := "11111111-1111-1111-1111-111111111111"
	unrelatedID := "22222222-2222-2222-2222-222222222222"
	caCert, caKey, caPEM := testCAForEnroll(t)

	t.Run("enrollment and credential registration", func(t *testing.T) {
		enroller := newMockEnrollRegistry()
		var srv *Server
		enroller.beforeRegister = func(endpoint registry.Endpoint) {
			if _, present := srv.fastPath.entries[endpoint.ID]; present {
				t.Fatal("enrollment registration began before endpoint eviction")
			}
			if snapshot := srv.fastPath.authoritySnapshot(endpoint.ID, endpoint.Fleet); snapshot.stable {
				t.Fatal("endpoint authority remained stable during registration")
			}
		}
		srv = New(Config{
			Enroller: enroller, CACert: caCert, CAKey: caKey, CACertPEM: caPEM,
			FastPath: FastPathConfig{Enabled: true, ServingProcesses: 1},
		})
		primeEndpointDecision(srv.fastPath, endpointID, "test-fleet")
		primeEndpointDecision(srv.fastPath, unrelatedID, "test-fleet")
		body, _ := json.Marshal(enrollRequest{Token: "enroll-secret", EndpointID: endpointID})
		req := httptest.NewRequest(http.MethodPost, "/v1/enroll", bytes.NewReader(body))
		rec := httptest.NewRecorder()
		srv.Handler().ServeHTTP(rec, req)
		if rec.Code != http.StatusOK {
			t.Fatalf("status = %d body = %s", rec.Code, rec.Body.String())
		}
		if _, present := srv.fastPath.entries[unrelatedID]; !present {
			t.Fatal("enrollment evicted unrelated endpoint")
		}
	})

	t.Run("delete revocation and label targeting", func(t *testing.T) {
		base := newMockAdmin()
		base.endpoints = []registry.Endpoint{{ID: endpointID, Fleet: "engineering"}, {ID: unrelatedID, Fleet: "engineering"}}
		admin := &observingEndpointAdmin{mockAdmin: base}
		opCred, err := pki.IssueOperatorCredential(caCert, caKey, "55555555-2222-3333-4444-555555555555")
		if err != nil {
			t.Fatal(err)
		}
		_ = admin.RegisterOperatorCredential(identity.Fingerprint(opCred.Cert))
		srv := New(Config{
			Admin: admin, CACert: caCert, CAKey: caKey, CACertPEM: caPEM,
			FastPath: FastPathConfig{Enabled: true, ServingProcesses: 1},
		})
		admin.server = srv
		primeEndpointDecision(srv.fastPath, endpointID, "engineering")
		primeEndpointDecision(srv.fastPath, unrelatedID, "engineering")
		labelReq := httptest.NewRequest(http.MethodPut, "/v1/admin/endpoints/"+endpointID+"/labels/site", bytes.NewBufferString(`{"value":"west"}`))
		labelReq.TLS = &tls.ConnectionState{PeerCertificates: []*x509.Certificate{opCred.Cert}}
		labelRec := httptest.NewRecorder()
		srv.Handler().ServeHTTP(labelRec, labelReq)
		if labelRec.Code != http.StatusOK {
			t.Fatalf("label status = %d body = %s", labelRec.Code, labelRec.Body.String())
		}
		primeEndpointDecision(srv.fastPath, endpointID, "engineering")
		reassignReq := httptest.NewRequest(http.MethodPut, "/v1/admin/endpoints/"+endpointID+"/fleet", bytes.NewBufferString(`{"fleet":"operations"}`))
		reassignReq.TLS = &tls.ConnectionState{PeerCertificates: []*x509.Certificate{opCred.Cert}}
		reassignRec := httptest.NewRecorder()
		srv.Handler().ServeHTTP(reassignRec, reassignReq)
		if reassignRec.Code != http.StatusOK {
			t.Fatalf("reassign status = %d body = %s", reassignRec.Code, reassignRec.Body.String())
		}
		primeEndpointDecision(srv.fastPath, endpointID, "engineering")
		deleteReq := httptest.NewRequest(http.MethodDelete, "/v1/admin/endpoints/"+endpointID, nil)
		deleteReq.TLS = &tls.ConnectionState{PeerCertificates: []*x509.Certificate{opCred.Cert}}
		deleteRec := httptest.NewRecorder()
		srv.Handler().ServeHTTP(deleteRec, deleteReq)
		if deleteRec.Code != http.StatusNoContent {
			t.Fatalf("delete status = %d body = %s", deleteRec.Code, deleteRec.Body.String())
		}
		if _, present := srv.fastPath.entries[unrelatedID]; !present {
			t.Fatal("endpoint mutations evicted unrelated endpoint")
		}
	})
}
