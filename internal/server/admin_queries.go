package server

import (
	"encoding/json"
	"errors"
	"net/http"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"

	"github.com/DavidHoenisch/remotr/internal/audit"
	"github.com/DavidHoenisch/remotr/internal/models"
	"github.com/DavidHoenisch/remotr/internal/registry"
	pgstore "github.com/DavidHoenisch/remotr/internal/store/postgres"
)

func (s *Server) queryAdminRegistry() registry.Admin {
	if s.cfg.Admin != nil {
		return s.cfg.Admin
	}
	if adminReg, ok := s.cfg.Registry.(registry.Admin); ok {
		return adminReg
	}
	return nil
}

type querySummaryItem struct {
	Fleet       string `json:"fleet"`
	Name        string `json:"name"`
	Description string `json:"description,omitempty"`
	Schedule    string `json:"schedule"`
	Mode        string `json:"mode"`
	MaxRows     int    `json:"max_rows"`
	TotalHosts  int    `json:"total_hosts"`
	Reported    int    `json:"reported"`
	Failed      int    `json:"failed"`
	Unsupported int    `json:"unsupported"`
}

type queryRunRequest struct {
	QueryName  string `json:"query_name,omitempty"`
	SQL        string `json:"sql,omitempty"`
	Fleet      string `json:"fleet,omitempty"`
	EndpointID string `json:"endpoint_id,omitempty"`
	TimeoutSec int    `json:"timeout_sec,omitempty"`
	MaxRows    int    `json:"max_rows,omitempty"`
	TTLSec     int    `json:"ttl_sec,omitempty"`
}

type adhocRunResponse struct {
	Request         pgstore.AdhocQueryRequestRecord  `json:"request"`
	ExpectedTargets int                              `json:"expected_targets"`
	Results         []pgstore.AdhocQueryResultRecord `json:"results"`
}

func (s *Server) handleListQueries(w http.ResponseWriter, r *http.Request) {
	fleetFilter := strings.TrimSpace(r.URL.Query().Get("fleet"))
	var eps []registry.Endpoint
	if adminReg := s.queryAdminRegistry(); adminReg != nil {
		var err error
		eps, err = adminReg.ListEndpoints()
		if err != nil {
			http.Error(w, "list endpoints failed", http.StatusInternalServerError)
			return
		}
	}
	fleetHosts := map[string]int{}
	for _, ep := range eps {
		if fleetFilter != "" && ep.Fleet != fleetFilter {
			continue
		}
		fleetHosts[ep.Fleet]++
	}
	if fleetFilter != "" {
		if _, ok := fleetHosts[fleetFilter]; !ok {
			fleetHosts[fleetFilter] = 0
		}
	}

	fleets := make([]string, 0, len(fleetHosts))
	for f := range fleetHosts {
		fleets = append(fleets, f)
	}
	sort.Strings(fleets)

	out := make([]querySummaryItem, 0)
	for _, fleet := range fleets {
		specs, _, ok, err := s.loadResolvedQueries(r.Context(), fleet, "")
		if err != nil || !ok {
			continue
		}
		for _, q := range specs {
			item := querySummaryItem{
				Fleet:       fleet,
				Name:        q.Name,
				Description: q.Description,
				Schedule:    q.Schedule,
				Mode:        string(q.EffectiveMode()),
				MaxRows:     q.EffectiveMaxRows(),
				TotalHosts:  fleetHosts[fleet],
			}
			if statuses, err := s.cfg.QueryStore.ListQueryStatuses(r.Context(), fleet, q.Name); err == nil {
				for _, st := range statuses {
					switch st.Status {
					case "success":
						item.Reported++
					case "failed":
						item.Failed++
					case "unsupported":
						item.Unsupported++
					}
				}
			}
			out = append(out, item)
		}
	}
	writeJSON(w, map[string]any{"queries": out})
}

func (s *Server) handleGetQueryStatus(w http.ResponseWriter, r *http.Request) {
	name := chi.URLParam(r, "name")
	fleet := strings.TrimSpace(r.URL.Query().Get("fleet"))
	statuses, err := s.cfg.QueryStore.ListQueryStatuses(r.Context(), fleet, name)
	if err != nil {
		http.Error(w, "query status lookup failed", http.StatusInternalServerError)
		return
	}
	writeJSON(w, map[string]any{
		"query_name": name,
		"fleet":      fleet,
		"statuses":   statuses,
	})
}

