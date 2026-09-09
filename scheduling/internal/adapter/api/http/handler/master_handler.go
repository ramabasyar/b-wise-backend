package handler

import (
	"net/http"

	"github.com/gin-gonic/gin"
	entity "github.com/rama/b-wise/scheduling/internal/domain/entity"
	service "github.com/rama/b-wise/scheduling/internal/domain/service"
	"gorm.io/gorm"
)

// ==================== MASTER DATA HANDLER (F0) ====================
// CRUD generik per resource + import CSV (idempotent upsert by code).
// Semua route dilindungi permission middleware di router.

type MasterHandler struct {
	Buildings *service.CrudService[entity.Building]
	Rooms      *service.CrudService[entity.Room]
	RoomTypes  *service.CrudService[entity.RoomType]
	FacilityTypes *service.CrudService[entity.FacilityType]
	CourseTypes *service.CrudService[entity.CourseType]
	Terms     *service.CrudService[entity.Term]
	Courses   *service.CrudService[entity.Course]
	Lecturers *service.CrudService[entity.Lecturer]
	Groups    *service.CrudService[entity.ClassGroup]
	Slots     *service.CrudService[entity.TimeSlot]
	Offerings *service.CrudService[entity.Offering]
	Avails    *service.CrudService[entity.LecturerAvailability]
}

func errJSON(c *gin.Context, err error) {
	status := http.StatusBadRequest
	if err == service.ErrNotFound {
		status = http.StatusNotFound
	}
	c.JSON(status, gin.H{"success": false, "error": gin.H{"message": err.Error()}})
}

// ---- helper generik ----

func listHandler[T any](svc *service.CrudService[T], preloads ...string) gin.HandlerFunc {
	return func(c *gin.Context) {
		list, err := svc.List(preloads...)
		if err != nil {
			errJSON(c, err)
			return
		}
		c.JSON(http.StatusOK, gin.H{"success": true, "data": list})
	}
}

func getHandler[T any](svc *service.CrudService[T], preloads ...string) gin.HandlerFunc {
	return func(c *gin.Context) {
		m, err := svc.Get(c.Param("id"), preloads...)
		if err != nil {
			errJSON(c, err)
			return
		}
		c.JSON(http.StatusOK, gin.H{"success": true, "data": m})
	}
}

func deleteHandler[T any](svc *service.CrudService[T]) gin.HandlerFunc {
	return func(c *gin.Context) {
		if err := svc.Delete(c.Param("id")); err != nil {
			errJSON(c, err)
			return
		}
		c.JSON(http.StatusOK, gin.H{"success": true})
	}
}

// updateHandler — patch map dari body JSON (field null dihapus).
func updateHandler[T any](svc *service.CrudService[T]) gin.HandlerFunc {
	return func(c *gin.Context) {
		var body map[string]any
		if err := c.ShouldBindJSON(&body); err != nil {
			errJSON(c, err)
			return
		}
		delete(body, "id")
		delete(body, "created_at")
		delete(body, "updated_at")
		m, err := svc.Update(c.Param("id"), body)
		if err != nil {
			errJSON(c, err)
			return
		}
		c.JSON(http.StatusOK, gin.H{"success": true, "data": m})
	}
}

func createHandler[T any](svc *service.CrudService[T]) gin.HandlerFunc {
	return func(c *gin.Context) {
		var m T
		if err := c.ShouldBindJSON(&m); err != nil {
			errJSON(c, err)
			return
		}
		if err := svcCreate(svc, &m); err != nil {
			errJSON(c, err)
			return
		}
		c.JSON(http.StatusCreated, gin.H{"success": true, "data": m})
	}
}

func svcCreate[T any](svc *service.CrudService[T], m *T) error {
	return svc.CreateRow(m)
}

// ---- exposure ----

func (h *MasterHandler) BuildingList() gin.HandlerFunc { return listHandler(h.Buildings) }
func (h *MasterHandler) BuildingGet() gin.HandlerFunc  { return getHandler(h.Buildings) }
func (h *MasterHandler) BuildingCreate() gin.HandlerFunc {
	return createHandler(h.Buildings)
}
func (h *MasterHandler) BuildingUpdate() gin.HandlerFunc { return updateHandler(h.Buildings) }
func (h *MasterHandler) BuildingDelete() gin.HandlerFunc { return deleteHandler(h.Buildings) }

