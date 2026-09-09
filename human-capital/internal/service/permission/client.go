package permission

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"time"
)

// PermissionClient handles communication with Permission Service
type PermissionClient struct {
	baseURL    string
	httpClient *http.Client
	// Reuses SSO client's service token
	getServiceToken func() (string, error)
}

// NewPermissionClient creates a new permission client
func NewPermissionClient(baseURL string, getServiceToken func() (string, error)) *PermissionClient {
	return &PermissionClient{
		baseURL:         baseURL,
		httpClient:      &http.Client{Timeout: 10 * time.Second},
		getServiceToken: getServiceToken,
	}
}

// GrantAccess grants user access to a service
func (c *PermissionClient) GrantAccess(userID, serviceID, grantedBy string) error {
	return c.doPost("/api/services/"+serviceID+"/access", map[string]interface{}{
		"user_id":    userID,
		"granted_by": grantedBy,
	})
}

// AssignRole assigns a role to a user
func (c *PermissionClient) AssignRole(userID, roleID, grantedBy string) error {
	return c.doPost("/api/roles/assign", map[string]interface{}{
		"user_id":   userID,
		"role_id":   roleID,
	})
}

// GetUserMenu gets the menu for a user
func (c *PermissionClient) GetUserMenu(userID string) ([]map[string]interface{}, error) {
	token, err := c.getServiceToken()
	if err != nil {
		return nil, err
	}

	req, _ := http.NewRequest("GET", c.baseURL+"/api/users/"+userID+"/menu", nil)
	req.Header.Set("Authorization", "Bearer "+token)

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	body, _ := io.ReadAll(resp.Body)
	var result struct {
		Success bool                     `json:"success"`
		Data    []map[string]interface{} `json:"data"`
	}
	if err := json.Unmarshal(body, &result); err != nil {
		return nil, err
	}

	return result.Data, nil
}

func (c *PermissionClient) doPost(path string, payload interface{}) error {
	token, err := c.getServiceToken()
	if err != nil {
		return err
	}

	body, _ := json.Marshal(payload)
	req, err := http.NewRequest("POST", c.baseURL+path, bytes.NewBuffer(body))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+token)

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return fmt.Errorf("permission service request failed: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode >= 400 {
		respBody, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("permission service error (%d): %s", resp.StatusCode, string(respBody))
	}

	return nil
}
