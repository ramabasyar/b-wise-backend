package handler

import (
	"errors"
	"net/http"
	"strconv"
	"time"

	"github.com/gin-gonic/gin"

	"github.com/rama/b-wise/web-management/internal/domain/service"
)

// ContentHandler — endpoint admin BWM (guard permission contents.* / media.*).
type ContentHandler struct {
	svc *service.ContentService
}

func NewContentHandler(svc *service.ContentService) *ContentHandler {
	return &ContentHandler{svc: svc}
}

// ---------- helpers ----------

func actorOf(c *gin.Context) string {
	a := c.GetString("user_id")
	if a == "" {
		a = "system"
	}
	return a
}

func errJSON(c *gin.Context, err error) {
	switch {
	case errors.Is(err, service.ErrNotFound):
		c.JSON(http.StatusNotFound, gin.H{"success": false, "error": gin.H{"message": err.Error()}})
	case errors.Is(err, service.ErrInvalid):
		c.JSON(http.StatusBadRequest, gin.H{"success": false, "error": gin.H{"message": err.Error()}})
	default:
		c.JSON(http.StatusInternalServerError, gin.H{"success": false, "error": gin.H{"message": err.Error()}})
	}
}

func pageParams(c *gin.Context) (int, int) {
	p, _ := strconv.Atoi(c.Query("page"))
	if p < 1 {
		p = 1
	}
	pp, _ := strconv.Atoi(c.Query("per_page"))
	if pp < 1 {
		pp = 20
	}
	return p, pp
}

// ==================== Posts ====================

func (h *ContentHandler) ListPosts(c *gin.Context) {
	page, per := pageParams(c)
	rows, total, err := h.svc.ListPosts(c.Query("status"), c.Query("type"), c.Query("q"), page, per)
	if err != nil {
		errJSON(c, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"success": true, "data": gin.H{"items": rows, "total": total, "page": page, "per_page": per}})
}

func (h *ContentHandler) GetPost(c *gin.Context) {
	p, err := h.svc.GetPost(c.Param("id"))
	if err != nil {
		errJSON(c, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"success": true, "data": p})
}

func (h *ContentHandler) CreatePost(c *gin.Context) {
	var in service.PostInput
	if err := c.ShouldBindJSON(&in); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"success": false, "error": gin.H{"message": err.Error()}})
		return
	}
	p, err := h.svc.CreatePost(in, actorOf(c))
	if err != nil {
		errJSON(c, err)
		return
	}
	c.JSON(http.StatusCreated, gin.H{"success": true, "data": p})
}

func (h *ContentHandler) UpdatePost(c *gin.Context) {
	var in service.PostInput
	if err := c.ShouldBindJSON(&in); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"success": false, "error": gin.H{"message": err.Error()}})
		return
	}
	p, err := h.svc.UpdatePost(c.Param("id"), in, actorOf(c))
	if err != nil {
		errJSON(c, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"success": true, "data": p})
}

func (h *ContentHandler) DeletePost(c *gin.Context) {
	if err := h.svc.DeletePost(c.Param("id")); err != nil {
		errJSON(c, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"success": true})
}

type publishReq struct {
	PublishAt *time.Time `json:"publish_at"`
}

func (h *ContentHandler) PublishPost(c *gin.Context) {
	var req publishReq
	_ = c.ShouldBindJSON(&req)
	p, err := h.svc.PublishPost(c.Param("id"), actorOf(c), req.PublishAt)
	if err != nil {
		errJSON(c, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"success": true, "data": p})
}

func (h *ContentHandler) ArchivePost(c *gin.Context) {
	p, err := h.svc.ArchivePost(c.Param("id"), actorOf(c))
	if err != nil {
		errJSON(c, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"success": true, "data": p})
}

// ==================== Events ====================

