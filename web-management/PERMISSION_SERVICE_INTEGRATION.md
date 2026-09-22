# Starter Kit - Permission Service Integration

This guide explains how to integrate the Starter Kit with the Permission Service for centralized access control.

## Overview

The Starter Kit now supports JWT-based authentication with role-based authorization that's compatible with the Permission Service.

## Configuration

### 1. JWT Secret

**IMPORTANT:** The JWT secret in Starter Kit MUST match the JWT secret in Permission Service.

**Starter Kit** (`configs/config.yaml`):
```yaml
jwt:
  secret: "your-super-secret-jwt-key-change-this-in-production"
```

**Permission Service** (`configs/config.yaml`):
```yaml
jwt:
  secret: "your-super-secret-jwt-key-change-this-in-production"
```

### 2. CORS Configuration

Allow Permission Service to make requests to your microservice:

```yaml
cors:
  allowed_origins:
    - "http://localhost:3000"
    - "http://localhost:8081"  # Permission Service
    - "https://myapp.com"
    - "https://admin.myapp.com"
  allow_credentials: true
```

### 3. Environment Variables

```bash
# JWT Secret (MUST match Permission Service)
JWT_SECRET=your-super-secret-jwt-key-change-this-in-production

# CORS (allow Permission Service)
CORS_ALLOWED_ORIGINS=http://localhost:3000,http://localhost:8081,https://myapp.com
```

## JWT Token Structure

The Starter Kit now generates JWT tokens with the following payload:

```json
{
  "user_id": "user-123",
  "email": "user@example.com",
  "roles": ["user", "moderator"],
  "exp": 1234567890,
  "iat": 1234560000,
  "iss": "starter-kit"
}
```

This matches the format expected by Permission Service.

## Using JWT Helper

### Generate Token

```go
import (
    "github.com/rama/kit/pkg/jwt"
    "github.com/rama/kit/internal/adapter/config"
)

func GenerateUserToken(userID, email string, roles []string, cfg config.JWTConfig) (string, error) {
    token, err := jwt.GenerateToken(userID, email, roles, cfg)
    if err != nil {
        return "", err
    }
    return token, nil
}
```

### Validate Token

```go
import (
    "github.com/rama/kit/pkg/jwt"
)

func ValidateUserToken(tokenString, secret string) (*jwt.Claims, error) {
    claims, err := jwt.ValidateToken(tokenString, secret)
    if err != nil {
        return nil, err
    }
    return claims, nil
}
```

### Extract User Info

```go
// Get User ID
userID, err := jwt.ExtractUserID(tokenString, secret)

// Get Roles
roles, err := jwt.ExtractRoles(tokenString, secret)
```

## Authentication Middleware

### Basic Authentication

```go
import (
    "github.com/rama/kit/internal/adapter/api/http/middleware"
)

auth := middleware.NewAuth(cfg.JWT.Secret)

// Apply to all protected routes
r.Use(auth.Authenticate())
```

### Role-Based Authorization

```go
// Require specific role
r.GET("/admin", auth.RequireRole("administrator"), adminHandler)

// Require any of multiple roles
r.GET("/moderator", auth.RequireAnyRole("moderator", "administrator"), moderatorHandler)
```

### Get User Info in Handler

```go
func MyHandler(c *gin.Context) {
    // Get user ID
    userID, exists := middleware.GetUserID(c)
    if !exists {
        c.JSON(401, gin.H{"error": "unauthorized"})
        return
    }

    // Get roles
    roles, exists := middleware.GetRoles(c)
    if !exists {
        c.JSON(401, gin.H{"error": "unauthorized"})
        return
    }

    c.JSON(200, gin.H{
        "user_id": userID,
        "roles":   roles,
    })
}
```

## Integration with Permission Service

### 1. Dashboard Example

```typescript
// Get user's accessible services
const getUserServices = async (userId: string, token: string) => {
  const response = await fetch(
    `http://localhost:8081/api/v1/users/${userId}/services`,
    {
      headers: {
        'Authorization': `Bearer ${token}`
      }
    }
  );
  const data = await response.json();
  return data.data.services; // ["hr", "iku", "finance"]
};

