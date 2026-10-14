package middleware

import (
	"strconv"

	"github.com/gin-gonic/gin"
)

// SecurityMiddleware handles security headers
type SecurityMiddleware struct {
	config SecurityConfig
}

// SecurityConfig defines security header configuration
type SecurityConfig struct {
	EnableHSTS               bool
	HSTSMaxAge               int64
	HSTSIncludeSubDomains    bool
	EnableCSP                bool
	CSPDirectives            string
	EnableFrameOptions       bool
	FrameOption              string
	EnableContentTypeOptions bool
	EnableXSSProtection      bool
	EnableReferrerPolicy     bool
	ReferrerPolicy           string
	EnablePermissionsPolicy  bool
	PermissionsPolicy        string
}

// NewSecurityMiddleware creates a new security middleware
func NewSecurityMiddleware(config SecurityConfig) *SecurityMiddleware {
	return &SecurityMiddleware{
		config: config,
	}
}

// Security adds security headers to HTTP responses
func (m *SecurityMiddleware) Security() gin.HandlerFunc {
	return func(c *gin.Context) {
		// X-Content-Type-Options: prevents MIME type sniffing
		if m.config.EnableContentTypeOptions {
			c.Header("X-Content-Type-Options", "nosniff")
		}

		// X-Frame-Options: prevents clickjacking
		if m.config.EnableFrameOptions {
			frameOption := m.config.FrameOption
			if frameOption == "" {
				frameOption = "DENY"
			}
			c.Header("X-Frame-Options", frameOption)
		}

		// X-XSS-Protection: enables XSS filtering
		if m.config.EnableXSSProtection {
			c.Header("X-XSS-Protection", "1; mode=block")
		}

		// Strict-Transport-Security (HSTS): enforces HTTPS
		if m.config.EnableHSTS {
			hstsValue := "max-age=" + strconv.FormatInt(m.config.HSTSMaxAge, 10)
			if m.config.HSTSIncludeSubDomains {
				hstsValue += "; includeSubDomains"
			}
			c.Header("Strict-Transport-Security", hstsValue)
		}

		// Content-Security-Policy (CSP)
		if m.config.EnableCSP {
			csp := m.config.CSPDirectives
			if csp == "" {
				csp = "default-src 'self'; frame-ancestors 'none';"
			}
			c.Header("Content-Security-Policy", csp)
		}

		// Referrer-Policy
		if m.config.EnableReferrerPolicy {
			referrerPolicy := m.config.ReferrerPolicy
			if referrerPolicy == "" {
				referrerPolicy = "strict-origin-when-cross-origin"
			}
			c.Header("Referrer-Policy", referrerPolicy)
		}

		// Permissions-Policy
		if m.config.EnablePermissionsPolicy {
			permissionsPolicy := m.config.PermissionsPolicy
			if permissionsPolicy == "" {
				permissionsPolicy = "geolocation=(), microphone=(), camera=()"
			}
			c.Header("Permissions-Policy", permissionsPolicy)
		}

		// Additional security headers
		c.Header("X-Permitted-Cross-Domain-Policies", "none")
		c.Header("Cross-Origin-Opener-Policy", "same-origin")
		c.Header("Cross-Origin-Resource-Policy", "same-origin")

		// Remove server information
		c.Header("Server", "")

		c.Next()
	}
}

// DefaultSecurityConfig returns default security configuration
func DefaultSecurityConfig() SecurityConfig {
	return SecurityConfig{
		EnableHSTS:               true,
		HSTSMaxAge:               31536000, // 1 year
		HSTSIncludeSubDomains:    true,
		EnableCSP:                true,
		EnableFrameOptions:       true,
		FrameOption:              "DENY",
		EnableContentTypeOptions: true,
		EnableXSSProtection:      true,
		EnableReferrerPolicy:     true,
		ReferrerPolicy:           "strict-origin-when-cross-origin",
		EnablePermissionsPolicy:  true,
	}
}
