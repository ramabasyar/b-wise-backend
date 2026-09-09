package service

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"strings"
	"time"

	"github.com/rama/b-wise/permission-service/internal/adapter/cache"
	"github.com/rama/b-wise/permission-service/internal/adapter/persistence/postgres"
	"github.com/rama/b-wise/permission-service/internal/domain/entity"
)

var (
	ErrServiceNotFound    = errors.New("service not found")
	ErrServiceExists      = errors.New("service already exists")
	ErrPermissionNotFound = errors.New("permission not found")
	ErrPermissionExists   = errors.New("permission already exists")
	ErrAccessNotFound     = errors.New("access not found")
	ErrAccessExists       = errors.New("access already exists")
	ErrNoAccess           = errors.New("user has no access to this service")
	ErrRoleNotFound       = errors.New("role not found")
	ErrRoleExists         = errors.New("role already exists")
)

// PermissionService handles all permission-related business logic
type PermissionService struct {
	superAdminIDs  map[string]struct{} // bypass semua cek akses/permission
	serviceRepo    *postgres.ServiceRepository
	permRepo       *postgres.ServicePermissionRepository
	accessRepo     *postgres.ServiceAccessRepository
	userPermRepo   *postgres.UserPermissionRepository
	roleRepo       *postgres.RoleRepository
	userRoleRepo   *postgres.UserRoleRepository
	menuRepo       *postgres.MenuItemRepository
	cache          *cache.Cache
	auditRepo      *postgres.AuditLogRepository
}

// NewPermissionService creates a new permission service.
// superAdminIDs: daftar user ID yang otomatis punya akses + semua permission
// di SEMUA service (env SUPER_ADMIN_IDS, dipisah koma).
func NewPermissionService(
	superAdminIDs []string,
	serviceRepo *postgres.ServiceRepository,
	permRepo *postgres.ServicePermissionRepository,
	accessRepo *postgres.ServiceAccessRepository,
	userPermRepo *postgres.UserPermissionRepository,
	roleRepo *postgres.RoleRepository,
	userRoleRepo *postgres.UserRoleRepository,
	menuRepo *postgres.MenuItemRepository,
	c *cache.Cache,
	auditRepo *postgres.AuditLogRepository,
) *PermissionService {
	return &PermissionService{
		superAdminIDs: toSet(superAdminIDs),
		serviceRepo:  serviceRepo,
		permRepo:     permRepo,
		accessRepo:   accessRepo,
		userPermRepo: userPermRepo,
		roleRepo:     roleRepo,
		userRoleRepo: userRoleRepo,
		menuRepo:     menuRepo,
		cache:        c,
		auditRepo:    auditRepo,
	}
}

// toSet — helper konversi slice ke set
func toSet(ids []string) map[string]struct{} {
	m := make(map[string]struct{}, len(ids))
	for _, id := range ids {
		if id = trimSpace(id); id != "" {
			m[id] = struct{}{}
		}
	}
	return m
}

func trimSpace(s string) string {
	start, end := 0, len(s)
	for start < end && (s[start] == ' ' || s[start] == '\t') {
		start++
	}
	for end > start && (s[end-1] == ' ' || s[end-1] == '\t') {
		end--
	}
	return s[start:end]
}

// isSuperAdmin — true jika userID terdaftar di superAdminIDs
func (s *PermissionService) isSuperAdmin(userID string) bool {
	_, ok := s.superAdminIDs[userID]
	return ok
}

// recordAudit — best-effort: kegagalan pencatatan TIDAK membatalkan mutasi
func (s *PermissionService) recordAudit(actor, action, targetUser, serviceID, objectType, objectID, detail string) {
	if s.auditRepo == nil {
		return
	}
	entry := &entity.AuditLog{
		ActorID:    actor,
		Action:     action,
		UserID:     targetUser,
		ServiceID:  serviceID,
		ObjectType: objectType,
		ObjectID:   objectID,
		Detail:     detail,
	}
	if entry.ActorID == "" {
		entry.ActorID = "system"
	}
	// kolom jsonb menolak string kosong — normalisasi ke objek kosong
	if strings.TrimSpace(entry.Detail) == "" {
		entry.Detail = "{}"
	}
	if err := s.auditRepo.Create(entry); err != nil {
		log.Printf("[audit] gagal mencatat %s: %v", action, err)
	}
}

// containsPerm — cek set permission dengan dukungan wildcard '*'
func containsPerm(set map[string]struct{}, perm string) bool {
	if _, ok := set[perm]; ok {
		return true
	}
	_, wildcard := set["*"]
	return wildcard
}

