# Permission Service - API Documentation

> **Port:** 8082 | **Database:** `permission_db` | **Auth:** JWT via JWKS (required for all /api endpoints)

---

## Authentication

Semua endpoint `/api/*` memerlukan header:

```
Authorization: Bearer <token>
```

Token bisa berupa:
- **User token** — dari login SSO
- **Service token** — dari OAuth2 client credentials

**Tanpa token →** `401 { "error": "authentication required" }`

---

## Response Format

### Success
```json
{
  "success": true,
  "data": { ... },
  "meta": { "page": 1, "page_size": 20, "total": 1 }
}
```

### Error
```json
{ "error": "error message" }
```

> **Note:** Internal fields (`created_by`, `updated_by`, `deleted_at`) are never exposed in responses.

---

## Health Check

### Get Health

```
GET /health
```

**Response:**
```json
{ "status": "ok", "message": "Service is running" }
```

> Public endpoint, tidak butuh token.

---

## Service Management

### Create Service

```
POST /api/services
```

**Request Body:**
```json
{
  "name": "human-capital",
  "description": "Human Capital Management",
  "base_url": "http://localhost:8081"
}
```

| Field | Type | Required | Description |
|-------|------|----------|-------------|
| name | string | ✅ | Unique service name |
| description | string | ❌ | Service description |
| base_url | string | ✅ | Service URL |

**Response (200):**
```json
{
  "success": true,
  "data": {
    "id": "1d4393e2-c94a-4ae2-9dff-ec43c108a331",
    "name": "human-capital",
    "description": "Human Capital Management",
    "base_url": "http://localhost:8081",
    "is_active": true,
    "created_at": "2026-05-20T14:59:00.700505+07:00",
    "updated_at": "2026-05-20T14:59:00.700505+07:00"
  }
}
```

---

### List Services

```
GET /api/services?page=1&page_size=20
```

| Query Param | Type | Default | Description |
|-------------|------|---------|-------------|
| page | int | 1 | Page number |
| page_size | int | 20 | Items per page |

**Response (200):**
```json
{
  "success": true,
  "data": [
    {
      "id": "1d4393e2-...",
      "name": "human-capital",
      "description": "Human Capital Management",
      "base_url": "http://localhost:8081",
      "is_active": true,
      "created_at": "2026-05-20T...",
      "updated_at": "2026-05-20T..."
    }
  ],
  "meta": { "page": 1, "page_size": 20, "total": 1 }
}
```

---

### Get Service

```
GET /api/services/:id
```

---

### Update Service

```
PUT /api/services/:id
```

**Request Body:**
```json
{
  "name": "human-capital-v2",
  "description": "Updated description",
  "base_url": "http://localhost:8081"
}
```

---

### Delete Service

```
DELETE /api/services/:id
```

---

### Activate Service

```
POST /api/services/:id/activate
```

**Response (200):**
```json
{ "success": true, "message": "service activated" }
```

---

### Deactivate Service

```
POST /api/services/:id/deactivate
```

---

## Permission Management

### Create Permission

```
POST /api/services/:id/permissions
```

**Request Body:**
```json
{
  "permission": "employees.read",
  "description": "Read employee data"
}
```

| Field | Type | Required | Description |
|-------|------|----------|-------------|
| permission | string | ✅ | Permission name (e.g. employees.read) |
| description | string | ❌ | What this permission allows |

**Response (200):**
```json
{
  "success": true,
  "data": {
    "id": "uuid",
    "service_id": "1d4393e2-...",
    "permission": "employees.read",
    "description": "Read employee data",
    "created_at": "2026-05-20T..."
  }
}
```

---

### Seed Permissions (Batch)

```
POST /api/services/:id/permissions/seed
```

**Request Body:**
```json
{
  "permissions": [
    "employees.read",
    "employees.write",
    "employees.delete",
    "departments.read",
    "departments.write",
    "departments.delete"
  ]
}
```

**Response (200):**
```json
{ "success": true, "message": "permissions seeded" }
```

---

### List Permissions

```
GET /api/services/:id/permissions
```

---

### Delete Permission

```
DELETE /api/permissions/:id
```

---

## Access Management

### Grant Service Access

```
POST /api/services/:id/access
```

**Request Body:**
```json
{
  "user_id": "6"
}
```

| Field | Type | Required | Description |
|-------|------|----------|-------------|
| user_id | string | ✅ | User ID from SSO |

