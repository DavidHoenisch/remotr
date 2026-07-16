package main

import (
	"bytes"
	"context"
	"crypto/tls"
	"encoding/json"
	"errors"
	"io"
	"log"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"sync"
	"testing"
	"time"
)

const rbacPrivateKeyCanary = "rbac-private-key-canary-never-bridge"
const rbacCertificateCanary = "rbac-certificate-canary-never-bridge"

type rbacParityState struct {
	mu        sync.Mutex
	requests  []string
	bodies    map[string][][]byte
	forbid    bool
	malformed bool
}

func TestRBACOperatorParityUsesCanonicalServerAuthorityAndProtectedCredentialOutput(t *testing.T) {
	var logOutput bytes.Buffer
	originalLogOutput := log.Writer()
	log.SetOutput(&logOutput)
	t.Cleanup(func() { log.SetOutput(originalLogOutput) })

	app, state, outputDir, settingsDir := newRBACParityTestApp(t)

	roles, err := app.ListDesktopRBACRoles()
	if err != nil {
		t.Fatalf("list roles: %v", err)
	}
	if len(roles) != 4 || roles[0].Name != "global_admin" || !roles[0].BuiltIn || roles[1].Name != "ops_team" || roles[2].Name != "read_only" || roles[3].Name != "security_logger" {
		t.Fatalf("roles = %#v", roles)
	}
	role, err := app.GetDesktopRBACRole("ops_team")
	if err != nil || len(role.Rules) != 1 || role.Rules[0].ID != "11111111-1111-4111-8111-111111111111" {
		t.Fatalf("get role = %#v, %v", role, err)
	}

	if _, err := app.CreateDesktopRBACRole(RBACRoleCreateRequest{Name: "bad role"}); err == nil {
		t.Fatal("invalid role name succeeded")
	}
	created, err := app.CreateDesktopRBACRole(RBACRoleCreateRequest{Name: "incident_response", Description: "Incident response operators"})
	if err != nil || created.Name != "incident_response" || created.BuiltIn {
		t.Fatalf("create role = %#v, %v", created, err)
	}

	if _, err := app.DeleteDesktopRBACRole(RBACRoleDeleteRequest{Name: "global_admin", Confirmation: "global_admin DELETE ROLE"}); err == nil {
		t.Fatal("built-in role deletion succeeded")
	}
	if _, err := app.DeleteDesktopRBACRole(RBACRoleDeleteRequest{Name: "ops_team", Confirmation: "ops_team"}); err == nil {
		t.Fatal("role deletion without exact scope succeeded")
	}
	deleted, err := app.DeleteDesktopRBACRole(RBACRoleDeleteRequest{Name: "ops_team", Confirmation: "ops_team DELETE ROLE"})
	if err != nil || deleted.Name != "ops_team" || deleted.Status != "deleted" {
		t.Fatalf("delete role = %#v, %v", deleted, err)
	}

	if _, err := app.AddDesktopRBACRule(RBACRuleAddRequest{RoleName: "ops_team", Method: "TRACE", PathPattern: "/v1/admin/endpoints"}); err == nil {
		t.Fatal("invalid rule method succeeded")
	}
	ruleView, err := app.AddDesktopRBACRule(RBACRuleAddRequest{RoleName: "ops_team", Method: "POST", PathPattern: "/v1/admin/endpoints/*/diagnostics/collect"})
	if err != nil || ruleView.Method != "POST" || ruleView.RoleName != "ops_team" {
		t.Fatalf("add rule = %#v, %v", ruleView, err)
	}
	if _, err := app.RemoveDesktopRBACRule(RBACRuleRemoveRequest{RoleName: "ops_team", RuleID: "11111111-1111-4111-8111-111111111111", Confirmation: "11111111-1111-4111-8111-111111111111"}); err == nil {
		t.Fatal("rule removal without role scope succeeded")
	}
	removed, err := app.RemoveDesktopRBACRule(RBACRuleRemoveRequest{RoleName: "ops_team", RuleID: "11111111-1111-4111-8111-111111111111", Confirmation: "ops_team/11111111-1111-4111-8111-111111111111 REMOVE RULE"})
	if err != nil || removed.Status != "removed" {
		t.Fatalf("remove rule = %#v, %v", removed, err)
	}

	operators, err := app.ListDesktopRBACOperators()
	if err != nil || len(operators) != 1 || operators[0].ID != "operator-target" || !slices.Equal(operators[0].Roles, []string{"read_only"}) {
		t.Fatalf("operators = %#v, %v", operators, err)
	}
	if _, err := app.SetDesktopOperatorRoles(OperatorRolesRequest{OperatorID: "operator-target", Roles: []string{"missing"}, Confirmation: "operator-target SET ROLES"}); err == nil {
		t.Fatal("unknown role assignment succeeded")
	}
	updated, err := app.SetDesktopOperatorRoles(OperatorRolesRequest{OperatorID: "operator-target", Roles: []string{"ops_team", "read_only"}, Confirmation: "operator-target SET ROLES"})
	if err != nil || !slices.Equal(updated.Roles, []string{"ops_team", "read_only"}) {
		t.Fatalf("set Operator roles = %#v, %v", updated, err)
	}

	if _, err := app.StampDesktopOperatorCredential(OperatorCredentialStampRequest{Label: "siem-collector", Roles: []string{"security_logger"}, Confirmation: "siem-collector"}); err == nil {
		t.Fatal("credential issuance without exact scope succeeded")
	}
	stamped, err := app.StampDesktopOperatorCredential(OperatorCredentialStampRequest{Label: "siem-collector", Roles: []string{"security_logger"}, Confirmation: "siem-collector ISSUE CREDENTIAL"})
	if err != nil {
		t.Fatalf("stamp credential: %v", err)
	}
	if stamped.OperatorID != "operator-issued" || stamped.Label != "siem-collector" || stamped.Status != "saved" || stamped.DirectoryName != filepath.Base(outputDir) || !slices.Equal(stamped.Roles, []string{"security_logger"}) {
		t.Fatalf("credential result = %#v", stamped)
	}
	assertProtectedCredentialOutput(t, outputDir)

	state.mu.Lock()
	bodies := cloneRBACBodies(state.bodies)
	state.mu.Unlock()
	assertJSONBody(t, bodies["create-role"], `{"description":"Incident response operators","name":"incident_response"}`)
	assertJSONBody(t, bodies["add-rule"], `{"method":"POST","path_pattern":"/v1/admin/endpoints/*/diagnostics/collect"}`)
	assertJSONBody(t, bodies["set-roles"], `{"roles":["ops_team","read_only"]}`)
	assertJSONBody(t, bodies["credential"], `{"label":"siem-collector","roles":["security_logger"]}`)

	failureDir := filepath.Join(t.TempDir(), "failed-credentials")
	if err := os.Mkdir(failureDir, 0o700); err != nil {
		t.Fatal(err)
	}
	app.rbacOperators.chooseDestination = func(context.Context) (string, error) { return failureDir, nil }
	app.rbacOperators.persist = func(directory, _, _, keyPEM, _ string) error {
		if err := os.WriteFile(filepath.Join(directory, "operator.key"), []byte(keyPEM), 0o600); err != nil {
			return err
		}
		return errors.New("synthetic persistence failure")
	}
	if _, err := app.StampDesktopOperatorCredential(OperatorCredentialStampRequest{Label: "siem-collector", Roles: []string{"security_logger"}, Confirmation: "siem-collector ISSUE CREDENTIAL"}); err == nil {
		t.Fatal("credential persistence failure succeeded")
	}
	if _, err := os.Stat(filepath.Join(failureDir, "operator.key")); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("failed credential fragment remains: %v", err)
	}

	state.mu.Lock()
	state.malformed = true
	state.mu.Unlock()
	if _, err := app.ListDesktopRBACRoles(); err == nil {
		t.Fatal("malformed RBAC metadata succeeded")
	}
	state.mu.Lock()
	state.malformed = false
	state.forbid = true
	state.mu.Unlock()
	_, err = app.ListDesktopRBACRoles()
	var forbidden *ActionFailure
	if !errors.As(err, &forbidden) || forbidden.Kind != ActionForbidden {
		t.Fatalf("forbidden RBAC error = %T %v", err, err)
	}

	encoded, err := json.Marshal([]any{roles, role, created, deleted, ruleView, removed, operators, updated, stamped})
	if err != nil {
		t.Fatal(err)
	}
	for _, canary := range []string{rbacPrivateKeyCanary, rbacCertificateCanary, "privateKey", "certPEM", "keyPEM"} {
		if bytes.Contains(bytes.ToLower(encoded), bytes.ToLower([]byte(canary))) {
			t.Fatalf("RBAC view exposed %q: %s", canary, encoded)
		}
		if strings.Contains(strings.ToLower(logOutput.String()), strings.ToLower(canary)) {
			t.Fatalf("RBAC log exposed %q: %s", canary, logOutput.String())
		}
	}
	assertPathsExcludeCanary(t, rbacPrivateKeyCanary, settingsDir)
}

