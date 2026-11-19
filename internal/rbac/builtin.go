package rbac

var builtInRoleMeta = map[string]struct{ Description string }{
	RoleGlobalAdmin: {
		Description: "Full administrative access to all operator API routes.",
	},
	RoleReadOnly: {
		Description: "Read-only access to all operator API routes.",
	},
	RoleSecurityLogger: {
		Description: "Read audit events and use the SIEM export endpoint.",
	},
	RolePackageManager: {
		Description: "Manage custom Remotr app packages in the catalog.",
	},
	RoleDiagnosticsCollector: {
		Description: "Request and download endpoint diagnostic bundles.",
	},
	RoleQueryReader: {
		Description: "Read scheduled fleet osquery definitions, status, snapshots, and diffs.",
	},
	RoleQueryRunner: {
		Description: "Read osquery results and trigger ad-hoc runs of GitOps-defined queries.",
	},
	RoleQuerySQLRunner: {
		Description: "Read osquery results and trigger ad-hoc named or inline read-only SQL queries.",
	},
}

var builtInRoleRules = map[string][]Rule{
	RoleGlobalAdmin: {
		{Method: "*", PathPattern: "/v1/admin/*"},
		{Method: "*", PathPattern: "/v1/exports/audit/*"},
	},
	RoleReadOnly: {
		{Method: "GET", PathPattern: "/v1/admin/*"},
	},
	RoleSecurityLogger: {
		{Method: "GET", PathPattern: "/v1/admin/audit-events"},
		{Method: "GET", PathPattern: "/v1/admin/audit-export"},
		{Method: "GET", PathPattern: "/v1/exports/audit/*"},
	},
	RolePackageManager: {
		{Method: "*", PathPattern: "/v1/admin/app-packages*"},
	},
	RoleDiagnosticsCollector: {
		{Method: "POST", PathPattern: "/v1/admin/endpoints/*/diagnostics/collect"},
		{Method: "GET", PathPattern: "/v1/admin/diagnostics/*"},
	},
	RoleQueryReader: {
		{Method: "GET", PathPattern: "/v1/admin/queries*"},
	},
	RoleQueryRunner: {
		{Method: "GET", PathPattern: "/v1/admin/queries*"},
		{Method: "POST", PathPattern: "/v1/admin/queries/run"},
	},
	RoleQuerySQLRunner: {
		{Method: "GET", PathPattern: "/v1/admin/queries*"},
		{Method: "POST", PathPattern: "/v1/admin/queries/run"},
		{Method: "POST", PathPattern: "/v1/admin/queries/run-sql"},
	},
}