// InvalidateUserCache — dipanggil saat role/permission/access user berubah
func (s *PermissionService) InvalidateUserCache(userID string) {
	if s.cache == nil {
		return
	}
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	if err := s.cache.InvalidateUser(ctx, userID); err != nil {
		log.Printf("[cache] invalidate user %s gagal: %v", userID, err)
	}
}

// ==================== SERVICE MANAGEMENT ====================

// CreateService registers a new microservice
func (s *PermissionService) CreateService(svc *entity.Service) error {
	// Check if service name already exists
	existing, err := s.serviceRepo.GetByName(svc.Name)
	if err == nil && existing != nil {
		return ErrServiceExists
	}
	return s.serviceRepo.Create(svc)
}

// GetService gets a service by ID
func (s *PermissionService) GetService(id string) (*entity.Service, error) {
	svc, err := s.serviceRepo.GetByID(id)
	if err != nil {
		return nil, ErrServiceNotFound
	}
	return svc, nil
}

// ListServices lists all services with pagination
func (s *PermissionService) ListServices(page, pageSize int) ([]entity.Service, int64, error) {
	if page < 1 {
		page = 1
	}
	if pageSize < 1 || pageSize > 100 {
		pageSize = 20
	}
	offset := (page - 1) * pageSize
	return s.serviceRepo.List(pageSize, offset)
}

// UpdateService updates a service
func (s *PermissionService) UpdateService(svc *entity.Service) error {
	if _, err := s.serviceRepo.GetByID(svc.ID); err != nil {
		return ErrServiceNotFound
	}
	return s.serviceRepo.Update(svc)
}

// DeleteService soft-deletes a service
func (s *PermissionService) DeleteService(id string) error {
	if _, err := s.serviceRepo.GetByID(id); err != nil {
		return ErrServiceNotFound
	}
	return s.serviceRepo.Delete(id)
}

// ActivateService activates a service
func (s *PermissionService) ActivateService(id string) error {
	return s.serviceRepo.Activate(id)
}

// DeactivateService deactivates a service
func (s *PermissionService) DeactivateService(id string) error {
	return s.serviceRepo.Deactivate(id)
}

// ==================== PERMISSION MANAGEMENT ====================

// CreatePermission creates a new permission for a service
func (s *PermissionService) CreatePermission(perm *entity.ServicePermission) error {
	// Verify service exists
	if _, err := s.serviceRepo.GetByID(perm.ServiceID); err != nil {
		return ErrServiceNotFound
	}
	return s.permRepo.Create(perm)
}

// ListPermissions lists permissions for a service
func (s *PermissionService) ListPermissions(serviceID string) ([]entity.ServicePermission, error) {
	if _, err := s.serviceRepo.GetByID(serviceID); err != nil {
		return nil, ErrServiceNotFound
	}
	return s.permRepo.ListByService(serviceID)
}

// UpdatePermission updates a permission
func (s *PermissionService) UpdatePermission(perm *entity.ServicePermission) error {
	if _, err := s.permRepo.GetByID(perm.ID); err != nil {
		return ErrPermissionNotFound
	}
	return s.permRepo.Update(perm)
}

// DeletePermission soft-deletes a permission
func (s *PermissionService) DeletePermission(id string) error {
	if _, err := s.permRepo.GetByID(id); err != nil {
		return ErrPermissionNotFound
	}
	return s.permRepo.Delete(id)
}

// ==================== ACCESS MANAGEMENT ====================

// GrantAccess grants a user access to a service
func (s *PermissionService) GrantAccess(access *entity.ServiceAccess) error {
	// Verify service exists
	if _, err := s.serviceRepo.GetByID(access.ServiceID); err != nil {
		return ErrServiceNotFound
	}
	// Check if access already exists
	existing, err := s.accessRepo.GetByUserAndService(access.UserID, access.ServiceID)
	if err == nil && existing != nil && existing.IsActive {
		return ErrAccessExists
	}
	// If access exists but revoked, reactivate
	if existing != nil && !existing.IsActive {
		existing.IsActive = true
		existing.GrantedBy = access.GrantedBy
		existing.RevokedAt = nil
		existing.RevokedBy = ""
		if err := s.accessRepo.Update(existing); err != nil {
			return err
		}
		s.InvalidateUserCache(access.UserID)
		s.recordAudit(access.GrantedBy, "grant_access", access.UserID, access.ServiceID, "access", existing.ID, `{"reactivated":true}`)
		return nil
	}
	if err := s.accessRepo.Create(access); err != nil {
		return err
	}
	s.InvalidateUserCache(access.UserID)
	s.recordAudit(access.GrantedBy, "grant_access", access.UserID, access.ServiceID, "access", access.ID, "")
	return nil
}

