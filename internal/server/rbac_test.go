package server

import (
	"context"
	"crypto/tls"
	"crypto/x509"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/DavidHoenisch/remotr/internal/identity"
	"github.com/DavidHoenisch/remotr/internal/pki"
	"github.com/DavidHoenisch/remotr/internal/rbac"
	"github.com/DavidHoenisch/remotr/internal/registry"
)

type mockRBAC struct {
	roles map[string][]string
}

func (m *mockRBAC) EnsureBuiltInRoles(context.Context) error { return nil }

func (m *mockRBAC) Authorize(_ context.Context, operatorID, method, path string) (bool, error) {
	roleNames := m.roles[operatorID]
	var rules []rbac.Rule
	for _, name := range roleNames {
		if role, ok := rbac.BuiltInRole(name); ok {
			rules = append(rules, role.Rules...)
		}
	}
	return rbac.Allow(rules, method, path), nil
}

func (m *mockRBAC) ListRBACRoles(context.Context) ([]rbac.Role, error) { return nil, nil }
func (m *mockRBAC) GetRBACRole(context.Context, string) (rbac.Role, error) {
	return rbac.Role{}, nil
}
func (m *mockRBAC) CreateRBACRole(context.Context, string, string) error { return nil }
func (m *mockRBAC) DeleteRBACRole(context.Context, string) error       { return nil }
func (m *mockRBAC) AddRBACRule(context.Context, string, rbac.Rule) (rbac.Rule, error) {
	return rbac.Rule{}, nil
}
func (m *mockRBAC) RemoveRBACRule(context.Context, string, string) error { return nil }
func (m *mockRBAC) ListOperators(context.Context) ([]registry.Operator, error) {
	return nil, nil
}
func (m *mockRBAC) SetOperatorRoles(context.Context, string, []string) error { return nil }
func (m *mockRBAC) OperatorRoles(_ context.Context, operatorID string) ([]string, error) {
	return m.roles[operatorID], nil
}
func (m *mockRBAC) RegisterOperator(_ context.Context, operatorID, _ string, roles []string) error {
	m.roles[operatorID] = roles
	return nil
}

func TestRequirePermission_deniesReadOnlyDelete(t *testing.T) {
	caCert, caKey, caPEM := testCAForEnroll(t)
	admin := newMockAdmin()
	operatorID := "11111111-1111-1111-1111-111111111111"
	opCred, err := pki.IssueOperatorCredential(caCert, caKey, operatorID)
	if err != nil {
		t.Fatal(err)
	}
	_ = admin.RegisterOperatorCredential(identity.Fingerprint(opCred.Cert))

	rbacStore := &mockRBAC{roles: map[string][]string{
		operatorID: {rbac.RoleReadOnly},
	}}
	srv := New(Config{Admin: admin, RBAC: rbacStore, CACert: caCert, CAKey: caKey, CACertPEM: caPEM})
	handler := srv.Handler()

	req := httptest.NewRequest(http.MethodDelete, "/v1/admin/endpoints/ep-1", nil)
	req.TLS = &tls.ConnectionState{PeerCertificates: []*x509.Certificate{opCred.Cert}}
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)
	if rec.Code != http.StatusForbidden {
		t.Fatalf("status = %d body=%s", rec.Code, rec.Body.String())
	}
}

func TestRequirePermission_allowsReadOnlyList(t *testing.T) {
	caCert, caKey, caPEM := testCAForEnroll(t)
	admin := newMockAdmin()
	operatorID := "11111111-1111-1111-1111-111111111111"
	opCred, err := pki.IssueOperatorCredential(caCert, caKey, operatorID)
	if err != nil {
		t.Fatal(err)
	}
	_ = admin.RegisterOperatorCredential(identity.Fingerprint(opCred.Cert))

	rbacStore := &mockRBAC{roles: map[string][]string{
		operatorID: {rbac.RoleReadOnly},
	}}
	srv := New(Config{Admin: admin, RBAC: rbacStore, CACert: caCert, CAKey: caKey, CACertPEM: caPEM})
	handler := srv.Handler()

	req := httptest.NewRequest(http.MethodGet, "/v1/admin/endpoints", nil)
	req.TLS = &tls.ConnectionState{PeerCertificates: []*x509.Certificate{opCred.Cert}}
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d body=%s", rec.Code, rec.Body.String())
	}
}
