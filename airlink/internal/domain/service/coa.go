package service

import (
	"bytes"
	"fmt"
	"os/exec"
	"strings"
)

// CoAKicker mengirim RADIUS Disconnect-Request (CoA, RFC 5176) ke NAS/AP
// lewat radclient di host FreeRADIUS. Dipakai revoke sertifikat & suspend user.
type CoAKicker struct {
	sshTarget string // user@host FreeRADIUS tempat radclient berjalan
	secret    string // shared secret NAS (clients.conf)
}

func NewCoAKicker(sshTarget, secret string) *CoAKicker {
	return &CoAKicker{sshTarget: sshTarget, secret: secret}
}

// stripCIDR membuang akhiran /NN dari representasi text tipe inet PostgreSQL.
func stripCIDR(s string) string {
	if i := strings.Index(s, "/"); i >= 0 {
		return s[:i]
	}
	return s
}

// Disconnect mengirim Disconnect-Request utk satu sesi.
// Return: nil jika Disconnect-ACK; error menjelaskan kegagalan sebaliknya.
func (k *CoAKicker) Disconnect(username, sessionID, nasIP, callingStationID string) error {
	if k == nil || k.sshTarget == "" {
		return fmt.Errorf("CoA: host FreeRADIUS tidak dikonfigurasi")
	}
	if username == "" || sessionID == "" || nasIP == "" {
		return fmt.Errorf("CoA: parameter tidak lengkap")
	}
	if k.secret == "" {
		return fmt.Errorf("CoA: secret NAS tidak dikonfigurasi")
	}
	nasIP = stripCIDR(nasIP)

	attrs := fmt.Sprintf("User-Name = %s\nAcct-Session-Id = %s\nNAS-IP-Address = %s\n", username, sessionID, nasIP)
	if callingStationID != "" {
		attrs += fmt.Sprintf("Calling-Station-Id = \"%s\"\n", callingStationID)
	}

	cmd := exec.Command("ssh", "-o", "BatchMode=yes", "-o", "ConnectTimeout=10",
		k.sshTarget,
		fmt.Sprintf("sudo -n sh -c 'printf \"%%s\" | timeout 8 radclient -r 1 -t 3 -x %s:3799 disconnect %s 2>&1'", attrs, k.secret))

	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("CoA exec: %v, stderr: %s", err, stderr.String())
	}
	output := stdout.String()
	switch {
	case strings.Contains(output, "Disconnect-ACK"):
		return nil
	case strings.Contains(output, "Disconnect-NAK"):
		return fmt.Errorf("NAS menolak disconnect (NAK): sesi mungkin sudah berakhir")
	case strings.Contains(output, "No reply"):
		return fmt.Errorf("NAS tidak merespons CoA (port 3799)")
	}
	return fmt.Errorf("CoA hasil tidak dikenal: %s", output)
}
