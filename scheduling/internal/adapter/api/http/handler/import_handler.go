package handler

import (
	"fmt"
	"net/http"
	"strings"

	"github.com/gin-gonic/gin"
	entity "github.com/rama/b-wise/scheduling/internal/domain/entity"
	service "github.com/rama/b-wise/scheduling/internal/domain/service"
	"gorm.io/gorm"
)

// ==================== IMPORT CSV (F0) ====================
// Idempotent: upsert by kolom unik (code/label). File header case-insensitive.
// Format tiap resource — lihat README (kolom: code,name,...). Nilai kosong = tidak diubah.

type ImportHandler struct{ db *gorm.DB }

func NewImportHandler(db *gorm.DB) *ImportHandler { return &ImportHandler{db: db} }

func (h *ImportHandler) imp(c *gin.Context, resource string, fn func(rows []map[string]string) (*service.ImportResult, error)) {
	fh, err := c.FormFile("file")
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"success": false, "error": gin.H{"message": "field file wajib (multipart CSV)"}})
		return
	}
	f, err := fh.Open()
	if err != nil {
		errJSON(c, err)
		return
	}
	defer f.Close()
	rows, err := service.ReadCSV(f)
	if err != nil {
		errJSON(c, err)
		return
	}
	if len(rows) == 0 {
		c.JSON(http.StatusBadRequest, gin.H{"success": false, "error": gin.H{"message": "CSV kosong / tanpa baris data"}})
		return
	}
	res, err := fn(rows)
	if err != nil {
		errJSON(c, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"success": true, "data": res})
}

// Buildings POST /api/master/buildings/import — kolom: code,name,address,notes
func (h *ImportHandler) Buildings(c *gin.Context) {
	h.imp(c, "buildings", func(rows []map[string]string) (*service.ImportResult, error) {
		svc := service.NewCrud[entity.Building](h.db)
		res := &service.ImportResult{Resource: "buildings", Total: len(rows)}
		for i, r := range rows {
			var b entity.Building
			if err := service.ParseCSV([]map[string]string{r}, &b); err != nil {
				res.Errors = append(res.Errors, fmt.Sprintf("baris %d: %v", i+2, err))
				continue
			}
			if b.Code == "" || b.Name == "" {
				res.Errors = append(res.Errors, fmt.Sprintf("baris %d: code & name wajib", i+2))
				continue
			}
			b.Source = "import"
			created, err := svc.UpsertByKey(&b, "code", b.Code, func(ex *entity.Building) {
				ex.Name = b.Name
				ex.Address = b.Address
				ex.Notes = b.Notes
				ex.IsActive = true
			})
			if err != nil {
				res.Errors = append(res.Errors, fmt.Sprintf("baris %d: %v", i+2, err))
				continue
			}
			if created {
				res.Created++
			} else {
				res.Updated++
			}
		}
		return res, nil
	})
}

// Rooms POST /api/master/rooms/import — kolom: code,building_code,name,type,capacity,floor,faculty_id,notes
func (h *ImportHandler) Rooms(c *gin.Context) {
	h.imp(c, "rooms", func(rows []map[string]string) (*service.ImportResult, error) {
		svc := service.NewCrud[entity.Room](h.db)
		res := &service.ImportResult{Resource: "rooms", Total: len(rows)}
		for i, r := range rows {
			var m entity.Room
			if err := service.ParseCSV([]map[string]string{r}, &m); err != nil {
				res.Errors = append(res.Errors, fmt.Sprintf("baris %d: %v", i+2, err))
				continue
			}
			if m.Code == "" {
				res.Errors = append(res.Errors, fmt.Sprintf("baris %d: code wajib", i+2))
				continue
			}
			// resolve building_code → id
			if bc := r["building_code"]; bc != "" {
				var b entity.Building
				if err := h.db.Where("code = ?", bc).First(&b).Error; err != nil {
					res.Errors = append(res.Errors, fmt.Sprintf("baris %d: building_code '%s' tidak dikenal", i+2, bc))
					continue
				}
				m.BuildingID = b.ID
			}
			if m.Type == "" {
				m.Type = "theory"
			}
			m.Source = "import"
			created, err := svc.UpsertByKey(&m, "code", m.Code, func(ex *entity.Room) {
				ex.Name, ex.Type, ex.Capacity, ex.Floor = m.Name, m.Type, m.Capacity, m.Floor
				ex.BuildingID, ex.FacultyID, ex.Notes = m.BuildingID, m.FacultyID, m.Notes
				ex.IsActive = true
			})
			if err != nil {
				res.Errors = append(res.Errors, fmt.Sprintf("baris %d: %v", i+2, err))
				continue
			}
			if created {
				res.Created++
			} else {
				res.Updated++
			}
		}
		return res, nil
	})
}

