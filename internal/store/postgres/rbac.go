package postgres

import (
	"context"
	"errors"
	"fmt"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"

	"github.com/DavidHoenisch/remotr/internal/rbac"
	"github.com/DavidHoenisch/remotr/internal/registry"
	"github.com/DavidHoenisch/remotr/internal/store/postgres/db"
)

// EnsureBuiltInRoles seeds built-in RBAC role metadata.
func (s *Store) EnsureBuiltInRoles(ctx context.Context) error {
	for _, role := range rbac.BuiltInRoles() {
		if err := s.q.UpsertRBACRole(ctx, db.UpsertRBACRoleParams{
			Name:        role.Name,
			Description: role.Description,
			BuiltIn:     true,
		}); err != nil {
			return err
		}
	}
	return nil
}

// RegisterOperator stores an operator credential and role assignments.
func (s *Store) RegisterOperator(ctx context.Context, operatorID, fingerprint string, roles []string) error {
	if err := validateOperatorRoles(roles); err != nil {
		return err
	}
	if _, err := s.q.RegisterOperatorCredential(ctx, db.RegisterOperatorCredentialParams{
		CertFingerprint: fingerprint,
		OperatorID:      textOrNull(operatorID),
	}); err != nil {
		return err
	}
	if operatorID == "" {
		return nil
	}
	return s.replaceOperatorRoles(ctx, operatorID, roles)
}

// Authorize reports whether an operator may access method+path.
func (s *Store) Authorize(ctx context.Context, operatorID, method, path string) (bool, error) {
	roles, err := s.q.ListOperatorRoleAssignmentsForOperator(ctx, operatorID)
	if err != nil {
		return false, err
	}
	if len(roles) == 0 {
		return false, nil
	}

	var rules []rbac.Rule
	for _, roleName := range roles {
		if builtIn, ok := rbac.BuiltInRole(roleName); ok {
			rules = append(rules, builtIn.Rules...)
			continue
		}
		customRules, err := s.q.ListRBACRulesForRole(ctx, roleName)
		if err != nil {
			return false, err
		}
		for _, row := range customRules {
			id, err := uuidString(row.ID)
			if err != nil {
				return false, err
			}
			rules = append(rules, rbac.Rule{
				ID:          id,
				Method:      row.Method,
				PathPattern: row.PathPattern,
			})
		}
	}
	return rbac.Allow(rules, method, path), nil
}

// ListRBACRoles returns all roles with their effective rules.
func (s *Store) ListRBACRoles(ctx context.Context) ([]rbac.Role, error) {
	rows, err := s.q.ListRBACRoles(ctx)
	if err != nil {
		return nil, err
	}
	out := make([]rbac.Role, 0, len(rows))
	for _, row := range rows {
		role, err := s.roleFromRow(ctx, row.Name, row.Description, row.BuiltIn)
		if err != nil {
			return nil, err
		}
		out = append(out, role)
	}
	return out, nil
}

// GetRBACRole returns one role with its effective rules.
func (s *Store) GetRBACRole(ctx context.Context, name string) (rbac.Role, error) {
	row, err := s.q.GetRBACRole(ctx, name)
	if err != nil {
		return rbac.Role{}, err
	}
	return s.roleFromRow(ctx, row.Name, row.Description, row.BuiltIn)
}

// CreateRBACRole creates a custom role.
func (s *Store) CreateRBACRole(ctx context.Context, name, description string) error {
	if err := rbac.ValidateRoleName(name); err != nil {
		return err
	}
	if rbac.IsBuiltInRole(name) {
		return fmt.Errorf("built-in role %q cannot be recreated", name)
	}
	return s.q.UpsertRBACRole(ctx, db.UpsertRBACRoleParams{
		Name:        name,
		Description: description,
		BuiltIn:     false,
	})
}

// DeleteRBACRole deletes a custom role.
func (s *Store) DeleteRBACRole(ctx context.Context, name string) error {
	if rbac.IsBuiltInRole(name) {
		return fmt.Errorf("built-in role %q cannot be deleted", name)
	}
	n, err := s.q.DeleteRBACRole(ctx, name)
	if err != nil {
		return err
	}
	if n == 0 {
		return pgx.ErrNoRows
	}
	return nil
}

