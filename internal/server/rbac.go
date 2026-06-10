package server

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"

	"github.com/go-chi/chi/v5"
	"github.com/jackc/pgx/v5"

	"github.com/DavidHoenisch/remotr/internal/audit"
	"github.com/DavidHoenisch/remotr/internal/identity"
	"github.com/DavidHoenisch/remotr/internal/rbac"
)

type operatorContextKey struct{}

func operatorIDFromContext(ctx context.Context) string {
	id, _ := ctx.Value(operatorContextKey{}).(string)
	return id
}

func (s *Server) requirePermission(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if s.cfg.RBAC == nil {
			next.ServeHTTP(w, r)
			return
		}

		cert := peerCert(r)
		operatorID, err := identity.OperatorIDFromCert(cert)
		if err != nil {
			http.Error(w, "forbidden", http.StatusForbidden)
			return
		}

		allowed, err := s.cfg.RBAC.Authorize(r.Context(), operatorID, r.Method, r.URL.Path)
		if err != nil || !allowed {
			annotateAudit(r, audit.ActionAuthzDenied, "operator", operatorID, map[string]any{
				"method": r.Method,
				"path":   r.URL.Path,
			})
			http.Error(w, "forbidden", http.StatusForbidden)
			return
		}

		next.ServeHTTP(w, r.WithContext(context.WithValue(r.Context(), operatorContextKey{}, operatorID)))
	})
}

func (s *Server) authorizeOperatorRequest(r *http.Request, method, path string) error {
	if s.cfg.RBAC == nil {
		return nil
	}
	cert := peerCert(r)
	operatorID, err := identity.OperatorIDFromCert(cert)
	if err != nil {
		return errNoClientCert
	}
	allowed, err := s.cfg.RBAC.Authorize(r.Context(), operatorID, method, path)
	if err != nil || !allowed {
		return errNoClientCert
	}
	return nil
}

type operatorMeResponse struct {
	OperatorID string   `json:"operator_id"`
	Roles      []string `json:"roles"`
}

func (s *Server) handleOperatorMe(w http.ResponseWriter, r *http.Request) {
	cert := peerCert(r)
	operatorID, err := identity.OperatorIDFromCert(cert)
	if err != nil {
		http.Error(w, "forbidden", http.StatusForbidden)
		return
	}
	roles := []string{}
	if s.cfg.RBAC != nil {
		roles, err = s.cfg.RBAC.OperatorRoles(r.Context(), operatorID)
		if err != nil {
			http.Error(w, "lookup failed", http.StatusInternalServerError)
			return
		}
	}
	writeJSON(w, operatorMeResponse{OperatorID: operatorID, Roles: roles})
}

type rbacRoleResponse struct {
	Name        string      `json:"name"`
	Description string      `json:"description"`
	BuiltIn     bool        `json:"built_in"`
	Rules       []rbac.Rule `json:"rules"`
}

func (s *Server) handleListRBACRoles(w http.ResponseWriter, r *http.Request) {
	if s.cfg.RBAC == nil {
		http.Error(w, "rbac unavailable", http.StatusServiceUnavailable)
		return
	}
	roles, err := s.cfg.RBAC.ListRBACRoles(r.Context())
	if err != nil {
		http.Error(w, "list failed", http.StatusInternalServerError)
		return
	}
	out := make([]rbacRoleResponse, 0, len(roles))
	for _, role := range roles {
		out = append(out, rbacRoleFromDomain(role))
	}
	writeJSON(w, out)
}

func (s *Server) handleGetRBACRole(w http.ResponseWriter, r *http.Request) {
	if s.cfg.RBAC == nil {
		http.Error(w, "rbac unavailable", http.StatusServiceUnavailable)
		return
	}
	name := chi.URLParam(r, "name")
	role, err := s.cfg.RBAC.GetRBACRole(r.Context(), name)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			http.Error(w, "not found", http.StatusNotFound)
			return
		}
		http.Error(w, "lookup failed", http.StatusInternalServerError)
		return
	}
	writeJSON(w, rbacRoleFromDomain(role))
}

type createRBACRoleRequest struct {
	Name        string `json:"name"`
	Description string `json:"description"`
}

func (s *Server) handleCreateRBACRole(w http.ResponseWriter, r *http.Request) {
	if s.cfg.RBAC == nil {
		http.Error(w, "rbac unavailable", http.StatusServiceUnavailable)
		return
	}
	var req createRBACRoleRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil && err != io.EOF {
		http.Error(w, "bad request", http.StatusBadRequest)
		return
	}
	if err := s.cfg.RBAC.CreateRBACRole(r.Context(), req.Name, req.Description); err != nil {
		http.Error(w, "create failed", http.StatusBadRequest)
		return
	}
	annotateAudit(r, audit.ActionRBACRoleCreate, "rbac_role", req.Name, nil)
	role, err := s.cfg.RBAC.GetRBACRole(r.Context(), req.Name)
	if err != nil {
		http.Error(w, "lookup failed", http.StatusInternalServerError)
		return
	}
	writeJSON(w, rbacRoleFromDomain(role))
}