**Response (200):**
```json
{
  "success": true,
  "data": {
    "id": "uuid",
    "user_id": "6",
    "service_id": "1d4393e2-...",
    "granted_at": "2026-05-20T15:43:22.679845+07:00",
    "is_active": true
  }
}
```

---

### Revoke Service Access

```
DELETE /api/access/:id
```

---

## User Permission Management

### Grant User Permission

```
POST /api/users/:user_id/permissions
```

**Request Body:**
```json
{
  "permission": "employees.read",
  "service_id": "1d4393e2-..."
}
```

| Field | Type | Required | Description |
|-------|------|----------|-------------|
| permission | string | ✅ | Permission name |
| service_id | string | ✅ | Service ID |

**Response (200):**
```json
{
  "success": true,
  "data": {
    "id": "uuid",
    "user_id": "6",
    "permission": "employees.read",
    "service_id": "1d4393e2-...",
    "granted_at": "2026-05-20T...",
    "is_active": true
  }
}
```

---

### Get User Permissions

```
GET /api/users/:user_id/permissions
```

---

### Get User Permissions by Service

```
GET /api/users/:user_id/permissions/service/:service_id
```

---

### Check User Permission

```
GET /api/users/:user_id/permissions/check?permission=employees.read&service_id=1d4393e2-...
```

| Query Param | Type | Required | Description |
|-------------|------|----------|-------------|
| permission | string | ✅ | Permission name |
| service_id | string | ✅ | Service ID |

**Response (200):**
```json
{
  "success": true,
  "data": {
    "has_permission": true,
    "permission": "employees.read",
    "service_id": "1d4393e2-..."
  }
}
```

---

### Revoke User Permission

```
DELETE /api/user-permissions/:id
```

---

## User Service Access

### Get User's Services

```
GET /api/users/:user_id/services
```

**Response (200):**
```json
{
  "success": true,
  "data": [
    {
      "service_id": "1d4393e2-...",
      "service_name": "human-capital",
      "is_active": true,
      "granted_at": "2026-05-20T..."
    }
  ]
}
```

---

### Check Service Access

```
GET /api/users/:user_id/services/:service_id/check
```

**Response (200):**
```json
{
  "success": true,
  "data": {
    "has_access": true,
    "service_id": "1d4393e2-...",
    "user_id": "6"
  }
}
```

---

## User Menu (for Frontend)

### Get User Menu

```
GET /api/users/:user_id/menu
```

**Response (200):**
```json
{
  "success": true,
  "data": [
    {
      "service_id": "1d4393e2-...",
      "service_name": "human-capital",
      "base_url": "http://localhost:8081",
      "permissions": [
        "employees.read",
        "employees.write",
        "departments.read",
        "departments.write",
        "positions.read",
        "positions.write"
      ]
    }
  ]
}
```

> Digunakan oleh frontend untuk render sidebar/menu berdasarkan service dan permission yang dimiliki user.

---

## Endpoint Summary

| Method | Endpoint | Description |
|--------|----------|-------------|
| GET | `/health` | Health check (public) |
| POST | `/api/services` | Create service |
| GET | `/api/services` | List services |
| GET | `/api/services/:id` | Get service |
| PUT | `/api/services/:id` | Update service |
| DELETE | `/api/services/:id` | Delete service |
| POST | `/api/services/:id/activate` | Activate service |
| POST | `/api/services/:id/deactivate` | Deactivate service |
| POST | `/api/services/:id/permissions` | Create permission |
| POST | `/api/services/:id/permissions/seed` | Seed permissions (batch) |
| GET | `/api/services/:id/permissions` | List permissions |
| DELETE | `/api/permissions/:id` | Delete permission |
| POST | `/api/services/:id/access` | Grant service access |
| DELETE | `/api/access/:id` | Revoke service access |
| POST | `/api/users/:user_id/permissions` | Grant user permission |
| GET | `/api/users/:user_id/permissions` | Get user permissions |
| GET | `/api/users/:user_id/permissions/service/:service_id` | Get perms by service |
| GET | `/api/users/:user_id/permissions/check` | Check user permission |
| DELETE | `/api/user-permissions/:id` | Revoke user permission |
| GET | `/api/users/:user_id/services` | Get user's services |
| GET | `/api/users/:user_id/services/:service_id/check` | Check service access |
| GET | `/api/users/:user_id/menu` | Get user menu (for frontend) |

---

*Last updated: 2026-05-20*