func (s *Server) handleGetQueryResults(w http.ResponseWriter, r *http.Request) {
	name := chi.URLParam(r, "name")
	fleet := strings.TrimSpace(r.URL.Query().Get("fleet"))
	endpointID := strings.TrimSpace(r.URL.Query().Get("endpoint"))
	snapshots, err := s.cfg.QueryStore.ListQuerySnapshots(r.Context(), name, fleet, endpointID)
	if err != nil {
		http.Error(w, "query results lookup failed", http.StatusInternalServerError)
		return
	}
	statuses, _ := s.cfg.QueryStore.ListQueryStatuses(r.Context(), fleet, name)
	writeJSON(w, map[string]any{
		"query_name": name,
		"fleet":      fleet,
		"endpoint":   endpointID,
		"snapshots":  snapshots,
		"statuses":   statuses,
	})
}

func (s *Server) handleGetQueryDiff(w http.ResponseWriter, r *http.Request) {
	name := chi.URLParam(r, "name")
	fleet := strings.TrimSpace(r.URL.Query().Get("fleet"))
	endpointID := strings.TrimSpace(r.URL.Query().Get("endpoint"))
	var since time.Time
	if rawSince := strings.TrimSpace(r.URL.Query().Get("since")); rawSince != "" {
		parsed, err := time.Parse(time.RFC3339, rawSince)
		if err != nil {
			http.Error(w, "invalid since timestamp (want RFC3339)", http.StatusBadRequest)
			return
		}
		since = parsed
	}
	limit := 50
	if rawLimit := strings.TrimSpace(r.URL.Query().Get("limit")); rawLimit != "" {
		if n, err := strconv.Atoi(rawLimit); err == nil && n > 0 && n <= 200 {
			limit = n
		}
	}
	diffs, err := s.cfg.QueryStore.ListQueryDiffs(r.Context(), name, fleet, endpointID, since, limit)
	if err != nil {
		http.Error(w, "query diff lookup failed", http.StatusInternalServerError)
		return
	}
	writeJSON(w, map[string]any{
		"query_name": name,
		"fleet":      fleet,
		"endpoint":   endpointID,
		"diffs":      diffs,
	})
}

func (s *Server) handleRunNamedQuery(w http.ResponseWriter, r *http.Request) {
	s.handleRunAdhocQuery(w, r, false)
}

func (s *Server) handleRunInlineSQLQuery(w http.ResponseWriter, r *http.Request) {
	s.handleRunAdhocQuery(w, r, true)
}