// Courses POST /api/master/courses/import — kolom: code,name,sks,type,curriculum_year
func (h *ImportHandler) Courses(c *gin.Context) {
	h.imp(c, "courses", func(rows []map[string]string) (*service.ImportResult, error) {
		svc := service.NewCrud[entity.Course](h.db)
		res := &service.ImportResult{Resource: "courses", Total: len(rows)}
		for i, r := range rows {
			var m entity.Course
			if err := service.ParseCSV([]map[string]string{r}, &m); err != nil {
				res.Errors = append(res.Errors, fmt.Sprintf("baris %d: %v", i+2, err))
				continue
			}
			if m.Code == "" || m.Name == "" {
				res.Errors = append(res.Errors, fmt.Sprintf("baris %d: code & name wajib", i+2))
				continue
			}
			if m.Sks == 0 {
				m.Sks = 2
			}
			m.Source = "import"
			created, err := svc.UpsertByKey(&m, "code", m.Code, func(ex *entity.Course) {
				ex.Name, ex.Sks, ex.Type, ex.CurriculumYear = m.Name, m.Sks, m.Type, m.CurriculumYear
			})
			if err != nil {
				res.Errors = append(res.Errors, fmt.Sprintf("baris %d: %v", i+2, err))
				continue
			}
			if created {
				res.Created++
			} else {
				res.Updated++
			}
		}
		return res, nil
	})
}

// Lecturers POST /api/master/lecturers/import — kolom: code,name,email,phone,rank,is_external,max_load_sks
func (h *ImportHandler) Lecturers(c *gin.Context) {
	h.imp(c, "lecturers", func(rows []map[string]string) (*service.ImportResult, error) {
		svc := service.NewCrud[entity.Lecturer](h.db)
		res := &service.ImportResult{Resource: "lecturers", Total: len(rows)}
		for i, r := range rows {
			var m entity.Lecturer
			if err := service.ParseCSV([]map[string]string{r}, &m); err != nil {
				res.Errors = append(res.Errors, fmt.Sprintf("baris %d: %v", i+2, err))
				continue
			}
			if m.Code == "" || m.Name == "" {
				res.Errors = append(res.Errors, fmt.Sprintf("baris %d: code & name wajib", i+2))
				continue
			}
			m.Source = "import"
			created, err := svc.UpsertByKey(&m, "code", m.Code, func(ex *entity.Lecturer) {
				ex.Name, ex.Email, ex.Phone, ex.Rank = m.Name, m.Email, m.Phone, m.Rank
				ex.IsExternal, ex.MaxLoadSks = m.IsExternal, m.MaxLoadSks
				if ex.MaxLoadSks == 0 {
					ex.MaxLoadSks = 12
				}
			})
			if err != nil {
				res.Errors = append(res.Errors, fmt.Sprintf("baris %d: %v", i+2, err))
				continue
			}
			if created {
				res.Created++
			} else {
				res.Updated++
			}
		}
		return res, nil
	})
}

// Groups POST /api/master/class-groups/import — kolom: code,name,program_name,faculty_id,cohort_year,size_est
func (h *ImportHandler) Groups(c *gin.Context) {
	h.imp(c, "class_groups", func(rows []map[string]string) (*service.ImportResult, error) {
		svc := service.NewCrud[entity.ClassGroup](h.db)
		res := &service.ImportResult{Resource: "class_groups", Total: len(rows)}
		for i, r := range rows {
			var m entity.ClassGroup
			if err := service.ParseCSV([]map[string]string{r}, &m); err != nil {
				res.Errors = append(res.Errors, fmt.Sprintf("baris %d: %v", i+2, err))
				continue
			}
			if m.Code == "" {
				res.Errors = append(res.Errors, fmt.Sprintf("baris %d: code wajib", i+2))
				continue
			}
			m.Source = "import"
			created, err := svc.UpsertByKey(&m, "code", m.Code, func(ex *entity.ClassGroup) {
				ex.Name, ex.ProgramID, ex.ProgramName = m.Name, m.ProgramID, m.ProgramName
				ex.FacultyID, ex.CohortYear, ex.SizeEst = m.FacultyID, m.CohortYear, m.SizeEst
			})
			if err != nil {
				res.Errors = append(res.Errors, fmt.Sprintf("baris %d: %v", i+2, err))
				continue
			}
			if created {
				res.Created++
			} else {
				res.Updated++
			}
		}
		return res, nil
	})
}

var _ = strings.TrimSpace
