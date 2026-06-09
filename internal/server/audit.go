package server

import (
	"context"
	"log/slog"
	"net"
	"net/http"
	"strings"
	"time"

	"github.com/go-chi/chi/v5/middleware"

	"github.com/DavidHoenisch/remotr/internal/audit"
	"github.com/DavidHoenisch/remotr/internal/identity"
)

type auditRecorder struct {
	action       string
	resourceType string
	resourceID   string
	details      map[string]any
}

type auditRecorderKey struct{}

type auditResponseWriter struct {
	http.ResponseWriter
	status int
}

func (w *auditResponseWriter) WriteHeader(code int) {
	w.status = code
	w.ResponseWriter.WriteHeader(code)
}

func (w *auditResponseWriter) Write(b []byte) (int, error) {
	if w.status == 0 {
		w.status = http.StatusOK
	}
	return w.ResponseWriter.Write(b)
}

func annotateAudit(r *http.Request, action, resourceType, resourceID string, details map[string]any) {
	rec, _ := r.Context().Value(auditRecorderKey{}).(*auditRecorder)
	if rec == nil {
		return
	}
	rec.action = action
	rec.resourceType = resourceType
	rec.resourceID = resourceID
	rec.details = details
}

func (s *Server) auditMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if s.cfg.AuditLog == nil || !shouldAuditPath(r.URL.Path) {
			next.ServeHTTP(w, r)
			return
		}

		rec := &auditRecorder{}
		ctx := context.WithValue(r.Context(), auditRecorderKey{}, rec)
		arw := &auditResponseWriter{ResponseWriter: w, status: http.StatusOK}
		next.ServeHTTP(arw, r.WithContext(ctx))
		s.recordAudit(r.WithContext(ctx), arw.status, rec)
	})
}

func shouldAuditPath(path string) bool {
	if path == "/healthz" {
		return false
	}
	return strings.HasPrefix(path, "/v1/")
}

func (s *Server) recordAudit(r *http.Request, status int, meta *auditRecorder) {
	if s.cfg.AuditLog == nil {
		return
	}

	action := audit.ActionAPIRequest
	resourceType := ""
	resourceID := ""
	var details map[string]any
	if meta != nil && meta.action != "" {
		action = meta.action
		resourceType = meta.resourceType
		resourceID = meta.resourceID
		details = meta.details
	}

	actorType, actorID, actorFP := actorFromRequest(r)
	event := audit.Event{
		OccurredAt:       time.Now().UTC(),
		RequestID:        middleware.GetReqID(r.Context()),
		ActorType:        actorType,
		ActorID:          actorID,
		ActorFingerprint: actorFP,
		Action:           action,
		Method:           r.Method,
		Path:             r.URL.Path,
		StatusCode:       status,
		ResourceType:     resourceType,
		ResourceID:       resourceID,
		ClientIP:         clientIP(r),
		Details:          details,
	}

	slog.Info("audit event",
		"request_id", event.RequestID,
		"action", event.Action,
		"method", event.Method,
		"path", event.Path,
		"status", event.StatusCode,
		"actor_type", event.ActorType,
		"actor_id", event.ActorID,
	)

	if err := s.cfg.AuditLog.RecordAuditEvent(r.Context(), event); err != nil {
		slog.Warn("persist audit event", "action", event.Action, "path", event.Path, "err", err)
	}
}

func actorFromRequest(r *http.Request) (actorType, actorID, fingerprint string) {
	cert := peerCert(r)
	if cert == nil {
		return audit.ActorAnonymous, "", ""
	}
	fp := identity.Fingerprint(cert)
	if id, err := identity.EndpointIDFromCert(cert); err == nil {
		return audit.ActorEndpoint, id, fp
	}
	if id, err := identity.OperatorIDFromCert(cert); err == nil {
		return audit.ActorOperator, id, fp
	}
	return audit.ActorAnonymous, "", fp
}

func clientIP(r *http.Request) string {
	if host, _, err := net.SplitHostPort(r.RemoteAddr); err == nil {
		return host
	}
	return r.RemoteAddr
}
