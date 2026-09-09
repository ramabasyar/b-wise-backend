# Human Capital Service - API Documentation

> **Port:** 8081 | **Database:** `hc_db` | **Auth:** JWT via JWKS + Permission Check

---

## Authentication & Authorization

Semua endpoint `/api/*` memerlukan:

1. **Valid JWT token** (user atau service) via `Authorization: Bearer <token>`
2. **Access ke service** (user harus di-grant access di Permission Service)
3. **Specific permission** (setiap endpoint punya permission yang required)

### Token Types

| Token | Source | Behavior |
|-------|--------|----------|
| **User Token** | Login SSO (`POST /api/auth/login`) | Permission check via Permission Service |
| **Service Token** | OAuth2 (`POST /api/oauth/token`) | Bypass permission check (trusted) |

### Authorization Flow

```
Request → JWT Valid? → User has access? → User has permission? → Data
               ↓              ↓                    ↓
            401 Invalid    403 No access      403 Insufficient
```

---

## Response Format

### Success
```json
{
  "success": true,
  "data": { ... },
  "meta": { "page": 1, "page_size": 20, "total": 10 }
}
```

### Error
```json
{ "error": "error message" }
{ "error": "insufficient permissions", "required_permission": "employees.write" }
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

## Employee Endpoints

### List Employees

```
GET /api/employees
```

**Permission Required:** `employees.read`

| Query Param | Type | Default | Description |
|-------------|------|---------|-------------|
| page | int | 1 | Page number |
| page_size | int | 20 | Items per page (max 100) |
| status | string | - | Filter by status (active, inactive, resigned) |
| department_id | string | - | Filter by department |

**Response (200):**
```json
{
  "success": true,
  "data": [
    {
      "id": "4f2e898c-6712-46db-ac1e-5fad85c89e32",
      "employee_id": "EMP001",
      "full_name": "Rama Pratama",
      "email": "rama@binawan.ac.id",
      "phone": "08123456789",
      "gender": "male",
      "address": "",
      "status": "active",
      "created_at": "2026-05-20T15:03:45.311021+07:00",
      "updated_at": "2026-05-20T15:03:45.311021+07:00"
    }
  ],
  "meta": {
    "page": 1,
    "page_size": 20,
    "total": 5
  }
}
```

---

### Get Employee

```
GET /api/employees/:id
```

**Permission Required:** `employees.read`

**Response (200):**
```json
{
  "success": true,
  "data": {
    "id": "4f2e898c-...",
    "employee_id": "EMP001",
    "full_name": "Rama Pratama",
    "email": "rama@binawan.ac.id",
    "phone": "08123456789",
    "gender": "male",
    "address": "",
    "status": "active",
    "created_at": "2026-05-20T15:03:45.311021+07:00"
  }
}
```

**Error (404):**
```json
{ "success": false, "error": "employee not found" }
```

---

### Create Employee

```
POST /api/employees
```

**Permission Required:** `employees.write`

**Request Body:**
```json
{
  "employee_id": "EMP001",
  "full_name": "Rama Pratama",
  "email": "rama@binawan.ac.id",
  "phone": "08123456789",
  "gender": "male",
  "address": "Jakarta, Indonesia",
  "status": "active"
}
```

| Field | Type | Required | Description |
|-------|------|----------|-------------|
| employee_id | string | ✅ | Unique employee number (NIP) |
| full_name | string | ✅ | Full name |
| email | string | ✅ | Unique email |
| phone | string | ❌ | Phone number |
| gender | string | ❌ | `male` / `female` |
| birth_date | string | ❌ | ISO date |
| address | string | ❌ | Address |
| department_id | string | ❌ | Department UUID |
| position_id | string | ❌ | Position UUID |
| status | string | ❌ | `active` (default), `inactive`, `resigned` |
| joined_date | string | ❌ | Join date |

**Response (200):**
```json
{
  "success": true,
  "data": {
    "id": "4f2e898c-...",
    "employee_id": "EMP001",
    "full_name": "Rama Pratama",
    "email": "rama@binawan.ac.id",
    "status": "active",
    "created_at": "2026-05-20T..."
  }
}
```

---

### Update Employee

```
PUT /api/employees/:id
```

**Permission Required:** `employees.write`

**Request Body:** (partial update, all fields optional)
```json
{
  "full_name": "Rama Pratama Updated",
  "phone": "08123456789"
}
```

**Response (200):**
```json
{
  "success": true,
  "data": {
    "id": "4f2e898c-...",
    "employee_id": "EMP001",
    "full_name": "Rama Pratama Updated",
    "phone": "08123456789",
    "updated_at": "2026-05-20T..."
  }
}
```

---

### Delete Employee

```
DELETE /api/employees/:id
```

**Permission Required:** `employees.delete`

**Response (200):**
```json
{
  "success": true,
  "message": "employee deleted"
}
```

---

## Department Endpoints

### List Departments

```
GET /api/departments
```

**Permission Required:** `departments.read`

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
      "id": "uuid",
      "code": "IT",
      "name": "Information Technology",
      "description": "IT Department",
      "is_active": true,
      "created_at": "2026-05-20T...",
      "updated_at": "2026-05-20T..."
    }
  ],
  "meta": { "page": 1, "page_size": 20, "total": 1 }
}
```

