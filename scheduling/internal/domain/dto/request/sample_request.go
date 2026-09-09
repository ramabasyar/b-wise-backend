package request

// SampleCreate is a SAMPLE request DTO — replace with your own.
type SampleCreate struct {
	Name        string `json:"name" binding:"required"`
	Description string `json:"description"`
}

// SampleUpdate is a SAMPLE request DTO — all fields optional (pointer).
type SampleUpdate struct {
	Name        *string `json:"name"`
	Description *string `json:"description"`
	Status      *string `json:"status"`
}

// Pagination is a shared pagination request.
type Pagination struct {
	Page     int    `form:"page" json:"page"`
	PageSize int    `form:"page_size" json:"page_size"`
	Search   string `form:"search" json:"search"`
}

// Defaults returns pagination with sensible defaults applied.
func (p *Pagination) Defaults() (offset, limit int) {
	if p.PageSize <= 0 || p.PageSize > 100 {
		p.PageSize = 20
	}
	if p.Page <= 0 {
		p.Page = 1
	}
	return (p.Page - 1) * p.PageSize, p.PageSize
}
