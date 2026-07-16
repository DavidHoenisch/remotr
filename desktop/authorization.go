package main

import (
	"errors"
	"net/http"

	"github.com/DavidHoenisch/remotr/internal/admin"
)

type ActionFailureKind string

const (
	ActionForbidden ActionFailureKind = "forbidden"
)

type ActionFailure struct {
	Kind     ActionFailureKind `json:"kind"`
	Message  string            `json:"message"`
	Guidance string            `json:"guidance"`
}

func (e *ActionFailure) Error() string {
	return e.Message
}

func classifyAuthenticatedActionError(err error) error {
	if err == nil {
		return nil
	}
	var responseError *admin.ResponseError
	if errors.As(err, &responseError) && responseError.StatusCode == http.StatusForbidden {
		return &ActionFailure{
			Kind:     ActionForbidden,
			Message:  "The current Operator is not authorized for this action.",
			Guidance: "Ask a Remotr administrator to review the Operator's assigned roles.",
		}
	}
	return err
}