func (h *ContentHandler) ListEvents(c *gin.Context) {
	page, per := pageParams(c)
	rows, total, err := h.svc.ListEvents(c.Query("status"), c.Query("q"), page, per)
	if err != nil {
		errJSON(c, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"success": true, "data": gin.H{"items": rows, "total": total, "page": page, "per_page": per}})
}

func (h *ContentHandler) GetEvent(c *gin.Context) {
	e, err := h.svc.GetEvent(c.Param("id"))
	if err != nil {
		errJSON(c, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"success": true, "data": e})
}

func (h *ContentHandler) CreateEvent(c *gin.Context) {
	var in service.EventInput
	if err := c.ShouldBindJSON(&in); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"success": false, "error": gin.H{"message": err.Error()}})
		return
	}
	e, err := h.svc.CreateEvent(in, actorOf(c))
	if err != nil {
		errJSON(c, err)
		return
	}
	c.JSON(http.StatusCreated, gin.H{"success": true, "data": e})
}

func (h *ContentHandler) UpdateEvent(c *gin.Context) {
	var in service.EventInput
	if err := c.ShouldBindJSON(&in); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"success": false, "error": gin.H{"message": err.Error()}})
		return
	}
	e, err := h.svc.UpdateEvent(c.Param("id"), in, actorOf(c))
	if err != nil {
		errJSON(c, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"success": true, "data": e})
}

func (h *ContentHandler) DeleteEvent(c *gin.Context) {
	if err := h.svc.DeleteEvent(c.Param("id")); err != nil {
		errJSON(c, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"success": true})
}

func (h *ContentHandler) PublishEvent(c *gin.Context) {
	var req publishReq
	_ = c.ShouldBindJSON(&req)
	e, err := h.svc.PublishEvent(c.Param("id"), actorOf(c), req.PublishAt)
	if err != nil {
		errJSON(c, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"success": true, "data": e})
}

func (h *ContentHandler) ArchiveEvent(c *gin.Context) {
	e, err := h.svc.ArchiveEvent(c.Param("id"), actorOf(c))
	if err != nil {
		errJSON(c, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"success": true, "data": e})
}

// ==================== Banners ====================

func (h *ContentHandler) ListBanners(c *gin.Context) {
	rows, err := h.svc.ListBanners()
	if err != nil {
		errJSON(c, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"success": true, "data": gin.H{"items": rows, "total": len(rows)}})
}

func (h *ContentHandler) CreateBanner(c *gin.Context) {
	var in service.BannerInput
	if err := c.ShouldBindJSON(&in); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"success": false, "error": gin.H{"message": err.Error()}})
		return
	}
	b, err := h.svc.CreateBanner(in, actorOf(c))
	if err != nil {
		errJSON(c, err)
		return
	}
	c.JSON(http.StatusCreated, gin.H{"success": true, "data": b})
}

func (h *ContentHandler) UpdateBanner(c *gin.Context) {
	var in service.BannerInput
	if err := c.ShouldBindJSON(&in); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"success": false, "error": gin.H{"message": err.Error()}})
		return
	}
	b, err := h.svc.UpdateBanner(c.Param("id"), in, actorOf(c))
	if err != nil {
		errJSON(c, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"success": true, "data": b})
}

func (h *ContentHandler) DeleteBanner(c *gin.Context) {
	if err := h.svc.DeleteBanner(c.Param("id")); err != nil {
		errJSON(c, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"success": true})
}

// ==================== Pages ====================

func (h *ContentHandler) ListPages(c *gin.Context) {
	page, per := pageParams(c)
	rows, total, err := h.svc.ListPages(c.Query("status"), page, per)
	if err != nil {
		errJSON(c, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"success": true, "data": gin.H{"items": rows, "total": total, "page": page, "per_page": per}})
}

func (h *ContentHandler) GetPage(c *gin.Context) {
	p, err := h.svc.GetPage(c.Param("id"))
	if err != nil {
		errJSON(c, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"success": true, "data": p})
}

