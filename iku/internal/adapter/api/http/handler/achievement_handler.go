package handler

import (
	"fmt"
	"io"
	"net/http"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/rama/b-wise/iku/internal/adapter/api/http/middleware"
	entity "github.com/rama/b-wise/iku/internal/domain/entity"
	service "github.com/rama/b-wise/iku/internal/domain/service"
	"github.com/rama/b-wise/iku/internal/service/hcclient"
	"github.com/rama/b-wise/iku/internal/service/storage"
)

// AchievementHandler — F2: capaian CRUD + workflow + evidence.
type AchievementHandler struct {
	svc     *service.AchievementService
	storage storage.Storage
	hc      UnitResolver // F8: unit-scope operator (nullable)
	perm    PermResolver // F8: cek role operator (nullable)
}

// UnitResolver — diimplement hcclient.Client; interface agar handler tak bergantung paket konkret.
type UnitResolver interface {
	EmployeeUnit(userID string) (*hcclient.EmployeeInfo, error)
}

// PermResolver — diimplement permclient.Client.
type PermResolver interface {
	IsOperator(userID string) bool
}

func NewAchievementHandler(svc *service.AchievementService, st storage.Storage, opts ...func(*AchievementHandler)) *AchievementHandler {
	h := &AchievementHandler{svc: svc, storage: st}
	for _, o := range opts {
		o(h)
	}
	return h
}

// WithUnitScope — inject hc+perm (F8).
func WithUnitScope(hc UnitResolver, perm PermResolver) func(*AchievementHandler) {
	return func(h *AchievementHandler) { h.hc, h.perm = hc, perm }
}

// operatorUnit — bila user = operator unit → kembalikan unit-nya (branch; fallback department).
// Reviewer/pimpinan/admin → "" (lihat semua).
func (h *AchievementHandler) operatorUnit(c *gin.Context) (string, string) {
	if h.perm == nil || h.hc == nil {
		return "", ""
	}
	uid := actorOf(c)
	if uid == "" || !h.perm.IsOperator(uid) {
		return "", ""
	}
	emp, err := h.hc.EmployeeUnit(uid)
	if err != nil || emp == nil {
		return "", "" // tanpa data HC → jangan kunci (fail-open; UI tetap menyembunyikan aksi)
	}
	if emp.BranchID != nil && *emp.BranchID != "" {
		return *emp.BranchID, "branch"
	}
	if emp.DepartmentID != nil && *emp.DepartmentID != "" {
		return *emp.DepartmentID, "department"
	}
	return "", ""
}

// ==================== CAPAIAN ====================

// ListAchievements GET /api/achievements?indicator_id=&unit_id=&period_id=&status=
func (h *AchievementHandler) ListAchievements(c *gin.Context) {
	unitFilter := c.Query("unit_id")
	if ou, _ := h.operatorUnit(c); ou != "" {
		unitFilter = ou // F8: operator hanya melihat unit-nya
	}
	list, err := h.svc.List(service.AchievementFilter{
		IndicatorID: c.Query("indicator_id"),
		UnitID:      unitFilter,
		PeriodID:    c.Query("period_id"),
		Status:      c.Query("status"),
	})
	if err != nil {
		errStatus(c, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"success": true, "data": list, "meta": gin.H{"total": len(list)}})
}

// GetAchievement GET /api/achievements/:id — detail + workflow logs
func (h *AchievementHandler) GetAchievement(c *gin.Context) {
	a, logs, err := h.svc.Get(c.Param("id"))
	if err != nil {
		errStatus(c, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"success": true, "data": gin.H{"achievement": a, "workflow_logs": logs}})
}

type UpsertAchievementRequest struct {
	IndicatorID string             `json:"indicator_id" binding:"required"`
	UnitID      string             `json:"unit_id" binding:"required"`
	UnitType    string             `json:"unit_type"`
	PeriodID    string             `json:"period_id" binding:"required"`
	RawData     map[string]float64 `json:"raw_data" binding:"required"`
	Source      string             `json:"source"`
}

// UpsertAchievement POST /api/achievements — input raw (draft) + kalkulasi
func (h *AchievementHandler) UpsertAchievement(c *gin.Context) {
	var req UpsertAchievementRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"success": false, "error": gin.H{"message": err.Error()}})
		return
	}
	if ou, otype := h.operatorUnit(c); ou != "" {
		if req.UnitID != ou {
			c.JSON(http.StatusForbidden, gin.H{"success": false, "error": gin.H{"message": "operator hanya dapat menginput capaian unit kerjanya sendiri"}})
			return
		}
		req.UnitType = otype
	}
	a, err := h.svc.Upsert(service.UpsertAchievementInput(req), actorOf(c))
	if err != nil {
		errStatus(c, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"success": true, "data": a})
}

