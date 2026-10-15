package credhash

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"strings"
)

// Normalize merapikan credential kartu: buang spasi, uppercase.
func Normalize(raw string) string {
	return strings.ToUpper(strings.Join(strings.Fields(strings.TrimSpace(raw)), ""))
}

// Lookup menghasilkan HMAC-SHA256 deterministik (hex) untuk disimpan/dicari.
// Raw credential tidak pernah disimpan dalam bentuk asli.
func Lookup(key, normalized string) string {
	mac := hmac.New(sha256.New, []byte(key))
	mac.Write([]byte(normalized))
	return hex.EncodeToString(mac.Sum(nil))
}

// Last4 untuk tampilan masked.
func Last4(normalized string) string {
	if len(normalized) <= 4 {
		return normalized
	}
	return normalized[len(normalized)-4:]
}

// HashSecret meng-hash secret perangkat kiosk (sha256 hex).
func HashSecret(secret string) string {
	sum := sha256.Sum256([]byte(secret))
	return hex.EncodeToString(sum[:])
}