func (h *ContentHandler) CreatePage(c *gin.Context) {
	var in service.PageInput
	if err := c.ShouldBindJSON(&in); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"success": false, "error": gin.H{"message": err.Error()}})
		return
	}
	p, err := h.svc.UpsertPage(in, actorOf(c), "")
	if err != nil {
		errJSON(c, err)
		return
	}
	c.JSON(http.StatusCreated, gin.H{"success": true, "data": p})
}

func (h *ContentHandler) UpdatePage(c *gin.Context) {
	var in service.PageInput
	if err := c.ShouldBindJSON(&in); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"success": false, "error": gin.H{"message": err.Error()}})
		return
	}
	p, err := h.svc.UpsertPage(in, actorOf(c), c.Param("id"))
	if err != nil {
		errJSON(c, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"success": true, "data": p})
}

func (h *ContentHandler) DeletePage(c *gin.Context) {
	if err := h.svc.DeletePage(c.Param("id")); err != nil {
		errJSON(c, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"success": true})
}

func (h *ContentHandler) PublishPage(c *gin.Context) {
	var req publishReq
	_ = c.ShouldBindJSON(&req)
	p, err := h.svc.PublishPage(c.Param("id"), actorOf(c), req.PublishAt)
	if err != nil {
		errJSON(c, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"success": true, "data": p})
}

// ==================== Documents ====================

func (h *ContentHandler) ListDocuments(c *gin.Context) {
	rows, err := h.svc.ListDocuments()
	if err != nil {
		errJSON(c, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"success": true, "data": gin.H{"items": rows, "total": len(rows)}})
}

func (h *ContentHandler) CreateDocument(c *gin.Context) {
	var in service.DocumentInput
	if err := c.ShouldBindJSON(&in); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"success": false, "error": gin.H{"message": err.Error()}})
		return
	}
	d, err := h.svc.UpsertDocument(in, actorOf(c), "")
	if err != nil {
		errJSON(c, err)
		return
	}
	c.JSON(http.StatusCreated, gin.H{"success": true, "data": d})
}

func (h *ContentHandler) UpdateDocument(c *gin.Context) {
	var in service.DocumentInput
	if err := c.ShouldBindJSON(&in); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"success": false, "error": gin.H{"message": err.Error()}})
		return
	}
	d, err := h.svc.UpsertDocument(in, actorOf(c), c.Param("id"))
	if err != nil {
		errJSON(c, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"success": true, "data": d})
}

func (h *ContentHandler) DeleteDocument(c *gin.Context) {
	if err := h.svc.DeleteDocument(c.Param("id")); err != nil {
		errJSON(c, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"success": true})
}

// ==================== F2: VERSIONING + ROLLBACK ====================

func (h *ContentHandler) PostVersions(c *gin.Context) {
	rows, err := h.svc.ListPostVersions(c.Param("id"))
	if err != nil {
		errJSON(c, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"success": true, "data": rows})
}

func (h *ContentHandler) PostVersionDetail(c *gin.Context) {
	v, err := strconv.Atoi(c.Param("v"))
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"success": false, "error": gin.H{"message": "versi tidak valid"}})
		return
	}
	snap, err := h.svc.GetPostVersion(c.Param("id"), v)
	if err != nil {
		errJSON(c, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"success": true, "data": snap})
}

func (h *ContentHandler) PostRollback(c *gin.Context) {
	v, err := strconv.Atoi(c.Param("v"))
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"success": false, "error": gin.H{"message": "versi tidak valid"}})
		return
	}
	p, err := h.svc.RollbackPost(c.Param("id"), v, actorOf(c))
	if err != nil {
		errJSON(c, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"success": true, "data": p})
}

// ==================== F2: EDIT LOCKING ====================

func (h *ContentHandler) PostLockStatus(c *gin.Context) {
	st, err := h.svc.GetPostLock(c.Param("id"))
	if err != nil {
		errJSON(c, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"success": true, "data": st})
}