type TransitionRequest struct {
	To    string `json:"to" binding:"required"`
	Notes string `json:"notes"`
}

// TransitionAchievement POST /api/achievements/:id/transition
// to = submitted|reviewed|approved|published|rejected
func (h *AchievementHandler) TransitionAchievement(c *gin.Context) {
	var req TransitionRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"success": false, "error": gin.H{"message": err.Error()}})
		return
	}
	role := c.GetString("perm_scope") // best-effort; role dipakai utk log
	a, err := h.svc.Transition(c.Param("id"), req.To, actorOf(c), role, req.Notes, c.ClientIP())
	if err != nil {
		errStatus(c, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"success": true, "data": a})
}

// ==================== EVIDENCE ====================

// uploadLimiter — F7 security pass: token bucket per-user sederhana (10 upload/menit).
var uploadLimiter = struct {
	sync.Mutex
	hits map[string][]time.Time
}{hits: map[string][]time.Time{}}

func uploadAllowed(uid string) bool {
	uploadLimiter.Lock()
	defer uploadLimiter.Unlock()
	now := time.Now()
	kept := uploadLimiter.hits[uid][:0]
	for _, t := range uploadLimiter.hits[uid] {
		if now.Sub(t) < time.Minute {
			kept = append(kept, t)
		}
	}
	if len(kept) >= 10 {
		uploadLimiter.hits[uid] = kept
		return false
	}
	uploadLimiter.hits[uid] = append(kept, now)
	return true
}

// UploadEvidence POST /api/achievements/:id/evidence (multipart: file, notes)
func (h *AchievementHandler) UploadEvidence(c *gin.Context) {
	if !uploadAllowed(actorOf(c)) {
		c.JSON(http.StatusTooManyRequests, gin.H{"success": false, "error": gin.H{"message": "terlalu banyak upload (maks 10/menit)"}})
		return
	}
	fh, err := c.FormFile("file")
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"success": false, "error": gin.H{"message": "file wajib diunggah"}})
		return
	}
	if fh.Size > 10<<20 { // 10MB
		c.JSON(http.StatusBadRequest, gin.H{"success": false, "error": gin.H{"message": "maksimal 10MB"}})
		return
	}
	// whitelist ekstensi ringan
	ext := strings.ToLower(filepath.Ext(fh.Filename))
	allowed := map[string]bool{".pdf": true, ".png": true, ".jpg": true, ".jpeg": true, ".xlsx": true, ".xls": true, ".csv": true, ".doc": true, ".docx": true}
	if !allowed[ext] {
		c.JSON(http.StatusBadRequest, gin.H{"success": false, "error": gin.H{"message": "tipe file tidak diizinkan: " + ext}})
		return
	}

	src, err := fh.Open()
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"success": false, "error": gin.H{"message": err.Error()}})
		return
	}
	defer src.Close()
	content, err := io.ReadAll(src)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"success": false, "error": gin.H{"message": err.Error()}})
		return
	}

	achID := c.Param("id")
	key := fmt.Sprintf("%s/%d-%s", achID, fh.Size, filepath.Base(fh.Filename))
	obj := storage.FileObject{Content: content, FileName: fh.Filename, FileType: fh.Header.Get("Content-Type"), Size: fh.Size}
	if err := h.storage.Put(c.Request.Context(), key, obj); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"success": false, "error": gin.H{"message": "storage: " + err.Error()}})
		return
	}

	ev := &entity.EvidenceDocument{
		AchievementID: achID, FileName: fh.Filename, FileType: obj.FileType,
		FileSize: fh.Size, StoragePath: key, StorageDriver: string(h.storage.Driver()),
		Metadata: fmt.Sprintf(`{"notes":%q}`, c.PostForm("notes")), UploadedBy: actorOf(c),
	}
	if err := h.svc.AttachEvidence(ev); err != nil {
		errStatus(c, err)
		return
	}
	c.JSON(http.StatusCreated, gin.H{"success": true, "data": ev})
}

// DownloadEvidence GET /api/evidence/:id
func (h *AchievementHandler) DownloadEvidence(c *gin.Context) {
	var ev entity.EvidenceDocument
	if err := h.svc.DB().Where("id = ?", c.Param("id")).First(&ev).Error; err != nil {
		errStatus(c, ErrNotFoundWrapper)
		return
	}
	obj, err := h.storage.Get(c.Request.Context(), ev.StoragePath)
	if err != nil {
		errStatus(c, err)
		return
	}
	c.Header("Content-Disposition", fmt.Sprintf("attachment; filename=%q", ev.FileName))
	c.Data(http.StatusOK, orDefaultCT(ev.FileType), obj.Content)
}

func orDefaultCT(ct string) string {
	if ct == "" {
		return "application/octet-stream"
	}
	return ct
}

var ErrNotFoundWrapper = service.ErrNotFound

var _ = middleware.GetUserID
