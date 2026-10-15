package middleware

import (
	"net/http"

	"github.com/gin-gonic/gin"

	"github.com/rama/b-wise/library/internal/domain/repository"
	"github.com/rama/b-wise/library/internal/pkg/credhash"
)

// KioskAuth — autentikasi perangkat kiosk via header X-Kiosk-Code + X-Kiosk-Secret.
// Secret disimpan sebagai sha256; pesan error sengaja generik (tidak membocorkan keberadaan kiosk).
type KioskAuth struct {
	kiosks repository.KioskRepository
}

func NewKioskAuth(k repository.KioskRepository) *KioskAuth {
	return &KioskAuth{kiosks: k}
}

func (a *KioskAuth) Require() gin.HandlerFunc {
	return func(c *gin.Context) {
		code := c.GetHeader("X-Kiosk-Code")
		secret := c.GetHeader("X-Kiosk-Secret")
		if code == "" || secret == "" {
			c.AbortWithStatusJSON(http.StatusUnauthorized, gin.H{"success": false, "error": "kiosk tidak valid"})
			return
		}
		k, err := a.kiosks.GetByCode(code)
		if err != nil || k == nil {
			c.AbortWithStatusJSON(http.StatusUnauthorized, gin.H{"success": false, "error": "kiosk tidak valid"})
			return
		}
		if k.Status != "active" {
			c.AbortWithStatusJSON(http.StatusForbidden, gin.H{"success": false, "error": "kiosk dinonaktifkan"})
			return
		}
		if credhash.HashSecret(secret) != k.CredentialHash {
			c.AbortWithStatusJSON(http.StatusUnauthorized, gin.H{"success": false, "error": "kiosk tidak valid"})
			return
		}
		c.Set("kiosk_id", k.ID)
		c.Set("kiosk_code", k.Code)
		c.Set("location_id", k.LocationID)
		c.Next()
	}
}
