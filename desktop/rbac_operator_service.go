package main

import (
	"context"
	"errors"
	"fmt"
	"path/filepath"
	"slices"
	"strings"
	"sync"
	"unicode"

	"github.com/DavidHoenisch/remotr/internal/admin"
	"github.com/DavidHoenisch/remotr/internal/rbac"
	"github.com/google/uuid"
	wailsruntime "github.com/wailsapp/wails/v2/pkg/runtime"
)

const (
	rbacRoleViewLimit     = 2_000
	rbacOperatorViewLimit = 10_000
)

type RBACRuleView struct {
	ID          string `json:"id"`
	RoleName    string `json:"roleName"`
	Method      string `json:"method"`
	PathPattern string `json:"pathPattern"`
}

type RBACRoleView struct {
	Name        string         `json:"name"`
	Description string         `json:"description"`
	BuiltIn     bool           `json:"builtIn"`
	Rules       []RBACRuleView `json:"rules"`
}

type RBACOperatorView struct {
	ID              string   `json:"id"`
	CertFingerprint string   `json:"certFingerprint"`
	Roles           []string `json:"roles"`
	CreatedAt       string   `json:"createdAt"`
}

type RBACRoleCreateRequest struct {
	Name        string `json:"name"`
	Description string `json:"description"`
}

type RBACRoleDeleteRequest struct {
	Name         string `json:"name"`
	Confirmation string `json:"confirmation"`
}

type RBACRuleAddRequest struct {
	RoleName    string `json:"roleName"`
	Method      string `json:"method"`
	PathPattern string `json:"pathPattern"`
}

type RBACRuleRemoveRequest struct {
	RoleName     string `json:"roleName"`
	RuleID       string `json:"ruleId"`
	Confirmation string `json:"confirmation"`
}

type OperatorRolesRequest struct {
	OperatorID   string   `json:"operatorId"`
	Roles        []string `json:"roles"`
	Confirmation string   `json:"confirmation"`
}

type OperatorCredentialStampRequest struct {
	Label        string   `json:"label"`
	Roles        []string `json:"roles"`
	Confirmation string   `json:"confirmation"`
}

type RBACMutationResult struct {
	Name   string `json:"name"`
	RuleID string `json:"ruleId"`
	Status string `json:"status"`
}

type OperatorCredentialStampResult struct {
	OperatorID    string   `json:"operatorId"`
	Label         string   `json:"label"`
	Roles         []string `json:"roles"`
	DirectoryName string   `json:"directoryName"`
	Status        string   `json:"status"`
}

type OperatorCredentialDirectoryDialog func(context.Context) (string, error)

type RBACOperatorService struct {
	mu                sync.Mutex
	chooseDestination OperatorCredentialDirectoryDialog
	persist           CredentialPersistence
	inflight          map[string]bool
}

func NewRBACOperatorService(dialog OperatorCredentialDirectoryDialog, persist CredentialPersistence) *RBACOperatorService {
	return &RBACOperatorService{chooseDestination: dialog, persist: persist, inflight: map[string]bool{}}
}

func defaultRBACOperatorService() *RBACOperatorService {
	return NewRBACOperatorService(func(ctx context.Context) (string, error) {
		return wailsruntime.OpenDirectoryDialog(ctx, wailsruntime.OpenDialogOptions{
			Title: "Choose empty Operator credential directory",
		})
	}, persistCredentialSet)
}

func (s *RBACOperatorService) ListRoles(ctx context.Context, client *admin.Client) ([]RBACRoleView, error) {
	if client == nil {
		return nil, ErrSessionNotConnected
	}
	records, err := client.ListRBACRolesContext(ctx)
	if err != nil {
		return nil, err
	}
	if len(records) > rbacRoleViewLimit {
		return nil, errors.New("RBAC role inventory exceeds the supported limit")
	}
	views := make([]RBACRoleView, 0, len(records))
	for _, record := range records {
		view, mapErr := mapRBACRoleView(record)
		if mapErr != nil {
			return nil, mapErr
		}
		views = append(views, view)
	}
	slices.SortFunc(views, func(left, right RBACRoleView) int { return strings.Compare(left.Name, right.Name) })
	return views, nil
}

