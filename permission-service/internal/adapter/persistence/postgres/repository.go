package postgres

import (
	"time"

	"github.com/rama/b-wise/permission-service/internal/domain/entity"
	"gorm.io/gorm"
)

// ServiceRepository handles service CRUD operations
type ServiceRepository struct {
	db *gorm.DB
}

// NewServiceRepository creates a new service repository
func NewServiceRepository(db *gorm.DB) *ServiceRepository {
	return &ServiceRepository{db: db}
}

// Create creates a new service
func (r *ServiceRepository) Create(svc *entity.Service) error {
	return r.db.Create(svc).Error
}

// GetByID gets a service by ID
func (r *ServiceRepository) GetByID(id string) (*entity.Service, error) {
	var svc entity.Service
	if err := r.db.Where("id = ?", id).First(&svc).Error; err != nil {
		return nil, err
	}
	return &svc, nil
}

// GetByName gets a service by name
func (r *ServiceRepository) GetByName(name string) (*entity.Service, error) {
	var svc entity.Service
	if err := r.db.Where("name = ?", name).First(&svc).Error; err != nil {
		return nil, err
	}
	return &svc, nil
}

// List lists all services with pagination
func (r *ServiceRepository) List(limit, offset int) ([]entity.Service, int64, error) {
	var services []entity.Service
	var total int64

	r.db.Model(&entity.Service{}).Count(&total)
	if err := r.db.Limit(limit).Offset(offset).Order("created_at DESC").Find(&services).Error; err != nil {
		return nil, 0, err
	}
	return services, total, nil
}

// Update updates a service
func (r *ServiceRepository) Update(svc *entity.Service) error {
	return r.db.Save(svc).Error
}

// Delete soft-deletes a service
func (r *ServiceRepository) Delete(id string) error {
	return r.db.Where("id = ?", id).Delete(&entity.Service{}).Error
}

// Activate activates a service
func (r *ServiceRepository) Activate(id string) error {
	return r.db.Model(&entity.Service{}).Where("id = ?", id).Update("is_active", true).Error
}

// Deactivate deactivates a service
func (r *ServiceRepository) Deactivate(id string) error {
	return r.db.Model(&entity.Service{}).Where("id = ?", id).Update("is_active", false).Error
}

// ServicePermissionRepository handles service permission CRUD
type ServicePermissionRepository struct {
	db *gorm.DB
}

// NewServicePermissionRepository creates a new repository
func NewServicePermissionRepository(db *gorm.DB) *ServicePermissionRepository {
	return &ServicePermissionRepository{db: db}
}

// Create creates a new permission
func (r *ServicePermissionRepository) Create(perm *entity.ServicePermission) error {
	return r.db.Create(perm).Error
}

// GetByID gets a permission by ID
func (r *ServicePermissionRepository) GetByID(id string) (*entity.ServicePermission, error) {
	var perm entity.ServicePermission
	if err := r.db.Where("id = ?", id).First(&perm).Error; err != nil {
		return nil, err
	}
	return &perm, nil
}

// ListByService lists permissions for a service
func (r *ServicePermissionRepository) ListByService(serviceID string) ([]entity.ServicePermission, error) {
	var perms []entity.ServicePermission
	if err := r.db.Where("service_id = ? AND deleted_at IS NULL AND is_active = ?", serviceID, true).Find(&perms).Error; err != nil {
		return nil, err
	}
	return perms, nil
}

// Update updates a permission
func (r *ServicePermissionRepository) Update(perm *entity.ServicePermission) error {
	return r.db.Save(perm).Error
}

// Delete soft-deletes a permission
func (r *ServicePermissionRepository) Delete(id string) error {
	return r.db.Where("id = ?", id).Delete(&entity.ServicePermission{}).Error
}

// ServiceAccessRepository handles service access CRUD
type ServiceAccessRepository struct {
	db *gorm.DB
}

