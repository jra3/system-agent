package errors

import (
	stdliberrors "errors"
)

var (
	ErrUnsupported = stdliberrors.ErrUnsupported

	As     = stdliberrors.As
	Is     = stdliberrors.Is
	Join   = stdliberrors.Join
	New    = stdliberrors.New
	Unwrap = stdliberrors.Unwrap
)

func NewRetryable(text string) RetryableError {
	return &retryableError{text}
}

func Retryable(err error) bool {
	var rerr RetryableError
	return As(err, &rerr)
}

type RetryableError interface {
	error
	Retryable()
}

type retryableError struct {
	text string
}

func (r *retryableError) Error() string {
	return r.text
}

func (r *retryableError) Retryable() {}
