package handler

import (
	"encoding/json"
	"fmt"
	"net/http"

	"github.com/gin-gonic/gin"
	entity "github.com/rama/b-wise/iku/internal/domain/entity"
	service "github.com/rama/b-wise/iku/internal/domain/service"
	"github.com/rama/b-wise/iku/internal/service/importer"
)

// BrokerHandler — F5: connector registry + import terkelola (preview → apply).
type BrokerHandler struct {
	broker *service.BrokerService
	ach    *service.AchievementService
}

func NewBrokerHandler(b *service.BrokerService, a *service.AchievementService) *BrokerHandler {
	return &BrokerHandler{broker: b, ach: a}
}

// ListConnectors GET /api/connectors?type=&indicator_id=
func (h *BrokerHandler) ListConnectors(c *gin.Context) {
	list, err := h.broker.ListConnectors(service.ConnectorFilter{
		Type:        c.Query("type"),
		IndicatorID: c.Query("indicator_id"),
		ActiveOnly:  c.DefaultQuery("active_only", "true") == "true",
	})
	if err != nil {
		errStatus(c, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"success": true, "data": list})
}

type UpsertConnectorRequest struct {
	ID          string                 `json:"id"`
	Name        string                 `json:"name" binding:"required"`
	IndicatorID string                 `json:"indicator_id"`
	Type        string                 `json:"type" binding:"required"`
	SystemOwner string                 `json:"system_owner"`
	Contract    []entity.ContractField `json:"contract"`
	Notes       string                 `json:"notes"`
}

// UpsertConnector POST /api/connectors
func (h *BrokerHandler) UpsertConnector(c *gin.Context) {
	var req UpsertConnectorRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"success": false, "error": gin.H{"message": err.Error()}})
		return
	}
	conn, err := h.broker.UpsertConnector(service.UpsertConnectorInput(req), actorOf(c))
	if err != nil {
		errStatus(c, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"success": true, "data": conn})
}

// ConnectorTemplate GET /api/connectors/:id/template?multi_unit= — CSV template utk diisi
func (h *BrokerHandler) ConnectorTemplate(c *gin.Context) {
	conn, err := h.broker.GetConnector(c.Param("id"))
	if err != nil {
		errStatus(c, err)
		return
	}
	fields, err := h.broker.ContractFields(conn)
	if err != nil {
		errStatus(c, err)
		return
	}
	multi := c.DefaultQuery("multi_unit", "false") == "true"
	header := importer.TemplateHeader(fields, multi)
	c.Header("Content-Type", "text/csv; charset=utf-8")
	c.Header("Content-Disposition", fmt.Sprintf("attachment; filename=template-%s.csv", conn.ID[:8]))
	for i, h := range header {
		if i > 0 {
			c.Writer.WriteString(",")
		}
		c.Writer.WriteString(h)
	}
	c.Writer.WriteString("\n")
}

// PreviewImport POST /api/connectors/:id/import/preview (multipart: file, period_id, unit_id?)
// → parse + validasi kontrak, TANPA menyentuh capaian. Return batch id.
func (h *BrokerHandler) PreviewImport(c *gin.Context) {
	conn, err := h.broker.GetConnector(c.Param("id"))
	if err != nil {
		errStatus(c, err)
		return
	}
	fh, err := c.FormFile("file")
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"success": false, "error": gin.H{"message": "file wajib (.xlsx/.csv)"}})
		return
	}
	unitID := c.PostForm("unit_id")
	periodID := c.PostForm("period_id")
	multi := c.PostForm("multi_unit") == "true"

	fields, err := h.broker.ContractFields(conn)
	if err != nil {
		errStatus(c, err)
		return
	}
	res, err := importer.ParseFile(fh, fields, multi)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"success": false, "error": gin.H{"message": err.Error()}})
		return
	}

	// preview JSON (max 50 baris utk response ringkas; sisanya disimpan)
	type previewRow struct {
		Line   int                `json:"line"`
		Unit   string             `json:"unit,omitempty"`
		Data   map[string]float64 `json:"data"`
		Errors []string           `json:"errors,omitempty"`
	}
	var pv []previewRow
	for i, r := range res.Rows {
		if i >= 50 {
			break
		}
		pv = append(pv, previewRow{Line: r.LineNumber, Unit: r.UnitID, Data: r.Data, Errors: r.Errors})
	}
	pvJSON, _ := json.Marshal(map[string]interface{}{"headers": res.Headers, "rows": pv})

	batch := &entity.ImportBatch{
		ConnectorID: conn.ID, IndicatorID: conn.IndicatorID,
		UnitID: unitID, PeriodID: periodID,
		FileName: fh.Filename, TotalRows: res.ValidCount + res.ErrorCount,
		ValidRows: res.ValidCount, ErrorRows: res.ErrorCount,
		RawPreview: string(pvJSON), ImportedBy: actorOf(c),
	}
	if err := h.broker.SavePreview(batch); err != nil {
		errStatus(c, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"success": true, "data": gin.H{
		"batch": batch, "preview": pv, "headers": res.Headers,
	}})
}