// NewServiceAccessRepository creates a new repository
func NewServiceAccessRepository(db *gorm.DB) *ServiceAccessRepository {
	return &ServiceAccessRepository{db: db}
}

// Create creates a new service access
func (r *ServiceAccessRepository) Create(access *entity.ServiceAccess) error {
	return r.db.Create(access).Error
}

// GetByID gets service access by ID
func (r *ServiceAccessRepository) GetByID(id string) (*entity.ServiceAccess, error) {
	var access entity.ServiceAccess
	if err := r.db.Where("id = ?", id).First(&access).Error; err != nil {
		return nil, err
	}
	return &access, nil
}

// GetByUserAndService gets active access for a user+service combination
func (r *ServiceAccessRepository) GetByUserAndService(userID, serviceID string) (*entity.ServiceAccess, error) {
	var access entity.ServiceAccess
	if err := r.db.Where("user_id = ? AND service_id = ? AND is_active = ?", userID, serviceID, true).First(&access).Error; err != nil {
		return nil, err
	}
	return &access, nil
}

// ListByUser lists active service accesses for a user
func (r *ServiceAccessRepository) ListByUser(userID string) ([]entity.ServiceAccess, error) {
	var accesses []entity.ServiceAccess
	if err := r.db.Where("user_id = ? AND is_active = ?", userID, true).Find(&accesses).Error; err != nil {
		return nil, err
	}
	return accesses, nil
}

// Revoke revokes service access
func (r *ServiceAccessRepository) Revoke(id, revokedBy string) error {
	now := time.Now()
	return r.db.Model(&entity.ServiceAccess{}).Where("id = ?", id).
		Updates(map[string]interface{}{
			"is_active":  false,
			"revoked_at": now,
			"revoked_by": revokedBy,
		}).Error
}

// UserPermissionRepository handles user permission CRUD
type UserPermissionRepository struct {
	db *gorm.DB
}

// NewUserPermissionRepository creates a new repository
func NewUserPermissionRepository(db *gorm.DB) *UserPermissionRepository {
	return &UserPermissionRepository{db: db}
}

// Create creates a new user permission
func (r *UserPermissionRepository) Create(perm *entity.UserPermission) error {
	return r.db.Create(perm).Error
}

// GetByID gets a user permission by ID
func (r *UserPermissionRepository) GetByID(id string) (*entity.UserPermission, error) {
	var perm entity.UserPermission
	if err := r.db.Where("id = ?", id).First(&perm).Error; err != nil {
		return nil, err
	}
	return &perm, nil
}

// ListByUser lists active permissions for a user
func (r *UserPermissionRepository) ListByUser(userID string) ([]entity.UserPermission, error) {
	var perms []entity.UserPermission
	if err := r.db.Where("user_id = ? AND is_active = ?", userID, true).Find(&perms).Error; err != nil {
		return nil, err
	}
	return perms, nil
}

// ListByUserAndService lists active permissions for a user in a specific service
func (r *UserPermissionRepository) ListByUserAndService(userID, serviceID string) ([]entity.UserPermission, error) {
	var perms []entity.UserPermission
	if err := r.db.Where("user_id = ? AND service_id = ? AND is_active = ?", userID, serviceID, true).Find(&perms).Error; err != nil {
		return nil, err
	}
	return perms, nil
}

// Revoke revokes a user permission
func (r *UserPermissionRepository) Revoke(id, revokedBy string) error {
	now := time.Now()
	return r.db.Model(&entity.UserPermission{}).Where("id = ?", id).
		Updates(map[string]interface{}{
			"is_active":  false,
			"revoked_at": now,
			"revoked_by": revokedBy,
		}).Error
}

// HasPermission checks if a user has a specific permission
func (r *UserPermissionRepository) HasPermission(userID, permission, serviceID string) (bool, error) {
	var count int64
	err := r.db.Model(&entity.UserPermission{}).
		Where("user_id = ? AND permission = ? AND service_id = ? AND is_active = ?", userID, permission, serviceID, true).
		Count(&count).Error
	return count > 0, err
}

