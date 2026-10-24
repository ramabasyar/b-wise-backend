package middleware

import (
	"net/http"

	"github.com/gin-gonic/gin"
)

// RequireServiceToken — hanya service token (machine-to-machine) yang boleh
// melanjutkan. Dipakai endpoint internal antar-service (audit M5:
// /api/org/resolve-approver tidak lagi bisa dipanggil user biasa).
func RequireServiceToken() gin.HandlerFunc {
	return func(c *gin.Context) {
		if !IsServiceToken(c) {
			c.AbortWithStatusJSON(http.StatusForbidden, gin.H{
				"success": false,
				"error":   gin.H{"message": "endpoint khusus service terpercaya"},
			})
			return
		}
		c.Next()
	}
}
