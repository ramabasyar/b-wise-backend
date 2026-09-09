package mapper

import (
	"github.com/rama/b-wise/permission-service/internal/domain/dto/request"
	"github.com/rama/b-wise/permission-service/internal/domain/dto/response"
	"github.com/rama/b-wise/permission-service/internal/domain/entity"
)

// ==================== TO ENTITY (Request → Entity) ====================

// ToServiceEntity converts CreateService request to entity
func ToServiceEntity(r *request.CreateService, createdBy string) *entity.Service {
	return &entity.Service{
		Name:        r.Name,
		Description: r.Description,
		BaseURL:     r.BaseURL,
		IsActive:    true,
		CreatedBy:   createdBy,
	}
}

// ==================== TO RESPONSE (Entity → Response) ====================

// ToServiceResponse converts entity to response DTO
func ToServiceResponse(e *entity.Service) response.Service {
	return response.Service{
		ID:          e.ID,
		Name:        e.Name,
		Description: e.Description,
		BaseURL:     e.BaseURL,
		IsActive:    e.IsActive,
		CreatedAt:   e.CreatedAt,
		UpdatedAt:   e.UpdatedAt,
	}
}

// ToServiceList converts slice of entities to response DTOs
func ToServiceList(services []entity.Service) []response.Service {
	result := make([]response.Service, len(services))
	for i := range services {
		result[i] = ToServiceResponse(&services[i])
	}
	return result
}

// ToPermissionResponse converts entity to response DTO
func ToPermissionResponse(e *entity.ServicePermission) response.Permission {
	return response.Permission{
		ID:          e.ID,
		ServiceID:   e.ServiceID,
		Permission:  e.Permission,
		Description: e.Description,
		CreatedAt:   e.CreatedAt,
	}
}

// ToPermissionList converts slice of entities to response DTOs
func ToPermissionList(perms []entity.ServicePermission) []response.Permission {
	result := make([]response.Permission, len(perms))
	for i := range perms {
		result[i] = ToPermissionResponse(&perms[i])
	}
	return result
}

// ToAccessResponse converts entity to response DTO
func ToAccessResponse(e *entity.ServiceAccess) response.Access {
	return response.Access{
		ID:        e.ID,
		UserID:    e.UserID,
		ServiceID: e.ServiceID,
		GrantedBy: e.GrantedBy,
		IsActive:  e.IsActive,
		GrantedAt: e.GrantedAt,
	}
}

// ToAccessList converts slice of entities to response DTOs
func ToAccessList(accesses []entity.ServiceAccess) []response.Access {
	result := make([]response.Access, len(accesses))
	for i := range accesses {
		result[i] = ToAccessResponse(&accesses[i])
	}
	return result
}

// ToUserPermissionResponse converts entity to response DTO
func ToUserPermissionResponse(e *entity.UserPermission) response.UserPermission {
	return response.UserPermission{
		ID:         e.ID,
		UserID:     e.UserID,
		Permission: e.Permission,
		ServiceID:  e.ServiceID,
		GrantedBy:  e.GrantedBy,
		IsActive:   e.IsActive,
		GrantedAt:  e.GrantedAt,
	}
}

// ToUserPermissionList converts slice of entities to response DTOs
func ToUserPermissionList(perms []entity.UserPermission) []response.UserPermission {
	result := make([]response.UserPermission, len(perms))
	for i := range perms {
		result[i] = ToUserPermissionResponse(&perms[i])
	}
	return result
}

// ==================== APPLY UPDATE ====================

// ApplyServiceUpdate applies update request to entity
func ApplyServiceUpdate(r *request.UpdateService, e *entity.Service, updatedBy string) {
	if r.Name != nil {
		e.Name = *r.Name
	}
	if r.Description != nil {
		e.Description = *r.Description
	}
	if r.BaseURL != nil {
		e.BaseURL = *r.BaseURL
	}
	e.UpdatedBy = updatedBy
}
