package server

import (
	"encoding/json"
	"errors"
	"io"
	"log/slog"
	"net/http"
	"strings"
	"time"

	"github.com/DavidHoenisch/remotr/internal/apppackages"
	"github.com/DavidHoenisch/remotr/internal/audit"
)

type createAppPackageRequest struct {
	Name     string              `json:"name"`
	Version  string              `json:"version"`
	S3Key    string              `json:"s3_key"`
	SHA256   string              `json:"sha256"`
	Manifest apppackages.Manifest `json:"manifest"`
}

type appPackageResponse struct {
	ID        string              `json:"id"`
	Name      string              `json:"name"`
	Version   string              `json:"version"`
	S3Key     string              `json:"s3_key"`
	SHA256    string              `json:"sha256"`
	Manifest  apppackages.Manifest `json:"manifest"`
	CreatedAt time.Time           `json:"created_at"`
}

type appPackageDownloadURLRequest struct {
	Name    string `json:"name"`
	Version string `json:"version"`
}

type appPackageDownloadURLResponse struct {
	URL       string    `json:"url"`
	SHA256    string    `json:"sha256"`
	ExpiresAt time.Time `json:"expires_at"`
}

func (s *Server) handleCreateAppPackage(w http.ResponseWriter, r *http.Request) {
	if s.cfg.AppPackages == nil {
		http.Error(w, "app packages unavailable", http.StatusServiceUnavailable)
		return
	}
	var req createAppPackageRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil && err != io.EOF {
		http.Error(w, "bad request", http.StatusBadRequest)
		return
	}
	req.Name = strings.TrimSpace(req.Name)
	req.Version = strings.TrimSpace(req.Version)
	req.S3Key = strings.TrimSpace(req.S3Key)
	req.SHA256 = strings.TrimSpace(strings.ToLower(req.SHA256))
	if req.Name == "" || req.Version == "" || req.S3Key == "" || req.SHA256 == "" {
		http.Error(w, "name, version, s3_key, and sha256 required", http.StatusBadRequest)
		return
	}
	if err := apppackages.ValidateManifest(req.Manifest); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	if req.Manifest.Name != req.Name || req.Manifest.Version != req.Version {
		http.Error(w, "manifest name/version must match request", http.StatusBadRequest)
		return
	}

	rec, err := s.cfg.AppPackages.Create(r.Context(), apppackages.PackageRecord{
		Name:     req.Name,
		Version:  req.Version,
		S3Key:    req.S3Key,
		SHA256:   req.SHA256,
		Manifest: req.Manifest,
	})
	if err != nil {
		if errors.Is(err, apppackages.ErrAlreadyExists) {
			http.Error(w, "package already exists", http.StatusConflict)
			return
		}
		http.Error(w, "create failed", http.StatusInternalServerError)
		return
	}

	annotateAudit(r, audit.ActionAdminAppPackageCreate, "app_package", rec.Name+"/"+rec.Version, map[string]any{
		"s3_key": rec.S3Key,
		"sha256": rec.SHA256,
	})
	writeJSON(w, appPackageToResponse(rec))
}

func (s *Server) handleListAppPackages(w http.ResponseWriter, r *http.Request) {
	if s.cfg.AppPackages == nil {
		http.Error(w, "app packages unavailable", http.StatusServiceUnavailable)
		return
	}
	prefix := strings.TrimSpace(r.URL.Query().Get("name"))
	recs, err := s.cfg.AppPackages.List(r.Context(), prefix)
	if err != nil {
		http.Error(w, "list failed", http.StatusInternalServerError)
		return
	}
	out := make([]appPackageResponse, 0, len(recs))
	for _, rec := range recs {
		out = append(out, appPackageToResponse(rec))
	}
	writeJSON(w, out)
}

