package sso

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"sync"
	"time"
)

// UserInfo represents user information from SSO
type UserInfo struct {
	ID            string                 `json:"id"`
	Username      string                 `json:"username"`
	Email         string                 `json:"email"`
	FirstName     string                 `json:"first_name"`
	LastName      string                 `json:"last_name"`
	Status        string                 `json:"status"`
	EmailVerified bool                   `json:"email_verified"`
	Metadata      map[string]interface{} `json:"metadata"`
}

// Permission represents a permission from SSO
type Permission struct {
	ID          string `json:"id"`
	Name        string `json:"name"`
	Resource    string `json:"resource"`
	Action      string `json:"action"`
	Description string `json:"description"`
}

// loginRequest represents SSO login request
type loginRequest struct {
	Email    string `json:"email"`
	Password string `json:"password"`
}

// loginResponse represents SSO login response
type loginResponse struct {
	Success bool `json:"success"`
	Data    struct {
		AccessToken string `json:"access_token"`
	} `json:"data"`
}

// apiResponse represents a standard SSO API response
type apiResponse struct {
	Success bool        `json:"success"`
	Data    interface{} `json:"data"`
	Error   struct {
		Message string `json:"message"`
		Code    int    `json:"code"`
	} `json:"error"`
}

// checkPermissionResponse represents SSO permission check response
type checkPermissionResponse struct {
	Success bool `json:"success"`
	Data    struct {
		HasPermission bool `json:"has_permission"`
	} `json:"data"`
}

// Client is an HTTP client for the SSO service
type Client struct {
	baseURL    string
	email      string
	password   string
	httpClient *http.Client
	token      string
	tokenExp   time.Time
	mu         sync.RWMutex
	cache      Cache
}

// NewClient creates a new SSO client
func NewClient(baseURL, email, password string, cache Cache) *Client {
	return &Client{
		baseURL:  baseURL,
		email:    email,
		password: password,
		httpClient: &http.Client{
			Timeout: 10 * time.Second,
		},
		cache: cache,
	}
}

// GetUser fetches user info from SSO (with cache)
func (c *Client) GetUser(ctx context.Context, userID string) (*UserInfo, error) {
	cacheKey := fmt.Sprintf("sso:user:%s", userID)

	// Try cache first
	if c.cache != nil {
		if cached, err := c.cache.Get(ctx, cacheKey); err == nil && cached != "" {
			var user UserInfo
			if err := json.Unmarshal([]byte(cached), &user); err == nil {
				return &user, nil
			}
		}
	}

	// Call SSO API
	url := fmt.Sprintf("%s/api/v1/users/%s", c.baseURL, userID)
	body, err := c.doRequest(ctx, http.MethodGet, url, nil)
	if err != nil {
		// Return cached data if available (graceful fallback)
		if c.cache != nil {
			if cached, cacheErr := c.cache.Get(ctx, cacheKey); cacheErr == nil && cached != "" {
				var user UserInfo
				if jsonErr := json.Unmarshal([]byte(cached), &user); jsonErr == nil {
					log.Printf("SSO unreachable, returning cached user data for %s", userID)
					return &user, nil
				}
			}
		}
		return nil, fmt.Errorf("failed to get user from SSO: %w", err)
	}

	var resp apiResponse
	resp.Data = &UserInfo{}
	if err := json.Unmarshal(body, &resp); err != nil {
		return nil, fmt.Errorf("failed to parse SSO response: %w", err)
	}

	if !resp.Success {
		return nil, fmt.Errorf("SSO error: %s", resp.Error.Message)
	}

	user, ok := resp.Data.(*UserInfo)
	if !ok {
		return nil, fmt.Errorf("unexpected SSO response format")
	}

	// Cache the result
	if c.cache != nil {
		if data, err := json.Marshal(user); err == nil {
			_ = c.cache.Set(ctx, cacheKey, string(data))
		}
	}

	return user, nil
}

// CheckPermission checks if a user has a specific permission (with cache)
func (c *Client) CheckPermission(ctx context.Context, userID, resource, action string) (bool, error) {
	cacheKey := fmt.Sprintf("sso:perm:%s:%s:%s", userID, resource, action)

	// Try cache first
	if c.cache != nil {
		if cached, err := c.cache.Get(ctx, cacheKey); err == nil && cached != "" {
			return cached == "true", nil
		}
	}

	// Call SSO API
	url := fmt.Sprintf("%s/api/v1/roles/users/%s/check?resource=%s&action=%s", c.baseURL, userID, resource, action)
	body, err := c.doRequest(ctx, http.MethodGet, url, nil)
	if err != nil {
		// Return cached data if available
		if c.cache != nil {
			if cached, cacheErr := c.cache.Get(ctx, cacheKey); cacheErr == nil && cached != "" {
				return cached == "true", nil
			}
		}
		return false, fmt.Errorf("failed to check permission from SSO: %w", err)
	}

	var resp checkPermissionResponse
	if err := json.Unmarshal(body, &resp); err != nil {
		return false, fmt.Errorf("failed to parse SSO response: %w", err)
	}

	result := resp.Data.HasPermission

	// Cache the result
	if c.cache != nil {
		resultStr := "false"
		if result {
			resultStr = "true"
		}
		_ = c.cache.Set(ctx, cacheKey, resultStr)
	}

	return result, nil
}

