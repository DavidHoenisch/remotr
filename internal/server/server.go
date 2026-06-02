package server

import (
	"compress/gzip"
	"context"
	"crypto"
	"crypto/x509"
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"strings"

	"github.com/go-chi/chi/v5"
	"github.com/go-chi/chi/v5/middleware"

	"github.com/DavidHoenisch/remotr/internal/configrepo"
	"github.com/DavidHoenisch/remotr/internal/identity"
	"github.com/DavidHoenisch/remotr/internal/registry"
)

type Config struct {
	ConfigRepoPath string
	ReleaseRef     string
	ReleaseRefSrc  ReleaseRefSource
	Registry       registry.Registry
	Enroller       registry.Enroller
	Admin          registry.Admin
	Bootstrap      *Bootstrap
	FleetSettings  FleetSettings
	Telemetry      SyncTelemetry
	GitWebhookPath string
	GitWebhook     http.Handler
	CACert         *x509.Certificate
	CAKey          crypto.PrivateKey
	CACertPEM      []byte
}

type Server struct {
	cfg Config
}

func New(cfg Config) *Server {
	if cfg.Registry == nil {
		cfg.Registry = registry.NewMemory()
	}
	return &Server{cfg: cfg}
}

type syncRequest struct {
	LastDigest   string                `json:"lastDigest"`
	Labels       map[string]string     `json:"labels,omitempty"`
	Drift        *driftReportPayload   `json:"drift,omitempty"`
	ApplyFailure *applyFailurePayload  `json:"applyFailure,omitempty"`
}

type syncResponse struct {
	Unchanged         bool   `json:"unchanged"`
	ReleaseRef        string `json:"releaseRef,omitempty"`
	Digest            string `json:"digest,omitempty"`
	ArtifactYAML      []byte `json:"artifactYaml,omitempty"`
	RemediationPolicy string `json:"remediationPolicy,omitempty"`
}

func (s *Server) Handler() http.Handler {
	r := chi.NewRouter()
	r.Use(middleware.RequestID)
	r.Get("/healthz", func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("ok"))
	})
	r.Post("/v1/enroll", s.handleEnroll)
	r.With(gzipMiddleware).Post("/v1/sync", s.handleSync)
	r.Post("/v1/admin/bootstrap", s.handleBootstrap)
	r.Group(func(r chi.Router) {
		r.Use(s.requireOperator)
		r.Get("/v1/admin/endpoints", s.handleListEndpoints)
		r.Get("/v1/admin/endpoints/{id}", s.handleGetEndpoint)
		r.Post("/v1/admin/enroll-tokens", s.handleCreateEnrollToken)
	})
	if s.cfg.GitWebhook != nil {
		r.Post("/v1/webhooks/git", s.cfg.GitWebhook.ServeHTTP)
		r.Post("/v1/admin/git-sync", s.cfg.GitWebhook.ServeHTTP)
		if path := s.cfg.GitWebhookPath; path != "" && path != "/v1/webhooks/git" && path != "/v1/admin/git-sync" {
			r.Post(path, s.cfg.GitWebhook.ServeHTTP)
		}
	}
	return r
}

func (s *Server) handleSync(w http.ResponseWriter, r *http.Request) {
	endpointID, err := endpointIDFromRequest(r)
	if err != nil {
		http.Error(w, "unauthorized", http.StatusUnauthorized)
		return
	}

	ep, ok := s.cfg.Registry.EndpointByID(endpointID)
	if !ok {
		http.Error(w, "unknown endpoint", http.StatusForbidden)
		return
	}

	var req syncRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil && err != io.EOF {
		http.Error(w, "bad request", http.StatusBadRequest)
		return
	}

	releaseRef := s.releaseRef(r.Context())
	s.persistTelemetry(r.Context(), endpointID, releaseRef, req)

	artifact, digest, err := configrepo.ResolveArtifact(s.cfg.ConfigRepoPath, ep.Fleet, endpointID)
	if err != nil {
		http.Error(w, "artifact unavailable", http.StatusInternalServerError)
		return
	}

	policy := s.remediationPolicy(r.Context(), ep.Fleet)

	if req.LastDigest == digest {
		writeJSON(w, syncResponse{
			Unchanged:         true,
			ReleaseRef:        releaseRef,
			Digest:            digest,
			RemediationPolicy: policy,
		})
		return
	}

	writeJSON(w, syncResponse{
		ReleaseRef:        releaseRef,
		Digest:            digest,
		ArtifactYAML:      artifact,
		RemediationPolicy: policy,
	})
}

