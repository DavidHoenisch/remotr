package server

import (
	"errors"
	"net/http"
	"strings"

	"github.com/jackc/pgx/v5/pgconn"

	"github.com/DavidHoenisch/remotr/internal/registry"
)

func enrollRegisterErrMessage(err error) string {
	if isInvalidEndpointIDErr(err) {
		return "invalid endpoint_id"
	}
	if errors.Is(err, registry.ErrEndpointExists) {
		return "endpoint_id already enrolled"
	}
	return "enrollment failed"
}

func enrollRegisterStatus(err error) int {
	if isInvalidEndpointIDErr(err) {
		return http.StatusBadRequest
	}
	if errors.Is(err, registry.ErrEndpointExists) {
		return http.StatusConflict
	}
	return http.StatusInternalServerError
}

func isInvalidEndpointIDErr(err error) bool {
	if strings.Contains(err.Error(), "invalid endpoint id") {
		return true
	}
	var pgErr *pgconn.PgError
	return errors.As(err, &pgErr) && pgErr.Code == "23514" && pgErr.ConstraintName == "endpoints_id_format_check"
}
