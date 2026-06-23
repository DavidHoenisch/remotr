package main

import (
	"context"
	"fmt"
	"strings"

	"github.com/DavidHoenisch/remotr/internal/admin"
	"github.com/urfave/cli/v3"
)

func rbacCommand() *cli.Command {
	roleFlags := outputFlags()
	roleCreateFlags := []cli.Flag{&cli.StringFlag{Name: "description", Usage: "role description"}, &cli.BoolFlag{Name: "json", Usage: "output result as JSON"}}
	ruleAddFlags := []cli.Flag{
		&cli.StringFlag{Name: "method", Value: "GET", Usage: "HTTP method or *"},
		&cli.StringFlag{Name: "path", Required: true, Usage: "path pattern, e.g. /v1/admin/endpoints/*"},
		&cli.BoolFlag{Name: "json", Usage: "output result as JSON"},
	}
	operatorSetRolesFlags := []cli.Flag{
		&cli.StringSliceFlag{Name: "role", Required: true, Usage: "role name (repeatable)"},
		&cli.BoolFlag{Name: "json", Usage: "output result as JSON"},
	}

	nested := []*cli.Command{
		{
			Name:  "role",
			Usage: "manage RBAC roles",
			Commands: []*cli.Command{
				{Name: "list", Usage: "list roles and their rules", Action: actionRBACRoleList, Flags: roleFlags},
				{Name: "create", Usage: "create a custom role", ArgsUsage: "[name]", Action: actionRBACRoleCreate, Flags: roleCreateFlags},
				{Name: "show", Usage: "show one role", ArgsUsage: "[name]", Action: actionRBACRoleShow, Flags: append(roleFlags, &cli.StringFlag{Name: "name", Usage: "role name (alternative to positional)"})},
				{Name: "delete", Usage: "delete a custom role", ArgsUsage: "[name]", Action: actionRBACRoleDelete, Flags: []cli.Flag{
					&cli.StringFlag{Name: "name", Usage: "role name (alternative to positional)"},
					confirmFlag("role name"),
				}},
			},
		},
		{
			Name:  "rule",
			Usage: "manage RBAC rules on custom roles",
			Commands: []*cli.Command{
				{Name: "add", Usage: "add a rule to a custom role", ArgsUsage: "[role-name]", Action: actionRBACRuleAdd, Flags: append(ruleAddFlags, &cli.StringFlag{Name: "role", Usage: "role name (alternative to positional)"})},
				{Name: "remove", Usage: "remove a rule from a custom role", ArgsUsage: "[role-name] [rule-id]", Action: actionRBACRuleRemove, Flags: []cli.Flag{
					&cli.StringFlag{Name: "role", Usage: "role name (alternative to positional)"},
					&cli.StringFlag{Name: "id", Usage: "rule id (alternative to second positional)"},
					confirmFlag("rule id"),
				}},
			},
		},
		{
			Name:  "operator",
			Usage: "manage operator role assignments",
			Commands: []*cli.Command{
				{Name: "list", Usage: "list operators and assigned roles", Action: actionRBACOperatorList, Flags: roleFlags},
				{Name: "set-roles", Usage: "replace roles assigned to an operator", ArgsUsage: "[operator-id]", Action: actionRBACOperatorSetRoles, Flags: append(operatorSetRolesFlags, &cli.StringFlag{Name: "operator", Usage: "operator id (alternative to positional)"})},
			},
		},
	}

	// Backward-compatible flat command names (hidden from help).
	aliases := []*cli.Command{
		{Name: "role-list", Hidden: true, Usage: "list roles", Action: actionRBACRoleList, Flags: roleFlags},
		{Name: "role-create", Hidden: true, Usage: "create role", Action: actionRBACRoleCreate, Flags: roleCreateFlags},
		{Name: "role-show", Hidden: true, Usage: "show role", Action: actionRBACRoleShow, Flags: roleFlags},
		{Name: "role-delete", Hidden: true, Usage: "delete role", Action: actionRBACRoleDelete, Flags: []cli.Flag{confirmFlag("role name")}},
		{Name: "rule-add", Hidden: true, Usage: "add rule", Action: actionRBACRuleAdd, Flags: ruleAddFlags},
		{Name: "rule-remove", Hidden: true, Usage: "remove rule", Action: actionRBACRuleRemove, Flags: []cli.Flag{confirmFlag("rule id")}},
		{Name: "operator-list", Hidden: true, Usage: "list operators", Action: actionRBACOperatorList, Flags: roleFlags},
		{Name: "operator-set-roles", Hidden: true, Usage: "set operator roles", Action: actionRBACOperatorSetRoles, Flags: operatorSetRolesFlags},
	}

	return &cli.Command{
		Name:        "rbac",
		Category:    catSecurity,
		Usage:       "manage role-based access control",
		Description: "Prefer nested commands (rbac role list). Flat names (rbac role-list) remain as hidden aliases.",
		Commands:    append(nested, aliases...),
	}
}