// RevokeAccess revokes a user's access to a service
func (s *PermissionService) RevokeAccess(id, revokedBy string) error {
	access, err := s.accessRepo.GetByID(id)
	if err != nil {
		return ErrAccessNotFound
	}
	if err := s.accessRepo.Revoke(id, revokedBy); err != nil {
		return err
	}
	s.InvalidateUserCache(access.UserID)
	s.recordAudit(revokedBy, "revoke_access", access.UserID, access.ServiceID, "access", id, "")
	return nil
}

// GetUserServices gets all services a user has access to
func (s *PermissionService) GetUserServices(userID string) ([]entity.ServiceAccess, error) {
	return s.accessRepo.ListByUser(userID)
}

// CheckAccess checks if a user has access to a service
func (s *PermissionService) CheckAccess(userID, serviceID string) (bool, error) {
	// Super admin: bypass semua cek akses
	if s.isSuperAdmin(userID) {
		return true, nil
	}

	// cache-aside access check (TTL 2m, otomatis invalid saat grant/revoke)
	if s.cache != nil {
		ctx, cancel := context.WithTimeout(context.Background(), 500*time.Millisecond)
		if has, found := s.cache.GetCachedAccess(ctx, userID, serviceID); found {
			cancel()
			return has, nil
		}
		cancel()
	}

	access, err := s.accessRepo.GetByUserAndService(userID, serviceID)
	if err != nil {
		return false, nil
	}
	has := access.IsActive

	if s.cache != nil {
		ctx, cancel := context.WithTimeout(context.Background(), 500*time.Millisecond)
		s.cache.SetCachedAccess(ctx, userID, serviceID, has)
		cancel()
	}
	return has, nil
}

// ==================== USER PERMISSION MANAGEMENT ====================

// GrantUserPermission grants a specific permission to a user
func (s *PermissionService) GrantUserPermission(perm *entity.UserPermission) error {
	// Verify service exists
	if _, err := s.serviceRepo.GetByID(perm.ServiceID); err != nil {
		return ErrServiceNotFound
	}
	// Check if user has access to the service
	hasAccess, err := s.CheckAccess(perm.UserID, perm.ServiceID)
	if err != nil {
		return err
	}
	if !hasAccess {
		return ErrNoAccess
	}
	if err := s.userPermRepo.Create(perm); err != nil {
		return err
	}
	s.InvalidateUserCache(perm.UserID)
	s.recordAudit(perm.GrantedBy, "grant_permission", perm.UserID, perm.ServiceID, "permission", perm.Permission, fmt.Sprintf(`{"permission":%q}`, perm.Permission))
	return nil
}

// RevokeUserPermission revokes a specific permission from a user
func (s *PermissionService) RevokeUserPermission(id, revokedBy string) error {
	perm, err := s.userPermRepo.GetByID(id)
	if err != nil {
		return ErrPermissionNotFound
	}
	if err := s.userPermRepo.Revoke(id, revokedBy); err != nil {
		return err
	}
	s.InvalidateUserCache(perm.UserID)
	s.recordAudit(revokedBy, "revoke_permission", perm.UserID, perm.ServiceID, "permission", perm.Permission, fmt.Sprintf(`{"permission":%q}`, perm.Permission))
	return nil
}

// GetUserPermissions gets all permissions for a user
func (s *PermissionService) GetUserPermissions(userID string) ([]entity.UserPermission, error) {
	return s.userPermRepo.ListByUser(userID)
}

// GetUserPermissionsByService gets permissions for a user in a specific service
func (s *PermissionService) GetUserPermissionsByService(userID, serviceID string) ([]entity.UserPermission, error) {
	return s.userPermRepo.ListByUserAndService(userID, serviceID)
}

