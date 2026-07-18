package executor

import "context"

// Preflighter is implemented by a high-risk handler that can verify its
// resource-specific safety conditions before mutation.
type Preflighter interface {
	Preflight(context.Context) error
}

// RollbackPreflighter proves that a rollback-advertising provider can reserve
// its current recovery payload without mutating the managed resource. The
// provider releases the probe reservation before returning; Apply still owns
// the authoritative reserve-and-arm operation immediately before mutation.
type RollbackPreflighter interface {
	PreflightRollback(context.Context) error
}
