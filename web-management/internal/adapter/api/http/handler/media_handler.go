package handler

import (
	"net/http"

	"github.com/gin-gonic/gin"

	"github.com/rama/b-wise/web-management/internal/domain/service"
)

// MediaHandler — endpoint media library (guard permission media.*).
type MediaHandler struct {
	svc *service.MediaService
}

func NewMediaHandler(svc *service.MediaService) *MediaHandler {
	return &MediaHandler{svc: svc}
}

// Upload — multipart: file + alt (opsional).
func (h *MediaHandler) Upload(c *gin.Context) {
	fh, err := c.FormFile("file")
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"success": false, "error": gin.H{"message": "field 'file' wajib (multipart)"}})
		return
	}
	f, err := fh.Open()
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"success": false, "error": gin.H{"message": err.Error()}})
		return
	}
	defer f.Close()

	asset, err := h.svc.Upload(c.Request.Context(), f, fh.Filename, fh.Header.Get("Content-Type"), fh.Size, c.PostForm("alt"), actorOf(c))
	if err != nil {
		errJSON(c, err)
		return
	}
	c.JSON(http.StatusCreated, gin.H{"success": true, "data": asset})
}

func (h *MediaHandler) List(c *gin.Context) {
	page, per := pageParams(c)
	rows, total, err := h.svc.List(c.Query("q"), page, per)
	if err != nil {
		errJSON(c, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"success": true, "data": gin.H{"items": rows, "total": total, "page": page, "per_page": per}})
}

func (h *MediaHandler) Get(c *gin.Context) {
	m, err := h.svc.Get(c.Param("id"))
	if err != nil {
		errJSON(c, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"success": true, "data": m})
}

func (h *MediaHandler) Delete(c *gin.Context) {
	if err := h.svc.Delete(c.Request.Context(), c.Param("id")); err != nil {
		errJSON(c, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"success": true})
}

type mediaMetaReq struct {
	Alt     string `json:"alt"`
	Caption string `json:"caption"`
	Credit  string `json:"credit"`
}

func (h *MediaHandler) UpdateMeta(c *gin.Context) {
	var req mediaMetaReq
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"success": false, "error": gin.H{"message": err.Error()}})
		return
	}
	m, err := h.svc.UpdateMeta(c.Param("id"), req.Alt, req.Caption, req.Credit)
	if err != nil {
		errJSON(c, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"success": true, "data": m})
}
