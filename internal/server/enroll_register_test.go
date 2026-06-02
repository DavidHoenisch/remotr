package server

import (
	"errors"
	"net/http"
	"testing"

	"github.com/jackc/pgx/v5/pgconn"
)

func TestEnrollRegisterStatus_invalidID(t *testing.T) {
	err := errors.New(`invalid endpoint id "bad_id": too short`)
	if got := enrollRegisterStatus(err); got != http.StatusBadRequest {
		t.Fatalf("status = %d, want %d", got, http.StatusBadRequest)
	}
	if enrollRegisterErrMessage(err) != "invalid endpoint_id" {
		t.Fatalf("message = %q", enrollRegisterErrMessage(err))
	}
}

func TestEnrollRegisterStatus_checkConstraint(t *testing.T) {
	err := &pgconn.PgError{Code: "23514", ConstraintName: "endpoints_id_format_check"}
	if got := enrollRegisterStatus(err); got != http.StatusBadRequest {
		t.Fatalf("status = %d, want %d", got, http.StatusBadRequest)
	}
}

func TestEnrollRegisterStatus_other(t *testing.T) {
	err := errors.New("connection reset")
	if got := enrollRegisterStatus(err); got != http.StatusInternalServerError {
		t.Fatalf("status = %d, want %d", got, http.StatusInternalServerError)
	}
}
