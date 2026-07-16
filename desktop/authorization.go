package main

import (
	"errors"
	"net/http"
	"strings"

	"github.com/DavidHoenisch/remotr/internal/admin"
)

type ActionFailureKind string

const (
	ActionForbidden  ActionFailureKind = "authorization"
	ActionConflict   ActionFailureKind = "conflict"
	ActionConnection ActionFailureKind = "connection"
	ActionNotFound   ActionFailureKind = "not_found"
	ActionUnexpected ActionFailureKind = "unexpected"
	ActionValidation ActionFailureKind = "validation"
)

type ActionFailure struct {
	Kind      ActionFailureKind `json:"kind"`
	Message   string            `json:"message"`
	Guidance  string            `json:"guidance"`
	Retryable bool              `json:"retryable"`
}

func (e *ActionFailure) Error() string {
	return e.Message
}

func classifyAuthenticatedActionError(err error) error {
	if err == nil {
		return nil
	}
	var actionFailure *ActionFailure
	if errors.As(err, &actionFailure) {
		return actionFailure
	}
	var responseError *admin.ResponseError
	if errors.As(err, &responseError) && responseError.StatusCode == http.StatusForbidden {
		return &ActionFailure{
			Kind:      ActionForbidden,
			Message:   "The current Operator is not authorized for this action.",
			Guidance:  "Ask a Remotr administrator to review the Operator's assigned roles.",
			Retryable: false,
		}
	}
	if errors.As(err, &responseError) {
		switch {
		case responseError.StatusCode == http.StatusBadRequest:
			return &ActionFailure{
				Kind:      ActionValidation,
				Message:   "The server rejected the action input.",
				Guidance:  "Review the requested values before submitting again.",
				Retryable: false,
			}
		case responseError.StatusCode == http.StatusNotFound:
			return &ActionFailure{
				Kind:      ActionNotFound,
				Message:   "The requested server resource no longer exists.",
				Guidance:  "Refresh the current evidence before choosing another target.",
				Retryable: false,
			}
		case responseError.StatusCode == http.StatusConflict:
			return &ActionFailure{
				Kind:      ActionConflict,
				Message:   "The action conflicts with current server state.",
				Guidance:  "Refresh the affected evidence before retrying.",
				Retryable: true,
			}
		case responseError.StatusCode == http.StatusTooManyRequests || responseError.StatusCode >= http.StatusInternalServerError:
			return &ActionFailure{
				Kind:      ActionConnection,
				Message:   "The server could not accept the action.",
				Guidance:  "Keep the current evidence visible and retry when the server is available.",
				Retryable: true,
			}
		}
	}
	if strings.Contains(strings.ToLower(err.Error()), "connection") || strings.Contains(strings.ToLower(err.Error()), "timeout") {
		return &ActionFailure{
			Kind:      ActionConnection,
			Message:   "The action request could not reach the server.",
			Guidance:  "Check the active connection and retry without changing the current evidence.",
			Retryable: true,
		}
	}
	return &ActionFailure{
		Kind:      ActionUnexpected,
		Message:   "The action could not be completed safely.",
		Guidance:  "Keep the current evidence visible, then retry or cancel the action.",
		Retryable: true,
	}
}
