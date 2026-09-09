package middleware

import "github.com/gin-gonic/gin"

const requestIDKey = "request_id"

// GetRequestID returns the request ID from context
func GetRequestID(c *gin.Context) string {
	if val, exists := c.Get(requestIDKey); exists {
		if str, ok := val.(string); ok {
			return str
		}
	}
	return ""
}

// SetRequestID sets the request ID in context
func SetRequestID(c *gin.Context, id string) {
	c.Set(requestIDKey, id)
}
