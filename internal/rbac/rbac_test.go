package rbac

import "testing"

func TestMatch(t *testing.T) {
	cases := []struct {
		method, path, ruleMethod, rulePattern string
		want                                  bool
	}{
		{"GET", "/v1/admin/endpoints", "GET", "/v1/admin/*", true},
		{"POST", "/v1/admin/endpoints", "GET", "/v1/admin/*", false},
		{"POST", "/v1/admin/enroll-tokens", "*", "/v1/admin/*", true},
		{"GET", "/v1/exports/audit/secret", "GET", "/v1/exports/audit/*", true},
		{"GET", "/v1/admin/audit-events", "GET", "/v1/admin/audit-events", true},
		{"GET", "/v1/admin/audit-events", "GET", "/v1/admin/audit-export", false},
		{"POST", "/v1/admin/endpoints/ep-1/diagnostics/collect", "POST", "/v1/admin/endpoints/*/diagnostics/collect", true},
		{"POST", "/v1/admin/endpoints/ep-1/agent-upgrade", "POST", "/v1/admin/endpoints/*/diagnostics/collect", false},
	}
	for _, tc := range cases {
		if got := Match(tc.method, tc.path, tc.ruleMethod, tc.rulePattern); got != tc.want {
			t.Fatalf("Match(%q,%q,%q,%q)=%v want %v", tc.method, tc.path, tc.ruleMethod, tc.rulePattern, got, tc.want)
		}
	}
}

func TestBuiltInRoles(t *testing.T) {
	role, ok := BuiltInRole(RoleSecurityLogger)
	if !ok {
		t.Fatal("expected security_logger role")
	}
	if !Allow(role.Rules, "GET", "/v1/exports/audit/abc123") {
		t.Fatal("expected export access")
	}
	if Allow(role.Rules, "DELETE", "/v1/admin/endpoints/ep-1") {
		t.Fatal("did not expect delete access")
	}
}

func TestBuiltInRoleDiagnosticsCollector(t *testing.T) {
	role, ok := BuiltInRole(RoleDiagnosticsCollector)
	if !ok {
		t.Fatal("expected diagnostics_collector role")
	}
	if !Allow(role.Rules, "POST", "/v1/admin/endpoints/ep-1/diagnostics/collect") {
		t.Fatal("expected collect access")
	}
	if !Allow(role.Rules, "GET", "/v1/admin/diagnostics/req-1") {
		t.Fatal("expected poll access")
	}
	if Allow(role.Rules, "GET", "/v1/admin/endpoints") {
		t.Fatal("did not expect general admin access")
	}
}

func TestBuiltInRolePackageManager(t *testing.T) {
	role, ok := BuiltInRole(RolePackageManager)
	if !ok {
		t.Fatal("expected package_manager role")
	}
	cases := []struct {
		method, path string
		want         bool
	}{
		{"GET", "/v1/admin/app-packages", true},
		{"GET", "/v1/admin/app-packages/detail", true},
		{"POST", "/v1/admin/app-packages", true},
		{"POST", "/v1/admin/app-packages/upload", true},
		{"DELETE", "/v1/admin/app-packages/detail", true},
		{"DELETE", "/v1/admin/endpoints/ep-1", false},
		{"GET", "/v1/admin/endpoints", false},
	}
	for _, tc := range cases {
		if got := Allow(role.Rules, tc.method, tc.path); got != tc.want {
			t.Fatalf("Allow(%q,%q)=%v want %v", tc.method, tc.path, got, tc.want)
		}
	}
}