// ApplyImport POST /api/imports/:id/apply — apply baris VALID → capaian draft (source=import).
// Single-unit: satu capaian (agregat baris = jumlah per variabel).
// Multi-unit: satu capaian per unit unik.
func (h *BrokerHandler) ApplyImport(c *gin.Context) {
	batch, err := h.broker.GetImport(c.Param("id"))
	if err != nil {
		errStatus(c, err)
		return
	}
	if batch.Applied {
		c.JSON(http.StatusBadRequest, gin.H{"success": false, "error": gin.H{"message": "batch sudah di-apply"}})
		return
	}
	if batch.ErrorRows > 0 {
		c.JSON(http.StatusBadRequest, gin.H{"success": false, "error": gin.H{"message": fmt.Sprintf("masih ada %d baris error — perbaiki file & upload ulang (preview tanpa error dulu)", batch.ErrorRows)}})
		return
	}
	if batch.IndicatorID == "" || batch.PeriodID == "" {
		c.JSON(http.StatusBadRequest, gin.H{"success": false, "error": gin.H{"message": "batch belum terikat indikator/periode — ulangi preview dengan parameter lengkap"}})
		return
	}

	// parse ulang preview tersimpan
	var pv struct {
		Rows []struct {
			Unit string             `json:"unit"`
			Data map[string]float64 `json:"data"`
		} `json:"rows"`
	}
	if err := json.Unmarshal([]byte(batch.RawPreview), &pv); err != nil {
		errStatus(c, err)
		return
	}
	if len(pv.Rows) == 0 {
		c.JSON(http.StatusBadRequest, gin.H{"success": false, "error": gin.H{"message": "tidak ada baris valid"}})
		return
	}

	// kelompokkan per unit
	byUnit := map[string]map[string]float64{}
	var unitOrder []string
	for _, r := range pv.Rows {
		key := r.Unit
		if key == "" {
			key = batch.UnitID
		}
		if key == "" {
			key = "institution"
		}
		if _, ok := byUnit[key]; !ok {
			byUnit[key] = map[string]float64{}
			unitOrder = append(unitOrder, key)
		}
		for k, v := range r.Data {
			byUnit[key][k] += v // agregasi: jumlah baris per variabel
		}
	}

	var created []string
	firstID := ""
	for _, u := range unitOrder {
		a, err := h.ach.Upsert(service.UpsertAchievementInput{
			IndicatorID: batch.IndicatorID, UnitID: u, PeriodID: batch.PeriodID,
			RawData: byUnit[u], Source: "import",
		}, batch.ImportedBy)
		if err != nil {
			errStatus(c, fmt.Errorf("apply gagal utk unit %s: %w", u, err))
			return
		}
		created = append(created, a.ID)
		if firstID == "" {
			firstID = a.ID
		}
	}
	if err := h.broker.MarkApplied(batch.ID, firstID); err != nil {
		errStatus(c, err)
		return
	}
	_ = h.broker.TouchSync(batch.ConnectorID, fmt.Sprintf("import %s: %d baris → %d capaian draft", batch.FileName, batch.ValidRows, len(created)))
	c.JSON(http.StatusOK, gin.H{"success": true, "data": gin.H{
		"applied": true, "achievements_created": created, "units": unitOrder,
	}})
}

// ListImports GET /api/imports?connector_id=
func (h *BrokerHandler) ListImports(c *gin.Context) {
	list, err := h.broker.ListImports(c.Query("connector_id"))
	if err != nil {
		errStatus(c, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"success": true, "data": list})
}
