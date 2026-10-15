package service

import (
	"bytes"
	"fmt"
	"os/exec"
	"regexp"
	"strings"
	"time"
)

// CertService — menerbitkan & mencabut sertifikat WiFi via SSH ke host FreeRADIUS
type CertService struct {
	coa     *CoAKicker // pengirim Disconnect-Request (CoA)
	sshHost string     // user@host
	certTTL time.Duration
}

func (s *CertService) SetCoaSecret(secret string) *CertService {
	s.coa = NewCoAKicker(s.sshHost, secret)
	return s
}

func NewCertService(sshUser, sshHost string) *CertService {
	return &CertService{
		sshHost: sshUser + "@" + sshHost,
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
	if !validUsername(username) || !validDevice(device) {
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

	if !result.OK || result.P12Path == "" || result.P12Pass == "" {
		return nil, fmt.Errorf("penerbitan sertifikat tidak menghasilkan paket yang lengkap")
	}
	return result, nil
}

// Revoke — cabut sertifikat via SSH
// Menjalankan: sudo /usr/local/sbin/revoke-wifi-cert.sh <username> <device>
func (s *CertService) Revoke(username, device, reason string) error {
	if !validUsername(username) || !validDevice(device) {
		return fmt.Errorf("input tidak valid")
	}

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

// FetchP12 — file root-only dibaca via sudo SSH, tidak melalui SCP user freerad.
func (s *CertService) FetchP12(username, device string) ([]byte, error) {
	if !validUsername(username) || !validDevice(device) {
		return nil, fmt.Errorf("input tidak valid")
	}
	remotePath := "/var/lib/radius-certs/" + username + "/" + device + ".p12"
	cmd := exec.Command("ssh", "-o", "BatchMode=yes", "-o", "ConnectTimeout=10",
		s.sshHost, "sudo cat "+remotePath)
	var stdout bytes.Buffer
	cmd.Stdout = &stdout
	if err := cmd.Run(); err != nil {
		return nil, fmt.Errorf("certificate fetch: %v", err)
	}
	if stdout.Len() == 0 || stdout.Len() > 1024*1024 {
		return nil, fmt.Errorf("ukuran paket sertifikat tidak valid")
	}
	return stdout.Bytes(), nil
}

var usernamePattern = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9@_-]{0,127}$`)
var devicePattern = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9_-]{0,63}$`)

func validUsername(s string) bool { return usernamePattern.MatchString(s) }
func validDevice(s string) bool   { return devicePattern.MatchString(s) }

func (s *CertService) CertMetadata(username, device string) (string, time.Time, error) {
	if !validUsername(username) || !validDevice(device) {
		return "", time.Time{}, fmt.Errorf("input tidak valid")
	}
	path := "/var/lib/radius-certs/" + username + "/" + device + ".crt"
	cmd := exec.Command("ssh", "-o", "BatchMode=yes", "-o", "ConnectTimeout=10",
		s.sshHost, "sudo openssl x509 -in "+path+" -noout -serial -enddate")
	out, err := cmd.Output()
	if err != nil {
		return "", time.Time{}, err
	}
	var serial string
	var expiry time.Time
	for _, line := range strings.Split(string(out), "\n") {
		if strings.HasPrefix(line, "serial=") {
			serial = strings.TrimPrefix(line, "serial=")
		}
		if strings.HasPrefix(line, "notAfter=") {
			expiry, err = time.Parse("Jan _2 15:04:05 2006 MST", strings.TrimPrefix(line, "notAfter="))
			if err != nil {
				return "", time.Time{}, err
			}
		}
	}
	if serial == "" || expiry.IsZero() {
		return "", time.Time{}, fmt.Errorf("metadata sertifikat tidak lengkap")
	}
	return serial, expiry, nil
}

// DisconnectSession mengirim RADIUS Disconnect-Request (CoA) ke NAS/AP.
// Return: error jika CoA gagal; nil jika Disconnect-ACK atau Disconnect-NAK.
func (s *CertService) DisconnectSession(username, sessionID, nasIP, callingStationID string) error {
	return s.coa.Disconnect(username, sessionID, nasIP, callingStationID)
}
