package response

import (
	"net/http"

	"github.com/gin-gonic/gin"
	"github.com/rama/b-wise/permission-service/pkg/errors"
)

// Response represents a standard API response
type Response struct {
	Success bool        `json:"success"`
	Data    interface{} `json:"data,omitempty"`
	Error   *ErrorInfo  `json:"error,omitempty"`
}

// ErrorInfo represents error details
type ErrorInfo struct {
	Message string `json:"message"`
	Code    int    `json:"code"`
}

// Success sends a success response
func Success(c *gin.Context, data interface{}) {
	c.JSON(http.StatusOK, Response{
		Success: true,
		Data:    data,
	})
}

// Created sends a created response
func Created(c *gin.Context, data interface{}) {
	c.JSON(http.StatusCreated, Response{
		Success: true,
		Data:    data,
	})
}

// Error sends an error response
func Error(c *gin.Context, err error) {
	appErr, ok := err.(*errors.AppError)
	if !ok {
		appErr = errors.Wrap(err, "internal server error")
	}

	status := appErr.HTTPStatus()
	c.JSON(status, Response{
		Success: false,
		Error: &ErrorInfo{
			Message: appErr.Message,
			Code:    status,
		},
	})
}

// BadRequest sends a bad request response
func BadRequest(c *gin.Context, message string) {
	c.JSON(http.StatusBadRequest, Response{
		Success: false,
		Error: &ErrorInfo{
			Message: message,
			Code:    http.StatusBadRequest,
		},
	})
}

// Unauthorized sends an unauthorized response
func Unauthorized(c *gin.Context, message string) {
	c.JSON(http.StatusUnauthorized, Response{
		Success: false,
		Error: &ErrorInfo{
			Message: message,
			Code:    http.StatusUnauthorized,
		},
	})
}

// Forbidden sends a forbidden response
func Forbidden(c *gin.Context, message string) {
	c.JSON(http.StatusForbidden, Response{
		Success: false,
		Error: &ErrorInfo{
			Message: message,
			Code:    http.StatusForbidden,
		},
	})
}

// NotFound sends a not found response
func NotFound(c *gin.Context, message string) {
	c.JSON(http.StatusNotFound, Response{
		Success: false,
		Error: &ErrorInfo{
			Message: message,
			Code:    http.StatusNotFound,
		},
	})
}

// Conflict sends a conflict response
func Conflict(c *gin.Context, message string) {
	c.JSON(http.StatusConflict, Response{
		Success: false,
		Error: &ErrorInfo{
			Message: message,
			Code:    http.StatusConflict,
		},
	})
}
