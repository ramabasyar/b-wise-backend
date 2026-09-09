# B-Wise API Documentation

> **Base URL**: `http://localhost:3002` (Frontend)  
> **Last Updated**: 2026-05-26  
> **Auth**: Bearer Token (JWT) via SSO

---

## Table of Contents

1. [Authentication](#1-authentication-sso-8080)
2. [Human Capital Service (8081)](#2-human-capital-service-8081)
3. [Permission Service (8082)](#3-permission-service-8082)
4. [Common Response Format](#4-common-response-format)
5. [Error Codes](#5-error-codes)
6. [Quick Start for Frontend](#6-quick-start-for-frontend)

---

## Common Response Format

### Success
```json
{
  "success": true,
  "data": { ... }       // or [ ... ] for lists
}
```

### Error
```json
{
  "success": false,
  "error": "error message"
}
```

### Paginated List
```json
{
  "success": true,
  "data": {
    "items": [ ... ],
    "total": 100,
    "limit": 20,
    "offset": 0
  }
}
```

---

## Authentication

All protected endpoints require a Bearer token in the `Authorization` header:
```
Authorization: Bearer <access_token>
```

Tokens are obtained via `POST /api/v1/auth/login` on the SSO service.

---

# 1. Authentication (SSO — Port 8080)

**Base URL**: `http://localhost:8080`

## Auth Endpoints

### POST /api/v1/auth/login
Login with email or username.

**Request:**
```json
{
  "login": "devops@binawan.ac.id",   // email OR username
  "password": "admin12345"
}
```

**Response (200):**
```json
{
  "success": true,
  "data": {
    "access_token": "eyJhbGciOiJIUzI1NiIs...",
    "refresh_token": "...",
    "user": {
      "id": "e4607511-2b13-4494-afdc-c17dbfceefef",
      "username": "admin",
      "email": "devops@binawan.ac.id",
      "first_name": "System",
      "last_name": "Administrator",
      "status": "active",
      "email_verified": true,
      "roles": [
        {
          "id": "...",
          "name": "administrator",
          "description": "Administrator role with full access"
        }
      ]
    }
  }
}
```

**Error (401):**
```json
{
  "success": false,
  "error": "invalid credentials"
}
```

---

### POST /api/v1/auth/register
Register a new user.

**Request:**
```json
{
  "username": "john.doe",
  "email": "john@binawan.ac.id",
  "password": "Password123!",
  "first_name": "John",
  "last_name": "Doe"
}
```

**Response (201):**
```json
{
  "success": true,
  "data": {
    "id": "...",
    "username": "john.doe",
    "email": "john@binawan.ac.id",
    "first_name": "John",
    "last_name": "Doe",
    "status": "pending_verification",
    "email_verified": false
  }
}
```

---

### POST /api/v1/auth/verify-email
Verify email with token received via email.

**Request:**
```json
{
  "token": "AFqv9K3JUvWLn4i4HIrZMAfsCf7v8pEZ"
}
```

**Response (200):**
```json
{
  "success": true,
  "message": "email verified successfully"
}
```

---

### POST /api/v1/auth/resend-verification-email
Resend verification email. **Rate limited**: 3 per hour.

**Request:**
```json
{
  "email": "john@binawan.ac.id"
}
```

**Response (200):**
```json
{
  "success": true,
  "message": "verification email sent"
}
```

---

### POST /api/v1/auth/logout
Logout (invalidates current session).

**Headers:** `Authorization: Bearer <token>`

**Response (200):**
```json
{
  "success": true,
  "message": "logged out successfully"
}
```

---

## User Endpoints

### GET /api/v1/users/me
Get current user profile.

**Headers:** `Authorization: Bearer <token>`

**Response (200):**
```json
{
  "success": true,
  "data": {
    "id": "e4607511-...",
    "username": "admin",
    "email": "devops@binawan.ac.id",
    "first_name": "System",
    "last_name": "Administrator",
    "avatar_url": "",
    "bio": "",
    "status": "active",
    "email_verified": true,
    "metadata": {},
    "last_login_at": null,
    "created_at": "2026-04-27T17:51:16.484332+07:00",
    "updated_at": "2026-04-27T17:51:16.484332+07:00"
  }
}
```

---

### GET /api/v1/users
List all users with pagination. **Requires admin role.**

**Headers:** `Authorization: Bearer <token>`

**Query Parameters:**
| Param | Type | Default | Description |
|-------|------|---------|-------------|
| `page` | int | 1 | Page number |
| `page_size` | int | 10 | Items per page (max 100) |

**Response (200):**
```json
{
  "success": true,
  "data": {
    "users": [
      {
        "id": "...",
        "username": "admin",
        "email": "devops@binawan.ac.id",
        "first_name": "System",
        "last_name": "Administrator",
        "status": "active",
        "email_verified": true,
        "created_at": "..."
      }
    ],
    "total": 5,
    "limit": 10,
    "offset": 0
  }
}
```

---

## Service User Registration

### POST /api/v1/service-users/register
Register user via service token (for onboarding flow). **Requires service token** (not user token).

**Headers:** `Authorization: Bearer <service_token>`

**Request:**
```json
{
  "email": "new.user@binawan.ac.id",
  "username": "new.user",
  "password": "Password123!",
  "first_name": "New",
  "last_name": "User",
  "base_role": "staff"
}
```

**Response (201):**
```json
{
  "success": true,
  "data": {
    "id": "...",
    "username": "new.user",
    "email": "new.user@binawan.ac.id"
  }
}
```

---

## Service Registry (Admin)

### GET /api/v1/admin/service-registry
List all registered services.

**Headers:** `Authorization: Bearer <token>`

**Response (200):**
```json
{
  "success": true,
  "data": [
    {
      "id": "bd2f143b-...",
      "name": "human-capital",
      "description": "Human Capital Management Service",
      "base_url": "http://localhost:8081",
      "client_id": "human-capita-e57117242848a51f",
      "client_secret_hash": "***",
      "scopes": ["human-capital:*"],
      "is_active": true,
      "created_at": "..."
    }
  ]
}
```

### POST /api/v1/admin/service-registry
Register a new service.

### PUT /api/v1/admin/service-registry/:id
Update service registry entry.

### DELETE /api/v1/admin/service-registry/:id
Delete service from registry.

---

## OAuth2 Client Credentials

### POST /oauth2/token
Get service-to-service access token.

**Request:**
```json
{
  "grant_type": "client_credentials",
  "client_id": "human-capita-e57117242848a51f",
  "client_secret": "b50ee71c..."
}
```

**Response (200):**
```json
{
  "access_token": "eyJhbGciOiJSUzI1NiIs...",
  "token_type": "Bearer",
  "expires_in": 3600
}
```

---

## JWKS

### GET /.well-known/jwks.json
Public key for JWT verification. Used by services to validate tokens.

**Response (200):**
```json
{
  "keys": [
    {
      "kty": "RSA",
      "kid": "...",
      "n": "...",
      "e": "AQAB"
    }
  ]
}
```

---

# 2. Human Capital Service (Port 8081)

**Base URL**: `http://localhost:8081`  
**All endpoints require:** `Authorization: Bearer <token>`  
**Route prefix:** `/api/`

## Health

### GET /health
**Public** — no auth required.

**Response (200):**
```json
{
  "status": "ok",
  "message": "Service is running"
}
```

---

## Departments

### GET /api/departments
List all departments.

**Response (200):**
```json
{
  "data": [
    {
      "id": "2bf2be6d-...",
      "code": "IT",
      "name": "Information Technology",
      "description": "IT Department",
      "is_active": true,
      "created_at": "2026-05-21T18:29:24Z+07:00",
      "updated_at": "2026-05-21T18:29:24Z+07:00"
    }
  ]
}
```

---

### GET /api/departments/:id
Get department by ID.

**Response (200):**
```json
{
  "data": {
    "id": "2bf2be6d-...",
    "code": "IT",
    "name": "Information Technology",
    "description": "IT Department",
    "is_active": true,
    "created_at": "...",
    "updated_at": "..."
  }
}
```

**Error (404):**
```json
{
  "success": false,
  "error": "resource not found"
}
```

---

### POST /api/departments
Create a new department. **Requires permission:** `departments.write`

**Request:**
```json
{
  "code": "IT",
  "name": "Information Technology",
  "description": "IT Department"
}
```

| Field | Type | Required | Description |
|-------|------|----------|-------------|
| `code` | string | ✅ | Short code (e.g., "IT", "HR") |
| `name` | string | ✅ | Full name |
| `description` | string | ❌ | Description |

**Response (201):**
```json
{
  "data": {
    "id": "...",
    "code": "IT",
    "name": "Information Technology",
    "description": "IT Department",
    "is_active": true,
    "created_at": "...",
    "updated_at": "..."
  }
}
```

---

### PUT /api/departments/:id
Update a department. **Requires permission:** `departments.write`

**Request (all fields optional — only send what you want to change):**
```json
{
  "code": "ITD",
  "name": "Information Technology Division",
  "description": "Updated description",
  "is_active": true
}
```

**Response (200):** Same as GET single department.

---

### DELETE /api/departments/:id
Delete a department. **Requires permission:** `departments.delete`

**Response (200):**
```json
{
  "data": null,
  "message": "Department deleted successfully"
}
```

---

## Positions

### GET /api/positions
List all positions.

**Response (200):**
```json
{
  "data": [
    {
      "id": "515382a8-...",
      "code": "STAFF",
      "name": "Staff",
      "description": "General Staff",
      "level": 2,
      "is_active": true,
      "created_at": "...",
      "updated_at": "..."
    }
  ]
}
```

---

### GET /api/positions/:id
Get position by ID.

---

### POST /api/positions
Create a new position. **Requires permission:** `positions.write`

**Request:**
```json
{
  "code": "MGR",
  "name": "Manager",
  "description": "Department Manager",
  "level": 5
}
```

| Field | Type | Required | Description |
|-------|------|----------|-------------|
| `code` | string | ✅ | Short code |
| `name` | string | ✅ | Full name |
| `description` | string | ❌ | Description |
| `level` | int | ❌ | Hierarchy level (1-10) |

---

### PUT /api/positions/:id
Update a position. **Requires permission:** `positions.write`

**Request (all fields optional):**
```json
{
  "name": "Senior Manager",
  "level": 6
}
```

---

### DELETE /api/positions/:id
Delete a position. **Requires permission:** `positions.delete`

---

## Employees

### GET /api/employees
List all employees with pagination.

**Query Parameters:**
| Param | Type | Default | Description |
|-------|------|---------|-------------|
| `page` | int | 1 | Page number |
| `page_size` | int | 20 | Items per page |
| `search` | string | "" | Search by name, email, or employee_id |

**Response (200):**
```json
{
  "data": {
    "items": [
      {
        "id": "5754874e-...",
        "employee_id": "86753865187",
        "full_name": "Manda Raihana",
        "email": "manda.raihanalaksita@binawan.ac.id",
        "phone": "082299655627",
        "gender": "female",
        "address": "",
        "department_id": "efd706ab-...",
        "position_id": "def0ef3d-...",
        "status": "active",
        "joined_date": "2002-01-01T07:00:00+07:00",
        "created_at": "...",
        "updated_at": "..."
      }
    ],
    "total": 5,
    "limit": 20,
    "offset": 0
  }
}
```

---

### GET /api/employees/:id
Get employee by ID.

---

### POST /api/employees
Create a new employee. **Requires permission:** `employees.write`

**Request:**
```json
{
  "employee_id": "199001012015012002",
  "full_name": "Budi Santoso",
  "email": "budi.santoso@binawan.ac.id",
  "phone": "081234567890",
  "gender": "male",
  "address": "Jakarta",
  "department_id": "uuid-of-department",
  "position_id": "uuid-of-position",
  "joined_date": "2024-01-01T00:00:00Z"
}
```

| Field | Type | Required | Description |
|-------|------|----------|-------------|
| `employee_id` | string | ✅ | NIP / Employee number |
| `full_name` | string | ✅ | Full name |
| `email` | string | ✅ | Email address |
| `phone` | string | ❌ | Phone number |
| `gender` | string | ❌ | "male" / "female" |
| `address` | string | ❌ | Address |
| `department_id` | string | ✅ | UUID of department |
| `position_id` | string | ✅ | UUID of position |
| `joined_date` | string | ❌ | ISO 8601 date |

---

### PUT /api/employees/:id
Update an employee. **Requires permission:** `employees.write`

**Request (all fields optional):** Same as POST.

---

### DELETE /api/employees/:id
Delete an employee. **Requires permission:** `employees.delete`

---

## Onboarding

### POST /api/onboard
Onboard a new employee (creates SSO account + employee record + grants permissions).

**Request:**
```json
{
  "step1": {
    "email": "new.user@binawan.ac.id",
    "username": "new.user",
    "password": "Password123!",
    "first_name": "New",
    "last_name": "User",
    "base_role": "staff"
  },
  "step2": {
    "employee_id": "199001012025012001",
    "full_name": "New User",
    "email": "new.user@binawan.ac.id",
    "phone": "081234567890",
    "gender": "male",
    "address": "Jakarta",
    "department_id": "uuid-of-department",
    "position_id": "uuid-of-position",
    "joined_date": "2025-01-01T00:00:00Z"
  },
  "step3": {
    "roles": ["role-uuid-1"],
    "permissions": ["employees.read", "departments.read"]
  },
  "service_id": "bd2f143b-b991-4d02-94df-dcc91e9362da"
}
```

**Response (201):**
```json
{
  "success": true,
  "data": {
    "user_id": "...",
    "employee_id": "...",
    "message": "Employee onboarded successfully"
  }
}
```

**Error (400):**
```json
{
  "success": false,
  "error": "onboarding failed",
  "details": "step 1 failed (SSO registration): ..."
}
```

---

# 3. Permission Service (Port 8082)

**Base URL**: `http://localhost:8082`  
**All endpoints require:** `Authorization: Bearer <token>`  
**Route prefix:** `/api/`

## Health

### GET /health
**Public** — no auth required.

---

## Services

### GET /api/services
List all registered services.

**Response (200):**
```json
{
  "success": true,
  "data": [
    {
      "id": "bd2f143b-...",
      "name": "human-capital",
      "description": "Human Capital Management Service",
      "base_url": "http://localhost:8081",
      "is_active": true,
      "created_at": "...",
      "updated_at": "..."
    }
  ]
}
```

---

### GET /api/services/:id
Get service by ID.

---

### POST /api/services
Register a new service.

**Request:**
```json
{
  "name": "payroll",
  "description": "Payroll Management Service",
  "base_url": "http://localhost:8083"
}
```

---

### PUT /api/services/:id
Update a service.

**Request (all fields optional):**
```json
{
  "name": "payroll-v2",
  "description": "Updated description",
  "base_url": "http://localhost:8083"
}
```

---

### DELETE /api/services/:id
Delete a service.

---

### POST /api/services/:id/activate
Activate a service.

**Response (200):**
```json
{
  "success": true,
  "data": { "id": "...", "is_active": true, ... }
}
```

---

### POST /api/services/:id/deactivate
Deactivate a service.

---

### POST /api/services/:id/access
Grant user access to a service.

**Request:**
```json
{
  "user_id": "e4607511-...",
  "granted_by": "admin"
}
```

**Response (201):**
```json
{
  "success": true,
  "data": {
    "id": "...",
    "user_id": "e4607511-...",
    "service_id": "bd2f143b-...",
    "is_active": true
  }
}
```

---

### DELETE /api/access/:id
Revoke user access to a service.

---

## Permissions

### GET /api/services/:id/permissions
List all permissions for a service.

**Response (200):**
```json
{
  "success": true,
  "data": [
    {
      "id": "...",
      "permission": "employees.read",
      "description": "Read employees",
      "service_id": "bd2f143b-..."
    }
  ]
}
```

---

### POST /api/services/:id/permissions
Create a new permission for a service.

**Request:**
```json
{
  "permission": "employees.export",
  "description": "Export employee data"
}
```

---

### POST /api/services/:id/permissions/seed
Seed default permissions for a service.

---

### DELETE /api/permissions/:id
Delete a permission.

---

## Roles

### GET /api/roles
List all roles. Optional filter by service.

**Query Parameters:**
| Param | Type | Description |
|-------|------|-------------|
| `service_id` | string | Filter roles by service |

**Response (200):**
```json
{
  "success": true,
  "data": {
    "items": [
      {
        "id": "6455029c-...",
        "name": "Admin HR",
        "description": "HR Administrator with full access",
        "service_id": "bd2f143b-...",
        "permissions": [
          {
            "id": "...",
            "permission": "employees.read"
          }
        ],
        "created_at": "...",
        "updated_at": "..."
      }
    ]
  }
}
```

---

### GET /api/roles/:id
Get role by ID.

---

### POST /api/roles
Create a new role.

**Request:**
```json
{
  "name": "Admin HR",
  "description": "HR Administrator",
  "service_id": "bd2f143b-..."
}
```

---

### PUT /api/roles/:id
Update a role.

**Request (all fields optional):**
```json
{
  "name": "Senior Admin HR",
  "description": "Updated description"
}
```

---

### DELETE /api/roles/:id
Delete a role.

---

### POST /api/roles/assign
Assign a role to a user.

**Request:**
```json
{
  "user_id": "e4607511-...",
  "role_id": "6455029c-..."
}
```

---

### DELETE /api/roles/users/:user_id/roles/:role_id
Revoke a role from a user.

---

### GET /api/roles/users/:user_id
Get all roles assigned to a user.

**Response (200):**
```json
{
  "success": true,
  "data": [
    {
      "id": "...",
      "user_id": "e4607511-...",
      "role_id": "6455029c-...",
      "role": {
        "id": "6455029c-...",
        "name": "Admin HR",
        "description": "...",
        "service_id": "bd2f143b-...",
        "permissions": [
          { "id": "...", "permission": "employees.read" }
        ]
      }
    }
  ]
}
```

---

## User Permissions

### GET /api/users/:user_id/permissions
Get all direct permissions for a user.

**Response (200):**
```json
{
  "success": true,
  "data": [
    {
      "id": "...",
      "user_id": "e4607511-...",
      "service_id": "bd2f143b-...",
      "permission": "employees.read",
      "is_active": true
    }
  ]
}
```

---

### GET /api/users/:user_id/permissions/service/:service_id
Get user permissions for a specific service.

---

### GET /api/users/:user_id/permissions/check
Check if user has a specific permission.

**Query Parameters:**
| Param | Type | Required | Description |
|-------|------|----------|-------------|
| `service_id` | string | ✅ | Service UUID |
| `permission` | string | ✅ | Permission string (e.g., "employees.read") |

**Response (200):**
```json
{
  "success": true,
  "data": {
    "has_permission": true,
    "permission": "employees.read"
  }
}
```

---

### POST /api/users/:user_id/permissions
Grant a direct permission to a user.

**Request:**
```json
{
  "service_id": "bd2f143b-...",
  "permission": "employees.read",
  "granted_by": "admin"
}
```

---

### DELETE /api/user-permissions/:id
Revoke a direct permission from a user.

---

## User Services (Access)

### GET /api/users/:user_id/services
Get all services a user has access to.

**Response (200):**
```json
{
  "success": true,
  "data": [
    {
      "id": "...",
      "user_id": "e4607511-...",
      "service_id": "bd2f143b-...",
      "is_active": true
    }
  ]
}
```

---

### GET /api/users/:user_id/services/:service_id/check
Check if user has access to a specific service.

**Response (200):**
```json
{
  "success": true,
  "data": {
    "has_access": true
  }
}
```

---

## User Menu

### GET /api/users/:user_id/menu
Get dynamic menu for a user (used by frontend sidebar).

**Response (200):**
```json
{
  "data": [
    {
      "service_id": "bd2f143b-...",
      "service_name": "human-capital",
      "description": "Human Capital Management Service",
      "base_url": "http://localhost:8081",
      "permissions": [
        "employees.read",
        "employees.write",
        "departments.read",
        "departments.write",
        "positions.read",
        "positions.write"
      ]
    },
    {
      "service_id": "c1aeffde-...",
      "service_name": "permission-service",
      "description": "Permission Management Service",
      "base_url": "http://localhost:8082",
      "permissions": [
        "services.read",
        "roles.read",
        "users.read"
      ]
    }
  ]
}
```

**Usage in Frontend:**
- Each item = one sidebar menu group
- `permissions` array determines which buttons to show/hide
- If `permissions` is empty or null → show all buttons (fallback)

---

# 5. Error Codes

| HTTP Status | Meaning |
|-------------|---------|
| `200` | Success |
| `201` | Created |
| `400` | Bad Request (validation error) |
| `401` | Unauthorized (missing/invalid token) |
| `403` | Forbidden (no permission) |
| `404` | Not Found |
| `409` | Conflict (duplicate) |
| `429` | Rate Limited |
| `500` | Internal Server Error |

---

# 6. Quick Start for Frontend

## Environment Variables

```env
# .env.local
NEXT_PUBLIC_SSO_URL=http://localhost:8080
NEXT_PUBLIC_HC_URL=http://localhost:8081
NEXT_PUBLIC_PERMISSION_URL=http://localhost:8082
```

## Authentication Flow

```
1. POST /api/v1/auth/login (SSO:8080)
   → Get access_token + user info + roles

2. GET /api/users/:user_id/menu (Permission:8082)
   → Get menu items + permissions for sidebar

3. Store token in localStorage/Zustand
   → Send as Bearer token in all subsequent requests
```

## API Client Setup

```typescript
// SSO client
const ssoClient = axios.create({
  baseURL: 'http://localhost:8080',
});

// HC client (with auth)
const hcClient = axios.create({
  baseURL: 'http://localhost:8081',
});
hcClient.interceptors.request.use((config) => {
  const token = localStorage.getItem('access_token');
  if (token) config.headers.Authorization = `Bearer ${token}`;
  return config;
});

// Permission client (with auth)
const permClient = axios.create({
  baseURL: 'http://localhost:8082',
});
// Same auth interceptor
```

## Permission Check Pattern

```typescript
// Check if user can perform an action
const canWrite = (permName: string) => {
  const perms = menuItems.find(m => m.service_name === 'human-capital')?.permissions;
  if (!perms || perms.length === 0) return true; // fallback: show all
  return perms.includes(permName);
};

// Usage
{canWrite('departments.write') && <Button>Create Department</Button>}
```

## Test Credentials

| Username/Email | Password | Role | Access |
|----------------|----------|------|--------|
| `devops@binawan.ac.id` | `admin12345` | Administrator | Full access |
| `budi.santoso` | `Budi@12345` | Staff HR | Read-only HC |
| `manda.raihanalaksita@binawan.ac.id` | — | Admin HR | Full HC access |

---

## Service Ports Summary

| Service | Port | DB | Route Prefix |
|---------|------|-----|-------------|
| SSO | 8080 | `samsungdb` | `/api/v1/` |
| Human Capital | 8081 | `hcdb` | `/api/` |
| Permission | 8082 | `permissiondb` | `/api/` |
| B-Wise Frontend | 3002 | — | — |