func rbacNameFromFlagOrArg(c *cli.Command) (string, bool) {
	if v := strings.TrimSpace(c.String("name")); v != "" {
		return v, true
	}
	if c.NArg() >= 1 {
		return strings.TrimSpace(c.Args().First()), true
	}
	return "", false
}

func actionRBACRoleList(_ context.Context, c *cli.Command) error {
	client, err := rbacAdminClient(c)
	if err != nil {
		return err
	}
	roles, err := client.ListRBACRoles()
	if err != nil {
		return apiErr(c, "rbac role list", err)
	}
	if resolveFormat(c) == formatJSON {
		return encodeJSON(roles)
	}
	if resolveFormat(c) == formatTable && !c.Bool("no-headers") {
		fmt.Println("NAME\tBUILT_IN\tRULES\tDESCRIPTION")
	}
	for _, role := range roles {
		fmt.Printf("%s\t%t\t%d\t%s\n", role.Name, role.BuiltIn, len(role.Rules), role.Description)
	}
	return nil
}

func actionRBACRoleCreate(_ context.Context, c *cli.Command) error {
	name, ok := rbacNameFromFlagOrArg(c)
	if !ok {
		return exitErr(2, "rbac role create: role name required")
	}
	client, err := rbacAdminClient(c)
	if err != nil {
		return err
	}
	role, err := client.CreateRBACRole(name, c.String("description"))
	if err != nil {
		return apiErr(c, "rbac role create", err)
	}
	if c.Bool("json") {
		return encodeJSON(role)
	}
	fmt.Printf("created role %s\n", role.Name)
	return nil
}

func actionRBACRoleShow(_ context.Context, c *cli.Command) error {
	name, ok := rbacNameFromFlagOrArg(c)
	if !ok {
		return exitErr(2, "rbac role show: role name required")
	}
	client, err := rbacAdminClient(c)
	if err != nil {
		return err
	}
	role, err := client.GetRBACRole(name)
	if err != nil {
		return apiErr(c, "rbac role show", err)
	}
	if resolveFormat(c) == formatJSON {
		return encodeJSON(role)
	}
	fmt.Printf("%s\tbuilt_in=%t\n%s\n", role.Name, role.BuiltIn, role.Description)
	for _, rule := range role.Rules {
		fmt.Printf("  %s %s", rule.Method, rule.PathPattern)
		if rule.ID != "" {
			fmt.Printf("  id=%s", rule.ID)
		}
		fmt.Println()
	}
	return nil
}

func actionRBACRoleDelete(_ context.Context, c *cli.Command) error {
	name, ok := rbacNameFromFlagOrArg(c)
	if !ok {
		return exitErr(2, "rbac role delete: role name required")
	}
	if err := requireConfirm(c, "rbac role delete", name); err != nil {
		return err
	}
	client, err := rbacAdminClient(c)
	if err != nil {
		return err
	}
	if err := client.DeleteRBACRole(name); err != nil {
		return apiErr(c, "rbac role delete", err)
	}
	fmt.Printf("deleted role %s\n", name)
	return nil
}

