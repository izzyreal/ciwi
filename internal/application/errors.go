package application

import (
	"errors"
	"fmt"
)

type ErrorKind string

const (
	ErrorInvalidArgument    ErrorKind = "invalid_argument"
	ErrorNotFound           ErrorKind = "not_found"
	ErrorConflict           ErrorKind = "conflict"
	ErrorFailedPrecondition ErrorKind = "failed_precondition"
	ErrorUnavailable        ErrorKind = "unavailable"
	ErrorUnsupported        ErrorKind = "unsupported"
	ErrorInternal           ErrorKind = "internal"
)

type Error struct {
	Kind    ErrorKind
	Message string
	Cause   error
}

func (e *Error) Error() string {
	if e == nil {
		return ""
	}
	if e.Message != "" {
		return e.Message
	}
	if e.Cause != nil {
		return e.Cause.Error()
	}
	return string(e.Kind)
}

func (e *Error) Unwrap() error { return e.Cause }

func NewError(kind ErrorKind, message string, cause error) error {
	if kind == "" {
		kind = ErrorInternal
	}
	return &Error{Kind: kind, Message: message, Cause: cause}
}

func ErrorKindOf(err error) ErrorKind {
	var appErr *Error
	if errors.As(err, &appErr) {
		return appErr.Kind
	}
	return ErrorInternal
}

func WrapInternal(operation string, err error) error {
	if err == nil {
		return nil
	}
	return NewError(ErrorInternal, fmt.Sprintf("%s: %v", operation, err), err)
}
