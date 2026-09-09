package middleware

import (
	"crypto/hmac"
	"crypto/rsa"
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/gin-gonic/gin"
)

// JWKSAuth middleware validates both user and service tokens via SSO JWKS + HMAC fallback
type JWKSAuth struct {
	ssoURL     string
	hmacSecret string // HMAC secret for user tokens
	publicKeys map[string]*rsa.PublicKey
	mu         sync.RWMutex
	cacheTTL   time.Duration
	lastFetch  time.Time
}

// NewJWKSAuth creates a new JWKS auth middleware
func NewJWKSAuth(ssoURL string) *JWKSAuth {
	return &JWKSAuth{
		ssoURL:     strings.TrimRight(ssoURL, "/"),
		publicKeys: make(map[string]*rsa.PublicKey),
		cacheTTL:   1 * time.Hour,
	}
}

// WithHMACSecret adds HMAC secret for validating user tokens
func (j *JWKSAuth) WithHMACSecret(secret string) *JWKSAuth {
	j.hmacSecret = secret
	return j
}

// Middleware validates JWT tokens (both user and service) via SSO JWKS
func (j *JWKSAuth) Middleware() gin.HandlerFunc {
	return func(c *gin.Context) {
		authHeader := c.GetHeader("Authorization")
		if authHeader == "" {
			// No auth - allow public endpoints only
			c.Next()
			return
		}

		parts := strings.SplitN(authHeader, " ", 2)
		if len(parts) != 2 || !strings.EqualFold(parts[0], "bearer") {
			c.AbortWithStatusJSON(http.StatusUnauthorized, gin.H{"error": "invalid authorization format"})
			return
		}

		claims, err := j.validateToken(parts[1])
		if err != nil {
			c.AbortWithStatusJSON(http.StatusUnauthorized, gin.H{"error": "invalid token", "details": err.Error()})
			return
		}

		// Detect token type and set context accordingly
		if tokenType, ok := claims["token_type"].(string); ok && tokenType == "service" {
			// Service token (service-to-service auth)
			c.Set("is_service_token", true)
			c.Set("service_name", claims["service_name"])
			c.Set("client_id", claims["client_id"])
			if scopes, ok := claims["scopes"].([]interface{}); ok {
				c.Set("scopes", scopes)
			}
		} else {
			// User token (user authentication)
			c.Set("is_service_token", false)
			// user_id can be float64 from JSON or string (UUID)
			switch v := claims["user_id"].(type) {
			case float64:
				c.Set("user_id", fmt.Sprintf("%.0f", v))
			case string:
				c.Set("user_id", v)
			}
			if email, ok := claims["email"].(string); ok {
				c.Set("email", email)
			}
			if username, ok := claims["username"].(string); ok {
				c.Set("username", username)
			}
			if roles, ok := claims["roles"].([]interface{}); ok {
				c.Set("roles", roles)
			}
		}

		c.Set("all_claims", claims)
		c.Next()
	}
}

// RequireAuth requires a valid token (user or service)
func (j *JWKSAuth) RequireAuth() gin.HandlerFunc {
	return func(c *gin.Context) {
		_, exists := c.Get("user_id")
		isService, _ := c.Get("is_service_token")
		if !exists && isService != true {
			c.AbortWithStatusJSON(http.StatusUnauthorized, gin.H{"error": "authentication required"})
			return
		}
		c.Next()
	}
}

// RequireUser requires a user token (not service token)
func (j *JWKSAuth) RequireUser() gin.HandlerFunc {
	return func(c *gin.Context) {
		isService, _ := c.Get("is_service_token")
		if isService == true {
			c.AbortWithStatusJSON(http.StatusForbidden, gin.H{"error": "user token required, service token not allowed"})
			return
		}
		userID, exists := c.Get("user_id")
		if !exists || userID == "" {
			c.AbortWithStatusJSON(http.StatusUnauthorized, gin.H{"error": "user authentication required"})
			return
		}
		c.Next()
	}
}

// RequireScope requires a service token with specific scope
func (j *JWKSAuth) RequireScope(scope string) gin.HandlerFunc {
	return func(c *gin.Context) {
		scopesRaw, exists := c.Get("scopes")
		if !exists {
			c.AbortWithStatusJSON(http.StatusForbidden, gin.H{"error": "no scopes in token"})
			return
		}
		scopes, ok := scopesRaw.([]interface{})
		if !ok {
			c.AbortWithStatusJSON(http.StatusForbidden, gin.H{"error": "invalid scopes"})
			return
		}
		for _, s := range scopes {
			if s == scope {
				c.Next()
				return
			}
		}
		c.AbortWithStatusJSON(http.StatusForbidden, gin.H{"error": "insufficient scope", "required": scope})
	}
}

// GetUserID extracts user ID from context (helper)
func GetUserID(c *gin.Context) string {
	if v, exists := c.Get("user_id"); exists {
		if s, ok := v.(string); ok {
			return s
		}
	}
	return ""
}