func (s *Server) handleRunAdhocQuery(w http.ResponseWriter, r *http.Request, allowInlineSQL bool) {
	var req queryRunRequest
	if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, 64<<10)).Decode(&req); err != nil {
		http.Error(w, "bad request", http.StatusBadRequest)
		return
	}
	req.Fleet = strings.TrimSpace(req.Fleet)
	req.EndpointID = strings.TrimSpace(req.EndpointID)
	req.QueryName = strings.TrimSpace(req.QueryName)
	req.SQL = strings.TrimSpace(req.SQL)

	if (req.Fleet == "" && req.EndpointID == "") || (req.Fleet != "" && req.EndpointID != "") {
		http.Error(w, "specify exactly one of fleet or endpoint_id", http.StatusBadRequest)
		return
	}

	targetFleet := req.Fleet
	expectedTargets := 0
	if req.EndpointID != "" {
		ep, exists := s.cfg.Registry.EndpointByID(req.EndpointID)
		if !exists {
			http.Error(w, "endpoint not found", http.StatusNotFound)
			return
		}
		targetFleet = ep.Fleet
		expectedTargets = 1
	} else if adminReg := s.queryAdminRegistry(); adminReg != nil {
		eps, err := adminReg.ListEndpoints()
		if err != nil {
			http.Error(w, "list endpoints failed", http.StatusInternalServerError)
			return
		}
		for _, ep := range eps {
			if ep.Fleet == req.Fleet {
				expectedTargets++
			}
		}
	}

	var sqlText string
	timeoutSec := int(models.DefaultQueryTimeout.Seconds())
	maxRows := models.DefaultQueryMaxRows

	if allowInlineSQL {
		if req.SQL == "" {
			http.Error(w, "sql is required", http.StatusBadRequest)
			return
		}
		if err := models.ValidateReadOnlySQL(req.SQL); err != nil {
			http.Error(w, "invalid read-only sql: "+err.Error(), http.StatusBadRequest)
			return
		}
		sqlText = req.SQL
	} else {
		if req.SQL != "" {
			http.Error(w, "inline sql requires /v1/admin/queries/run-sql", http.StatusForbidden)
			return
		}
		if req.QueryName == "" {
			http.Error(w, "query_name is required", http.StatusBadRequest)
			return
		}
		specs, _, ok, err := s.loadResolvedQueries(r.Context(), targetFleet, req.EndpointID)
		if err != nil || !ok {
			http.Error(w, "query not found in fleet", http.StatusNotFound)
			return
		}
		var found *models.QuerySpec
		for i := range specs {
			if specs[i].Name == req.QueryName {
				found = &specs[i]
				break
			}
		}
		if found == nil {
			http.Error(w, "query not found in fleet", http.StatusNotFound)
			return
		}
		sqlText = found.SQL
		timeoutSec = int(found.EffectiveTimeout().Seconds())
		maxRows = found.EffectiveMaxRows()
	}

	if req.TimeoutSec > 0 && req.TimeoutSec <= int(models.MaxQueryTimeout.Seconds()) {
		timeoutSec = req.TimeoutSec
	}
	if req.MaxRows > 0 && req.MaxRows <= models.MaxQueryMaxRows {
		maxRows = req.MaxRows
	}
	ttl := 10 * time.Minute
	if req.TTLSec > 0 && req.TTLSec <= 3600 {
		ttl = time.Duration(req.TTLSec) * time.Second
	}

	now := s.cfg.Now().UTC()
	actor := operatorIDFromContext(r.Context())
	record := pgstore.AdhocQueryRequestRecord{
		RequestID:  uuid.NewString(),
		Actor:      actor,
		Fleet:      req.Fleet,
		EndpointID: req.EndpointID,
		QueryName:  req.QueryName,
		SQL:        sqlText,
		IsInline:   allowInlineSQL,
		TimeoutSec: timeoutSec,
		MaxRows:    maxRows,
		CreatedAt:  now,
		ExpiresAt:  now.Add(ttl),
	}
	if err := s.cfg.QueryStore.CreateAdhocQueryRequest(r.Context(), record); err != nil {
		http.Error(w, "create ad-hoc query request failed", http.StatusInternalServerError)
		return
	}

	action := audit.ActionAdminQueryRun
	if allowInlineSQL {
		action = audit.ActionAdminQueryRunSQL
	}
	targetType, targetID := "fleet", req.Fleet
	if req.EndpointID != "" {
		targetType, targetID = "endpoint", req.EndpointID
	}
	annotateAudit(r, action, targetType, targetID, auditDetails(
		audit.PublicDetail("request_id", record.RequestID),
		audit.PublicDetail("query_name", record.QueryName),
	))

	writeJSON(w, adhocRunResponse{
		Request:         record,
		ExpectedTargets: expectedTargets,
		Results:         []pgstore.AdhocQueryResultRecord{},
	})
}

func (s *Server) handleGetAdhocQueryRun(w http.ResponseWriter, r *http.Request) {
	requestID := chi.URLParam(r, "requestId")
	if strings.TrimSpace(requestID) == "" {
		http.Error(w, "request id required", http.StatusBadRequest)
		return
	}
	req, err := s.cfg.QueryStore.GetAdhocQueryRequest(r.Context(), requestID)
	if err != nil {
		if errors.Is(err, pgstore.ErrAdhocQueryNotFound) {
			http.Error(w, "not found", http.StatusNotFound)
			return
		}
		http.Error(w, "lookup failed", http.StatusInternalServerError)
		return
	}
	results, err := s.cfg.QueryStore.ListAdhocQueryResults(r.Context(), requestID)
	if err != nil {
		http.Error(w, "results lookup failed", http.StatusInternalServerError)
		return
	}
	expected := 1
	if req.EndpointID == "" && req.Fleet != "" {
		expected = 0
		if adminReg := s.queryAdminRegistry(); adminReg != nil {
			if eps, err := adminReg.ListEndpoints(); err == nil {
				for _, ep := range eps {
					if ep.Fleet == req.Fleet {
						expected++
					}
				}
			}
		}
	}
	writeJSON(w, adhocRunResponse{
		Request:         req,
		ExpectedTargets: expected,
		Results:         results,
	})
}
