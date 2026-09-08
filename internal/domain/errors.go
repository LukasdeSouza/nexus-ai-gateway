// Package domain defines the core domain entities, values, and errors for Nexus AI Gateway.
package domain

import (
	"errors"
	"fmt"
)

// Code represents a domain error code.
type Code string

const (
	CodeNotFound            Code = "not_found"
	CodeUnauthorized        Code = "unauthorized"
	CodeForbidden           Code = "forbidden"
	CodeBadRequest          Code = "bad_request"
	CodeConflict            Code = "conflict"
	CodeProviderError       Code = "provider_error"
	CodeProviderTimeout     Code = "provider_timeout"
	CodeProviderUnavailable Code = "provider_unavailable"
	CodeRateLimitExceeded   Code = "rate_limit_exceeded"
	CodeBudgetExceeded      Code = "budget_exceeded"
	CodeInternal            Code = "internal_error"
)

// Error is a structured domain error.
type Error struct {
	Code      Code   `json:"code"`
	Message   string `json:"message"`
	Retryable bool   `json:"retryable,omitempty"`
	Cause     error  `json:"-"`
}

func (e *Error) Error() string {
	if e.Cause != nil {
		return fmt.Sprintf("%s: %s (cause: %v)", e.Code, e.Message, e.Cause)
	}
	return fmt.Sprintf("%s: %s", e.Code, e.Message)
}

func (e *Error) Unwrap() error {
	return e.Cause
}

// New creates a new domain Error.
func New(code Code, message string) *Error {
	return &Error{
		Code:      code,
		Message:   message,
		Retryable: isDefaultRetryable(code),
	}
}

// Wrap creates a domain Error wrapping an underlying cause.
func Wrap(code Code, message string, cause error) *Error {
	return &Error{
		Code:      code,
		Message:   message,
		Retryable: isDefaultRetryable(code),
		Cause:     cause,
	}
}

// Sentinel domain errors.
var (
	ErrNotFound          = New(CodeNotFound, "resource not found")
	ErrUnauthorized      = New(CodeUnauthorized, "invalid or missing API key")
	ErrForbidden         = New(CodeForbidden, "insufficient permissions for this action")
	ErrRateLimitExceeded = New(CodeRateLimitExceeded, "rate limit exceeded, please slow down")
	ErrBudgetExceeded    = New(CodeBudgetExceeded, "budget limit exceeded for this project")
	ErrKeyExpired        = New(CodeUnauthorized, "API key has expired")
	ErrKeyRevoked        = New(CodeUnauthorized, "API key has been revoked")
)

// IsRetryable returns true if the error or any wrapped error is marked retryable.
func IsRetryable(err error) bool {
	if err == nil {
		return false
	}
	var domErr *Error
	if errors.As(err, &domErr) {
		return domErr.Retryable
	}
	return false
}

// CodeOf extracts the domain Code from an error, or returns CodeInternal.
func CodeOf(err error) Code {
	if err == nil {
		return ""
	}
	var domErr *Error
	if errors.As(err, &domErr) {
		return domErr.Code
	}
	return CodeInternal
}

func isDefaultRetryable(code Code) bool {
	switch code {
	case CodeProviderTimeout, CodeProviderUnavailable:
		return true
	default:
		return false
	}
}