// HasPermission checks if a user has a specific permission (direct or via role)
func (s *PermissionService) HasPermission(userID, permission, serviceID string) (bool, error) {
	// Super admin: semua permission diizinkan
	if s.isSuperAdmin(userID) {
		return true, nil
	}

	// Cache-aside: Redis dulu (jika aktif), DB sebagai fallback + isi cache
	if s.cache != nil {
		ctx, cancel := context.WithTimeout(context.Background(), 500*time.Millisecond)
		if has, found := s.cache.GetCachedPermission(ctx, userID, serviceID, permission); found {
			cancel()
			return has, nil
		}
		cancel()
	}

	has, err := s.userPermRepo.HasAnyPermission(userID, serviceID, permission)
	if err != nil {
		return false, err
	}

	if s.cache != nil {
		ctx, cancel := context.WithTimeout(context.Background(), 500*time.Millisecond)
		s.cache.SetCachedPermission(ctx, userID, serviceID, permission, has)
		cancel()
	}
	return has, nil
}

// HasPermissionsBatch — cek banyak permission sekali jalan.
// Cache per-perm dicek dulu; yang miss dihitung dari satu query set efektif.
func (s *PermissionService) HasPermissionsBatch(userID, serviceID string, permissions []string) (map[string]bool, error) {
	results := make(map[string]bool, len(permissions))

	// Super admin: semua permission diizinkan
	if s.isSuperAdmin(userID) {
		for _, p := range permissions {
			results[p] = true
		}
		return results, nil
	}

	var missing []string

	if s.cache != nil {
		ctx, cancel := context.WithTimeout(context.Background(), 500*time.Millisecond)
		for _, p := range permissions {
			if has, found := s.cache.GetCachedPermission(ctx, userID, serviceID, p); found {
				results[p] = has
			} else {
				missing = append(missing, p)
			}
		}
		cancel()
	} else {
		missing = append(missing, permissions...)
	}

	if len(missing) > 0 {
		set, err := s.GetEffectivePermissions(userID, serviceID)
		if err != nil {
			return nil, err
		}
		if s.cache != nil {
			ctx, cancel := context.WithTimeout(context.Background(), 500*time.Millisecond)
			for _, p := range missing {
				results[p] = containsPerm(set, p)
				s.cache.SetCachedPermission(ctx, userID, serviceID, p, results[p])
			}
			cancel()
		} else {
			for _, p := range missing {
				results[p] = containsPerm(set, p)
			}
		}
	}

	return results, nil
}

// ListAuditLogs membaca jejak audit dengan filter
func (s *PermissionService) ListAuditLogs(f postgres.AuditFilter) ([]entity.AuditLog, int64, error) {
	return s.auditRepo.List(f)
}

// GetUserMenu gets the menu structure for a user (services they can access).
// FIX: permissions = UNION(direct grants, role permissions) — sebelumnya hanya
// direct sehingga user yang diberi akses via role melihat menu tanpa permissions.
// Response: tree MenuItem per service (difilter required_permission) + flat scopes.
func (s *PermissionService) GetUserMenu(userID string) ([]map[string]interface{}, error) {
	// cache-aside menu
	if s.cache != nil {
		ctx, cancel := context.WithTimeout(context.Background(), 500*time.Millisecond)
		if raw, found := s.cache.GetCachedMenu(ctx, userID); found {
			var cached []map[string]interface{}
			if jsonErr := json.Unmarshal(raw, &cached); jsonErr == nil {
				cancel()
				return cached, nil
			}
		}
		cancel()
	}

	menu, err := s.buildUserMenu(userID)
	if err != nil {
		return nil, err
	}

	if s.cache != nil {
		ctx, cancel := context.WithTimeout(context.Background(), 500*time.Millisecond)
		if raw, jsonErr := json.Marshal(menu); jsonErr == nil {
			s.cache.SetCachedMenu(ctx, userID, raw)
		}
		cancel()
	}
	return menu, nil
}

// buildUserMenu — logika menu dari DB (dipisah dari caching wrapper)
func (s *PermissionService) buildUserMenu(userID string) ([]map[string]interface{}, error) {
	// Super admin: menu dari SEMUA service aktif + permission wildcard
	if s.isSuperAdmin(userID) {
		return s.buildSuperAdminMenu()
	}

	accesses, err := s.accessRepo.ListByUser(userID)
	if err != nil {
		return nil, err
	}

	var menu []map[string]interface{}
	for _, access := range accesses {
		svc, err := s.serviceRepo.GetByID(access.ServiceID)
		if err != nil {
			continue
		}
		if !svc.IsActive {
			continue
		}

		permSet, err := s.GetEffectivePermissions(userID, access.ServiceID)
		if err != nil {
			continue
		}
		permList := make([]string, 0, len(permSet))
		for p := range permSet {
			permList = append(permList, p)
		}

		// Menu tree service ini — tampilkan hanya item yang required_permission-nya
		// terpenuhi (kosong = tampil untuk semua yang ber-akses)
		items, _ := s.menuRepo.ListByService(access.ServiceID)
		tree := buildMenuTree(items, permSet, "")

		menu = append(menu, map[string]interface{}{
			"service_id":   svc.ID,
			"service_name": svc.Name,
			"description":  svc.Description,
			"base_url":     svc.BaseURL,
			"permissions":  permList, // flat scopes untuk conditional rendering
			"menu":         tree,
		})
	}

	return menu, nil
}

