package middleware

import (
	"time"

	"github.com/gin-gonic/gin"
	"go.uber.org/zap"
)

// Logger middleware logs HTTP requests
type Logger struct {
	logger *zap.Logger
}

// NewLogger creates a new Logger middleware
func NewLogger(logger *zap.Logger) *Logger {
	return &Logger{logger: logger}
}

// Log logs HTTP requests
func (m *Logger) Log() gin.HandlerFunc {
	return func(c *gin.Context) {
		start := time.Now()
		path := c.Request.URL.Path
		query := c.Request.URL.RawQuery

		c.Next()

		latency := time.Since(start)
		status := c.Writer.Status()
		method := c.Request.Method
		ip := c.ClientIP()
		userAgent := c.Request.UserAgent()
		referer := c.Request.Referer()
		errors := c.Errors.String()
		requestID := GetRequestID(c)

		fields := []zap.Field{
			zap.Int("status", status),
			zap.String("method", method),
			zap.String("path", path),
			zap.String("query", query),
			zap.String("ip", ip),
			zap.String("user-agent", userAgent),
			zap.String("referer", referer),
			zap.Duration("latency", latency),
			zap.String("errors", errors),
		}

		if requestID != "" {
			fields = append(fields, zap.String("request_id", requestID))
		}

		m.logger.Info("HTTP request", fields...)
	}
}