func (s *RBACOperatorService) GetRole(ctx context.Context, client *admin.Client, name string) (RBACRoleView, error) {
	if client == nil {
		return RBACRoleView{}, ErrSessionNotConnected
	}
	if err := validateDesktopRoleName(name); err != nil {
		return RBACRoleView{}, err
	}
	record, err := client.GetRBACRoleContext(ctx, name)
	if err != nil {
		return RBACRoleView{}, err
	}
	view, err := mapRBACRoleView(record)
	if err != nil {
		return RBACRoleView{}, err
	}
	if view.Name != name {
		return RBACRoleView{}, errors.New("server returned a different RBAC role identity")
	}
	return view, nil
}

func (s *RBACOperatorService) CreateRole(ctx context.Context, client *admin.Client, request RBACRoleCreateRequest) (RBACRoleView, error) {
	if client == nil {
		return RBACRoleView{}, ErrSessionNotConnected
	}
	if err := validateDesktopRoleName(request.Name); err != nil {
		return RBACRoleView{}, err
	}
	if rbac.IsBuiltInRole(request.Name) {
		return RBACRoleView{}, rbacValidationFailure("Choose a custom role name that is not reserved by Remotr.")
	}
	if err := validateRoleDescription(request.Description); err != nil {
		return RBACRoleView{}, err
	}
	release, err := s.begin("create-role\x00" + request.Name)
	if err != nil {
		return RBACRoleView{}, err
	}
	defer release()
	record, err := client.CreateRBACRoleContext(ctx, request.Name, request.Description)
	if err != nil {
		return RBACRoleView{}, err
	}
	view, err := mapRBACRoleView(record)
	if err != nil {
		return RBACRoleView{}, err
	}
	if view.Name != request.Name || view.BuiltIn {
		return RBACRoleView{}, errors.New("server returned inconsistent created RBAC role metadata")
	}
	return view, nil
}

func (s *RBACOperatorService) DeleteRole(ctx context.Context, client *admin.Client, request RBACRoleDeleteRequest) (RBACMutationResult, error) {
	role, err := s.GetRole(ctx, client, request.Name)
	if err != nil {
		return RBACMutationResult{}, err
	}
	if role.BuiltIn {
		return RBACMutationResult{}, rbacValidationFailure("Built-in Remotr roles cannot be deleted.")
	}
	if request.Confirmation != request.Name+" DELETE ROLE" {
		return RBACMutationResult{}, rbacValidationFailure("Type the exact role name followed by DELETE ROLE.")
	}
	release, err := s.begin("delete-role\x00" + request.Name)
	if err != nil {
		return RBACMutationResult{}, err
	}
	defer release()
	if err := client.DeleteRBACRoleContext(ctx, request.Name); err != nil {
		return RBACMutationResult{}, err
	}
	return RBACMutationResult{Name: request.Name, Status: "deleted"}, nil
}

func (s *RBACOperatorService) AddRule(ctx context.Context, client *admin.Client, request RBACRuleAddRequest) (RBACRuleView, error) {
	role, err := s.GetRole(ctx, client, request.RoleName)
	if err != nil {
		return RBACRuleView{}, err
	}
	if role.BuiltIn {
		return RBACRuleView{}, rbacValidationFailure("Built-in Remotr role rules are fixed.")
	}
	method, pathPattern, err := validateDesktopRBACRule(request.Method, request.PathPattern)
	if err != nil {
		return RBACRuleView{}, err
	}
	release, err := s.begin("add-rule\x00" + request.RoleName)
	if err != nil {
		return RBACRuleView{}, err
	}
	defer release()
	record, err := client.AddRBACRuleContext(ctx, request.RoleName, method, pathPattern)
	if err != nil {
		return RBACRuleView{}, err
	}
	view, err := mapRBACRuleView(request.RoleName, record, false)
	if err != nil {
		return RBACRuleView{}, err
	}
	if view.Method != method || view.PathPattern != pathPattern {
		return RBACRuleView{}, errors.New("server returned different RBAC rule metadata")
	}
	return view, nil
}