// buildSuperAdminMenu — semua service aktif, permissions ["*"] (wildcard).
func (s *PermissionService) buildSuperAdminMenu() ([]map[string]interface{}, error) {
	services, _, err := s.serviceRepo.List(1000, 0)
	if err != nil {
		return nil, err
	}

	menu := make([]map[string]interface{}, 0, len(services))
	for _, svc := range services {
		if !svc.IsActive {
			continue
		}
		items, _ := s.menuRepo.ListByService(svc.ID)
		tree := buildMenuTree(items, map[string]struct{}{"*": {}}, "")
		menu = append(menu, map[string]interface{}{
			"service_id":   svc.ID,
			"service_name": svc.Name,
			"description":  svc.Description,
			"base_url":     svc.BaseURL,
			"permissions":  []string{"*"},
			"menu":         tree,
		})
	}
	return menu, nil
}

// GetEffectivePermissions — union permission direct + via roles (satu sumber kebenaran).
func (s *PermissionService) GetEffectivePermissions(userID, serviceID string) (map[string]struct{}, error) {
	set := map[string]struct{}{}

	direct, err := s.userPermRepo.ListByUserAndService(userID, serviceID)
	if err == nil {
		for _, p := range direct {
			set[p.Permission] = struct{}{}
		}
	}

	// via roles (service-scoped)
	userRoles, err := s.userRoleRepo.GetUserRolesByService(userID, serviceID)
	if err == nil {
		for _, ur := range userRoles {
			if ur.Role == nil {
				continue
			}
			for _, rp := range ur.Role.Permissions {
				set[rp.Permission] = struct{}{}
			}
		}
	}

	return set, nil
}

// buildMenuTree bangun hierarchy menu + filter permission; parent difilter dulu,
// anak tanpa permission khusus ikut parent-nya.
func buildMenuTree(items []entity.MenuItem, perms map[string]struct{}, parentID string) []map[string]interface{} {
	var out []map[string]interface{}
	for _, it := range items {
		p := ""
		if it.ParentID != nil {
			p = *it.ParentID
		}
		if p != parentID {
			continue
		}
		// filter: required_permission harus dimiliki user (wildcard '*' lolos semua)
		if it.RequiredPermission != "" {
			if !containsPerm(perms, it.RequiredPermission) {
				continue
			}
		}
		node := map[string]interface{}{
			"id":       it.ID,
			"label":    it.Label,
			"path":     it.Path,
			"icon":     it.Icon,
			"sort":     it.SortOrder,
			"children": buildMenuTree(items, perms, it.ID),
		}
		out = append(out, node)
	}
	return out
}

// SeedServicePermissions creates default permissions for a service
func (s *PermissionService) SeedServicePermissions(serviceID, createdBy string, permissions []string) error {
	for _, perm := range permissions {
		p := &entity.ServicePermission{
			ServiceID:   serviceID,
			Permission:  perm,
			Description: fmt.Sprintf("Permission: %s", perm),
			CreatedBy:   createdBy,
		}
		if err := s.permRepo.Create(p); err != nil {
			// Skip if already exists
			continue
		}
	}
	return nil
}

// ==================== ROLE MANAGEMENT ====================

// CreateRole creates a new role for a service
func (s *PermissionService) CreateRole(role *entity.Role) error {
	if _, err := s.serviceRepo.GetByID(role.ServiceID); err != nil {
		return ErrServiceNotFound
	}
	return s.roleRepo.Create(role)
}

// GetRole gets a role by ID
func (s *PermissionService) GetRole(id string) (*entity.Role, error) {
	role, err := s.roleRepo.GetByID(id)
	if err != nil {
		return nil, ErrRoleNotFound
	}
	return role, nil
}

// ListRoles lists all roles with pagination
func (s *PermissionService) ListRoles(page, pageSize int) ([]entity.Role, int64, error) {
	if page < 1 {
		page = 1
	}
	if pageSize < 1 || pageSize > 100 {
		pageSize = 20
	}
	offset := (page - 1) * pageSize
	return s.roleRepo.List(pageSize, offset)
}

