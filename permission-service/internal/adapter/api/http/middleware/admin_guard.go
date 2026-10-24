package middleware

import (
	"net/http"
	"strings"

	"github.com/gin-gonic/gin"
)

// AdminGuard — otorisasi endpoint manajemen/tulis permission-service.
// Diizinkan HANYA: service token (machine-to-machine, typ=service) atau
// superadmin (SUPER_ADMIN_IDS). Menutup celah eskalasi privilege ketika
// user biasa (termasuk hasil register publik) memanggil endpoint tulis
// permission-service (audit HC+SSO 24 Okt 2026 — temuan C1).
type AdminGuard struct {
	superAdminIDs map[string]bool
}

// NewAdminGuard — superAdminIDs dari cfg.SSO.SuperAdminIDs (env SUPER_ADMIN_IDS, dipisah koma).
func NewAdminGuard(superAdminIDs []string) *AdminGuard {
	m := make(map[string]bool, len(superAdminIDs))
	for _, id := range superAdminIDs {
		if id = strings.TrimSpace(id); id != "" {
			m[id] = true
		}
	}
	return &AdminGuard{superAdminIDs: m}
}

// RequireServiceOrSuperAdmin — gin middleware; WAJIB dipasang setelah jwksAuth.RequireAuth()
// (butuh context user_id / is_service_token yang diisi middleware JWKS).
func (g *AdminGuard) RequireServiceOrSuperAdmin() gin.HandlerFunc {
	return func(c *gin.Context) {
		if IsServiceToken(c) {
			c.Next()
			return
		}
		if uid := GetUserID(c); uid != "" && g.superAdminIDs[uid] {
			c.Next()
			return
		}
		c.AbortWithStatusJSON(http.StatusForbidden, gin.H{
			"success": false,
			"error": gin.H{
				"message": "akses ditolak: endpoint ini hanya untuk service terpercaya atau superadmin",
			},
		})
	}
}
