package errors

import (
	"errors"
)

var (
	ErrNoOp              error = errors.New("op-op")
	ErrOperationCanceled error = errors.New("Operation was canceled")
	ErrStateAlreadyMet   error = errors.New("State already met")
)
