package middleware

import (
	"net/http"

	"github.com/gin-gonic/gin"
	"github.com/rama/b-wise/web-management/pkg/response"
)

// Recovery middleware recovers from panics
func Recovery() gin.HandlerFunc {
	return func(c *gin.Context) {
		defer func() {
			if err := recover(); err != nil {
				// Check if response was already written
				if !c.Writer.Written() {
					c.JSON(http.StatusInternalServerError, response.Response{
						Success: false,
						Error: &response.ErrorInfo{
							Message: "internal server error",
							Code:    http.StatusInternalServerError,
						},
					})
				}
				c.Abort()
			}
		}()
		c.Next()
	}
}