func actionRBACRuleAdd(_ context.Context, c *cli.Command) error {
	roleName := strings.TrimSpace(c.String("role"))
	if roleName == "" && c.NArg() >= 1 {
		roleName = strings.TrimSpace(c.Args().First())
	}
	if roleName == "" {
		return exitErr(2, "rbac rule add: role name required")
	}
	client, err := rbacAdminClient(c)
	if err != nil {
		return err
	}
	rule, err := client.AddRBACRule(roleName, c.String("method"), c.String("path"))
	if err != nil {
		return apiErr(c, "rbac rule add", err)
	}
	if c.Bool("json") {
		return encodeJSON(rule)
	}
	fmt.Printf("added rule %s to %s\n", rule.ID, roleName)
	return nil
}

func actionRBACRuleRemove(_ context.Context, c *cli.Command) error {
	roleName := strings.TrimSpace(c.String("role"))
	if roleName == "" && c.NArg() >= 1 {
		roleName = strings.TrimSpace(c.Args().First())
	}
	ruleID := strings.TrimSpace(c.String("id"))
	if ruleID == "" && c.NArg() >= 2 {
		ruleID = strings.TrimSpace(c.Args().Get(1))
	}
	if roleName == "" || ruleID == "" {
		return exitErr(2, "rbac rule remove: role name and rule id required")
	}
	if err := requireConfirm(c, "rbac rule remove", ruleID); err != nil {
		return err
	}
	client, err := rbacAdminClient(c)
	if err != nil {
		return err
	}
	if err := client.DeleteRBACRule(roleName, ruleID); err != nil {
		return apiErr(c, "rbac rule remove", err)
	}
	fmt.Printf("removed rule %s from %s\n", ruleID, roleName)
	return nil
}

func actionRBACOperatorList(_ context.Context, c *cli.Command) error {
	client, err := rbacAdminClient(c)
	if err != nil {
		return err
	}
	ops, err := client.ListOperators()
	if err != nil {
		return apiErr(c, "rbac operator list", err)
	}
	if resolveFormat(c) == formatJSON {
		return encodeJSON(ops)
	}
	if resolveFormat(c) == formatTable && !c.Bool("no-headers") {
		fmt.Println("ID\tFINGERPRINT\tROLES")
	}
	for _, op := range ops {
		fmt.Printf("%s\t%s\t%s\n", op.ID, op.CertFingerprint, strings.Join(op.Roles, ","))
	}
	return nil
}

func actionRBACOperatorSetRoles(_ context.Context, c *cli.Command) error {
	operatorID := strings.TrimSpace(c.String("operator"))
	if operatorID == "" && c.NArg() >= 1 {
		operatorID = strings.TrimSpace(c.Args().First())
	}
	if operatorID == "" {
		return exitErr(2, "rbac operator set-roles: operator id required")
	}
	client, err := rbacAdminClient(c)
	if err != nil {
		return err
	}
	if err := client.SetOperatorRoles(operatorID, c.StringSlice("role")); err != nil {
		return apiErr(c, "rbac operator set-roles", err)
	}
	if c.Bool("json") {
		return encodeJSON(map[string]any{"operator_id": operatorID, "roles": c.StringSlice("role")})
	}
	fmt.Printf("updated roles for %s\n", operatorID)
	return nil
}

func rbacAdminClient(c *cli.Command) (*admin.Client, error) {
	settings, err := resolveSettings(c)
	if err != nil {
		return nil, exitErr(2, "%v", err)
	}
	if err := requireOperatorCLI(settings, "rbac"); err != nil {
		return nil, err
	}
	client, err := newAdminClient(settings)
	if err != nil {
		return nil, exitErr(1, "%v", err)
	}
	return client, nil
}