// ==================== Role Repository ====================

// RoleRepository handles role CRUD operations
type RoleRepository struct {
	db *gorm.DB
}

// NewRoleRepository creates a new role repository
func NewRoleRepository(db *gorm.DB) *RoleRepository {
	return &RoleRepository{db: db}
}

// Create creates a new role
func (r *RoleRepository) Create(role *entity.Role) error {
	return r.db.Create(role).Error
}

// GetByID gets a role by ID
func (r *RoleRepository) GetByID(id string) (*entity.Role, error) {
	var role entity.Role
	if err := r.db.Preload("Service").Preload("Permissions").Where("id = ?", id).First(&role).Error; err != nil {
		return nil, err
	}
	return &role, nil
}

// GetByServiceID lists roles for a specific service
func (r *RoleRepository) GetByServiceID(serviceID string) ([]entity.Role, error) {
	var roles []entity.Role
	if err := r.db.Preload("Permissions").Where("service_id = ?", serviceID).Order("name ASC").Find(&roles).Error; err != nil {
		return nil, err
	}
	return roles, nil
}

// List lists all roles with pagination
func (r *RoleRepository) List(limit, offset int) ([]entity.Role, int64, error) {
	var roles []entity.Role
	var total int64
	r.db.Model(&entity.Role{}).Count(&total)
	if err := r.db.Preload("Service").Preload("Permissions").Limit(limit).Offset(offset).Order("created_at DESC").Find(&roles).Error; err != nil {
		return nil, 0, err
	}
	return roles, total, nil
}

// Update updates a role
func (r *RoleRepository) Update(role *entity.Role) error {
	return r.db.Save(role).Error
}

// Delete soft-deletes a role
func (r *RoleRepository) Delete(id string) error {
	return r.db.Where("id = ?", id).Delete(&entity.Role{}).Error
}

// AddPermission adds a permission to a role
func (r *RoleRepository) AddPermission(roleID, permissionID string) error {
	return r.db.Table("role_permissions").Create(map[string]interface{}{
		"role_id":               roleID,
		"service_permission_id": permissionID,
	}).Error
}

// RemovePermission removes a permission from a role
func (r *RoleRepository) RemovePermission(roleID, permissionID string) error {
	return r.db.Table("role_permissions").
		Where("role_id = ? AND service_permission_id = ?", roleID, permissionID).
		Delete(nil).Error
}

// SetPermissions replaces all permissions for a role
func (r *RoleRepository) SetPermissions(roleID string, permissionIDs []string) error {
	tx := r.db.Begin()
	// Remove existing
	if err := tx.Table("role_permissions").Where("role_id = ?", roleID).Delete(nil).Error; err != nil {
		tx.Rollback()
		return err
	}
	// Add new
	for _, pid := range permissionIDs {
		if err := tx.Table("role_permissions").Create(map[string]interface{}{
			"role_id":               roleID,
			"service_permission_id": pid,
		}).Error; err != nil {
			tx.Rollback()
			return err
		}
	}
	return tx.Commit().Error
}

// ==================== User Role Repository ====================

// UserRoleRepository handles user-role assignments
type UserRoleRepository struct {
	db *gorm.DB
}

// NewUserRoleRepository creates a new user role repository
func NewUserRoleRepository(db *gorm.DB) *UserRoleRepository {
	return &UserRoleRepository{db: db}
}

// Assign assigns a role to a user
func (r *UserRoleRepository) Assign(userRole *entity.UserRole) error {
	return r.db.Create(userRole).Error
}

// Revoke removes a role from a user
func (r *UserRoleRepository) Revoke(userID, roleID string) error {
	return r.db.Where("user_id = ? AND role_id = ?", userID, roleID).Delete(&entity.UserRole{}).Error
}

