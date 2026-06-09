package main

import (
	"encoding/json"
	"fmt"
	"os"
	"strings"

	"github.com/DavidHoenisch/remotr/internal/admin"
	"github.com/urfave/cli/v2"
)

func rbacCommand() *cli.Command {
	return &cli.Command{
		Name:  "rbac",
		Usage: "manage role-based access control",
		Subcommands: []*cli.Command{
			{
				Name:   "role-list",
				Usage:  "list roles and their rules",
				Action: actionRBACRoleList,
				Flags:  []cli.Flag{&cli.BoolFlag{Name: "json", Usage: "output JSON"}},
			},
			{
				Name:      "role-create",
				Usage:     "create a custom role",
				ArgsUsage: "<name>",
				Action:    actionRBACRoleCreate,
				Flags: []cli.Flag{
					&cli.StringFlag{Name: "description", Usage: "role description"},
				},
			},
			{
				Name:      "role-show",
				Usage:     "show one role",
				ArgsUsage: "<name>",
				Action:    actionRBACRoleShow,
				Flags:     []cli.Flag{&cli.BoolFlag{Name: "json", Usage: "output JSON"}},
			},
			{
				Name:      "role-delete",
				Usage:     "delete a custom role",
				ArgsUsage: "<name>",
				Action:    actionRBACRoleDelete,
			},
			{
				Name:      "rule-add",
				Usage:     "add a rule to a custom role",
				ArgsUsage: "<role-name>",
				Action:    actionRBACRuleAdd,
				Flags: []cli.Flag{
					&cli.StringFlag{Name: "method", Value: "GET", Usage: "HTTP method or *"},
					&cli.StringFlag{Name: "path", Required: true, Usage: "path pattern, e.g. /v1/admin/endpoints/*"},
				},
			},
			{
				Name:      "rule-remove",
				Usage:     "remove a rule from a custom role",
				ArgsUsage: "<role-name> <rule-id>",
				Action:    actionRBACRuleRemove,
			},
			{
				Name:   "operator-list",
				Usage:  "list operators and assigned roles",
				Action: actionRBACOperatorList,
				Flags:  []cli.Flag{&cli.BoolFlag{Name: "json", Usage: "output JSON"}},
			},
			{
				Name:      "operator-set-roles",
				Usage:     "replace roles assigned to an operator",
				ArgsUsage: "<operator-id>",
				Action:    actionRBACOperatorSetRoles,
				Flags: []cli.Flag{
					&cli.StringSliceFlag{Name: "role", Required: true, Usage: "role name (repeatable)"},
				},
			},
		},
	}
}

func actionRBACRoleList(c *cli.Context) error {
	client, err := rbacAdminClient(c)
	if err != nil {
		return err
	}
	roles, err := client.ListRBACRoles()
	if err != nil {
		return exitErr(1, "rbac role-list: %v", err)
	}
	if c.Bool("json") {
		return encodeJSON(roles)
	}
	for _, role := range roles {
		fmt.Printf("%s\tbuilt_in=%t\trules=%d\t%s\n", role.Name, role.BuiltIn, len(role.Rules), role.Description)
	}
	return nil
}

func actionRBACRoleCreate(c *cli.Context) error {
	name := strings.TrimSpace(c.Args().First())
	if name == "" {
		return exitErr(2, "rbac role-create: role name required")
	}
	client, err := rbacAdminClient(c)
	if err != nil {
		return err
	}
	role, err := client.CreateRBACRole(name, c.String("description"))
	if err != nil {
		return exitErr(1, "rbac role-create: %v", err)
	}
	fmt.Printf("created role %s\n", role.Name)
	return nil
}

func actionRBACRoleShow(c *cli.Context) error {
	name := strings.TrimSpace(c.Args().First())
	if name == "" {
		return exitErr(2, "rbac role-show: role name required")
	}
	client, err := rbacAdminClient(c)
	if err != nil {
		return err
	}
	role, err := client.GetRBACRole(name)
	if err != nil {
		return exitErr(1, "rbac role-show: %v", err)
	}
	if c.Bool("json") {
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

func actionRBACRoleDelete(c *cli.Context) error {
	name := strings.TrimSpace(c.Args().First())
	if name == "" {
		return exitErr(2, "rbac role-delete: role name required")
	}
	client, err := rbacAdminClient(c)
	if err != nil {
		return err
	}
	if err := client.DeleteRBACRole(name); err != nil {
		return exitErr(1, "rbac role-delete: %v", err)
	}
	fmt.Printf("deleted role %s\n", name)
	return nil
}

func actionRBACRuleAdd(c *cli.Context) error {
	roleName := strings.TrimSpace(c.Args().First())
	if roleName == "" {
		return exitErr(2, "rbac rule-add: role name required")
	}
	client, err := rbacAdminClient(c)
	if err != nil {
		return err
	}
	rule, err := client.AddRBACRule(roleName, c.String("method"), c.String("path"))
	if err != nil {
		return exitErr(1, "rbac rule-add: %v", err)
	}
	fmt.Printf("added rule %s to %s\n", rule.ID, roleName)
	return nil
}

func actionRBACRuleRemove(c *cli.Context) error {
	roleName := strings.TrimSpace(c.Args().First())
	ruleID := strings.TrimSpace(c.Args().Get(1))
	if roleName == "" || ruleID == "" {
		return exitErr(2, "rbac rule-remove: role name and rule id required")
	}
	client, err := rbacAdminClient(c)
	if err != nil {
		return err
	}
	if err := client.DeleteRBACRule(roleName, ruleID); err != nil {
		return exitErr(1, "rbac rule-remove: %v", err)
	}
	fmt.Printf("removed rule %s from %s\n", ruleID, roleName)
	return nil
}

func actionRBACOperatorList(c *cli.Context) error {
	client, err := rbacAdminClient(c)
	if err != nil {
		return err
	}
	ops, err := client.ListOperators()
	if err != nil {
		return exitErr(1, "rbac operator-list: %v", err)
	}
	if c.Bool("json") {
		return encodeJSON(ops)
	}
	for _, op := range ops {
		fmt.Printf("%s\tfp=%s\troles=%s\n", op.ID, op.CertFingerprint, strings.Join(op.Roles, ","))
	}
	return nil
}

func actionRBACOperatorSetRoles(c *cli.Context) error {
	operatorID := strings.TrimSpace(c.Args().First())
	if operatorID == "" {
		return exitErr(2, "rbac operator-set-roles: operator id required")
	}
	client, err := rbacAdminClient(c)
	if err != nil {
		return err
	}
	if err := client.SetOperatorRoles(operatorID, c.StringSlice("role")); err != nil {
		return exitErr(1, "rbac operator-set-roles: %v", err)
	}
	fmt.Printf("updated roles for %s\n", operatorID)
	return nil
}

func rbacAdminClient(c *cli.Context) (*admin.Client, error) {
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

func encodeJSON(v any) error {
	enc := json.NewEncoder(os.Stdout)
	enc.SetIndent("", "  ")
	return enc.Encode(v)
}