func (s *RBACOperatorService) RemoveRule(ctx context.Context, client *admin.Client, request RBACRuleRemoveRequest) (RBACMutationResult, error) {
	role, err := s.GetRole(ctx, client, request.RoleName)
	if err != nil {
		return RBACMutationResult{}, err
	}
	if role.BuiltIn {
		return RBACMutationResult{}, rbacValidationFailure("Built-in Remotr role rules are fixed.")
	}
	if _, err := uuid.Parse(request.RuleID); err != nil {
		return RBACMutationResult{}, rbacValidationFailure("Select an exact current rule before removing it.")
	}
	if !slices.ContainsFunc(role.Rules, func(rule RBACRuleView) bool { return rule.ID == request.RuleID }) {
		return RBACMutationResult{}, rbacValidationFailure("Select an exact current rule from the role.")
	}
	want := request.RoleName + "/" + request.RuleID + " REMOVE RULE"
	if request.Confirmation != want {
		return RBACMutationResult{}, rbacValidationFailure("Type the exact role/rule identity followed by REMOVE RULE.")
	}
	release, err := s.begin("remove-rule\x00" + request.RoleName + "\x00" + request.RuleID)
	if err != nil {
		return RBACMutationResult{}, err
	}
	defer release()
	if err := client.DeleteRBACRuleContext(ctx, request.RoleName, request.RuleID); err != nil {
		return RBACMutationResult{}, err
	}
	return RBACMutationResult{Name: request.RoleName, RuleID: request.RuleID, Status: "removed"}, nil
}

func (s *RBACOperatorService) ListOperators(ctx context.Context, client *admin.Client) ([]RBACOperatorView, error) {
	if client == nil {
		return nil, ErrSessionNotConnected
	}
	records, err := client.ListOperatorsContext(ctx)
	if err != nil {
		return nil, err
	}
	if len(records) > rbacOperatorViewLimit {
		return nil, errors.New("Operator inventory exceeds the supported limit")
	}
	views := make([]RBACOperatorView, 0, len(records))
	for _, record := range records {
		view, mapErr := mapRBACOperatorView(record)
		if mapErr != nil {
			return nil, mapErr
		}
		views = append(views, view)
	}
	slices.SortFunc(views, func(left, right RBACOperatorView) int { return strings.Compare(left.ID, right.ID) })
	return views, nil
}

func (s *RBACOperatorService) SetOperatorRoles(ctx context.Context, client *admin.Client, request OperatorRolesRequest) (RBACOperatorView, error) {
	if err := validateOperatorID(request.OperatorID); err != nil {
		return RBACOperatorView{}, err
	}
	roles, err := s.validatedRoleSelection(ctx, client, request.Roles)
	if err != nil {
		return RBACOperatorView{}, err
	}
	operators, err := s.ListOperators(ctx, client)
	if err != nil {
		return RBACOperatorView{}, err
	}
	index := slices.IndexFunc(operators, func(operator RBACOperatorView) bool { return operator.ID == request.OperatorID })
	if index < 0 {
		return RBACOperatorView{}, rbacValidationFailure("Select an existing Operator from the current server.")
	}
	if request.Confirmation != request.OperatorID+" SET ROLES" {
		return RBACOperatorView{}, rbacValidationFailure("Type the exact Operator ID followed by SET ROLES.")
	}
	release, err := s.begin("set-roles\x00" + request.OperatorID)
	if err != nil {
		return RBACOperatorView{}, err
	}
	defer release()
	if err := client.SetOperatorRolesContext(ctx, request.OperatorID, roles); err != nil {
		return RBACOperatorView{}, err
	}
	updated := operators[index]
	updated.Roles = slices.Clone(roles)
	return updated, nil
}