// Get user's permissions for a service
const getServicePermissions = async (userId: string, serviceId: string, token: string) => {
  const response = await fetch(
    `http://localhost:8081/api/v1/users/${userId}/permissions/service/${serviceId}`,
    {
      headers: {
        'Authorization': `Bearer ${token}`
      }
    }
  );
  const data = await response.json();
  return data.data.permissions; // ["hr.employees.read", "hr.employees.create"]
};

// Check if user has specific permission
const checkPermission = async (userId: string, permissionId: string, token: string) => {
  const response = await fetch(
    `http://localhost:8081/api/v1/users/${userId}/permissions/${permissionId}/check`,
    {
      headers: {
        'Authorization': `Bearer ${token}`
      }
    }
  );
  const data = await response.json();
  return data.data.has_permission; // true/false
};
```

### 2. Microservice Authorization Example

```go
func (h *EmployeeHandler) CreateEmployee(c *gin.Context) {
    // Get user ID from JWT
    userID, exists := middleware.GetUserID(c)
    if !exists {
        c.JSON(401, gin.H{"error": "unauthorized"})
        return
    }

    // Get permissions from Permission Service
    perms, err := h.permService.GetUserPermissionsByService(userID, "hr")
    if err != nil {
        c.JSON(500, gin.H{"error": "failed to check permissions"})
        return
    }

    // Check if user has create permission
    if !hasPermission(perms, "hr.employees.create") {
        c.JSON(403, gin.H{"error": "no permission"})
        return
    }

    // ... create employee logic
}

func hasPermission(permissions []string, required string) bool {
    for _, perm := range permissions {
        if perm == required {
            return true
        }
    }
    return false
}
```

## Complete Example: Login Handler with JWT

```go
package handler

import (
    "github.com/gin-gonic/gin"
    "github.com/rama/kit/internal/adapter/config"
    "github.com/rama/kit/pkg/jwt"
    "github.com/rama/kit/pkg/response"
)

type AuthHandler struct {
    cfg     config.Config
    userService UserService
}

func NewAuthHandler(cfg config.Config, userService UserService) *AuthHandler {
    return &AuthHandler{
        cfg:          cfg,
        userService:  userService,
    }
}

func (h *AuthHandler) Login(c *gin.Context) {
    var req LoginRequest
    if err := c.ShouldBindJSON(&req); err != nil {
        response.ValidationError(c, err.Error())
        return
    }

    // Validate credentials
    user, err := h.userService.ValidateCredentials(req.Email, req.Password)
    if err != nil {
        response.Unauthorized(c, "invalid credentials")
        return
    }

    // Get user roles (from database)
    roles, err := h.userService.GetUserRoles(user.ID)
    if err != nil {
        response.InternalError(c, "failed to get user roles")
        return
    }

    // Generate JWT token
    token, err := jwt.GenerateToken(user.ID, user.Email, roles, h.cfg.JWT)
    if err != nil {
        response.InternalError(c, "failed to generate token")
        return
    }

    response.Success(c, 200, "Login successful", gin.H{
        "token": token,
        "user": gin.H{
            "id":    user.ID,
            "email": user.Email,
            "roles": roles,
        },
    })
}
```

## Testing Integration

### 1. Generate Test Token

```bash
# Use the JWT example in docs/jwt_example.go
cd /Users/macbook/project/go/starter/kit
go run docs/jwt_example.go
```

### 2. Test Protected Endpoint

```bash
# Login to get token
TOKEN=$(curl -X POST http://localhost:8081/api/v1/auth/login \
  -H "Content-Type: application/json" \
  -d '{"email":"user@example.com","password":"password"}' \
  | jq -r '.data.token')

# Use token to access protected endpoint
curl -X GET http://localhost:8081/api/v1/users/me \
  -H "Authorization: Bearer $TOKEN"
```

### 3. Test Permission Service Integration

```bash
# Get user's accessible services
curl -X GET http://localhost:8081/api/v1/users/user-123/services \
  -H "Authorization: Bearer $TOKEN"

# Get user's permissions for HR service
curl -X GET http://localhost:8081/api/v1/users/user-123/permissions/service/hr \
  -H "Authorization: Bearer $TOKEN"
