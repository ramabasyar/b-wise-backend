package middleware

import (
	"net/http"
	"strconv"
	"strings"

	"github.com/gin-gonic/gin"
)

// CORSConfig defines CORS configuration
type CORSConfig struct {
	AllowedOrigins   []string
	AllowCredentials bool
	MaxAge           int
}

// CORS middleware handles CORS
type CORS struct {
	config CORSConfig
}

// NewCORS creates a new CORS middleware
func NewCORS(config CORSConfig) *CORS {
	return &CORS{
		config: config,
	}
}

// Handle handles CORS
func (m *CORS) Handle() gin.HandlerFunc {
	return func(c *gin.Context) {
		origin := c.Request.Header.Get("Origin")

		// Check if origin is allowed
		allowed := m.isOriginAllowed(origin)

		if allowed {
			// Set CORS headers
			if len(m.config.AllowedOrigins) == 1 && m.config.AllowedOrigins[0] == "*" {
				c.Writer.Header().Set("Access-Control-Allow-Origin", "*")
			} else {
				c.Writer.Header().Set("Access-Control-Allow-Origin", origin)
			}

			if m.config.AllowCredentials {
				c.Writer.Header().Set("Access-Control-Allow-Credentials", "true")
			}

			c.Writer.Header().Set("Access-Control-Allow-Methods", "GET, POST, PUT, DELETE, PATCH, OPTIONS")
			c.Writer.Header().Set("Access-Control-Allow-Headers", "Content-Type, Authorization, X-Request-ID")

			if m.config.MaxAge > 0 {
				c.Writer.Header().Set("Access-Control-Max-Age", strconv.Itoa(m.config.MaxAge))
			}
		}

		// Handle pre-flight request
		if c.Request.Method == http.MethodOptions {
			c.AbortWithStatus(http.StatusNoContent)
			return
		}

		c.Next()
	}
}

// isOriginAllowed checks if origin is in allowed list
func (m *CORS) isOriginAllowed(origin string) bool {
	for _, allowedOrigin := range m.config.AllowedOrigins {
		if allowedOrigin == "*" || origin == allowedOrigin {
			return true
		}
	}

	// Allow if no origin (same-origin requests)
	if origin == "" {
		return true
	}

	return false
}

// NewCORSWithOrigins creates a new CORS middleware with simple origin list
func NewCORSWithOrigins(origins []string) *CORS {
	return NewCORS(CORSConfig{
		AllowedOrigins:   origins,
		AllowCredentials: true,
	})
}

// Ensure strings import is used
var _ = strings.TrimSpace