func (h *MasterHandler) RoomList() gin.HandlerFunc { return listHandler(h.Rooms, "Building") }
func (h *MasterHandler) RoomGet() gin.HandlerFunc  { return getHandler(h.Rooms, "Building") }
func (h *MasterHandler) RoomCreate() gin.HandlerFunc {
	return createHandler(h.Rooms)
}
func (h *MasterHandler) RoomUpdate() gin.HandlerFunc { return updateHandler(h.Rooms) }
func (h *MasterHandler) RoomDelete() gin.HandlerFunc { return deleteHandler(h.Rooms) }

func (h *MasterHandler) RoomTypeList() gin.HandlerFunc { return listHandler(h.RoomTypes) }
func (h *MasterHandler) RoomTypeGet() gin.HandlerFunc  { return getHandler(h.RoomTypes) }
func (h *MasterHandler) RoomTypeCreate() gin.HandlerFunc {
	return createHandler(h.RoomTypes)
}
func (h *MasterHandler) RoomTypeUpdate() gin.HandlerFunc { return updateHandler(h.RoomTypes) }
func (h *MasterHandler) RoomTypeDelete() gin.HandlerFunc { return deleteHandler(h.RoomTypes) }

func (h *MasterHandler) FacilityTypeList() gin.HandlerFunc   { return listHandler(h.FacilityTypes) }
func (h *MasterHandler) FacilityTypeGet() gin.HandlerFunc    { return getHandler(h.FacilityTypes) }
func (h *MasterHandler) FacilityTypeCreate() gin.HandlerFunc { return createHandler(h.FacilityTypes) }
func (h *MasterHandler) FacilityTypeUpdate() gin.HandlerFunc { return updateHandler(h.FacilityTypes) }
func (h *MasterHandler) FacilityTypeDelete() gin.HandlerFunc { return deleteHandler(h.FacilityTypes) }

func (h *MasterHandler) CourseTypeList() gin.HandlerFunc   { return listHandler(h.CourseTypes) }
func (h *MasterHandler) CourseTypeGet() gin.HandlerFunc    { return getHandler(h.CourseTypes) }
func (h *MasterHandler) CourseTypeCreate() gin.HandlerFunc { return createHandler(h.CourseTypes) }
func (h *MasterHandler) CourseTypeUpdate() gin.HandlerFunc { return updateHandler(h.CourseTypes) }
func (h *MasterHandler) CourseTypeDelete() gin.HandlerFunc { return deleteHandler(h.CourseTypes) }

func (h *MasterHandler) TermList() gin.HandlerFunc   { return listHandler(h.Terms) }
func (h *MasterHandler) TermGet() gin.HandlerFunc    { return getHandler(h.Terms) }
func (h *MasterHandler) TermCreate() gin.HandlerFunc { return createHandler(h.Terms) }
func (h *MasterHandler) TermUpdate() gin.HandlerFunc { return updateHandler(h.Terms) }
func (h *MasterHandler) TermDelete() gin.HandlerFunc { return deleteHandler(h.Terms) }

func (h *MasterHandler) CourseList() gin.HandlerFunc   { return listHandler(h.Courses) }
func (h *MasterHandler) CourseGet() gin.HandlerFunc    { return getHandler(h.Courses) }
func (h *MasterHandler) CourseCreate() gin.HandlerFunc { return createHandler(h.Courses) }
func (h *MasterHandler) CourseUpdate() gin.HandlerFunc { return updateHandler(h.Courses) }
func (h *MasterHandler) CourseDelete() gin.HandlerFunc { return deleteHandler(h.Courses) }

func (h *MasterHandler) LecturerList() gin.HandlerFunc { return listHandler(h.Lecturers) }
func (h *MasterHandler) LecturerGet() gin.HandlerFunc  { return getHandler(h.Lecturers) }
func (h *MasterHandler) LecturerCreate() gin.HandlerFunc {
	return createHandler(h.Lecturers)
}
func (h *MasterHandler) LecturerUpdate() gin.HandlerFunc { return updateHandler(h.Lecturers) }
func (h *MasterHandler) LecturerDelete() gin.HandlerFunc { return deleteHandler(h.Lecturers) }

func (h *MasterHandler) GroupList() gin.HandlerFunc { return listHandler(h.Groups) }
func (h *MasterHandler) GroupGet() gin.HandlerFunc  { return getHandler(h.Groups) }
func (h *MasterHandler) GroupCreate() gin.HandlerFunc {
	return createHandler(h.Groups)
}
func (h *MasterHandler) GroupUpdate() gin.HandlerFunc { return updateHandler(h.Groups) }
func (h *MasterHandler) GroupDelete() gin.HandlerFunc { return deleteHandler(h.Groups) }