```

## Best Practices

1. **Never hardcode JWT secrets** - Always use environment variables
2. **Keep JWT secrets consistent** - Same secret across all services
3. **Validate tokens on every protected request** - Use the auth middleware
4. **Check permissions before operations** - Query Permission Service when needed
5. **Cache permission results** - Use Redis to reduce Permission Service calls
6. **Use role-based access** - Implement `RequireRole()` for sensitive operations
7. **Log authentication failures** - Track failed login attempts
8. **Rotate secrets periodically** - Change JWT secrets in production
9. **Use HTTPS in production** - Never send tokens over HTTP
10. **Set appropriate token expiration** - Balance security and UX

## Troubleshooting

### Token Validation Fails

- Check JWT secret matches between services
- Verify token hasn't expired
- Ensure token format is correct (Bearer <token>)
- Check token hasn't been tampered with

### Permission Service Returns 401

- Verify JWT is valid and not expired
- Check JWT contains user_id and roles
- Ensure Permission Service JWT secret matches
- Verify user_id exists in Permission Service database

### CORS Errors

- Add Permission Service origin to CORS allowed_origins
- Ensure allow_credentials is true
- Check CORS max_age is sufficient

### Role Check Fails

- Verify roles array in JWT token
- Check role names match (case-sensitive)
- Ensure RequireRole() is applied after Authenticate()

## Next Steps

1. Implement user roles in your database
2. Update login handler to generate JWT with roles
3. Add auth middleware to protected routes
4. Integrate with Permission Service for authorization checks
5. Implement permission caching for performance
6. Add audit logging for sensitive operations

## References

- [Permission Service API Documentation](../permission-service/API_ENDPOINTS.md)
- [JWT Example Code](jwt_example.go)
- [Starter Kit README](../README.md)

---

## Update 2026-08-18: Middleware v2 (selaras Permission Service Tahap 1-3)

### Perubahan API
```go
// DULU (hardcode fallback "human-capital"):
permCheck := middleware.NewPermissionCheck(permissionURL).WithSSO(...)

// SEKARANG (serviceName wajib dari config SERVICE_NAME):
permCheck := middleware.NewPermissionCheck(permissionURL, serviceName).WithSSO(...)
```

### Fitur baru
| Middleware | Fungsi |
|---|---|
| `RequirePermission(p)` | Single check — server mendukung wildcard `*` (grant `*` = semua) |
| `RequireAnyPermission(a, b, ...)` | OR — batch check 1x call |
| `RequireAllPermissions(a, b, ...)` | AND — batch check 1x call |
| `CheckAccess()` | Akses service (resolve service_id di-cache 5 menit) |
| `CheckPermissions(userID, perms)` | API handler-level → `map[string]bool` |

### Fix penting (bug laten template lama)
1. **Service token endpoint**: `/oauth2/token` ❌ → **`/api/oauth/token`** ✅ (route SSO service-registry)
2. **Response parse**: SSO membungkus token di `data.access_token` (fallback top-level tetap ada)
3. **Token expiry**: sekarang track `expires_in` + refresh 30 detik sebelum expiry + retry saat 401
4. **serviceID cache**: dulu list semua service di SETIAP check → sekarang cache 5 menit
5. **No hardcode**: `serviceName` dari config (`SERVICE_NAME`); error jelas bila kosong/belum register

### Catatan SSO client registration
Service consumer harus terdaftar di SSO service-registry:
```
POST /api/admin/service-registry  {"service_name":"human-capital","allowed_scopes":["human-capital:*"],"created_by":"..."}
```
→ simpan `client_id`/`client_secret` ke `.env` service (`SERVICE_CLIENT_ID`/`SERVICE_CLIENT_SECRET`).

---

## Update 2026-08-18 (2): Self-Register Manifest

Service baru tidak perlu curl manual ke Permission Service. Deklarasi tunggal:

1. **`perm.manifest.yaml`** di root service (di-generate `bwise create-service`, diedit seiring fitur bertambah, ter-review bareng code)
2. Saat boot, `internal/service/permissionsync` (sudah ter-wire di `cmd/server/main.go`):
   - Baca manifest → service token dari SSO → `POST {PERMISSION_SERVICE_URL}/api/services/self/sync`
   - Idempotent upsert; item yang dihapus dari manifest → **deactivate** (bukan delete)
   - Best-effort: permission-service down → warning + retry backoff, service tetap jalan
3. Constraint: `SERVICE_NAME` harus == `service.name` di manifest (validasi dua sisi)

Opt-out per-service: `service.sync_enabled: false` di manifest.