// ListRolesByService lists roles for a specific service
func (s *PermissionService) ListRolesByService(serviceID string) ([]entity.Role, error) {
	if _, err := s.serviceRepo.GetByID(serviceID); err != nil {
		return nil, ErrServiceNotFound
	}
	return s.roleRepo.GetByServiceID(serviceID)
}

// UpdateRole updates a role
func (s *PermissionService) UpdateRole(role *entity.Role) error {
	if _, err := s.roleRepo.GetByID(role.ID); err != nil {
		return ErrRoleNotFound
	}
	return s.roleRepo.Update(role)
}

// DeleteRole soft-deletes a role
func (s *PermissionService) DeleteRole(id string) error {
	if _, err := s.roleRepo.GetByID(id); err != nil {
		return ErrRoleNotFound
	}
	return s.roleRepo.Delete(id)
}

// SetRolePermissions sets all permissions for a role
func (s *PermissionService) SetRolePermissions(roleID string, permissionIDs []string) error {
	if _, err := s.roleRepo.GetByID(roleID); err != nil {
		return ErrRoleNotFound
	}
	return s.roleRepo.SetPermissions(roleID, permissionIDs)
}

// AssignRoleToUser assigns a role to a user (also grants service access + permissions)
func (s *PermissionService) AssignRoleToUser(userID, roleID, grantedBy string) error {
	role, err := s.roleRepo.GetByID(roleID)
	if err != nil {
		return ErrRoleNotFound
	}

	// Grant service access first
	s.GrantAccess(&entity.ServiceAccess{
		UserID:    userID,
		ServiceID: role.ServiceID,
		GrantedBy: grantedBy,
		IsActive:  true,
	})

	// Assign role (GrantAccess di atas sudah flush cache; flush lagi untuk role perms)
	userRole := &entity.UserRole{
		UserID:    userID,
		RoleID:    roleID,
		GrantedBy: grantedBy,
	}
	if err := s.userRoleRepo.Assign(userRole); err != nil {
		return err
	}

	// CATATAN DESAIN (Tahap 3): TIDAK lagi snapshot direct user_permissions.
	// Permission efektif = union(direct, via role) — dihitung saat check/menu,
	// sehingga revoke role otomatis mencabut permission turunannya.

	// Flush cache user
	s.InvalidateUserCache(userID)
	s.recordAudit(grantedBy, "assign_role", userID, role.ServiceID, "role", roleID, fmt.Sprintf(`{"role":%q}`, role.Name))
	return nil
}

// RevokeRoleFromUser removes a role from a user
func (s *PermissionService) RevokeRoleFromUser(userID, roleID, actor string) error {
	role, err := s.roleRepo.GetByID(roleID)
	if err != nil {
		return ErrRoleNotFound
	}
	if err := s.userRoleRepo.Revoke(userID, roleID); err != nil {
		return err
	}
	s.InvalidateUserCache(userID)
	s.recordAudit(actor, "revoke_role", userID, role.ServiceID, "role", roleID, fmt.Sprintf(`{"role":%q}`, role.Name))
	return nil
}

// GetUserRoles gets all roles for a user
func (s *PermissionService) GetUserRoles(userID string) ([]entity.UserRole, error) {
	return s.userRoleRepo.GetUserRoles(userID)
}

// GetUserRolesByService gets user roles for a specific service
func (s *PermissionService) GetUserRolesByService(userID, serviceID string) ([]entity.UserRole, error) {
	return s.userRoleRepo.GetUserRolesByService(userID, serviceID)
}

// ==================== MENU ITEM MANAGEMENT ====================

// CreateMenuItem adds a menu item (dipakai microservice saat register menu-nya)
func (s *PermissionService) CreateMenuItem(item *entity.MenuItem) error {
	if err := s.menuRepo.Create(item); err != nil {
		return err
	}
	s.flushMenuCache()
	return nil
}

// GetMenuItem gets a menu item by ID
func (s *PermissionService) GetMenuItem(id string) (*entity.MenuItem, error) {
	return s.menuRepo.GetByID(id)
}

// ListMenuItemsByService lists menu items of a service
func (s *PermissionService) ListMenuItemsByService(serviceID string) ([]entity.MenuItem, error) {
	return s.menuRepo.ListByService(serviceID)
}

// ListAllMenuItems lists all menu items (admin)
func (s *PermissionService) ListAllMenuItems() ([]entity.MenuItem, error) {
	return s.menuRepo.ListAll()
}

