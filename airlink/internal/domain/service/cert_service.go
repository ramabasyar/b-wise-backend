package service

import (
	"bytes"
	"fmt"
	"os/exec"
	"strings"
	"time"
)

// CertService — menerbitkan & mencabut sertifikat WiFi via SSH ke host FreeRADIUS
type CertService struct {
	sshHost  string // user@host
	certTTL  time.Duration
}

func NewCertService(sshUser, sshHost string) *CertService {
	return &CertService{
		sshHost:  sshUser + "@" + sshHost,
		certTTL: 1460 * 24 * time.Hour, // 4 tahun
	}
}

// IssueResult — hasil dari issue-wifi-cert.sh
type IssueResult struct {
	OK         bool   `json:"ok"`
	P12Path    string `json:"p12_path"`
	P12Pass    string `json:"-" gorm:"-"` // tidak diserialisasi ke response
	Username   string `json:"username"`
	DeviceName string `json:"device_name"`
	Total      int    `json:"total"`
	Raw        string `json:"-" gorm:"-"`
}

// Issue — terbitkan sertifikat baru via SSH
// Menjalankan: sudo /usr/local/sbin/issue-wifi-cert.sh <username> <device>
func (s *CertService) Issue(username, device string) (*IssueResult, error) {
	if username == "" || device == "" {
		return nil, fmt.Errorf("username dan device wajib diisi")
	}
	// Sanitize: hanya alphanumeric, dash, underscore, titik
	username = sanitizeInput(username)
	device = sanitizeInput(device)
	if username == "" || device == "" {
		return nil, fmt.Errorf("input tidak valid")
	}

	cmd := exec.Command("ssh", "-o", "BatchMode=yes", "-o", "ConnectTimeout=10",
		s.sshHost,
		fmt.Sprintf("sudo /usr/local/sbin/issue-wifi-cert.sh %s %s", username, device))

	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	if err := cmd.Run(); err != nil {
		return nil, fmt.Errorf("ssh exec: %v, stderr: %s", err, stderr.String())
	}

	output := stdout.String()
	result := &IssueResult{
		OK:         strings.Contains(output, "OK p12="),
		Username:   username,
		DeviceName: device,
		Raw:        output,
	}

	// Parse output: "OK p12=/var/lib/radius-certs/user/device.p12 pass=XXX cn=user ou=device total=N"
	if result.OK {
		for _, line := range strings.Split(output, "\n") {
			if strings.HasPrefix(line, "OK p12=") {
				fields := strings.Fields(line)
				for _, f := range fields {
					if strings.HasPrefix(f, "p12=") {
						result.P12Path = strings.TrimPrefix(f, "p12=")
					}
					if strings.HasPrefix(f, "pass=") {
						result.P12Pass = strings.TrimPrefix(f, "pass=")
					}
					if strings.HasPrefix(f, "total=") {
						fmt.Sscanf(f, "total=%d", &result.Total)
					}
				}
			}
			if strings.HasPrefix(line, "LIMIT:") {
				return nil, fmt.Errorf("limit tercapai: %s", line)
			}
		}
	}

	return result, nil
}

// Revoke — cabut sertifikat via SSH
// Menjalankan: sudo /usr/local/sbin/revoke-wifi-cert.sh <username> <device>
func (s *CertService) Revoke(username, device, reason string) error {
	if username == "" || device == "" {
		return fmt.Errorf("username dan device wajib diisi")
	}
	username = sanitizeInput(username)
	device = sanitizeInput(device)

	cmd := exec.Command("ssh", "-o", "BatchMode=yes", "-o", "ConnectTimeout=10",
		s.sshHost,
		fmt.Sprintf("sudo /usr/local/sbin/revoke-wifi-cert.sh %s %s", username, device))

	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	if err := cmd.Run(); err != nil {
		return fmt.Errorf("ssh exec revoke: %v, stderr: %s", err, stderr.String())
	}

	output := stdout.String()
	if !strings.Contains(output, "REVOKED") {
		return fmt.Errorf("revoke gagal: %s", output)
	}
	return nil
}

// FetchP12 — ambil file .p12 dari server RADIUS via SCP
func (s *CertService) FetchP12(remotePath string) ([]byte, error) {
	if !strings.HasPrefix(remotePath, "/var/lib/radius-certs/") {
		return nil, fmt.Errorf("path tidak valid: harus di bawah /var/lib/radius-certs/")
	}
	cmd := exec.Command("scp", "-o", "BatchMode=yes", "-o", "ConnectTimeout=10",
		s.sshHost+":"+remotePath, "/dev/stdout")
	var stdout bytes.Buffer
	cmd.Stdout = &stdout
	if err := cmd.Run(); err != nil {
		return nil, fmt.Errorf("scp fetch: %v", err)
	}
	return stdout.Bytes(), nil
}

func sanitizeInput(s string) string {
	var b strings.Builder
	for _, r := range s {
		if (r >= 'a' && r <= 'z') || (r >= 'A' && r <= 'Z') || (r >= '0' && r <= '9') || r == '-' || r == '_' || r == '.' {
			b.WriteRune(r)
		}
	}
	return b.String()
}
