package sso

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"sync"
	"time"
)

// SSOClient handles communication with SSO service
type SSOClient struct {
	ssoURL     string
	httpClient *http.Client

	// Service token cache
	serviceToken     string
	serviceClientID  string
	serviceSecret    string
	tokenMu          sync.Mutex
	tokenExpiry      time.Time
}

// NewSSOClient creates a new SSO client
func NewSSOClient(ssoURL, clientID, clientSecret string) *SSOClient {
	return &SSOClient{
		ssoURL:           ssoURL,
		serviceClientID:  clientID,
		serviceSecret:    clientSecret,
		httpClient:       &http.Client{Timeout: 10 * time.Second},
	}
}

// RegisterUserRequest represents the body for service-user registration
type RegisterUserRequest struct {
	Email     string   `json:"email"`
	Username  string   `json:"username"`
	Password  string   `json:"password"`
	FirstName string   `json:"first_name"`
	LastName  string   `json:"last_name"`
	BaseRole  string   `json:"base_role"` // realm primer (kompatibilitas)
	Roles     []string `json:"roles"`    // multi-realm (mis. dosen+staff)
}

// RegisterUserResponse represents the response from SSO
type RegisterUserResponse struct {
	Success bool `json:"success"`
	Data    struct {
		ID           string   `json:"id"`
		Username     string   `json:"username"`
		Email        string   `json:"email"`
		FirstName    string   `json:"first_name"`
		LastName     string   `json:"last_name"`
		BaseRole     string   `json:"base_role"` // kompatibilitas: display realm pertama
		Roles        []string `json:"roles"`     // semua realm slug
		Status       string   `json:"status"`
		RegisteredBy string   `json:"registered_by"`
	} `json:"data"`
	Error struct {
		Message string `json:"message"`
	} `json:"error"`
}

// GetServiceToken is the exported version for external use
func (c *SSOClient) GetServiceToken() (string, error) {
	return c.getServiceToken()
}

// getServiceToken gets a fresh service token from SSO
func (c *SSOClient) getServiceToken() (string, error) {
	c.tokenMu.Lock()
	defer c.tokenMu.Unlock()

	// Return cached token if still valid
	if c.serviceToken != "" && time.Now().Before(c.tokenExpiry) {
		return c.serviceToken, nil
	}

	body := fmt.Sprintf(`{"client_id":"%s","client_secret":"%s","grant_type":"client_credentials"}`,
		c.serviceClientID, c.serviceSecret)

	resp, err := c.httpClient.Post(c.ssoURL+"/api/oauth/token", "application/json", bytes.NewBufferString(body))
	if err != nil {
		return "", fmt.Errorf("SSO token request failed: %w", err)
	}
	defer resp.Body.Close()

	respBody, _ := io.ReadAll(resp.Body)
	var result struct {
		Data struct {
			AccessToken string `json:"access_token"`
			ExpiresIn   int    `json:"expires_in"`
		} `json:"data"`
		AccessToken string `json:"access_token"` // fallback top-level
		ExpiresIn   int    `json:"expires_in"`
	}
	if err := json.Unmarshal(respBody, &result); err != nil {
		return "", fmt.Errorf("failed to parse SSO token response (%s): %w", string(respBody[:min(len(respBody), 120)]), err)
	}

	token, expiry := result.Data.AccessToken, result.Data.ExpiresIn
	if token == "" {
		token, expiry = result.AccessToken, result.ExpiresIn
	}
	if token == "" {
		return "", fmt.Errorf("empty access token from SSO: %s", string(respBody[:min(len(respBody), 120)]))
	}
	if expiry <= 0 {
		expiry = 900
	}

	c.serviceToken = token
	c.tokenExpiry = time.Now().Add(time.Duration(expiry-60) * time.Second) // 1 min buffer

	return c.serviceToken, nil
}

func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}

// RegisterUser mendaftarkan user via endpoint publik SSO /api/auth/register.
// (SSO tidak punya endpoint service-user register — publik register + password policy.)
func (c *SSOClient) RegisterUser(req RegisterUserRequest) (*RegisterUserResponse, error) {
	lastName := req.LastName
	if len(lastName) < 3 { // SSO: min 3
		lastName = lastName + "."
	}
	payload := map[string]interface{}{
		"firstName": req.FirstName,
		"lastName":  lastName,
		"username":  req.Username,
		"email":     req.Email,
		"password":  req.Password,
	}
	// Realm roles (staff/lecture/student) — SSO hanya menghormati ini berauth service token.
	// Multi-realm (dosen+staff) dikirim via roles[]; single fallback via role.
	if len(req.Roles) > 0 {
		payload["roles"] = req.Roles
	} else if role := strings.TrimSpace(req.BaseRole); role != "" {
		payload["role"] = role
	}
	reqBody, _ := json.Marshal(payload)
	httpReq, err := http.NewRequest("POST", c.ssoURL+"/api/auth/register", bytes.NewBuffer(reqBody))
	if err != nil {
		return nil, err
	}
	httpReq.Header.Set("Content-Type", "application/json")
	// Selalu sertakan service token — supaya role realm dihormati SSO
	if token, tErr := c.GetServiceToken(); tErr == nil {
		httpReq.Header.Set("Authorization", "Bearer "+token)
	}

	resp, err := c.httpClient.Do(httpReq)
	if err != nil {
		return nil, fmt.Errorf("SSO register request failed: %w", err)
	}
	defer resp.Body.Close()

	respBody, _ := io.ReadAll(resp.Body)
	var raw struct {
		Success bool `json:"success"`
		Message string `json:"message"`
		Data    struct {
			User struct {
				ID        json.Number `json:"id"`
				Username  string `json:"username"`
				Email     string `json:"email"`
				FirstName string `json:"first_name"`
				LastName  string `json:"last_name"`
				Role      string `json:"role"`
				Roles     []string `json:"roles"`
			} `json:"user"`
		} `json:"data"`
		Error string `json:"error"` // bentuk: "REGISTER_ERROR" (kode)
	}
	if err := json.Unmarshal(respBody, &raw); err != nil {
		return nil, fmt.Errorf("failed to parse SSO response (%s): %w", string(respBody[:min(len(respBody), 120)]), err)
	}
	if resp.StatusCode >= 400 || !raw.Success {
		msg := raw.Message
		if msg == "" {
			msg = fmt.Sprintf("SSO registration failed (HTTP %d, code %s)", resp.StatusCode, raw.Error)
		}
		return nil, fmt.Errorf("%s", msg)
	}

	out := &RegisterUserResponse{Success: true}
	out.Data.ID = raw.Data.User.ID.String()
	out.Data.Username = raw.Data.User.Username
	out.Data.Email = raw.Data.User.Email
	out.Data.FirstName = raw.Data.User.FirstName
	out.Data.LastName = raw.Data.User.LastName
	out.Data.BaseRole = raw.Data.User.Role // utk verifikasi realm primer
	out.Data.Roles = raw.Data.User.Roles  // utk verifikasi semua realm
	out.Data.Status = "active"
	return out, nil
}
