package main

import (
	"context"
	"errors"
	"net/http"
	"reflect"
	"strings"
	"time"

	"github.com/DavidHoenisch/remotr/internal/admin"
	"github.com/DavidHoenisch/remotr/internal/diagnostics"
	"github.com/DavidHoenisch/remotr/internal/identity"
)

type DiagnosticCapabilities struct {
	Collectors         []string `json:"collectors"`
	MaxTimeSpanSeconds int64    `json:"maxTimeSpanSeconds"`
}

type DiagnosticCollectionRequest struct {
	EndpointID string   `json:"endpointId"`
	Collectors []string `json:"collectors"`
	Since      string   `json:"since"`
	Until      string   `json:"until"`
}

type DiagnosticCollectionResult struct {
	RequestID  string   `json:"requestId"`
	EndpointID string   `json:"endpointId"`
	Status     string   `json:"status"`
	Collectors []string `json:"collectors"`
	Since      string   `json:"since"`
	Until      string   `json:"until"`
	CreatedAt  string   `json:"createdAt,omitempty"`
	ExpiresAt  string   `json:"expiresAt,omitempty"`
}

type DiagnosticCollectionService struct{}

func NewDiagnosticCollectionService() *DiagnosticCollectionService {
	return &DiagnosticCollectionService{}
}

func (s *DiagnosticCollectionService) Capabilities() DiagnosticCapabilities {
	return DiagnosticCapabilities{
		Collectors:         diagnostics.DefaultCollectors(),
		MaxTimeSpanSeconds: int64(diagnostics.MaxTimeSpan / time.Second),
	}
}

func (s *DiagnosticCollectionService) RequestConnected(ctx context.Context, client *admin.Client, request DiagnosticCollectionRequest) (DiagnosticCollectionResult, error) {
	if err := identity.ValidateEndpointID(request.EndpointID); err != nil {
		return DiagnosticCollectionResult{}, diagnosticCollectionValidationFailure("Select one exact Endpoint before requesting diagnostics.")
	}
	collectors, err := validateDiagnosticCollectors(request.Collectors)
	if err != nil {
		return DiagnosticCollectionResult{}, err
	}
	since, until, err := validateDiagnosticInterval(request.Since, request.Until)
	if err != nil {
		return DiagnosticCollectionResult{}, err
	}
	if client == nil {
		return DiagnosticCollectionResult{}, ErrSessionNotConnected
	}

	created, err := client.RequestDiagnosticsCollectContext(ctx, request.EndpointID, admin.CollectDiagnosticsOptions{
		Collectors: collectors,
		Since:      since,
		Until:      until,
	})
	if err != nil {
		var responseError *admin.ResponseError
		if errors.As(err, &responseError) && responseError.StatusCode == http.StatusConflict {
			return DiagnosticCollectionResult{}, &ActionFailure{
				Kind:      ActionConflict,
				Message:   "This Endpoint already has an active diagnostic request.",
				Guidance:  "Inspect the existing request before starting another collection.",
				Retryable: false,
			}
		}
		return DiagnosticCollectionResult{}, err
	}
	if created.ID == "" || created.EndpointID != request.EndpointID || created.Status == "" {
		return DiagnosticCollectionResult{}, errors.New("server returned incomplete diagnostic request evidence")
	}
	if !reflect.DeepEqual(created.Spec.Collectors, collectors) || !created.Spec.Since.Equal(since) || !created.Spec.Until.Equal(until) {
		return DiagnosticCollectionResult{}, errors.New("server returned diagnostic request evidence for different collection parameters")
	}

	return DiagnosticCollectionResult{
		RequestID:  created.ID,
		EndpointID: created.EndpointID,
		Status:     created.Status,
		Collectors: append([]string(nil), created.Spec.Collectors...),
		Since:      created.Spec.Since.UTC().Format(time.RFC3339),
		Until:      created.Spec.Until.UTC().Format(time.RFC3339),
	}, nil
}

func validateDiagnosticCollectors(raw []string) ([]string, error) {
	if len(raw) == 0 {
		return nil, diagnosticCollectionValidationFailure("Select at least one supported collector.")
	}
	seen := make(map[string]struct{}, len(raw))
	collectors := make([]string, 0, len(raw))
	for _, collector := range raw {
		if collector == "" || strings.TrimSpace(collector) != collector || !diagnostics.ValidCollector(collector) {
			return nil, diagnosticCollectionValidationFailure("Select only collectors supported by this server.")
		}
		if _, exists := seen[collector]; exists {
			continue
		}
		seen[collector] = struct{}{}
		collectors = append(collectors, collector)
	}
	if len(collectors) == 0 {
		return nil, diagnosticCollectionValidationFailure("Select at least one supported collector.")
	}
	return collectors, nil
}

func validateDiagnosticInterval(rawSince, rawUntil string) (time.Time, time.Time, error) {
	if rawSince == "" || rawUntil == "" {
		return time.Time{}, time.Time{}, diagnosticCollectionValidationFailure("Enter absolute since and until timestamps.")
	}
	since, err := time.Parse(time.RFC3339, rawSince)
	if err != nil {
		return time.Time{}, time.Time{}, diagnosticCollectionValidationFailure("Enter since as an absolute RFC 3339 timestamp.")
	}
	until, err := time.Parse(time.RFC3339, rawUntil)
	if err != nil {
		return time.Time{}, time.Time{}, diagnosticCollectionValidationFailure("Enter until as an absolute RFC 3339 timestamp.")
	}
	if !until.After(since) {
		return time.Time{}, time.Time{}, diagnosticCollectionValidationFailure("Until must be after since.")
	}
	if until.Sub(since) > diagnostics.MaxTimeSpan {
		return time.Time{}, time.Time{}, diagnosticCollectionValidationFailure("The collection interval must be 7 days or less.")
	}
	return since, until, nil
}

func diagnosticCollectionValidationFailure(guidance string) error {
	return &ActionFailure{
		Kind:      ActionValidation,
		Message:   "The diagnostic collection request is invalid.",
		Guidance:  guidance,
		Retryable: false,
	}
}