func (s *Server) handleDeleteRBACRole(w http.ResponseWriter, r *http.Request) {
	if s.cfg.RBAC == nil {
		http.Error(w, "rbac unavailable", http.StatusServiceUnavailable)
		return
	}
	name := chi.URLParam(r, "name")
	if err := s.cfg.RBAC.DeleteRBACRole(r.Context(), name); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			http.Error(w, "not found", http.StatusNotFound)
			return
		}
		http.Error(w, "delete failed", http.StatusBadRequest)
		return
	}
	annotateAudit(r, audit.ActionRBACRoleDelete, "rbac_role", name, nil)
	w.WriteHeader(http.StatusNoContent)
}

type createRBACRuleRequest struct {
	Method      string `json:"method"`
	PathPattern string `json:"path_pattern"`
}

func (s *Server) handleCreateRBACRule(w http.ResponseWriter, r *http.Request) {
	if s.cfg.RBAC == nil {
		http.Error(w, "rbac unavailable", http.StatusServiceUnavailable)
		return
	}
	roleName := chi.URLParam(r, "name")
	var req createRBACRuleRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil && err != io.EOF {
		http.Error(w, "bad request", http.StatusBadRequest)
		return
	}
	rule, err := s.cfg.RBAC.AddRBACRule(r.Context(), roleName, rbac.Rule{
		Method:      req.Method,
		PathPattern: req.PathPattern,
	})
	if err != nil {
		http.Error(w, "create failed", http.StatusBadRequest)
		return
	}
	annotateAudit(r, audit.ActionRBACRuleCreate, "rbac_role", roleName, map[string]any{
		"rule_id":      rule.ID,
		"method":       rule.Method,
		"path_pattern": rule.PathPattern,
	})
	writeJSON(w, rule)
}

func (s *Server) handleDeleteRBACRule(w http.ResponseWriter, r *http.Request) {
	if s.cfg.RBAC == nil {
		http.Error(w, "rbac unavailable", http.StatusServiceUnavailable)
		return
	}
	roleName := chi.URLParam(r, "name")
	ruleID := chi.URLParam(r, "ruleID")
	if err := s.cfg.RBAC.RemoveRBACRule(r.Context(), roleName, ruleID); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			http.Error(w, "not found", http.StatusNotFound)
			return
		}
		http.Error(w, "delete failed", http.StatusBadRequest)
		return
	}
	annotateAudit(r, audit.ActionRBACRuleDelete, "rbac_role", roleName, map[string]any{"rule_id": ruleID})
	w.WriteHeader(http.StatusNoContent)
}

func (s *Server) handleListOperators(w http.ResponseWriter, r *http.Request) {
	if s.cfg.RBAC == nil {
		http.Error(w, "rbac unavailable", http.StatusServiceUnavailable)
		return
	}
	ops, err := s.cfg.RBAC.ListOperators(r.Context())
	if err != nil {
		http.Error(w, "list failed", http.StatusInternalServerError)
		return
	}
	writeJSON(w, ops)
}

type setOperatorRolesRequest struct {
	Roles []string `json:"roles"`
}

func (s *Server) handleSetOperatorRoles(w http.ResponseWriter, r *http.Request) {
	if s.cfg.RBAC == nil {
		http.Error(w, "rbac unavailable", http.StatusServiceUnavailable)
		return
	}
	operatorID := chi.URLParam(r, "operator_id")
	var req setOperatorRolesRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil && err != io.EOF {
		http.Error(w, "bad request", http.StatusBadRequest)
		return
	}
	if err := s.cfg.RBAC.SetOperatorRoles(r.Context(), operatorID, req.Roles); err != nil {
		http.Error(w, "update failed", http.StatusBadRequest)
		return
	}
	annotateAudit(r, audit.ActionRBACOperatorRolesSet, "operator", operatorID, map[string]any{"roles": req.Roles})
	w.WriteHeader(http.StatusNoContent)
}

func rbacRoleFromDomain(role rbac.Role) rbacRoleResponse {
	return rbacRoleResponse{
		Name:        role.Name,
		Description: role.Description,
		BuiltIn:     role.BuiltIn,
		Rules:       role.Rules,
	}
}