func newRBACParityTestApp(t *testing.T) (*App, *rbacParityState, string, string) {
	t.Helper()
	fixture := newConnectionTLSFixture(t)
	state := &rbacParityState{bodies: map[string][][]byte{}}
	server := httptest.NewUnstartedServer(http.HandlerFunc(func(response http.ResponseWriter, request *http.Request) {
		if request.TLS == nil || len(request.TLS.PeerCertificates) != 1 {
			http.Error(response, "verified Operator certificate required", http.StatusUnauthorized)
			return
		}
		response.Header().Set("Content-Type", "application/json")
		state.mu.Lock()
		state.requests = append(state.requests, request.Method+" "+request.URL.Path)
		forbidden := state.forbid
		malformed := state.malformed
		state.mu.Unlock()
		if forbidden && request.URL.Path != "/v1/admin/me" {
			http.Error(response, rbacPrivateKeyCanary, http.StatusForbidden)
			return
		}
		switch {
		case request.Method == http.MethodGet && request.URL.Path == "/v1/admin/me":
			_, _ = response.Write([]byte(`{"operator_id":"operator-admin","roles":["global_admin"]}`))
		case request.Method == http.MethodGet && request.URL.Path == "/v1/admin/rbac/roles":
			if malformed {
				_, _ = response.Write([]byte(`[{"name":"bad role","description":"","built_in":false,"rules":[],"key_pem":"` + rbacPrivateKeyCanary + `"}]`))
				return
			}
			_, _ = response.Write([]byte(roleListJSON()))
		case request.Method == http.MethodGet && request.URL.Path == "/v1/admin/rbac/roles/ops_team":
			_, _ = response.Write([]byte(customRoleJSON("ops_team")))
		case request.Method == http.MethodGet && request.URL.Path == "/v1/admin/rbac/roles/global_admin":
			_, _ = response.Write([]byte(`{"name":"global_admin","description":"Full access","built_in":true,"rules":[{"method":"*","path_pattern":"/v1/admin/*"}]}`))
		case request.Method == http.MethodPost && request.URL.Path == "/v1/admin/rbac/roles":
			recordRBACBody(t, state, "create-role", request)
			_, _ = response.Write([]byte(`{"name":"incident_response","description":"Incident response operators","built_in":false,"rules":[]}`))
		case request.Method == http.MethodDelete && request.URL.Path == "/v1/admin/rbac/roles/ops_team":
			response.WriteHeader(http.StatusNoContent)
		case request.Method == http.MethodPost && request.URL.Path == "/v1/admin/rbac/roles/ops_team/rules":
			recordRBACBody(t, state, "add-rule", request)
			_, _ = response.Write([]byte(`{"id":"22222222-2222-4222-8222-222222222222","method":"POST","path_pattern":"/v1/admin/endpoints/*/diagnostics/collect"}`))
		case request.Method == http.MethodDelete && request.URL.Path == "/v1/admin/rbac/roles/ops_team/rules/11111111-1111-4111-8111-111111111111":
			response.WriteHeader(http.StatusNoContent)
		case request.Method == http.MethodGet && request.URL.Path == "/v1/admin/operators":
			_, _ = response.Write([]byte(`[{"id":"operator-target","cert_fingerprint":"` + strings.Repeat("b", 64) + `","roles":["read_only"],"created_at":"2032-03-04T05:05:07Z"}]`))
		case request.Method == http.MethodPut && request.URL.Path == "/v1/admin/operators/operator-target/roles":
			recordRBACBody(t, state, "set-roles", request)
			response.WriteHeader(http.StatusNoContent)
		case request.Method == http.MethodPost && request.URL.Path == "/v1/admin/operator-credentials":
			recordRBACBody(t, state, "credential", request)
			_, _ = response.Write([]byte(`{"operator_id":"operator-issued","label":"siem-collector","roles":["security_logger"],"cert_pem":"` + rbacCertificateCanary + `","key_pem":"` + rbacPrivateKeyCanary + `","ca_pem":"ca-certificate-canary"}`))
		default:
			http.NotFound(response, request)
		}
	}))
	server.TLS = &tls.Config{
		Certificates: []tls.Certificate{fixture.serverCert}, ClientAuth: tls.RequireAndVerifyClientCert,
		ClientCAs: connectionCertPool(t, fixture.caPEM), MinVersion: tls.VersionTLS12,
		Time: func() time.Time { return connectionTestTime },
	}
	server.StartTLS()
	t.Cleanup(server.Close)

	stateDir := fixture.saveClientState(t, "operator-admin", connectionTestTime.Add(-time.Hour), connectionTestTime.Add(time.Hour), fixture.caPEM)
	manager := NewSessionManager(NewConnectionService().ConnectSession)
	if err := manager.SwitchProfile(t.Context(), connectionProfileForServer(t, "Production", server.URL, stateDir)); err != nil {
		t.Fatalf("connect RBAC Operator: %v", err)
	}
	outputDir := filepath.Join(t.TempDir(), "siem-credentials")
	if err := os.Mkdir(outputDir, 0o700); err != nil {
		t.Fatal(err)
	}
	service := NewRBACOperatorService(func(context.Context) (string, error) { return outputDir, nil }, persistCredentialSet)
	app := NewApp("test", WithRBACOperatorService(service))
	app.sessions = manager
	settingsDir := t.TempDir()
	app.profiles = NewProfileService(filepath.Join(settingsDir, "desktop-profiles.json"), filepath.Join(settingsDir, "operator-config.yaml"))
	return app, state, outputDir, settingsDir
}

