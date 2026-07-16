package main

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"unicode"

	"github.com/DavidHoenisch/remotr/internal/admin"
	"github.com/DavidHoenisch/remotr/internal/diagnostics"
	wailsruntime "github.com/wailsapp/wails/v2/pkg/runtime"
)

type DiagnosticBundleSaveDialog func(context.Context, string) (string, error)

type DiagnosticBundleSaveResult struct {
	Status    string `json:"status"`
	Path      string `json:"path,omitempty"`
	SizeBytes int64  `json:"sizeBytes,omitempty"`
}

type DiagnosticBundleSaveService struct {
	chooseDestination DiagnosticBundleSaveDialog
}

func NewDiagnosticBundleSaveService(dialog DiagnosticBundleSaveDialog) *DiagnosticBundleSaveService {
	return &DiagnosticBundleSaveService{chooseDestination: dialog}
}

func defaultDiagnosticBundleSaveDialog(ctx context.Context, suggestedName string) (string, error) {
	return wailsruntime.SaveFileDialog(ctx, wailsruntime.SaveDialogOptions{
		Title:                "Save diagnostic bundle",
		DefaultFilename:      suggestedName,
		CanCreateDirectories: true,
		Filters: []wailsruntime.FileFilter{{
			DisplayName: "Compressed tar archive (*.tar.gz)",
			Pattern:     "*.tar.gz",
		}},
	})
}

func (s *DiagnosticBundleSaveService) SaveConnected(ctx context.Context, client *admin.Client, requestID string) (DiagnosticBundleSaveResult, error) {
	if strings.TrimSpace(requestID) == "" || strings.TrimSpace(requestID) != requestID {
		return DiagnosticBundleSaveResult{}, diagnosticBundleValidationFailure("Select one exact diagnostic request before saving its bundle.")
	}
	if client == nil {
		return DiagnosticBundleSaveResult{}, ErrSessionNotConnected
	}
	request, err := client.GetDiagnosticRequestContext(ctx, requestID)
	if err != nil {
		var responseError *admin.ResponseError
		if errors.As(err, &responseError) && responseError.StatusCode == http.StatusNotFound {
			return DiagnosticBundleSaveResult{}, &ActionFailure{
				Kind:      ActionNotFound,
				Message:   "Diagnostic request was not found.",
				Guidance:  "Return to Diagnostics and select a request that still exists.",
				Retryable: false,
			}
		}
		return DiagnosticBundleSaveResult{}, err
	}
	if request.ID != requestID {
		return DiagnosticBundleSaveResult{}, errors.New("server returned a different diagnostic request identity")
	}
	if request.Status != diagnostics.StatusReady {
		return DiagnosticBundleSaveResult{}, diagnosticBundleLifecycleFailure(request.Status)
	}
	if request.SizeBytes < 0 || request.SizeBytes > diagnostics.MaxBundleBytes {
		return DiagnosticBundleSaveResult{}, errors.New("server returned an invalid diagnostic bundle size")
	}
	if request.SHA256 != "" {
		decoded, decodeErr := hex.DecodeString(request.SHA256)
		if decodeErr != nil || len(decoded) != sha256.Size {
			return DiagnosticBundleSaveResult{}, errors.New("server returned an invalid diagnostic bundle digest")
		}
	}
	if s == nil || s.chooseDestination == nil {
		return DiagnosticBundleSaveResult{}, errors.New("native diagnostic save dialog is unavailable")
	}
	destination, err := s.chooseDestination(ctx, diagnosticBundleSuggestedName(requestID))
	if err != nil {
		return DiagnosticBundleSaveResult{}, fmt.Errorf("choose diagnostic bundle destination: %w", err)
	}
	if destination == "" {
		return DiagnosticBundleSaveResult{Status: "canceled"}, nil
	}
	destination = filepath.Clean(destination)
	if filepath.Base(destination) == "." || filepath.Base(destination) == string(filepath.Separator) {
		return DiagnosticBundleSaveResult{}, errors.New("diagnostic bundle destination must name a file")
	}

	written, err := s.downloadVerified(ctx, client, request, destination)
	if err != nil {
		return DiagnosticBundleSaveResult{}, err
	}
	return DiagnosticBundleSaveResult{
		Status:    "saved",
		Path:      destination,
		SizeBytes: written,
	}, nil
}

