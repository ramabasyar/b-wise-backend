package middleware

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/gin-gonic/gin"
)

// PermissionCheck verifies user permissions via Permission Service.
//
// Selaras dengan Permission Service hasil Tahap 1-3:
//   - HasPermission mendukung wildcard '*' (grant '*' = semua permission di service)
//   - Batch check: POST /api/users/:id/permissions/check-batch
//   - Cache invalidation otomatis di sisi Permission Service (Redis, TTL 2m)
//
// Pemakaian:
//
//	permCheck := middleware.NewPermissionCheck(cfg.SSO.PermissionServiceURL, cfg.SSO.ServiceName).
//		WithSSO(cfg.SSO.URL, cfg.SSO.ServiceClientID, cfg.SSO.ServiceClientSecret)
//	employees.GET("", permCheck.RequirePermission("employees.read"), h.List)
//	tools.GET("", permCheck.RequireAnyPermission("tools.read", "tools.admin"), h.List)
type PermissionCheck struct {
	permissionServiceURL string
	serviceName          string // nama service ini terdaftar di Permission Service (REQUIRED)

	serviceClientID     string
	serviceClientSecret string
	ssoURL              string
	httpClient          *http.Client

	// Service token cache (dengan expiry — token SSO service 15 menit)
	tokenMu      sync.Mutex
	serviceToken string
	tokenExpiry  time.Time

	// Service ID cache (name → UUID resolution, TTL 5 menit)
	idMu        sync.Mutex
	serviceID   string
	idFetchedAt time.Time
}

// NewPermissionCheck creates a new permission checker.
// serviceName WAJIB diisi: nama service ini di Permission Service
// (dipakai resolve service_id & menghindari hardcode per repo).
func NewPermissionCheck(permissionServiceURL, serviceName string) *PermissionCheck {
	return &PermissionCheck{
		permissionServiceURL: strings.TrimRight(permissionServiceURL, "/"),
		serviceName:          serviceName,
		httpClient:           &http.Client{Timeout: 5 * time.Second},
	}
}

// WithSSO configures SSO credentials for service-to-service auth to Permission Service
func (p *PermissionCheck) WithSSO(ssoURL, clientID, clientSecret string) *PermissionCheck {
	p.ssoURL = strings.TrimRight(ssoURL, "/")
	p.serviceClientID = clientID
	p.serviceClientSecret = clientSecret
	return p
}

// ==================== Gin Middleware ====================

// RequirePermission checks if user has a specific permission for this service.
// Wildcard '*' di sisi server meloloskan semua permission.
func (p *PermissionCheck) RequirePermission(permission string) gin.HandlerFunc {
	return func(c *gin.Context) {
		if IsServiceToken(c) {
			c.Next()
			return
		}

		userID, ok := GetUserID(c)
		if !ok || userID == "" {
			c.AbortWithStatusJSON(http.StatusUnauthorized, gin.H{"error": "user authentication required"})
			return
		}

		results, err := p.CheckPermissions(userID, []string{permission})
		if err != nil {
			c.AbortWithStatusJSON(http.StatusInternalServerError, gin.H{
				"error":   "failed to check permission",
				"details": err.Error(),
			})
			return
		}

		if !results[permission] {
			c.AbortWithStatusJSON(http.StatusForbidden, gin.H{
				"error":                "insufficient permissions",
				"required_permission": permission,
			})
			return
		}

		c.Next()
	}
}

// RequireAnyPermission meloloskan jika user punya SALAH SATU permission (batch check, 1x call).
func (p *PermissionCheck) RequireAnyPermission(perms ...string) gin.HandlerFunc {
	return func(c *gin.Context) {
		if IsServiceToken(c) {
			c.Next()
			return
		}

		userID, ok := GetUserID(c)
		if !ok || userID == "" {
			c.AbortWithStatusJSON(http.StatusUnauthorized, gin.H{"error": "user authentication required"})
			return
		}

		results, err := p.CheckPermissions(userID, perms)
		if err != nil {
			c.AbortWithStatusJSON(http.StatusInternalServerError, gin.H{
				"error":   "failed to check permissions",
				"details": err.Error(),
			})
			return
		}

		for _, perm := range perms {
			if results[perm] {
				c.Next()
				return
			}
		}

		c.AbortWithStatusJSON(http.StatusForbidden, gin.H{
			"error":                 "insufficient permissions",
			"required_permissions": perms,
			"mode":                  "any",
		})
	}
}

