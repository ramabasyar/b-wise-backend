package handler

import (
	"net/http"

	"github.com/gin-gonic/gin"
	"github.com/rama/b-wise/human-capital/internal/domain/entity"
	"gorm.io/gorm"
	"github.com/rama/b-wise/human-capital/pkg/response"
)

// DictionaryHandler — expose kamus kepegawaian (config-driven wizard) + CRUD admin.
type DictionaryHandler struct {
	db *gorm.DB
}

func NewDictionaryHandler(db *gorm.DB) *DictionaryHandler { return &DictionaryHandler{db: db} }

// GetAll GET /api/dictionaries — semua kamus aktif sekali fetch (utk wizard & UI).
func (h *DictionaryHandler) GetAll(c *gin.Context) {
	out := gin.H{}
	var empTypes []entity.EmployeeType
	h.db.Where("is_active = ?", true).Order("sort").Find(&empTypes)
	out["employee_types"] = empTypes

	var unitTypes []entity.OrgUnitType
	h.db.Where("is_active = ?", true).Order("sort").Find(&unitTypes)
	out["org_unit_types"] = unitTypes

	var levels []entity.EmploymentLevel
	h.db.Where("is_active = ?", true).Order("sort").Find(&levels)
	out["employment_levels"] = levels

	var ranks []entity.AcademicRank
	h.db.Where("is_active = ?", true).Order("sort").Find(&ranks)
	out["academic_ranks"] = ranks

	var positions []entity.StructuralPosition
	h.db.Where("is_active = ?", true).Order("sort").Find(&positions)
	out["structural_positions"] = positions

	response.Success(c, out)
}

// upsert generik per kamus — body: {code, label, ...}. Admin-only (di-route dgn permission dictionaries.write).
func (h *DictionaryHandler) Upsert(c *gin.Context) {
	kind := c.Param("kind")
	var body map[string]interface{}
	if err := c.ShouldBindJSON(&body); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"success": false, "error": gin.H{"message": err.Error()}})
		return
	}
	code, _ := body["code"].(string)
	label, _ := body["label"].(string)
	if code == "" || label == "" {
		c.JSON(http.StatusBadRequest, gin.H{"success": false, "error": gin.H{"message": "code & label wajib"}})
		return
	}

	getStr := func(k string) string { v, _ := body[k].(string); return v }
	getBool := func(k string) bool { v, _ := body[k].(bool); return v }
	getInt := func(k string) int { v, _ := body[k].(float64); return int(v) }

	switch kind {
	case "employee-types":
		row := entity.EmployeeType{Code: code, Label: label, RealmRoles: getStr("realm_roles"),
			HasLevel: getBool("has_level"), HasRank: getBool("has_rank"), DualUnitAllowed: getBool("dual_unit_allowed"),
			Sort: getInt("sort")}
		var exist entity.EmployeeType
		err := h.db.Where("code = ?", code).First(&exist).Error
		if err != nil {
			row.IsSystem = false
			h.db.Create(&row)
		} else {
			exist.Label, exist.RealmRoles = label, getStr("realm_roles")
			exist.HasLevel, exist.HasRank, exist.DualUnitAllowed = getBool("has_level"), getBool("has_rank"), getBool("dual_unit_allowed")
			exist.Sort = getInt("sort")
			h.db.Save(&exist)
		}
	case "org-unit-types":
		row := entity.OrgUnitType{Code: code, Label: label, AllowedParents: getStr("allowed_parents"), Sort: getInt("sort")}
		var exist entity.OrgUnitType
		if err := h.db.Where("code = ?", code).First(&exist).Error; err != nil {
			h.db.Create(&row)
		} else {
			exist.Label, exist.AllowedParents, exist.Sort = label, getStr("allowed_parents"), getInt("sort")
			h.db.Save(&exist)
		}
	case "employment-levels":
		row := entity.EmploymentLevel{Code: code, Label: label, AppliesTo: getStr("applies_to"), Sort: getInt("sort")}
		var exist entity.EmploymentLevel
		if err := h.db.Where("code = ?", code).First(&exist).Error; err != nil {
			h.db.Create(&row)
		} else {
			exist.Label, exist.AppliesTo, exist.Sort = label, getStr("applies_to"), getInt("sort")
			h.db.Save(&exist)
		}
	case "academic-ranks":
		row := entity.AcademicRank{Code: code, Label: label, Sort: getInt("sort")}
		var exist entity.AcademicRank
		if err := h.db.Where("code = ?", code).First(&exist).Error; err != nil {
			h.db.Create(&row)
		} else {
			exist.Label, exist.Sort = label, getInt("sort")
			h.db.Save(&exist)
		}
	case "structural-positions":
		row := entity.StructuralPosition{Code: code, Label: label, ScopeUnitTypes: getStr("scope_unit_types"),
			IsApprovalHead: getBool("is_approval_head"), Sort: getInt("sort")}
		var exist entity.StructuralPosition
		if err := h.db.Where("code = ?", code).First(&exist).Error; err != nil {
			h.db.Create(&row)
		} else {
			exist.Label, exist.ScopeUnitTypes, exist.IsApprovalHead, exist.Sort = label, getStr("scope_unit_types"), getBool("is_approval_head"), getInt("sort")
			h.db.Save(&exist)
		}
	default:
		c.JSON(http.StatusNotFound, gin.H{"success": false, "error": gin.H{"message": "kamus tidak dikenal"}})
		return
	}
	response.Success(c, gin.H{"ok": true})
}

// Delete — soft delete (is_active=false); baris is_system ditolak.
func (h *DictionaryHandler) Delete(c *gin.Context) {
	kind := c.Param("kind")
	code := c.Param("code")
	var res *gorm.DB
	switch kind {
	case "employee-types":
		var row entity.EmployeeType
		if err := h.db.Where("code = ?", code).First(&row).Error; err == nil {
			if row.IsSystem {
				c.JSON(http.StatusForbidden, gin.H{"success": false, "error": gin.H{"message": "baris sistem tidak dapat dihapus — nonaktifkan saja"}})
				return
			}
			res = h.db.Model(&row).Update("is_active", false)
		}
	case "org-unit-types":
		var row entity.OrgUnitType
		if err := h.db.Where("code = ?", code).First(&row).Error; err == nil {
			if row.IsSystem {
				c.JSON(http.StatusForbidden, gin.H{"success": false, "error": gin.H{"message": "baris sistem tidak dapat dihapus"}})
				return
			}
			res = h.db.Model(&row).Update("is_active", false)
		}
	case "employment-levels":
		res = h.db.Model(&entity.EmploymentLevel{}).Where("code = ? AND is_system = false", code).Update("is_active", false)
	case "academic-ranks":
		res = h.db.Model(&entity.AcademicRank{}).Where("code = ?", code).Update("is_active", false)
	case "structural-positions":
		var row entity.StructuralPosition
		if err := h.db.Where("code = ?", code).First(&row).Error; err == nil {
			if row.IsSystem {
				c.JSON(http.StatusForbidden, gin.H{"success": false, "error": gin.H{"message": "baris sistem tidak dapat dihapus"}})
				return
			}
			res = h.db.Model(&row).Update("is_active", false)
		}
	default:
		c.JSON(http.StatusNotFound, gin.H{"success": false, "error": gin.H{"message": "kamus tidak dikenal"}})
		return
	}
	if res != nil && res.Error != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"success": false, "error": gin.H{"message": res.Error.Error()}})
		return
	}
	response.Success(c, gin.H{"ok": true})
}
