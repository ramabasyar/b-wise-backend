package permission

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
)

// RoleInfo — ringkasan role dari permission-service (dipakai validasi wizard onboard).
type RoleInfo struct {
	ID          string `json:"id"`
	Name        string `json:"name"`
	ServiceID   string `json:"service_id"`
	Description string `json:"description"`
}

// ServiceInfo — ringkasan service terdaftar di permission-service.
type ServiceInfo struct {
	ID   string `json:"id"`
	Name string `json:"name"`
}

// GetRole — GET /api/roles/:id (service token). Dipakai validasi role⊆mapping (audit M4).
func (c *PermissionClient) GetRole(roleID string) (*RoleInfo, error) {
	token, err := c.getServiceToken()
	if err != nil {
		return nil, err
	}
	req, _ := http.NewRequest("GET", c.baseURL+"/api/roles/"+roleID, nil)
	req.Header.Set("Authorization", "Bearer "+token)
	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("permission service request failed: %w", err)
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(resp.Body)
	var out struct {
		Success bool     `json:"success"`
		Data    RoleInfo `json:"data"`
	}
	if err := json.Unmarshal(body, &out); err != nil || !out.Success || resp.StatusCode >= 400 {
		return nil, fmt.Errorf("role %s tidak ditemukan di permission-service (HTTP %d)", roleID, resp.StatusCode)
	}
	return &out.Data, nil
}

// ListServices — GET /api/services (service token). Dipakai UI mapping & validasi SetDepartmentServices.
func (c *PermissionClient) ListServices() ([]ServiceInfo, error) {
	token, err := c.getServiceToken()
	if err != nil {
		return nil, err
	}
	req, _ := http.NewRequest("GET", c.baseURL+"/api/services?page=1&page_size=100", nil)
	req.Header.Set("Authorization", "Bearer "+token)
	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("permission service request failed: %w", err)
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(resp.Body)
	var out struct {
		Success bool          `json:"success"`
		Data    []ServiceInfo `json:"data"`
	}
	if err := json.Unmarshal(body, &out); err != nil || !out.Success {
		return nil, fmt.Errorf("gagal memuat daftar service dari permission-service (HTTP %d)", resp.StatusCode)
	}
	return out.Data, nil
}
