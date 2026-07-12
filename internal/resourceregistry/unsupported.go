package resourceregistry

import (
	"context"
	"errors"

	"github.com/DavidHoenisch/remotr/internal/executor"
)

type unsupportedProvider struct {
	name   string
	reason error
}

func (u unsupportedProvider) Name() string                      { return u.name }
func (u unsupportedProvider) Description() string               { return u.reason.Error() }
func (u unsupportedProvider) State(context.Context) (any, bool) { return nil, false }
func (u unsupportedProvider) Apply(context.Context) error {
	return errors.New("unsupported provider cannot apply")
}
func (u unsupportedProvider) Revert(context.Context) error { return nil }
func (u unsupportedProvider) Check(context.Context) executor.CheckResult {
	return executor.CheckResult{
		Status: executor.Unsupported, ReasonCode: executor.ReasonProviderUnavailable,
		ObservedSummary: executor.RedactedSummary(u.reason.Error()),
	}
}