---

### Get Department

```
GET /api/departments/:id
```

**Permission Required:** `departments.read`

---

### Create Department

```
POST /api/departments
```

**Permission Required:** `departments.write`

**Request Body:**
```json
{
  "code": "IT",
  "name": "Information Technology",
  "description": "IT Department"
}
```

| Field | Type | Required | Description |
|-------|------|----------|-------------|
| code | string | ✅ | Unique department code |
| name | string | ✅ | Department name |
| description | string | ❌ | Description |

---

### Update Department

```
PUT /api/departments/:id
```

**Permission Required:** `departments.write`

---

### Delete Department

```
DELETE /api/departments/:id
```

**Permission Required:** `departments.delete`

---

## Position Endpoints

### List Positions

```
GET /api/positions
```

**Permission Required:** `positions.read`

**Response (200):**
```json
{
  "success": true,
  "data": [
    {
      "id": "uuid",
      "code": "DEV",
      "name": "Software Developer",
      "description": "Full-stack developer",
      "level": 3,
      "is_active": true,
      "created_at": "2026-05-20T...",
      "updated_at": "2026-05-20T..."
    }
  ],
  "meta": { "page": 1, "page_size": 20, "total": 1 }
}
```

---

### Create Position

```
POST /api/positions
```

**Permission Required:** `positions.write`

**Request Body:**
```json
{
  "code": "DEV",
  "name": "Software Developer",
  "description": "Full-stack developer",
  "level": 3,
  "department_id": "uuid-of-department"
}
```

| Field | Type | Required | Description |
|-------|------|----------|-------------|
| code | string | ✅ | Unique position code |
| name | string | ✅ | Position name |
| description | string | ❌ | Description |
| level | int | ❌ | Job level/grade (default 1) |
| department_id | string | ❌ | Department UUID |

---

### Delete Position

```
DELETE /api/positions/:id
```

**Permission Required:** `positions.delete`

---

## Error Responses

### 401 Unauthorized

```json
{ "error": "authentication required" }
{ "error": "invalid token", "details": "token expired" }
{ "error": "invalid token", "details": "signature verification failed" }
```

### 403 Forbidden

```json
{ "error": "no access to this service" }
{ "error": "insufficient permissions", "required_permission": "employees.write" }
```

### 404 Not Found

```json
{ "success": false, "error": "employee not found" }
{ "success": false, "error": "department not found" }
```

### 422 Validation Error

```json
{
  "success": false,
  "error": "Key: 'CreateEmployee.EmployeeID' Error:Field validation for 'EmployeeID' failed on the 'required' tag\nKey: 'CreateEmployee.Email' Error:Field validation for 'Email' failed on the 'required' tag"
}
```

---

## cURL Examples

### With User Token

```bash
# Login
TOKEN=$(curl -s -X POST http://localhost:8080/api/auth/login \
  -H "Content-Type: application/json" \
  -d '{"email":"admin@example.com","password":"Admin@123"}' \
  | jq -r '.data.access_token')

# List employees
curl -s http://localhost:8081/api/employees \
  -H "Authorization: Bearer $TOKEN" | jq

# Create employee
curl -s -X POST http://localhost:8081/api/employees \
  -H "Authorization: Bearer $TOKEN" \
  -H "Content-Type: application/json" \
  -d '{
    "employee_id": "EMP004",
    "full_name": "Ahmad Fauzi",
    "email": "ahmad@binawan.ac.id",
    "gender": "male",
    "status": "active"
  }' | jq

# Update employee
curl -s -X PUT http://localhost:8081/api/employees/<uuid> \
  -H "Authorization: Bearer $TOKEN" \
  -H "Content-Type: application/json" \
  -d '{"phone": "081299990000"}' | jq
```

### With Service Token

```bash
# Get service token
SVC_TOKEN=$(curl -s -X POST http://localhost:8080/api/oauth/token \
  -H "Content-Type: application/json" \
  -d '{
    "client_id": "human-capi-e89a8afc",
    "client_secret": "<secret>",
    "grant_type": "client_credentials"
  }' | jq -r '.data.access_token')

# Service token bypasses permission check
curl -s http://localhost:8081/api/employees \
  -H "Authorization: Bearer $SVC_TOKEN" | jq
```

---

## Permission Summary

| Endpoint | Method | Permission |
|----------|--------|------------|
| `/api/employees` | GET | `employees.read` |
| `/api/employees/:id` | GET | `employees.read` |
| `/api/employees` | POST | `employees.write` |
| `/api/employees/:id` | PUT | `employees.write` |
| `/api/employees/:id` | DELETE | `employees.delete` |
| `/api/departments` | GET | `departments.read` |
| `/api/departments/:id` | GET | `departments.read` |
| `/api/departments` | POST | `departments.write` |
| `/api/departments/:id` | PUT | `departments.write` |
| `/api/departments/:id` | DELETE | `departments.delete` |
| `/api/positions` | GET | `positions.read` |
| `/api/positions` | POST | `positions.write` |
| `/api/positions/:id` | DELETE | `positions.delete` |

---

*Last updated: 2026-05-20*