func roleListJSON() string {
	return `[{"name":"global_admin","description":"Full access","built_in":true,"rules":[{"method":"*","path_pattern":"/v1/admin/*"}]},` + customRoleJSON("ops_team") + `,{"name":"read_only","description":"Read access","built_in":true,"rules":[{"method":"GET","path_pattern":"/v1/admin/*"}]},{"name":"security_logger","description":"Audit access","built_in":true,"rules":[{"method":"GET","path_pattern":"/v1/admin/audit-events"}]}]`
}

func customRoleJSON(name string) string {
	return `{"name":"` + name + `","description":"Operations","built_in":false,"rules":[{"id":"11111111-1111-4111-8111-111111111111","method":"GET","path_pattern":"/v1/admin/endpoints/*"}]}`
}

func recordRBACBody(t *testing.T, state *rbacParityState, key string, request *http.Request) {
	t.Helper()
	body, err := io.ReadAll(request.Body)
	if err != nil {
		t.Errorf("read %s body: %v", key, err)
		return
	}
	state.mu.Lock()
	state.bodies[key] = append(state.bodies[key], slices.Clone(body))
	state.mu.Unlock()
}

func cloneRBACBodies(values map[string][][]byte) map[string][][]byte {
	cloned := make(map[string][][]byte, len(values))
	for key, bodies := range values {
		cloned[key] = cloneByteSlices(bodies)
	}
	return cloned
}

func assertJSONBody(t *testing.T, bodies [][]byte, want string) {
	t.Helper()
	if len(bodies) != 1 || !bytes.Equal(bodies[0], []byte(want)) {
		t.Fatalf("request bodies = %q, want [%s]", bodies, want)
	}
}

func assertProtectedCredentialOutput(t *testing.T, directory string) {
	t.Helper()
	info, err := os.Stat(directory)
	if err != nil || info.Mode().Perm() != 0o700 {
		t.Fatalf("credential directory mode = %v, %v", info, err)
	}
	for _, name := range []string{"operator.crt", "operator.key", "ca.crt", "state.json"} {
		info, err := os.Stat(filepath.Join(directory, name))
		if err != nil || info.Mode().Perm() != 0o600 {
			t.Fatalf("credential file %s mode = %v, %v", name, info, err)
		}
	}
	key, err := os.ReadFile(filepath.Join(directory, "operator.key"))
	if err != nil || string(key) != rbacPrivateKeyCanary {
		t.Fatalf("protected private key output = %q, %v", key, err)
	}
}
