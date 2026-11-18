package registry

import (
	"errors"
	"time"
)

var (
	// ErrEndpointEnrolled reports a reservation or release attempt on an enrolled endpoint.
	ErrEndpointEnrolled = errors.New("endpoint is already enrolled")
	// ErrReservationFleetConflict reports a reservation of an ID reserved in another fleet.
	ErrReservationFleetConflict = errors.New("endpoint is reserved in another fleet")
	// ErrReservationNotFound reports a release of an unknown endpoint ID.
	ErrReservationNotFound = errors.New("endpoint reservation not found")
	// ErrBoundTokenInvalid reports a bound enrollment token that is consumed, expired, revoked, or unknown.
	ErrBoundTokenInvalid = errors.New("invalid or expired enrollment token")
)

// EndpointReservations is the optional capability for reserving endpoint IDs
// before enrollment with single-use tokens bound to one fleet and endpoint.
type EndpointReservations interface {
	// ReserveEndpoint creates (or re-issues) a reservation, revoking any
	// previously bound token for the ID and binding token to it.
	ReserveEndpoint(id, fleet, token string, expiresAt time.Time) error
	// UnreserveEndpoint removes an unclaimed reservation and its bound tokens.
	UnreserveEndpoint(id string) error
	// EnrollmentTokenBinding reports the endpoint a token is bound to,
	// regardless of whether the token is still redeemable.
	EnrollmentTokenBinding(token string) (endpointID string, bound bool)
	// ClaimReservedEndpoint atomically consumes the bound token and enrolls
	// the reserved endpoint with certFingerprint, returning its fleet.
	ClaimReservedEndpoint(token, id, certFingerprint string) (fleet string, err error)
}