// UpdateMenuItem updates a menu item (label/path/icon/sort/permission/active)
func (s *PermissionService) UpdateMenuItem(item *entity.MenuItem) error {
	if _, err := s.menuRepo.GetByID(item.ID); err != nil {
		return errors.New("menu item not found")
	}
	if err := s.menuRepo.Update(item); err != nil {
		return err
	}
	s.flushMenuCache()
	return nil
}

// DeleteMenuItem soft-deletes a menu item
func (s *PermissionService) DeleteMenuItem(id string) error {
	if err := s.menuRepo.Delete(id); err != nil {
		return err
	}
	s.flushMenuCache()
	return nil
}

// ==================== SELF-SYNC (MANIFEST) ====================

// ManifestPermissionsInput — deklarasi service untuk self-sync
type ManifestInput struct {
	Service struct {
		Name        string `json:"name" binding:"required"`
		Description string `json:"description"`
		BaseURL     string `json:"base_url"`
		SyncEnabled bool   `json:"sync_enabled"`
	} `json:"service"`
	Permissions []struct {
		Permission  string `json:"permission" binding:"required"`
		Description string `json:"description"`
	} `json:"permissions"`
	Menu []ManifestMenuItem `json:"menu"`
	Roles []ManifestRole    `json:"roles"`
}

// ManifestRole — role default yang dideklarasikan service (Fase B staging bootstrap).
// Sync = UPSERT + ATTACH permissions (union — tidak pernah mencabut permission yang
// ditambahkan admin manual, tidak pernah menonaktifkan role). Role buatan admin bebas.
type ManifestRole struct {
	Name        string   `json:"name" binding:"required"`
	Description string   `json:"description"`
	Permissions []string `json:"permissions"`
}

// ManifestMenuItem — nested (children) maupun flat
type ManifestMenuItem struct {
	Label              string              `json:"label" binding:"required"`
	Path               string              `json:"path" binding:"required"`
	Icon               string              `json:"icon"`
	SortOrder          int                 `json:"sort_order"`
	RequiredPermission string              `json:"required_permission"`
	Children           []ManifestMenuItem  `json:"children"`
}