// IsServiceToken checks if current token is a service token
func IsServiceToken(c *gin.Context) bool {
	if v, exists := c.Get("is_service_token"); exists {
		if b, ok := v.(bool); ok {
			return b
		}
	}
	return false
}

// ==================== Internal ====================

type tokenClaims map[string]interface{}

func (j *JWKSAuth) validateToken(tokenStr string) (tokenClaims, error) {
	parts := strings.Split(tokenStr, ".")
	if len(parts) != 3 {
		return nil, fmt.Errorf("invalid token format")
	}

	header, err := base64Decode(parts[0])
	if err != nil {
		return nil, fmt.Errorf("invalid token header")
	}

	var headerMap map[string]interface{}
	if err := json.Unmarshal(header, &headerMap); err != nil {
		return nil, fmt.Errorf("invalid token header json")
	}

	kid, _ := headerMap["kid"].(string)

	// If no kid → try HMAC (user token from SSO login)
	if kid == "" {
		if j.hmacSecret == "" {
			return nil, fmt.Errorf("missing key id and no HMAC secret configured")
		}
		return j.validateHMAC(parts, j.hmacSecret)
	}

	// Has kid → RSA token (service token)
	pubKey, err := j.getPublicKey(kid)
	if err != nil {
		return nil, fmt.Errorf("failed to get public key: %w", err)
	}

	payload := parts[0] + "." + parts[1]
	signature, err := base64URLDecode(parts[2])
	if err != nil {
		return nil, fmt.Errorf("invalid signature encoding")
	}

	if err := verifyRS256([]byte(payload), signature, pubKey); err != nil {
		return nil, fmt.Errorf("signature verification failed: %w", err)
	}

	payloadBytes, err := base64Decode(parts[1])
	if err != nil {
		return nil, fmt.Errorf("invalid token payload")
	}

	var claims tokenClaims
	if err := json.Unmarshal(payloadBytes, &claims); err != nil {
		return nil, fmt.Errorf("invalid token claims")
	}

	if exp, ok := claims["exp"].(float64); ok {
		if time.Now().Unix() > int64(exp) {
			return nil, fmt.Errorf("token expired")
		}
	}

	return claims, nil
}

// validateHMAC validates HMAC-SHA256 tokens (user tokens from SSO)
func (j *JWKSAuth) validateHMAC(parts []string, secret string) (tokenClaims, error) {
	// Verify HMAC-SHA256 signature
	payload := parts[0] + "." + parts[1]
	signatureBytes, err := base64URLDecode(parts[2])
	if err != nil {
		return nil, fmt.Errorf("invalid HMAC signature encoding")
	}

	mac := hmac.New(sha256.New, []byte(secret))
	mac.Write([]byte(payload))
	expectedSig := mac.Sum(nil)

	if !hmac.Equal(signatureBytes, expectedSig) {
		return nil, fmt.Errorf("HMAC signature verification failed")
	}

	payloadBytes, err := base64Decode(parts[1])
	if err != nil {
		return nil, fmt.Errorf("invalid token payload")
	}

	var claims tokenClaims
	if err := json.Unmarshal(payloadBytes, &claims); err != nil {
		return nil, fmt.Errorf("invalid token claims")
	}

	if exp, ok := claims["exp"].(float64); ok {
		if time.Now().Unix() > int64(exp) {
			return nil, fmt.Errorf("token expired")
		}
	}

	return claims, nil
}


func (j *JWKSAuth) getPublicKey(kid string) (*rsa.PublicKey, error) {
	j.mu.RLock()
	if key, ok := j.publicKeys[kid]; ok && time.Since(j.lastFetch) < j.cacheTTL {
		j.mu.RUnlock()
		return key, nil
	}
	j.mu.RUnlock()

	if err := j.fetchJWKS(); err != nil {
		return nil, err
	}

	j.mu.RLock()
	defer j.mu.RUnlock()
	if key, ok := j.publicKeys[kid]; ok {
		return key, nil
	}
	return nil, fmt.Errorf("key not found: %s", kid)
}

func (j *JWKSAuth) fetchJWKS() error {
	resp, err := http.Get(j.ssoURL + "/.well-known/jwks.json")
	if err != nil {
		return fmt.Errorf("failed to fetch JWKS: %w", err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return fmt.Errorf("failed to read JWKS: %w", err)
	}

	var jwks struct {
		Keys []struct {
			Kid string `json:"kid"`
			Kty string `json:"kty"`
			N   string `json:"n"`
			E   string `json:"e"`
		} `json:"keys"`
	}

	if err := json.Unmarshal(body, &jwks); err != nil {
		return fmt.Errorf("failed to parse JWKS: %w", err)
	}

	j.mu.Lock()
	defer j.mu.Unlock()
	for _, key := range jwks.Keys {
		if key.Kty == "RSA" {
			if pubKey, err := buildRSAPublicKey(key.N, key.E); err == nil {
				j.publicKeys[key.Kid] = pubKey
			}
		}
	}
	j.lastFetch = time.Now()
	return nil
}