func (s *RBACOperatorService) StampCredential(ctx context.Context, client *admin.Client, request OperatorCredentialStampRequest) (OperatorCredentialStampResult, error) {
	if client == nil {
		return OperatorCredentialStampResult{}, ErrSessionNotConnected
	}
	if err := validateCredentialLabel(request.Label); err != nil {
		return OperatorCredentialStampResult{}, err
	}
	roles, err := s.validatedRoleSelection(ctx, client, request.Roles)
	if err != nil {
		return OperatorCredentialStampResult{}, err
	}
	if request.Confirmation != request.Label+" ISSUE CREDENTIAL" {
		return OperatorCredentialStampResult{}, rbacValidationFailure("Type the exact label followed by ISSUE CREDENTIAL.")
	}
	if s == nil || s.chooseDestination == nil || s.persist == nil {
		return OperatorCredentialStampResult{}, errors.New("native protected credential output is unavailable")
	}
	release, err := s.begin("credential\x00" + request.Label)
	if err != nil {
		return OperatorCredentialStampResult{}, err
	}
	defer release()
	destination, err := s.chooseDestination(ctx)
	if err != nil {
		return OperatorCredentialStampResult{}, fmt.Errorf("choose Operator credential directory: %w", err)
	}
	if destination == "" {
		return OperatorCredentialStampResult{}, nil
	}
	if !filepath.IsAbs(destination) || filepath.Clean(destination) != destination {
		return OperatorCredentialStampResult{}, errors.New("native credential destination must be clean and absolute")
	}
	response, err := client.CreateOperatorCredentialContext(ctx, request.Label, roles)
	if err != nil {
		return OperatorCredentialStampResult{}, err
	}
	defer func() {
		response.CertPEM = ""
		response.KeyPEM = ""
		response.CAPEM = ""
	}()
	if response.OperatorID == "" || response.Label != request.Label || !slices.Equal(response.Roles, roles) {
		return OperatorCredentialStampResult{}, errors.New("server returned inconsistent Operator credential metadata")
	}
	if cause := context.Cause(ctx); cause != nil {
		return OperatorCredentialStampResult{}, cause
	}
	if err := s.persist(destination, response.OperatorID, response.CertPEM, response.KeyPEM, response.CAPEM); err != nil {
		cleanupCredentialFragments(destination)
		return OperatorCredentialStampResult{}, errors.New("protected Operator credential output could not be saved")
	}
	return OperatorCredentialStampResult{
		OperatorID: response.OperatorID, Label: response.Label, Roles: slices.Clone(response.Roles),
		DirectoryName: filepath.Base(destination), Status: "saved",
	}, nil
}

func (s *RBACOperatorService) validatedRoleSelection(ctx context.Context, client *admin.Client, selected []string) ([]string, error) {
	if len(selected) == 0 || len(selected) > 64 {
		return nil, rbacValidationFailure("Select one or more current canonical roles.")
	}
	roles, err := s.ListRoles(ctx, client)
	if err != nil {
		return nil, err
	}
	available := make(map[string]struct{}, len(roles))
	for _, role := range roles {
		available[role.Name] = struct{}{}
	}
	canonical := slices.Clone(selected)
	for _, name := range canonical {
		if err := validateDesktopRoleName(name); err != nil {
			return nil, err
		}
		if _, ok := available[name]; !ok {
			return nil, rbacValidationFailure("Select only roles returned by the current server.")
		}
	}
	slices.Sort(canonical)
	canonical = slices.Compact(canonical)
	if len(canonical) != len(selected) {
		return nil, rbacValidationFailure("Each selected role must appear only once.")
	}
	return canonical, nil
}

func (s *RBACOperatorService) begin(key string) (func(), error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.inflight[key] {
		return nil, &ActionFailure{Kind: ActionConflict, Message: "The RBAC action is already in progress.", Guidance: "Wait for the current action to finish before retrying.", Retryable: false}
	}
	s.inflight[key] = true
	return func() {
		s.mu.Lock()
		delete(s.inflight, key)
		s.mu.Unlock()
	}, nil
}

func (s *RBACOperatorService) Clear() {
	if s == nil {
		return
	}
	s.mu.Lock()
	clear(s.inflight)
	s.mu.Unlock()
}

func mapRBACRoleView(record admin.RBACRole) (RBACRoleView, error) {
	if err := rbac.ValidateRoleName(record.Name); err != nil || len(record.Description) > 4_096 {
		return RBACRoleView{}, errors.New("server returned invalid RBAC role metadata")
	}
	rules := make([]RBACRuleView, 0, len(record.Rules))
	if len(record.Rules) > 5_000 {
		return RBACRoleView{}, errors.New("RBAC role rule inventory exceeds the supported limit")
	}
	for _, recordRule := range record.Rules {
		view, err := mapRBACRuleView(record.Name, recordRule, record.BuiltIn)
		if err != nil {
			return RBACRoleView{}, err
		}
		rules = append(rules, view)
	}
	slices.SortFunc(rules, func(left, right RBACRuleView) int {
		if order := strings.Compare(left.Method, right.Method); order != 0 {
			return order
		}
		return strings.Compare(left.PathPattern, right.PathPattern)
	})
	return RBACRoleView{Name: record.Name, Description: record.Description, BuiltIn: record.BuiltIn, Rules: rules}, nil
}