// GetUserPermissions fetches all permissions for a user (with cache)
func (c *Client) GetUserPermissions(ctx context.Context, userID string) ([]Permission, error) {
	cacheKey := fmt.Sprintf("sso:perms:%s", userID)

	// Try cache first
	if c.cache != nil {
		if cached, err := c.cache.Get(ctx, cacheKey); err == nil && cached != "" {
			var perms []Permission
			if err := json.Unmarshal([]byte(cached), &perms); err == nil {
				return perms, nil
			}
		}
	}

	// Call SSO API
	url := fmt.Sprintf("%s/api/v1/roles/users/%s/permissions", c.baseURL, userID)
	body, err := c.doRequest(ctx, http.MethodGet, url, nil)
	if err != nil {
		// Return cached data if available
		if c.cache != nil {
			if cached, cacheErr := c.cache.Get(ctx, cacheKey); cacheErr == nil && cached != "" {
				var perms []Permission
				if jsonErr := json.Unmarshal([]byte(cached), &perms); jsonErr == nil {
					return perms, nil
				}
			}
		}
		return nil, fmt.Errorf("failed to get user permissions from SSO: %w", err)
	}

	var resp struct {
		Success bool        `json:"success"`
		Data    interface{} `json:"data"`
	}
	if err := json.Unmarshal(body, &resp); err != nil {
		return nil, fmt.Errorf("failed to parse SSO response: %w", err)
	}

	// The data can be in different formats - try to parse as permissions list
	dataBytes, err := json.Marshal(resp.Data)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal SSO response data: %w", err)
	}

	var perms []Permission
	if err := json.Unmarshal(dataBytes, &perms); err != nil {
		// Try as object with permissions field
		var permObj struct {
			Permissions []Permission `json:"permissions"`
		}
		if err := json.Unmarshal(dataBytes, &permObj); err != nil {
			return nil, fmt.Errorf("failed to parse permissions from SSO response: %w", err)
		}
		perms = permObj.Permissions
	}

	// Cache the result
	if c.cache != nil {
		if data, err := json.Marshal(perms); err == nil {
			_ = c.cache.Set(ctx, cacheKey, string(data))
		}
	}

	return perms, nil
}

// doRequest makes an authenticated HTTP request to SSO
func (c *Client) doRequest(ctx context.Context, method, url string, body interface{}) ([]byte, error) {
	// Ensure we have a valid token
	if err := c.ensureToken(ctx); err != nil {
		return nil, fmt.Errorf("failed to authenticate with SSO: %w", err)
	}

	var reqBody io.Reader
	if body != nil {
		data, err := json.Marshal(body)
		if err != nil {
			return nil, fmt.Errorf("failed to marshal request body: %w", err)
		}
		reqBody = bytes.NewReader(data)
	}

	req, err := http.NewRequestWithContext(ctx, method, url, reqBody)
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}

	req.Header.Set("Authorization", "Bearer "+c.getToken())
	req.Header.Set("Content-Type", "application/json")

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("request failed: %w", err)
	}
	defer resp.Body.Close()

	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("failed to read response: %w", err)
	}

	if resp.StatusCode == http.StatusUnauthorized {
		// Token might be expired, retry once
		c.mu.Lock()
		c.token = ""
		c.mu.Unlock()

		if err := c.ensureToken(ctx); err != nil {
			return nil, fmt.Errorf("token refresh failed: %w", err)
		}

		// Retry the request
		req.Header.Set("Authorization", "Bearer "+c.getToken())
		resp, err = c.httpClient.Do(req)
		if err != nil {
			return nil, fmt.Errorf("retry request failed: %w", err)
		}
		defer resp.Body.Close()

		respBody, err = io.ReadAll(resp.Body)
		if err != nil {
			return nil, fmt.Errorf("failed to read retry response: %w", err)
		}
	}

	if resp.StatusCode >= 400 {
		return nil, fmt.Errorf("SSO returned status %d: %s", resp.StatusCode, string(respBody))
	}

	return respBody, nil
}

// ensureToken ensures we have a valid service account token
func (c *Client) ensureToken(ctx context.Context) error {
	c.mu.RLock()
	if c.token != "" && time.Now().Before(c.tokenExp) {
		c.mu.RUnlock()
		return nil
	}
	c.mu.RUnlock()

	c.mu.Lock()
	defer c.mu.Unlock()

	// Double-check after acquiring write lock
	if c.token != "" && time.Now().Before(c.tokenExp) {
		return nil
	}

	// Login to SSO
	loginReq := loginRequest{
		Email:    c.email,
		Password: c.password,
	}

	data, err := json.Marshal(loginReq)
	if err != nil {
		return fmt.Errorf("failed to marshal login request: %w", err)
	}

	url := c.baseURL + "/api/v1/auth/login"
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(data))
	if err != nil {
		return fmt.Errorf("failed to create login request: %w", err)
	}

	req.Header.Set("Content-Type", "application/json")

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return fmt.Errorf("login request failed: %w", err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return fmt.Errorf("failed to read login response: %w", err)
	}

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("SSO login failed with status %d: %s", resp.StatusCode, string(body))
	}

	var loginResp loginResponse
	if err := json.Unmarshal(body, &loginResp); err != nil {
		return fmt.Errorf("failed to parse login response: %w", err)
	}

	if !loginResp.Success || loginResp.Data.AccessToken == "" {
		return fmt.Errorf("SSO login unsuccessful")
	}

	c.token = loginResp.Data.AccessToken
	c.tokenExp = time.Now().Add(50 * time.Minute) // Refresh 10 min before typical 1h expiry

	return nil
}

// getToken returns the current token
func (c *Client) getToken() string {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return c.token
}