// GetUserRoles gets all roles for a user
func (r *UserRoleRepository) GetUserRoles(userID string) ([]entity.UserRole, error) {
	var userRoles []entity.UserRole
	if err := r.db.Preload("Role").Preload("Role.Service").Preload("Role.Permissions").
		Where("user_id = ?", userID).Find(&userRoles).Error; err != nil {
		return nil, err
	}
	return userRoles, nil
}

// GetUserRolesByService gets user roles for a specific service
func (r *UserRoleRepository) GetUserRolesByService(userID, serviceID string) ([]entity.UserRole, error) {
	var userRoles []entity.UserRole
	if err := r.db.Joins("JOIN roles ON roles.id = user_roles.role_id").
		Where("user_roles.user_id = ? AND roles.service_id = ?", userID, serviceID).
		Preload("Role").Preload("Role.Permissions").
		Find(&userRoles).Error; err != nil {
		return nil, err
	}
	return userRoles, nil
}

// HasRole checks if user has a specific role
func (r *UserRoleRepository) HasRole(userID, roleID string) (bool, error) {
	var count int64
	if err := r.db.Model(&entity.UserRole{}).Where("user_id = ? AND role_id = ?", userID, roleID).Count(&count).Error; err != nil {
		return false, err
	}
	return count > 0, nil
}

// ==================== Menu Item Repository ====================

// MenuItemRepository handles menu item persistence
type MenuItemRepository struct {
	db *gorm.DB
}

// NewMenuItemRepository creates a new menu item repository
func NewMenuItemRepository(db *gorm.DB) *MenuItemRepository {
	return &MenuItemRepository{db: db}
}

// Create stores a new menu item
func (r *MenuItemRepository) Create(item *entity.MenuItem) error {
	return r.db.Create(item).Error
}

// GetByID gets a menu item by ID
func (r *MenuItemRepository) GetByID(id string) (*entity.MenuItem, error) {
	var item entity.MenuItem
	if err := r.db.Where("id = ?", id).First(&item).Error; err != nil {
		return nil, err
	}
	return &item, nil
}

// ListByService lists active menu items of a service (flat, urut sort_order)
func (r *MenuItemRepository) ListByService(serviceID string) ([]entity.MenuItem, error) {
	var items []entity.MenuItem
	if err := r.db.Where("service_id = ? AND is_active = ?", serviceID, true).
		Order("sort_order ASC, label ASC").Find(&items).Error; err != nil {
		return nil, err
	}
	return items, nil
}

// ListAll lists all menu items (admin)
func (r *MenuItemRepository) ListAll() ([]entity.MenuItem, error) {
	var items []entity.MenuItem
	if err := r.db.Order("service_id ASC, sort_order ASC").Find(&items).Error; err != nil {
		return nil, err
	}
	return items, nil
}

// Update updates a menu item
func (r *MenuItemRepository) Update(item *entity.MenuItem) error {
	return r.db.Save(item).Error
}

// Delete soft-deletes a menu item
func (r *MenuItemRepository) Delete(id string) error {
	return r.db.Delete(&entity.MenuItem{}, "id = ?", id).Error
}

// HasAnyPermission single-query union: direct grants ATAU via role, service-scoped.
// Menggantikan N+1 loop per role di service layer.
func (r *UserPermissionRepository) HasAnyPermission(userID, serviceID, permission string) (bool, error) {
	var count int64

	// direct — '*' = wildcard semua permission di service tsb
	q1 := r.db.Model(&entity.UserPermission{}).
		Where("user_id = ? AND service_id = ? AND permission IN (?, '*') AND is_active = ?",
			userID, serviceID, permission, true)

	// via role (user_roles -> roles service-scoped -> role_permissions -> service_permissions)
	q2 := r.db.Table("user_roles ur").
		Joins("JOIN roles ro ON ro.id = ur.role_id").
		Joins("JOIN role_permissions rp ON rp.role_id = ro.id").
		Joins("JOIN service_permissions sp ON sp.id = rp.service_permission_id").
		// NB: user_roles = hard delete (tanpa deleted_at); service_permissions soft delete
		Where("ur.user_id = ? AND ro.service_id = ? AND sp.permission IN (?, '*') AND sp.deleted_at IS NULL",
			userID, serviceID, permission)

	if err := q1.Session(&gorm.Session{}).Count(&count).Error; err != nil {
		return false, err
	}
	if count > 0 {
		return true, nil
	}
	if err := q2.Session(&gorm.Session{}).Count(&count).Error; err != nil {
		return false, err
	}
	return count > 0, nil
}

