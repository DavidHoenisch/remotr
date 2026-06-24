package apppackages

import "errors"

var (
	ErrNotFound     = errors.New("app package not found")
	ErrUnavailable  = errors.New("app packages unavailable")
	ErrAlreadyExists = errors.New("app package already exists")
)