func (s *DiagnosticBundleSaveService) downloadVerified(ctx context.Context, client *admin.Client, request admin.DiagnosticRequest, destination string) (int64, error) {
	directory := filepath.Dir(destination)
	temporary, err := os.CreateTemp(directory, ".remotr-diagnostic-*.tmp")
	if err != nil {
		return 0, fmt.Errorf("create temporary diagnostic destination: %w", err)
	}
	temporaryPath := temporary.Name()
	committed := false
	defer func() {
		_ = temporary.Close()
		if !committed {
			_ = os.Remove(temporaryPath)
		}
	}()
	if err := temporary.Chmod(0o600); err != nil {
		return 0, fmt.Errorf("protect temporary diagnostic destination: %w", err)
	}

	digest := sha256.New()
	bounded := &diagnosticBundleBoundedWriter{
		writer: io.MultiWriter(temporary, digest),
		limit:  diagnostics.MaxBundleBytes,
	}
	written, err := client.DownloadDiagnosticBundleToContext(ctx, request.ID, bounded)
	if err != nil {
		return 0, fmt.Errorf("download diagnostic bundle: %w", err)
	}
	if written != bounded.written {
		return 0, errors.New("diagnostic bundle stream count did not match the destination")
	}
	if request.SizeBytes > 0 && written != request.SizeBytes {
		return 0, fmt.Errorf("diagnostic bundle size mismatch: received %d bytes, expected %d", written, request.SizeBytes)
	}
	digestHex := hex.EncodeToString(digest.Sum(nil))
	if request.SHA256 != "" && !strings.EqualFold(digestHex, request.SHA256) {
		return 0, errors.New("diagnostic bundle SHA-256 mismatch")
	}
	if err := temporary.Sync(); err != nil {
		return 0, fmt.Errorf("sync diagnostic bundle: %w", err)
	}
	if err := temporary.Close(); err != nil {
		return 0, fmt.Errorf("close diagnostic bundle: %w", err)
	}
	if err := os.Rename(temporaryPath, destination); err != nil {
		return 0, fmt.Errorf("place diagnostic bundle: %w", err)
	}
	if err := syncDiagnosticDirectory(directory); err != nil {
		_ = os.Remove(destination)
		return 0, err
	}
	committed = true
	return written, nil
}

type diagnosticBundleBoundedWriter struct {
	writer  io.Writer
	limit   int64
	written int64
}

func (w *diagnosticBundleBoundedWriter) Write(data []byte) (int, error) {
	remaining := w.limit - w.written
	if remaining <= 0 {
		return 0, errors.New("diagnostic bundle exceeds the supported size limit")
	}
	if int64(len(data)) > remaining {
		data = data[:remaining]
		n, err := w.writer.Write(data)
		w.written += int64(n)
		if err != nil {
			return n, err
		}
		return n, errors.New("diagnostic bundle exceeds the supported size limit")
	}
	n, err := w.writer.Write(data)
	w.written += int64(n)
	return n, err
}

func syncDiagnosticDirectory(directory string) error {
	if runtime.GOOS == "windows" {
		return nil
	}
	handle, err := os.Open(directory)
	if err != nil {
		return fmt.Errorf("open diagnostic destination directory: %w", err)
	}
	defer handle.Close()
	if err := handle.Sync(); err != nil {
		return fmt.Errorf("sync diagnostic destination directory: %w", err)
	}
	return nil
}

func diagnosticBundleSuggestedName(requestID string) string {
	safe := strings.Map(func(value rune) rune {
		if unicode.IsLetter(value) || unicode.IsDigit(value) || value == '-' || value == '_' || value == '.' {
			return value
		}
		return '_'
	}, requestID)
	return safe + ".tar.gz"
}

func diagnosticBundleLifecycleFailure(status string) error {
	condition := strings.TrimSpace(status)
	if condition == "" {
		condition = "unknown"
	}
	return &ActionFailure{
		Kind:      ActionConflict,
		Message:   fmt.Sprintf("Diagnostic request is %s; its bundle is not ready.", condition),
		Guidance:  "Inspect the request lifecycle before trying to save again.",
		Retryable: condition == diagnostics.StatusPending || condition == diagnostics.StatusDispatched || condition == diagnostics.StatusRunning,
	}
}

func diagnosticBundleValidationFailure(guidance string) error {
	return &ActionFailure{
		Kind:      ActionValidation,
		Message:   "The diagnostic bundle save request is invalid.",
		Guidance:  guidance,
		Retryable: false,
	}
}