func (s *Server) handleGetAppPackage(w http.ResponseWriter, r *http.Request) {
	if s.cfg.AppPackages == nil {
		http.Error(w, "app packages unavailable", http.StatusServiceUnavailable)
		return
	}
	name := strings.TrimSpace(r.URL.Query().Get("name"))
	version := strings.TrimSpace(r.URL.Query().Get("version"))
	if name == "" || version == "" {
		http.Error(w, "name and version query params required", http.StatusBadRequest)
		return
	}
	rec, err := s.cfg.AppPackages.Get(r.Context(), name, version)
	if err != nil {
		if errors.Is(err, apppackages.ErrNotFound) {
			http.Error(w, "not found", http.StatusNotFound)
			return
		}
		http.Error(w, "lookup failed", http.StatusInternalServerError)
		return
	}
	writeJSON(w, appPackageToResponse(rec))
}

func (s *Server) handleDeleteAppPackage(w http.ResponseWriter, r *http.Request) {
	if s.cfg.AppPackages == nil {
		http.Error(w, "app packages unavailable", http.StatusServiceUnavailable)
		return
	}
	name := strings.TrimSpace(r.URL.Query().Get("name"))
	version := strings.TrimSpace(r.URL.Query().Get("version"))
	if name == "" || version == "" {
		http.Error(w, "name and version query params required", http.StatusBadRequest)
		return
	}
	deleteObject := strings.EqualFold(r.URL.Query().Get("delete_object"), "true")

	rec, err := s.cfg.AppPackages.Get(r.Context(), name, version)
	if err != nil {
		if errors.Is(err, apppackages.ErrNotFound) {
			http.Error(w, "not found", http.StatusNotFound)
			return
		}
		http.Error(w, "lookup failed", http.StatusInternalServerError)
		return
	}

	if err := s.cfg.AppPackages.Delete(r.Context(), name, version); err != nil {
		if errors.Is(err, apppackages.ErrNotFound) {
			http.Error(w, "not found", http.StatusNotFound)
			return
		}
		http.Error(w, "delete failed", http.StatusInternalServerError)
		return
	}

	if deleteObject && s.cfg.AppPackageBlobs != nil {
		if err := s.cfg.AppPackageBlobs.DeleteObject(r.Context(), rec.S3Key); err != nil {
			slogWarnAppPackageDeleteObject(name, version, err)
		}
	}

	annotateAudit(r, audit.ActionAdminAppPackageDelete, "app_package", name+"/"+version, nil)
	w.WriteHeader(http.StatusNoContent)
}

func (s *Server) handleAppPackageDownloadURL(w http.ResponseWriter, r *http.Request) {
	if s.cfg.AppPackageURLs == nil {
		http.Error(w, "app packages unavailable", http.StatusServiceUnavailable)
		return
	}
	endpointID, err := endpointIDFromRequest(r)
	if err != nil {
		http.Error(w, "unauthorized", http.StatusUnauthorized)
		return
	}

	var req appPackageDownloadURLRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil && err != io.EOF {
		http.Error(w, "bad request", http.StatusBadRequest)
		return
	}
	req.Name = strings.TrimSpace(req.Name)
	req.Version = strings.TrimSpace(req.Version)
	if req.Name == "" || req.Version == "" {
		http.Error(w, "name and version required", http.StatusBadRequest)
		return
	}

	out, err := s.cfg.AppPackageURLs.DownloadURL(r.Context(), req.Name, req.Version)
	if err != nil {
		if errors.Is(err, apppackages.ErrNotFound) {
			http.Error(w, "not found", http.StatusNotFound)
			return
		}
		http.Error(w, "download url failed", http.StatusInternalServerError)
		return
	}

	annotateAudit(r, audit.ActionAgentAppPackageDownload, "endpoint", endpointID, map[string]any{
		"package": req.Name,
		"version": req.Version,
	})
	writeJSON(w, appPackageDownloadURLResponse{
		URL:       out.URL,
		SHA256:    out.SHA256,
		ExpiresAt: out.ExpiresAt,
	})
}

func appPackageToResponse(rec apppackages.PackageRecord) appPackageResponse {
	return appPackageResponse{
		ID:        rec.ID,
		Name:      rec.Name,
		Version:   rec.Version,
		S3Key:     rec.S3Key,
		SHA256:    rec.SHA256,
		Manifest:  rec.Manifest,
		CreatedAt: rec.CreatedAt,
	}
}

func slogWarnAppPackageDeleteObject(name, version string, err error) {
	slog.Warn("app package s3 delete failed", "name", name, "version", version, "err", err)
}