// SyncManifest — upsert idempotent dari deklarasi service (dipanggil service itu sendiri via /self/sync).
// Semantik: tambah/ubah bebas; yang hilang dari manifest → DEACTIVATE (bukan delete).
// Return ringkasan perubahan untuk keperluan log/observability.
func (s *PermissionService) SyncManifest(m *ManifestInput) (map[string]int, error) {
	summary := map[string]int{"services_created": 0, "permissions_created": 0, "permissions_activated": 0, "permissions_deactivated": 0, "menu_created": 0, "menu_updated": 0, "menu_deactivated": 0, "roles_created": 0}

	// 1. Upsert service by name
	svc, err := s.serviceRepo.GetByName(m.Service.Name)
	if err != nil {
		svc = &entity.Service{
			Name:        m.Service.Name,
			Description: m.Service.Description,
			BaseURL:     m.Service.BaseURL,
			IsActive:    true,
		}
		if err := s.serviceRepo.Create(svc); err != nil {
			return nil, err
		}
		summary["services_created"]++
	} else {
		changed := false
		if m.Service.Description != "" && svc.Description != m.Service.Description {
			svc.Description = m.Service.Description
			changed = true
		}
		if m.Service.BaseURL != "" && svc.BaseURL != m.Service.BaseURL {
			svc.BaseURL = m.Service.BaseURL
			changed = true
		}
		if changed {
			if err := s.serviceRepo.Update(svc); err != nil {
				return nil, err
			}
		}
	}

	// 2. Permissions: upsert by name; deactivate yang hilang dari manifest
	existingPerms, err := s.permRepo.ListByServiceIncludeInactive(svc.ID)
	if err != nil {
		return nil, err
	}
	permByName := map[string]*entity.ServicePermission{}
	for i := range existingPerms {
		permByName[existingPerms[i].Permission] = &existingPerms[i]
	}
	declared := map[string]bool{}
	for _, mp := range m.Permissions {
		declared[mp.Permission] = true
		if ex, ok := permByName[mp.Permission]; ok {
			if !ex.IsActive {
				if err := s.permRepo.SetActive(ex.ID, true); err != nil {
					return nil, err
				}
				summary["permissions_activated"]++
			}
		} else {
			if err := s.permRepo.Create(&entity.ServicePermission{
				ServiceID:   svc.ID,
				Permission:  mp.Permission,
				Description: mp.Description,
				IsActive:    true,
				CreatedBy:   "self-sync",
			}); err != nil {
				return nil, err
			}
			summary["permissions_created"]++
		}
	}
	for name, ex := range permByName {
		if !declared[name] && ex.IsActive {
			if err := s.permRepo.SetActive(ex.ID, false); err != nil {
				return nil, err
			}
			summary["permissions_deactivated"]++
		}
	}

	// 2b. Roles (manifest): upsert by name + attach declared permissions (union, non-destruktif)
	if len(m.Roles) > 0 {
		permIDByCode := map[string]string{}
		for _, ex := range existingPerms {
			permIDByCode[ex.Permission] = ex.ID
		}
		existingRoles, _ := s.roleRepo.GetByServiceID(svc.ID)
		roleByName := map[string]*entity.Role{}
		for i := range existingRoles {
			roleByName[existingRoles[i].Name] = &existingRoles[i]
		}
		for _, mr := range m.Roles {
			role, ok := roleByName[mr.Name]
			if !ok {
				role = &entity.Role{
					Name: mr.Name, ServiceID: svc.ID, Description: mr.Description,
					CreatedBy: "self-sync",
				}
				if cErr := s.roleRepo.Create(role); cErr != nil {
					return nil, cErr
				}
				summary["roles_created"]++
			} else if role.Description != mr.Description && mr.Description != "" {
				role.Description = mr.Description
				_ = s.roleRepo.Update(role)
			}
			for _, pc := range mr.Permissions {
				if pid, ok2 := permIDByCode[pc]; ok2 {
					_ = s.roleRepo.AddPermission(role.ID, pid) // idempoten
				}
			}
		}
	}

	// 3. Menu: upsert by path (parent dulu); deactivate yang hilang
	flatPaths := map[string]bool{}
	type flatItem struct {
		item     ManifestMenuItem
		parentID string
	}
	var parentPtr func(id string) *string
	parentPtr = func(id string) *string {
		if id == "" {
			return nil
		}
		return &id
	}
	var walk func(items []ManifestMenuItem, parentID string) error
	walk = func(items []ManifestMenuItem, parentID string) error {
		for _, mi := range items {
			flatPaths[mi.Path] = true
			existing, err := s.menuRepo.GetByPath(svc.ID, mi.Path)
			if err != nil {
				newItem := &entity.MenuItem{
					ServiceID:           svc.ID,
					ParentID:            parentPtr(parentID),
					Label:               mi.Label,
					Path:                mi.Path,
					Icon:                mi.Icon,
					SortOrder:           mi.SortOrder,
					RequiredPermission:  mi.RequiredPermission,
					IsActive:            true,
				}
				if err := s.menuRepo.Create(newItem); err != nil {
					return err
				}
				summary["menu_created"]++
				if err := walk(mi.Children, newItem.ID); err != nil {
					return err
				}
			} else {
				existing.Label = mi.Label
				existing.Icon = mi.Icon
				existing.SortOrder = mi.SortOrder
				existing.RequiredPermission = mi.RequiredPermission
				existing.ParentID = parentPtr(parentID)
				existing.IsActive = true
				if err := s.menuRepo.Update(existing); err != nil {
					return err
				}
				summary["menu_updated"]++
				if err := walk(mi.Children, existing.ID); err != nil {
					return err
				}
			}
		}
		return nil
	}
	if err := walk(m.Menu, ""); err != nil {
		return nil, err
	}
	existingMenu, err := s.menuRepo.ListByServiceIncludeInactive(svc.ID)
	if err != nil {
		return nil, err
	}
	for _, it := range existingMenu {
		if !flatPaths[it.Path] && it.IsActive {
			it.IsActive = false
			if err := s.menuRepo.Update(&it); err != nil {
				return nil, err
			}
			summary["menu_deactivated"]++
		}
	}

	// 4. Struktur menu berubah → flush cache menu SEMUA user
	s.flushMenuCache()

	s.recordAudit(m.Service.Name, "self_sync", "", svc.ID, "service", svc.ID, fmt.Sprintf(`{"summary":%v}`, summary))
	return summary, nil
}

// flushMenuCache hapus semua key menu:* (menu global berubah — semua user terdampak)
func (s *PermissionService) flushMenuCache() {
	if s.cache == nil {
		return
	}
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	if err := s.cache.FlushPattern(ctx, "menu:"); err != nil {
		log.Printf("[cache] flush menu gagal: %v", err)
	}
}
