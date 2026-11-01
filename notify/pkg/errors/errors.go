package errors

import (
	"fmt"
	"net/http"
)

// AppError represents an application error
type AppError struct {
	Code    int    `json:"code"`
	Message string `json:"message"`
	Err     error  `json:"-"`
}

// Error implements the error interface
func (e *AppError) Error() string {
	if e.Err != nil {
		return fmt.Sprintf("%s: %v", e.Message, e.Err)
	}
	return e.Message
}

// Unwrap returns the underlying error
func (e *AppError) Unwrap() error {
	return e.Err
}

// HTTPStatus returns the HTTP status code for this error
func (e *AppError) HTTPStatus() int {
	return e.Code
}

// Common errors
var (
	ErrBadRequest = &AppError{
		Code:    http.StatusBadRequest,
		Message: "bad request",
	}
	ErrUnauthorized = &AppError{
		Code:    http.StatusUnauthorized,
		Message: "unauthorized",
	}
	ErrForbidden = &AppError{
		Code:    http.StatusForbidden,
		Message: "forbidden",
	}
	ErrNotFound = &AppError{
		Code:    http.StatusNotFound,
		Message: "not found",
	}
	ErrConflict = &AppError{
		Code:    http.StatusConflict,
		Message: "conflict",
	}
	ErrInternalServerError = &AppError{
		Code:    http.StatusInternalServerError,
		Message: "internal server error",
	}
)

// NewBadRequestError creates a new bad request error
func NewBadRequestError(message string) *AppError {
	return &AppError{
		Code:    http.StatusBadRequest,
		Message: message,
	}
}

// NewUnauthorizedError creates a new unauthorized error
func NewUnauthorizedError(message string) *AppError {
	return &AppError{
		Code:    http.StatusUnauthorized,
		Message: message,
	}
}

// NewForbiddenError creates a new forbidden error
func NewForbiddenError(message string) *AppError {
	return &AppError{
		Code:    http.StatusForbidden,
		Message: message,
	}
}

// NewNotFoundError creates a new not found error
func NewNotFoundError(message string) *AppError {
	return &AppError{
		Code:    http.StatusNotFound,
		Message: message,
	}
}

// NewConflictError creates a new conflict error
func NewConflictError(message string) *AppError {
	return &AppError{
		Code:    http.StatusConflict,
		Message: message,
	}
}

// Wrap wraps an error with additional context
func Wrap(err error, message string) *AppError {
	return &AppError{
		Code:    http.StatusInternalServerError,
		Message: message,
		Err:     err,
	}
}