// RequireAllPermissions meloloskan jika user punya SEMUA permission (batch check, 1x call).
func (p *PermissionCheck) RequireAllPermissions(perms ...string) gin.HandlerFunc {
	return func(c *gin.Context) {
		if IsServiceToken(c) {
			c.Next()
			return
		}

		userID, ok := GetUserID(c)
		if !ok || userID == "" {
			c.AbortWithStatusJSON(http.StatusUnauthorized, gin.H{"error": "user authentication required"})
			return
		}

		results, err := p.CheckPermissions(userID, perms)
		if err != nil {
			c.AbortWithStatusJSON(http.StatusInternalServerError, gin.H{
				"error":   "failed to check permissions",
				"details": err.Error(),
			})
			return
		}

		for _, perm := range perms {
			if !results[perm] {
				c.AbortWithStatusJSON(http.StatusForbidden, gin.H{
					"error":                 "insufficient permissions",
					"required_permissions": perms,
					"mode":                  "all",
					"missing":               perm,
				})
				return
			}
		}

		c.Next()
	}
}

// CheckAccess checks if user has access to this service
func (p *PermissionCheck) CheckAccess() gin.HandlerFunc {
	return func(c *gin.Context) {
		if IsServiceToken(c) {
			c.Next()
			return
		}

		userID, ok := GetUserID(c)
		if !ok || userID == "" {
			c.AbortWithStatusJSON(http.StatusUnauthorized, gin.H{"error": "user authentication required"})
			return
		}

		serviceID, err := p.getServiceID()
		if err != nil {
			c.AbortWithStatusJSON(http.StatusInternalServerError, gin.H{"error": "failed to get service info"})
			return
		}

		hasAccess, err := p.checkAccess(userID, serviceID)
		if err != nil {
			c.AbortWithStatusJSON(http.StatusInternalServerError, gin.H{"error": "failed to check access"})
			return
		}

		if !hasAccess {
			c.AbortWithStatusJSON(http.StatusForbidden, gin.H{"error": "no access to this service"})
			return
		}

		c.Next()
	}
}

// ==================== Handler-level API ====================

// CheckPermissions batch-check banyak permission sekali call.
// Dipakai RequirePermission/RequireAny/RequireAll; boleh juga dipanggil
// langsung dari handler (e.g., untuk conditional logic).
func (p *PermissionCheck) CheckPermissions(userID string, permissions []string) (map[string]bool, error) {
	serviceID, err := p.getServiceID()
	if err != nil {
		return nil, err
	}

	payload, _ := json.Marshal(map[string]interface{}{
		"service_id":  serviceID,
		"permissions": permissions,
	})

	body, err := p.doRequest("POST",
		p.permissionServiceURL+"/api/users/"+userID+"/permissions/check-batch",
		payload)
	if err != nil {
		return nil, err
	}

	var result struct {
		Success bool `json:"success"`
		Data    struct {
			ServiceID string          `json:"service_id"`
			Results   map[string]bool `json:"results"`
		} `json:"data"`
	}
	if err := json.Unmarshal(body, &result); err != nil {
		return nil, err
	}
	if !result.Success || result.Data.Results == nil {
		return nil, fmt.Errorf("permission service rejected batch check")
	}
	return result.Data.Results, nil
}

// ==================== Internal ====================

// getServiceID resolve nama service → UUID, di-cache 5 menit
// (dulu: list semua service di SETIAP check — boros).
func (p *PermissionCheck) getServiceID() (string, error) {
	if p.serviceName == "" {
		return "", fmt.Errorf("service name not configured — set SERVICE_NAME (sso.service_name)")
	}

	p.idMu.Lock()
	defer p.idMu.Unlock()
	if p.serviceID != "" && time.Since(p.idFetchedAt) < 5*time.Minute {
		return p.serviceID, nil
	}

	body, err := p.doRequest("GET", p.permissionServiceURL+"/api/services?page=1&page_size=100", nil)
	if err != nil {
		return "", err
	}

	var result struct {
		Success bool              `json:"success"`
		Data    []serviceListItem `json:"data"`
	}
	if err := json.Unmarshal(body, &result); err != nil {
		return "", err
	}

	for _, svc := range result.Data {
		if svc.Name == p.serviceName {
			p.serviceID = svc.ID
			p.idFetchedAt = time.Now()
			return p.serviceID, nil
		}
	}
	return "", fmt.Errorf("service not registered in permission service: %q (jalankan POST /api/services)", p.serviceName)
}