func (s *Server) releaseRef(ctx context.Context) string {
	if s.cfg.ReleaseRefSrc != nil {
		if ref := s.cfg.ReleaseRefSrc.ReleaseRef(ctx); ref != "" {
			return ref
		}
	}
	return s.cfg.ReleaseRef
}

func (s *Server) remediationPolicy(ctx context.Context, fleet string) string {
	if s.cfg.FleetSettings == nil {
		return "auto"
	}
	policy, err := s.cfg.FleetSettings.RemediationPolicy(ctx, fleet)
	if err != nil {
		slog.Warn("remediation policy lookup", "fleet", fleet, "err", err)
		return "auto"
	}
	if policy == "" {
		return "auto"
	}
	return policy
}

func (s *Server) persistTelemetry(ctx context.Context, endpointID, releaseRef string, req syncRequest) {
	if s.cfg.Telemetry == nil {
		return
	}
	if len(req.Labels) > 0 {
		if err := s.cfg.Telemetry.UpsertEndpointLabels(ctx, endpointID, req.Labels); err != nil {
			slog.Warn("persist endpoint labels", "endpoint", endpointID, "err", err)
		}
	}
	if req.Drift != nil && len(req.Drift.Report) > 0 {
		digest := req.Drift.Digest
		if digest == "" {
			digest = req.LastDigest
		}
		if err := s.cfg.Telemetry.InsertDriftReport(ctx, endpointID, releaseRef, digest, req.Drift.Report); err != nil {
			slog.Warn("persist drift report", "endpoint", endpointID, "err", err)
		}
	}
	if req.ApplyFailure != nil && req.ApplyFailure.ResourceAddress != "" {
		if err := s.cfg.Telemetry.InsertApplyFailure(
			ctx,
			endpointID,
			releaseRef,
			req.ApplyFailure.ResourceAddress,
			req.ApplyFailure.Message,
		); err != nil {
			slog.Warn("persist apply failure", "endpoint", endpointID, "err", err)
		}
	}
}

func endpointIDFromRequest(r *http.Request) (string, error) {
	if r.TLS == nil || len(r.TLS.PeerCertificates) == 0 {
		return "", errNoClientCert
	}
	return identity.EndpointIDFromCert(r.TLS.PeerCertificates[0])
}

var errNoClientCert = errString("no client certificate")

type errString string

func (e errString) Error() string { return string(e) }

func writeJSON(w http.ResponseWriter, v any) {
	w.Header().Set("Content-Type", "application/json")
	enc := json.NewEncoder(w)
	_ = enc.Encode(v)
}

func gzipMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !acceptsGzip(r) {
			next.ServeHTTP(w, r)
			return
		}
		w.Header().Set("Content-Encoding", "gzip")
		gz := gzip.NewWriter(w)
		defer gz.Close()
		gzw := &gzipResponseWriter{Writer: gz, ResponseWriter: w}
		next.ServeHTTP(gzw, r)
	})
}

func acceptsGzip(r *http.Request) bool {
	for _, v := range r.Header.Values("Accept-Encoding") {
		if strings.Contains(v, "gzip") {
			return true
		}
	}
	return false
}

type gzipResponseWriter struct {
	io.Writer
	http.ResponseWriter
	wroteHeader bool
}

func (w *gzipResponseWriter) WriteHeader(code int) {
	if !w.wroteHeader {
		w.wroteHeader = true
		w.ResponseWriter.WriteHeader(code)
	}
}

func (w *gzipResponseWriter) Write(b []byte) (int, error) {
	if !w.wroteHeader {
		w.WriteHeader(http.StatusOK)
	}
	return w.Writer.Write(b)
}