func (h *ContentHandler) PostLockAcquire(c *gin.Context) {
	var req struct {
		Force bool `json:"force"`
	}
	_ = c.ShouldBindJSON(&req)
	st, ok, err := h.svc.AcquirePostLock(c.Param("id"), actorOf(c), req.Force)
	if err != nil {
		errJSON(c, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"success": true, "data": gin.H{"lock": st, "acquired": ok}})
}

func (h *ContentHandler) PostLockRelease(c *gin.Context) {
	var req struct {
		Force bool `json:"force"`
	}
	_ = c.ShouldBindJSON(&req)
	if err := h.svc.ReleasePostLock(c.Param("id"), actorOf(c), req.Force); err != nil {
		errJSON(c, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"success": true})
}

// ==================== F2: TAKSONOMI ====================

func (h *ContentHandler) ListTerms(c *gin.Context) {
	rows, err := h.svc.Tax().ListTerms(c.Query("taxonomy"))
	if err != nil {
		errJSON(c, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"success": true, "data": rows})
}

func (h *ContentHandler) CreateTerm(c *gin.Context) {
	var in struct {
		Taxonomy string `json:"taxonomy"`
		Slug     string `json:"slug"`
		NameID   string `json:"name_id"`
		NameEN   string `json:"name_en"`
	}
	if err := c.ShouldBindJSON(&in); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"success": false, "error": gin.H{"message": err.Error()}})
		return
	}
	t, err := h.svc.Tax().CreateTerm(in.Taxonomy, in.Slug, in.NameID, in.NameEN)
	if err != nil {
		errJSON(c, err)
		return
	}
	h.svc.FlushPublicCache()
	c.JSON(http.StatusCreated, gin.H{"success": true, "data": t})
}

func (h *ContentHandler) UpdateTerm(c *gin.Context) {
	var in struct {
		NameID string `json:"name_id"`
		NameEN string `json:"name_en"`
	}
	if err := c.ShouldBindJSON(&in); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"success": false, "error": gin.H{"message": err.Error()}})
		return
	}
	t, err := h.svc.Tax().UpdateTerm(c.Param("id"), in.NameID, in.NameEN)
	if err != nil {
		errJSON(c, err)
		return
	}
	h.svc.FlushPublicCache()
	c.JSON(http.StatusOK, gin.H{"success": true, "data": t})
}

func (h *ContentHandler) DeleteTerm(c *gin.Context) {
	if err := h.svc.Tax().DeleteTerm(c.Param("id")); err != nil {
		errJSON(c, err)
		return
	}
	h.svc.FlushPublicCache()
	c.JSON(http.StatusOK, gin.H{"success": true})
}

// FindOrCreateTerm — untuk chips tag di editor (cari by slug, buat bila belum).
func (h *ContentHandler) FindOrCreateTerm(c *gin.Context) {
	var in struct {
		Taxonomy string `json:"taxonomy"`
		Name     string `json:"name"`
	}
	if err := c.ShouldBindJSON(&in); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"success": false, "error": gin.H{"message": err.Error()}})
		return
	}
	t, err := h.svc.Tax().FindOrCreateByName(in.Taxonomy, in.Name)
	if err != nil {
		errJSON(c, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"success": true, "data": t})
}

// SetPostTerms — ganti seluruh term milik post {term_ids: [...]}.
func (h *ContentHandler) SetPostTerms(c *gin.Context) {
	var in struct {
		TermIDs []string `json:"term_ids"`
	}
	if err := c.ShouldBindJSON(&in); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"success": false, "error": gin.H{"message": err.Error()}})
		return
	}
	if err := h.svc.Tax().SetPostTerms(c.Param("id"), in.TermIDs); err != nil {
		errJSON(c, err)
		return
	}
	h.svc.FlushPublicCache()
	c.JSON(http.StatusOK, gin.H{"success": true})
}