type serviceListItem struct {
	ID   string `json:"id"`
	Name string `json:"name"`
}

// getServiceToken gets or refreshes a service token from SSO (dengan expiry)
func (p *PermissionCheck) getServiceToken(force bool) (string, error) {
	p.tokenMu.Lock()
	defer p.tokenMu.Unlock()

	if !force && p.serviceToken != "" && time.Now().Before(p.tokenExpiry) {
		return p.serviceToken, nil
	}

	if p.ssoURL == "" || p.serviceClientID == "" {
		// No SSO configured - try without token (backwards compatibility)
		return "", nil
	}

	payload, _ := json.Marshal(map[string]string{
		"client_id":     p.serviceClientID,
		"client_secret": p.serviceClientSecret,
		"grant_type":    "client_credentials",
	})

	// SSO service-registry: POST /api/oauth/token (client credentials)
	resp, err := p.httpClient.Post(p.ssoURL+"/api/oauth/token", "application/json", bytes.NewReader(payload))
	if err != nil {
		return "", fmt.Errorf("failed to get service token: %w", err)
	}
	defer resp.Body.Close()

	respBody, _ := io.ReadAll(resp.Body)
	// SSO membungkus token di data{...}; fallback top-level utk kompatibilitas
	var result struct {
		AccessToken string `json:"access_token"`
		ExpiresIn   int64  `json:"expires_in"`
		Data        struct {
			AccessToken string `json:"access_token"`
			ExpiresIn   int64  `json:"expires_in"`
		} `json:"data"`
	}
	if err := json.Unmarshal(respBody, &result); err != nil {
		return "", fmt.Errorf("failed to parse service token response (status %d)", resp.StatusCode)
	}
	accessToken := result.Data.AccessToken
	expiresIn := result.Data.ExpiresIn
	if accessToken == "" {
		accessToken = result.AccessToken
		expiresIn = result.ExpiresIn
	}
	if accessToken == "" {
		snippet := respBody
		if len(snippet) > 120 {
			snippet = snippet[:120]
		}
		return "", fmt.Errorf("empty service token from SSO (status %d): %s", resp.StatusCode, string(snippet))
	}

	p.serviceToken = accessToken
	// refresh 30 detik sebelum expiry
	ttl := time.Duration(expiresIn) * time.Second
	if ttl <= 0 {
		ttl = 10 * time.Minute // fallback aman (token service SSO 15 menit)
	}
	p.tokenExpiry = time.Now().Add(ttl - 30*time.Second)
	return p.serviceToken, nil
}

// doRequest executes HTTP dengan auth; auto-retry sekali saat token expired (401)
func (p *PermissionCheck) doRequest(method, url string, reqBody []byte) ([]byte, error) {
	token, _ := p.getServiceToken(false)

	do := func(tok string) (*http.Response, error) {
		var rdr io.Reader
		if reqBody != nil {
			rdr = bytes.NewReader(reqBody)
		}
		req, err := http.NewRequest(method, url, rdr)
		if err != nil {
			return nil, err
		}
		if reqBody != nil {
			req.Header.Set("Content-Type", "application/json")
		}
		if tok != "" {
			req.Header.Set("Authorization", "Bearer "+tok)
		}
		return p.httpClient.Do(req)
	}

	resp, err := do(token)
	if err != nil {
		return nil, fmt.Errorf("permission service request failed: %w", err)
	}

	// Token expired → force refresh sekali
	if resp.StatusCode == http.StatusUnauthorized && token != "" {
		resp.Body.Close()
		newToken, _ := p.getServiceToken(true)
		if newToken != "" && newToken != token {
			resp, err = do(newToken)
			if err != nil {
				return nil, fmt.Errorf("permission service request failed: %w", err)
			}
		}
	}
	defer resp.Body.Close()

	body, _ := io.ReadAll(resp.Body)
	return body, nil
}

func (p *PermissionCheck) checkAccess(userID, serviceID string) (bool, error) {
	body, err := p.doRequest("GET",
		p.permissionServiceURL+"/api/users/"+userID+"/services/"+serviceID+"/check", nil)
	if err != nil {
		return false, err
	}

	var result struct {
		Success bool                   `json:"success"`
		Data    map[string]interface{} `json:"data"`
	}
	if err := json.Unmarshal(body, &result); err != nil {
		return false, err
	}

	if has, ok := result.Data["has_access"].(bool); ok {
		return has, nil
	}
	return false, nil
}
