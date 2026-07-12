package executor

import "context"

// Preflighter is implemented by a high-risk handler that can verify its
// resource-specific safety conditions before mutation.
type Preflighter interface {
	Preflight(context.Context) error
}