// AddRBACRule adds a custom rule to a role.
func (s *Store) AddRBACRule(ctx context.Context, roleName string, rule rbac.Rule) (rbac.Rule, error) {
	if rbac.IsBuiltInRole(roleName) {
		return rbac.Rule{}, fmt.Errorf("built-in role %q rules are fixed", roleName)
	}
	if _, err := s.q.GetRBACRole(ctx, roleName); err != nil {
		return rbac.Rule{}, err
	}
	row, err := s.q.InsertRBACRule(ctx, db.InsertRBACRuleParams{
		ID:          newUUID(),
		RoleName:    roleName,
		Method:      rule.Method,
		PathPattern: rule.PathPattern,
	})
	if err != nil {
		return rbac.Rule{}, err
	}
	id, err := uuidString(row.ID)
	if err != nil {
		return rbac.Rule{}, err
	}
	return rbac.Rule{ID: id, Method: row.Method, PathPattern: row.PathPattern}, nil
}

// RemoveRBACRule deletes a custom role rule.
func (s *Store) RemoveRBACRule(ctx context.Context, roleName, ruleID string) error {
	if rbac.IsBuiltInRole(roleName) {
		return fmt.Errorf("built-in role %q rules are fixed", roleName)
	}
	parsed, err := uuid.Parse(ruleID)
	if err != nil {
		return err
	}
	n, err := s.q.DeleteRBACRule(ctx, db.DeleteRBACRuleParams{
		ID:       pgtype.UUID{Bytes: parsed, Valid: true},
		RoleName: roleName,
	})
	if err != nil {
		return err
	}
	if n == 0 {
		return pgx.ErrNoRows
	}
	return nil
}

// ListOperators returns active operators and their roles.
func (s *Store) ListOperators(ctx context.Context) ([]registry.Operator, error) {
	rows, err := s.q.ListActiveOperators(ctx)
	if err != nil {
		return nil, err
	}
	assignments, err := s.q.ListOperatorRoleAssignments(ctx)
	if err != nil {
		return nil, err
	}
	rolesByOperator := map[string][]string{}
	for _, row := range assignments {
		rolesByOperator[row.OperatorID] = append(rolesByOperator[row.OperatorID], row.RoleName)
	}

	out := make([]registry.Operator, 0, len(rows))
	for _, row := range rows {
		op := registry.Operator{
			CertFingerprint: row.CertFingerprint,
			CreatedAt:       row.CreatedAt.Time.UTC(),
		}
		if row.OperatorID.Valid {
			op.ID = row.OperatorID.String
			op.Roles = append([]string(nil), rolesByOperator[op.ID]...)
		}
		out = append(out, op)
	}
	return out, nil
}

// SetOperatorRoles replaces role assignments for an operator.
func (s *Store) SetOperatorRoles(ctx context.Context, operatorID string, roles []string) error {
	if operatorID == "" {
		return errors.New("operator id required")
	}
	if err := validateOperatorRoles(roles); err != nil {
		return err
	}
	return s.replaceOperatorRoles(ctx, operatorID, roles)
}

// OperatorRoles returns assigned roles for an operator.
func (s *Store) OperatorRoles(ctx context.Context, operatorID string) ([]string, error) {
	return s.q.ListOperatorRoleAssignmentsForOperator(ctx, operatorID)
}

func (s *Store) replaceOperatorRoles(ctx context.Context, operatorID string, roles []string) error {
	if err := s.q.ReplaceOperatorRoleAssignments(ctx, operatorID); err != nil {
		return err
	}
	for _, roleName := range roles {
		if _, err := s.q.GetRBACRole(ctx, roleName); err != nil {
			return fmt.Errorf("unknown role %q", roleName)
		}
		if err := s.q.InsertOperatorRoleAssignment(ctx, db.InsertOperatorRoleAssignmentParams{
			OperatorID: operatorID,
			RoleName:   roleName,
		}); err != nil {
			return err
		}
	}
	return nil
}

func (s *Store) roleFromRow(ctx context.Context, name, description string, builtIn bool) (rbac.Role, error) {
	if builtIn {
		role, ok := rbac.BuiltInRole(name)
		if ok {
			return role, nil
		}
	}
	rows, err := s.q.ListRBACRulesForRole(ctx, name)
	if err != nil {
		return rbac.Role{}, err
	}
	rules := make([]rbac.Rule, 0, len(rows))
	for _, row := range rows {
		id, err := uuidString(row.ID)
		if err != nil {
			return rbac.Role{}, err
		}
		rules = append(rules, rbac.Rule{
			ID:          id,
			Method:      row.Method,
			PathPattern: row.PathPattern,
		})
	}
	return rbac.Role{
		Name:        name,
		Description: description,
		BuiltIn:     builtIn,
		Rules:       rules,
	}, nil
}

func validateOperatorRoles(roles []string) error {
	seen := map[string]struct{}{}
	for _, role := range roles {
		if role == "" {
			return errors.New("empty role name")
		}
		if _, ok := seen[role]; ok {
			continue
		}
		seen[role] = struct{}{}
		if !rbac.IsBuiltInRole(role) {
			continue
		}
	}
	return nil
}