func mapRBACRuleView(roleName string, record admin.RBACRule, builtIn bool) (RBACRuleView, error) {
	method, pattern, err := validateDesktopRBACRule(record.Method, record.PathPattern)
	if err != nil {
		return RBACRuleView{}, errors.New("server returned invalid RBAC rule metadata")
	}
	if !builtIn {
		if _, err := uuid.Parse(record.ID); err != nil {
			return RBACRuleView{}, errors.New("server returned invalid RBAC rule identity")
		}
	} else if record.ID != "" {
		if _, err := uuid.Parse(record.ID); err != nil {
			return RBACRuleView{}, errors.New("server returned invalid built-in RBAC rule identity")
		}
	}
	return RBACRuleView{ID: record.ID, RoleName: roleName, Method: method, PathPattern: pattern}, nil
}

func mapRBACOperatorView(record admin.OperatorInfo) (RBACOperatorView, error) {
	if err := validateOperatorID(record.ID); err != nil || !validSHA256(record.CertFingerprint) || record.CreatedAt.IsZero() || len(record.Roles) > 64 {
		return RBACOperatorView{}, errors.New("server returned invalid Operator metadata")
	}
	roles := slices.Clone(record.Roles)
	for _, role := range roles {
		if err := rbac.ValidateRoleName(role); err != nil {
			return RBACOperatorView{}, errors.New("server returned invalid Operator role metadata")
		}
	}
	slices.Sort(roles)
	if len(slices.Compact(slices.Clone(roles))) != len(roles) {
		return RBACOperatorView{}, errors.New("server returned duplicate Operator role metadata")
	}
	return RBACOperatorView{ID: record.ID, CertFingerprint: record.CertFingerprint, Roles: roles, CreatedAt: formatTimestamp(record.CreatedAt)}, nil
}

func validateDesktopRoleName(name string) error {
	if strings.TrimSpace(name) != name || len(name) > 128 || rbac.ValidateRoleName(name) != nil {
		return rbacValidationFailure("Enter a valid canonical role name without spaces or slashes.")
	}
	return nil
}

func validateRoleDescription(description string) error {
	if len(description) > 4_096 || strings.ContainsAny(description, "\x00\r\n") {
		return rbacValidationFailure("Keep the role description on one line and under 4,096 bytes.")
	}
	return nil
}

func validateDesktopRBACRule(method, pathPattern string) (string, string, error) {
	method = strings.ToUpper(method)
	if !slices.Contains([]string{"*", "GET", "POST", "PUT", "PATCH", "DELETE"}, method) {
		return "", "", rbacValidationFailure("Choose a supported Admin API method or *.")
	}
	if pathPattern == "" || strings.TrimSpace(pathPattern) != pathPattern || len(pathPattern) > 2_048 || strings.ContainsAny(pathPattern, "\x00\r\n?#") ||
		(!strings.HasPrefix(pathPattern, "/v1/admin/") && !strings.HasPrefix(pathPattern, "/v1/exports/audit/")) {
		return "", "", rbacValidationFailure("Enter a bounded Admin or audit-export API path pattern.")
	}
	return method, pathPattern, nil
}

func validateOperatorID(value string) error {
	if value == "" || strings.TrimSpace(value) != value || len(value) > 256 || strings.IndexFunc(value, unicode.IsControl) >= 0 {
		return rbacValidationFailure("Select an exact current Operator ID.")
	}
	return nil
}

func validateCredentialLabel(value string) error {
	if value == "" || strings.TrimSpace(value) != value || len(value) > 128 || strings.IndexFunc(value, unicode.IsControl) >= 0 {
		return rbacValidationFailure("Enter a short case-sensitive credential label.")
	}
	return nil
}

func rbacValidationFailure(guidance string) error {
	return &ActionFailure{Kind: ActionValidation, Message: "The RBAC action input is invalid.", Guidance: guidance, Retryable: false}
}
