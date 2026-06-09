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
}
