# API Endpoints Documentation

Go Starter Kit API - User Management Service

**Base URL**: `http://localhost:8080`

---

## Table of Contents

- [Public Routes](#public-routes)
- [Protected Routes](#protected-routes)
  - [User Management](#user-management)
- [Authentication](#authentication)
- [Response Format](#response-format)
- [Error Responses](#error-responses)

---

## Public Routes

### Health Check

#### GET /health

Health check endpoint untuk monitoring service status.

**Request:**
```bash
curl -X GET http://localhost:8080/health
```

**Response (200 OK):**
```json
{
  "status": "ok",
  "timestamp": "2026-04-28T07:55:00Z"
}
```

**Headers:**
- `X-Request-ID`: Unique request identifier
- `Content-Type`: application/json

---

## Protected Routes

Semua protected routes memerlukan authentication header:

```
Authorization: Bearer <JWT_TOKEN>
```

### User Management

Base path: `/api/users`

#### POST /api/users

Create new user (employee).

**Authentication:** Required (Bearer token)

**Request Body:**
```json
{
  "employee_id": "EMP-001",
  "full_name": "Budi Santoso",
  "email": "budi@example.com",
  "phone": "081234567890",
  "gender": "Laki-laki",
  "religion": "Islam",
  "marital_status": "Menikah",
  "npwp": "12.345.678.9-012.345",
  "address": "Jl. Sudirman No. 1, Jakarta",
  "birth_date": "1990-01-01T00:00:00Z"
}
```

**Validation:**
- `full_name`: Required, string
- `email`: Required, valid email format, must be unique
- `phone`: Optional, string
- `employee_id`: Optional, string
- `gender`: Optional, must be "Laki-laki" or "Perempuan"
- `religion`: Optional, string
- `marital_status`: Optional, string
- `npwp`: Optional, string
- `address`: Optional, string
- `birth_date`: Optional, ISO 8601 datetime

**Response (201 Created):**
```json
{
  "success": true,
  "data": {
    "id": "550e8400-e29b-41d4-a716-446655440000",
    "employee_id": "EMP-001",
    "full_name": "Budi Santoso",
    "email": "budi@example.com",
    "phone": "081234567890",
    "gender": "Laki-laki",
    "religion": "Islam",
    "marital_status": "Menikah",
    "npwp": "12.345.678.9-012.345",
    "address": "Jl. Sudirman No. 1, Jakarta",
    "birth_date": "1990-01-01T00:00:00Z",
    "status": "active",
    "created_by": "user-123",
    "updated_by": "user-123",
    "created_at": "2026-04-28T07:55:00Z",
    "updated_at": "2026-04-28T07:55:00Z"
  }
}
```

**Error Responses:**
- `400 Bad Request`: Invalid input
- `401 Unauthorized`: Missing or invalid token
- `409 Conflict`: Email already exists

**Example:**
```bash
curl -X POST http://localhost:8080/api/users \
  -H "Authorization: Bearer YOUR_JWT_TOKEN" \
  -H "Content-Type: application/json" \
  -d '{
    "employee_id": "EMP-001",
    "full_name": "Budi Santoso",
    "email": "budi@example.com",
    "phone": "081234567890",
    "gender": "Laki-laki"
  }'
```

---

#### GET /api/users

List users with filters and pagination.

**Authentication:** Required (Bearer token)

**Query Parameters:**}]}
- `search` (optional): Search in full_name, email, or employee_id
- `status` (optional): Filter by status ("active" or "inactive")
- `page` (optional): Page number (default: 1)
- `limit` (optional): Items per page (default: 10, max: 100)

**Request:**
```bash
curl -X GET "http://localhost:8080/api/users?search=Budi&status=active&page=1&limit=10" \
  -H "Authorization: Bearer YOUR_JWT_TOKEN"
```

**Response (200 OK):**
```json
{
  "success": true,
  "data": {
    "users": [
      {
        "id": "550e8400-e29b-41d4-a716-446655440000",
        "employee_id": "EMP-001",
        "full_name": "Budi Santoso",
        "email": "budi@example.com",
        "phone": "081234567890",
        "gender": "Laki-laki",
        "religion": "Islam",
        "marital_status": "Menikah",
        "npwp": "12.345.678.9-012.345",
        "address": "Jl. Sudirman No. 1, Jakarta",
        "birth_date": "1990-01-01T00:00:00Z",
        "status": "active",
        "created_by": "user-123",
        "updated_by": "user-123",
        "created_at": "2026-04-28T07:55:00Z",
        "updated_at": "2026-04-28T07:55:00Z"
      }
    ],
    "total": 1,
    "page": 1,
    "limit": 10
  }
}
```

---

#### GET /api/users/:id

Get user by ID.

**Authentication:** Required (Bearer token)

**Request:**
```bash
curl -X GET http://localhost:8080/api/users/550e8400-e29b-41d4-a716-446655440000 \
  -H "Authorization: Bearer YOUR_JWT_TOKEN"
```

**Response (200 OK):**
```json
{
  "success": true,
  "data": {
    "id": "550e8400-e29b-41d4-a716-446655440000",
    "employee_id": "EMP-001",
    "full_name": "Budi Santoso",
    "email": "budi@example.com",
    "phone": "081234567890",
    "gender": "Laki-laki",
    "religion": "Islam",
    "marital_status": "Menikah",
    "npwp": "12.345.678.9-012.345",
    "address": "Jl. Sudirman No. 1, Jakarta",
    "birth_date": "1990-01-01T00:00:00Z",
    "status": "active",
    "created_by": "user-123",
    "updated_by": "user-123",
    "created_at": "2026-04-28T07:55:00Z",
    "updated_at": "2026-04-28T07:55:00Z"
  }
}
```

**Error Responses:**
- `404 Not Found`: User not found

---

#### PUT /api/users/:id

Update user (partial update).

**Authentication:** Required (Bearer token)

**Request Body (all fields optional):**
```json
{
  "full_name": "Budi Santoso Updated",
  "email": "budi.updated@example.com",
  "phone": "081987654321",
  "employee_id": "EMP-002",
  "gender": "Perempuan",
  "religion": "Kristen",
  "marital_status": "Belum Menikah",
  "npwp": "98.765.432.1-000.000",
  "address": "Jl. Thamrin No. 2, Jakarta",
  "birth_date": "1992-05-15T00:00:00Z",
  "status": "inactive"
}
```

**Validation:**
- `email`: If provided, must be valid format and unique
- `gender`: If provided, must be "Laki-laki" or "Perempuan"
- `status`: If provided, must be "active" or "inactive"

**Request:**
```bash
curl -X PUT http://localhost:8080/api/users/550e8400-e29b-41d4-a716-446655440000 \
  -H "Authorization: Bearer YOUR_JWT_TOKEN" \
  -H "Content-Type: application/json" \
  -d '{
    "full_name": "Budi Santoso Updated",
    "phone": "081987654321"
  }'
```

**Response (200 OK):**
```json
{
  "success": true,
  "data": {
    "id": "550e8400-e29b-41d4-a716-446655440000",
    "employee_id": "EMP-001",
    "full_name": "Budi Santoso Updated",
    "email": "budi@example.com",
    "phone": "081987654321",
    "gender": "Laki-laki",
    "religion": "Islam",
    "marital_status": "Menikah",
    "npwp": "12.345.678.9-012.345",
    "address": "Jl. Sudirman No. 1, Jakarta",
    "birth_date": "1990-01-01T00:00:00Z",
    "status": "active",
    "created_by": "user-123",
    "updated_by": "user-456",
    "created_at": "2026-04-28T07:55:00Z",
    "updated_at": "2026-04-28T08:00:00Z"
  }
}
```

**Error Responses:**
- `400 Bad Request`: Invalid input or validation error
- `404 Not Found`: User not found
- `409 Conflict`: Email already exists (if email changed)

---

#### DELETE /api/users/:id

Delete user (soft delete).

**Authentication:** Required (Bearer token)

**Request:**
```bash
curl -X DELETE http://localhost:8080/api/users/550e8400-e29b-41d4-a716-446655440000 \
  -H "Authorization: Bearer YOUR_JWT_TOKEN"
```

**Response (200 OK):**
```json
{
  "success": true,
  "data": {
    "message": "user deleted"
  }
}
```

**Error Responses:**
- `404 Not Found`: User not found

---

## Authentication

### JWT Token Format

All protected routes require a JWT token in the Authorization header:

```
Authorization: Bearer eyJhbGciOiJIUzI1NiIsInR5cCI6IkpXVCJ9...
```

The JWT token must be obtained from the SSO service and include:
- `user_id`: User ID in claims
- `permissions`: User permissions (optional, embedded in JWT)

### Token Validation

The service validates JWT tokens using a shared secret (configured via `SSO_JWT_SECRET` environment variable).

### Authentication Flow

1. Frontend authenticates with SSO service
2. SSO returns JWT token
3. Frontend includes token in Authorization header
4. This service validates token locally (no network call to SSO)
5. If valid, request proceeds to handler

### Fallback to SSO API

If permissions are not embedded in JWT, the service will:
1. Check Redis cache first (15 min TTL)
2. If not in cache, call SSO API: `GET /api/v1/roles/users/:user_id/permissions`
3. Cache the result for future requests
4. Return permissions to handler

---

## Response Format

### Success Response

```json
{
  "success": true,
  "data": { ... }
}
```

### Error Response

```json
{
  "success": false,
  "error": {
    "message": "Error description",
    "code": 400
  }
}
```

---

## Error Responses

### Common HTTP Status Codes

| Status | Description | Example |
|--------|-------------|---------|
| 200 | OK | Success |
| 201 | Created | Resource created successfully |
| 400 | Bad Request | Invalid input, validation error |
| 401 | Unauthorized | Missing or invalid token |
| 404 | Not Found | Resource not found |
| 409 | Conflict | Duplicate resource (e.g., email exists) |
| 429 | Too Many Requests | Rate limit exceeded |
| 500 | Internal Server Error | Server error |

### Error Response Structure

```json
{
  "success": false,
  "error": {
    "message": "user with email test@example.com already exists",
    "code": 409
  }
}
```

---

## Response Headers

All responses include the following headers:

| Header | Description |
|--------|-------------|
| `Content-Type` | Response content type (usually `application/json`) |
| `X-Request-ID` | Unique request identifier for tracing |
| `X-RateLimit-Limit` | Rate limit for the endpoint |
| `X-RateLimit-Remaining` | Remaining requests in current window |
| `X-RateLimit-Window` | Time window in seconds |
| `Retry-After` | Seconds until retry (for 429 responses) |

### Security Headers

| Header | Value | Description |
|--------|-------|-------------|
| `X-Content-Type-Options` | nosniff | Prevent MIME type sniffing |
| `X-Frame-Options` | DENY | Prevent clickjacking |
| `X-XSS-Protection` | 1; mode=block | Enable XSS filtering |
| `Strict-Transport-Security` | max-age=31536000; includeSubDomains | Enforce HTTPS |
| `Content-Security-Policy` | default-src 'self' ... | Control resource loading |
| `Referrer-Policy` | strict-origin-when-cross-origin | Control referrer info |

---

## Rate Limiting

### Default Rate Limits

| Endpoint | Limit | Window | Description |
|----------|-------|--------|-------------|
| All requests | 100 req/min | 60s | Per user |
| General | 1000 req/min | 60s | Per IP |
| Login | 5 req/15min | 900s | Per IP |
| Register | 3 req/hour | 3600s | Per IP |

### Rate Limit Headers

```http
X-RateLimit-Limit: 100
X-RateLimit-Remaining: 95
X-RateLimit-Window: 60
```

### 429 Response

```json
{
  "success": false,
  "error": {
    "message": "rate limit exceeded",
    "code": 429,
    "retry_after": 300
  }
}
```

---

## Notes

### Middleware Chain

Requests pass through the following middleware (in order):

1. **Recovery** - Panic recovery
2. **Request ID** - Generate unique request ID
3. **Logging** - Log HTTP requests
4. **CORS** - Handle cross-origin requests
5. **Security Headers** - Add security headers
6. **Rate Limiting** - Enforce rate limits
7. **Authentication** - Validate JWT token

### User Status

- `active`: User is active and can access resources
- `inactive`: User is inactive (soft deleted)

### Gender Values

- `Laki-laki`: Male
- `Perempuan`: Female

### Status Values

- `active`: Active user
- `inactive`: Inactive user

### NPWP Format

Indonesian Tax ID format: `XX.XXX.XXX.X-XXX.XXX`

Example: `12.345.678.9-012.345`

### Date Format

All dates are in ISO 8601 format (UTC timezone):
- `birth_date`: `YYYY-MM-DDTHH:MM:SSZ`

### Pagination

- Default page: 1
- Default limit: 10
- Maximum limit: 100
- Response includes: `users`, `total`, `page`, `limit`

### Search

Search field searches in:
- `full_name` (case-insensitive)
- `email` (case-insensitive)
- `employee_id` (case-insensitive)

---

## Quick Test Sequence

```bash
# 1. Health check
curl http://localhost:8080/health

# 2. Create user (requires JWT token)
curl -X POST http://localhost:8080/api/users \
  -H "Authorization: Bearer YOUR_JWT_TOKEN" \
  -H "Content-Type: application/json" \
  -d '{
    "employee_id": "EMP-001",
    "full_name": "Budi Santoso",
    "email": "budi@example.com",
    "phone": "081234567890",
    "gender": "Laki-laki"
  }'

# 3. List users
curl -X GET http://localhost:8080/api/users \
  -H "Authorization: Bearer YOUR_JWT_TOKEN"

# 4. Get user by ID
curl -X GET http://localhost:8080/api/users/USER_ID \
  -H "Authorization: Bearer YOUR_JWT_TOKEN"

# 5. Update user
curl -X PUT http://localhost:8080/api/users/USER_ID \
  -H "Authorization: Bearer YOUR_JWT_TOKEN" \
  -H "Content-Type: application/json" \
  -d '{"phone": "081987654321"}'

# 6. Delete user
curl -X DELETE http://localhost:8080/api/users/USER_ID \
  -H "Authorization: Bearer YOUR_JWT_TOKEN"
```

---

**Documentation Version:** 1.0
**Last Updated:** 2026-04-28
