package mapper

import (
	"github.com/rama/b-wise/scheduling/internal/domain/dto/request"
	"github.com/rama/b-wise/scheduling/internal/domain/dto/response"
	"github.com/rama/b-wise/scheduling/internal/domain/entity"
)

// ToSampleResponse maps entity → response DTO
func ToSampleResponse(e *entity.SampleEntity) *response.SampleResponse {
	return &response.SampleResponse{
		ID:          e.ID,
		Name:        e.Name,
		Description: e.Description,
		Status:      e.Status,
		CreatedAt:   e.CreatedAt,
		UpdatedAt:   e.UpdatedAt,
	}
}

// ToSampleEntity maps create request → entity
func ToSampleEntityFromCreate(req *request.SampleCreate) *entity.SampleEntity {
	return &entity.SampleEntity{
		Name:        req.Name,
		Description: req.Description,
		Status:      "active",
	}
}

// ApplySampleUpdate applies update request to existing entity (only non-nil fields)
func ApplySampleUpdate(e *entity.SampleEntity, req *request.SampleUpdate) {
	if req.Name != nil {
		e.Name = *req.Name
	}
	if req.Description != nil {
		e.Description = *req.Description
	}
	if req.Status != nil {
		e.Status = *req.Status
	}
}