// Update updates a service access record
func (r *ServiceAccessRepository) Update(access *entity.ServiceAccess) error {
	return r.db.Save(access).Error
}


// ==================== AUDIT LOG ====================

// AuditLogRepository merekam dan membaca jejak mutasi assignment
type AuditLogRepository struct {
	db *gorm.DB
}

// NewAuditLogRepository creates a new audit log repository
func NewAuditLogRepository(db *gorm.DB) *AuditLogRepository {
	return &AuditLogRepository{db: db}
}

// Create records an audit entry
func (r *AuditLogRepository) Create(log *entity.AuditLog) error {
	return r.db.Create(log).Error
}

// AuditFilter filter untuk List
type AuditFilter struct {
	UserID    string
	Action    string
	ServiceID string
	Limit     int
	Offset    int
}

// List mengambil audit logs terbaru dulu
func (r *AuditLogRepository) List(f AuditFilter) ([]entity.AuditLog, int64, error) {
	q := r.db.Model(&entity.AuditLog{})
	if f.UserID != "" {
		q = q.Where("user_id = ?", f.UserID)
	}
	if f.Action != "" {
		q = q.Where("action = ?", f.Action)
	}
	if f.ServiceID != "" {
		q = q.Where("service_id = ?", f.ServiceID)
	}
	var total int64
	if err := q.Count(&total).Error; err != nil {
		return nil, 0, err
	}
	limit := f.Limit
	if limit <= 0 || limit > 200 {
		limit = 50
	}
	var logs []entity.AuditLog
	if err := q.Order("created_at DESC").Limit(limit).Offset(f.Offset).Find(&logs).Error; err != nil {
		return nil, 0, err
	}
	return logs, total, nil
}


// ListByServiceIncludeInactive — untuk self-sync (perlu lihat yang nonaktif juga)
func (r *ServicePermissionRepository) ListByServiceIncludeInactive(serviceID string) ([]entity.ServicePermission, error) {
	var perms []entity.ServicePermission
	if err := r.db.Unscoped().Where("service_id = ? AND deleted_at IS NULL", serviceID).Find(&perms).Error; err != nil {
		return nil, err
	}
	return perms, nil
}

// SetActive activate/deactivate permission tanpa menghapus
func (r *ServicePermissionRepository) SetActive(id string, active bool) error {
	return r.db.Model(&entity.ServicePermission{}).Where("id = ?", id).Update("is_active", active).Error
}

// GetByPath — cari menu item by path (include nonaktif, untuk upsert self-sync)
func (r *MenuItemRepository) GetByPath(serviceID, path string) (*entity.MenuItem, error) {
	var item entity.MenuItem
	if err := r.db.Where("service_id = ? AND path = ?", serviceID, path).First(&item).Error; err != nil {
		return nil, err
	}
	return &item, nil
}

// ListByServiceIncludeInactive — untuk self-sync deactivate detection
func (r *MenuItemRepository) ListByServiceIncludeInactive(serviceID string) ([]entity.MenuItem, error) {
	var items []entity.MenuItem
	if err := r.db.Where("service_id = ? AND deleted_at IS NULL", serviceID).
		Order("sort_order ASC, label ASC").Find(&items).Error; err != nil {
		return nil, err
	}
	return items, nil
}