func (h *MasterHandler) SlotList() gin.HandlerFunc   { return listHandler(h.Slots) }
func (h *MasterHandler) SlotGet() gin.HandlerFunc    { return getHandler(h.Slots) }
func (h *MasterHandler) SlotCreate() gin.HandlerFunc { return createHandler(h.Slots) }
func (h *MasterHandler) SlotUpdate() gin.HandlerFunc { return updateHandler(h.Slots) }
func (h *MasterHandler) SlotDelete() gin.HandlerFunc { return deleteHandler(h.Slots) }

func (h *MasterHandler) OfferingList() gin.HandlerFunc {
	return listHandler(h.Offerings, "Term", "Course", "Lecturer", "ClassGroup")
}
func (h *MasterHandler) OfferingGet() gin.HandlerFunc {
	return getHandler(h.Offerings, "Term", "Course", "Lecturer", "ClassGroup")
}
func (h *MasterHandler) OfferingCreate() gin.HandlerFunc { return createHandler(h.Offerings) }
func (h *MasterHandler) OfferingUpdate() gin.HandlerFunc { return updateHandler(h.Offerings) }
func (h *MasterHandler) OfferingDelete() gin.HandlerFunc { return deleteHandler(h.Offerings) }

func (h *MasterHandler) AvailList() gin.HandlerFunc   { return listHandler(h.Avails) }
func (h *MasterHandler) AvailGet() gin.HandlerFunc    { return getHandler(h.Avails) }
func (h *MasterHandler) AvailCreate() gin.HandlerFunc { return createHandler(h.Avails) }
func (h *MasterHandler) AvailUpdate() gin.HandlerFunc { return updateHandler(h.Avails) }
func (h *MasterHandler) AvailDelete() gin.HandlerFunc { return deleteHandler(h.Avails) }

// NewMasterHandlerDB — wiring semua CrudService dari koneksi DB.
func NewMasterHandlerDB(db *gorm.DB) *MasterHandler {
	return &MasterHandler{
		Buildings: service.NewCrud[entity.Building](db),
		Rooms:     service.NewCrud[entity.Room](db),
		RoomTypes: service.NewCrud[entity.RoomType](db),
		FacilityTypes: service.NewCrud[entity.FacilityType](db),
		CourseTypes: service.NewCrud[entity.CourseType](db),
		Terms:     service.NewCrud[entity.Term](db),
		Courses:   service.NewCrud[entity.Course](db),
		Lecturers: service.NewCrud[entity.Lecturer](db),
		Groups:    service.NewCrud[entity.ClassGroup](db),
		Slots:     service.NewCrud[entity.TimeSlot](db),
		Offerings: service.NewCrud[entity.Offering](db),
		Avails:    service.NewCrud[entity.LecturerAvailability](db),
	}
}

// SeedTimeSlots POST /api/master/time-slots/seed-default — template Senin-Jumat 5 slot (idempotent by label).
func (h *MasterHandler) SeedTimeSlots(c *gin.Context) {
	type row struct {
		Label      string
		Day, Order int
		S, E       string
	}
	rows := []row{}
	names := []string{"Senin", "Selasa", "Rabu", "Kamis", "Jumat"}
	for d := 1; d <= 5; d++ {
		rows = append(rows,
			row{names[d-1] + " 07:30-09:10", d, 1, "07:30", "09:10"},
			row{names[d-1] + " 09:20-11:00", d, 2, "09:20", "11:00"},
			row{names[d-1] + " 11:10-12:50", d, 3, "11:10", "12:50"},
			row{names[d-1] + " 13:30-15:10", d, 4, "13:30", "15:10"},
			row{names[d-1] + " 15:20-17:00", d, 5, "15:20", "17:00"},
		)
	}
	created := 0
	for _, r := range rows {
		var ex entity.TimeSlot
		if err := h.Slots.DB().Where("label = ?", r.Label).First(&ex).Error; err == nil {
			continue
		}
		slot := entity.TimeSlot{
			Label: r.Label, Day: r.Day, StartTime: r.S, EndTime: r.E, Order: r.Order, IsActive: true,
		}
		if err := h.Slots.CreateRow(&slot); err == nil {
			created++
		}
	}
	c.JSON(http.StatusOK, gin.H{"success": true, "data": gin.H{"created": created, "total_slots": len(rows)}})
}
