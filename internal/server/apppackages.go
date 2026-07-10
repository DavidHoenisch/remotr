package server

import (
	"bytes"
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
	Name     string               `json:"name"`
	Version  string               `json:"version"`
	S3Key    string               `json:"s3_key"`
	SHA256   string               `json:"sha256"`
	Manifest apppackages.Manifest `json:"manifest"`
}

type appPackageResponse struct {
	ID        string               `json:"id"`
	Name      string               `json:"name"`
	Version   string               `json:"version"`
	S3Key     string               `json:"s3_key"`
	SHA256    string               `json:"sha256"`
	Manifest  apppackages.Manifest `json:"manifest"`
	CreatedAt time.Time            `json:"created_at"`
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

func (s *Server) handleUploadAppPackage(w http.ResponseWriter, r *http.Request) {
	if s.cfg.AppPackages == nil || s.cfg.AppPackageBlobs == nil {
		http.Error(w, "app packages unavailable", http.StatusServiceUnavailable)
		return
	}
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}

	data, err := readAppPackageUpload(w, r)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	sum, err := apppackages.ValidateZip(data)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	s3Key, err := canonicalUploadS3Key(r, sum.Manifest)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	ctx := r.Context()
	if _, err := s.cfg.AppPackages.Get(ctx, sum.Manifest.Name, sum.Manifest.Version); err == nil {
		http.Error(w, "package already exists", http.StatusConflict)
		return
	} else if !errors.Is(err, apppackages.ErrNotFound) {
		http.Error(w, "lookup failed", http.StatusInternalServerError)
		return
	}

	if err := s.cfg.AppPackageBlobs.UploadNew(ctx, s3Key, bytes.NewReader(data), int64(len(data))); err != nil {
		http.Error(w, "upload failed", http.StatusInternalServerError)
		return
	}

	rec, err := s.cfg.AppPackages.Create(ctx, apppackages.PackageRecord{
		Name:     sum.Manifest.Name,
		Version:  sum.Manifest.Version,
		S3Key:    s3Key,
		SHA256:   sum.SHA256,
		Manifest: sum.Manifest,
	})
	if err != nil {
		if !errors.Is(err, apppackages.ErrAlreadyExists) {
			if delErr := s.cfg.AppPackageBlobs.DeleteObject(ctx, s3Key); delErr != nil {
				slogWarnAppPackageDeleteObject(sum.Manifest.Name, sum.Manifest.Version, delErr)
			}
		}
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
		"upload": true,
	})
	writeJSON(w, appPackageToResponse(rec))
}

func canonicalUploadS3Key(r *http.Request, manifest apppackages.Manifest) (string, error) {
	canonical := apppackages.DefaultS3Key(manifest.Name, manifest.Version)
	requested := strings.TrimSpace(r.URL.Query().Get("s3_key"))
	if requested != "" && requested != canonical {
		return "", errors.New("s3_key must match canonical package key")
	}
	return canonical, nil
}

func readAppPackageUpload(w http.ResponseWriter, r *http.Request) ([]byte, error) {
	r.Body = http.MaxBytesReader(w, r.Body, apppackages.MaxPackageZipBytes+1)

	ct := strings.ToLower(strings.TrimSpace(r.Header.Get("Content-Type")))
	if strings.HasPrefix(ct, "multipart/form-data") {
		if err := r.ParseMultipartForm(apppackages.MaxPackageZipBytes); err != nil {
			return nil, errors.New("invalid multipart upload")
		}
		for _, field := range []string{"package", "file"} {
			fhs := r.MultipartForm.File[field]
			if len(fhs) == 0 {
				continue
			}
			fh := fhs[0]
			f, err := fh.Open()
			if err != nil {
				return nil, errors.New("open upload file failed")
			}
			defer f.Close()
			data, err := io.ReadAll(io.LimitReader(f, apppackages.MaxPackageZipBytes+1))
			if err != nil {
				return nil, errors.New("read upload file failed")
			}
			if int64(len(data)) > apppackages.MaxPackageZipBytes {
				return nil, errors.New("package zip too large")
			}
			return data, nil
		}
		return nil, errors.New("multipart upload requires package or file field")
	}

	data, err := io.ReadAll(r.Body)
	if err != nil {
		return nil, errors.New("read request body failed")
	}
	if len(data) == 0 {
		return nil, errors.New("empty package zip")
	}
	if int64(len(data)) > apppackages.MaxPackageZipBytes {
		return nil, errors.New("package zip too large")
	}
	return data, nil
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
