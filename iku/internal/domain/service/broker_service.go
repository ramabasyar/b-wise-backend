package service

import (
	"encoding/json"
	"errors"
	"fmt"

	entity "github.com/rama/b-wise/iku/internal/domain/entity"
	"gorm.io/gorm"
)

// ==================== BROKER SERVICE (F5) ====================

type BrokerService struct {
	db *gorm.DB
}

func NewBrokerService(db *gorm.DB) *BrokerService { return &BrokerService{db: db} }

// ---------- connector CRUD ----------

type ConnectorFilter struct {
	Type        string
	IndicatorID string
	ActiveOnly  bool
}

func (s *BrokerService) ListConnectors(f ConnectorFilter) ([]entity.DataSourceConnector, error) {
	q := s.db.Model(&entity.DataSourceConnector{})
	if f.Type != "" {
		q = q.Where("type = ?", f.Type)
	}
	if f.IndicatorID != "" {
		q = q.Where("indicator_id = ? OR indicator_id = ''", f.IndicatorID)
	}
	if f.ActiveOnly {
		q = q.Where("status = 'active'")
	}
	var list []entity.DataSourceConnector
	return list, q.Order("name ASC").Find(&list).Error
}

type UpsertConnectorInput struct {
	ID          string
	Name        string
	IndicatorID string
	Type        string
	SystemOwner string
	Contract    []entity.ContractField
	Notes       string
}

func (s *BrokerService) UpsertConnector(in UpsertConnectorInput, actor string) (*entity.DataSourceConnector, error) {
	if in.Name == "" || in.Type == "" {
		return nil, fmt.Errorf("%w: nama & tipe connector wajib", ErrValidation)
	}
	switch entity.ConnectorType(in.Type) {
	case entity.ConnectorManual, entity.ConnectorFileImport, entity.ConnectorAPI, entity.ConnectorDB:
	default:
		return nil, fmt.Errorf("%w: tipe harus manual|file-import|api|db", ErrValidation)
	}
	contractJSON := ""
	if len(in.Contract) > 0 {
		b, _ := json.Marshal(map[string]interface{}{"fields": in.Contract})
		contractJSON = string(b)
	}

	var c entity.DataSourceConnector
	if in.ID != "" {
		if err := s.db.Where("id = ?", in.ID).First(&c).Error; err != nil {
			return nil, ErrNotFound
		}
	} else {
		// nama unik
		var n int64
		s.db.Model(&entity.DataSourceConnector{}).Where("name = ?", in.Name).Count(&n)
		if n > 0 {
			return nil, fmt.Errorf("%w: nama connector sudah dipakai", ErrValidation)
		}
		c = entity.DataSourceConnector{CreatedBy: actor}
	}
	c.Name, c.IndicatorID, c.Type = in.Name, in.IndicatorID, entity.ConnectorType(in.Type)
	c.SystemOwner, c.Notes, c.ContractJSON = in.SystemOwner, in.Notes, contractJSON
	c.Status = "active"

	if err := s.db.Save(&c).Error; err != nil {
		return nil, err
	}
	return &c, nil
}

func (s *BrokerService) GetConnector(id string) (*entity.DataSourceConnector, error) {
	var c entity.DataSourceConnector
	if err := s.db.Where("id = ?", id).First(&c).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, ErrNotFound
		}
		return nil, err
	}
	return &c, nil
}

// ContractFields — parse kontrak utk dipakai importer.
func (s *BrokerService) ContractFields(c *entity.DataSourceConnector) ([]entity.ContractField, error) {
	if c.ContractJSON == "" {
		return nil, fmt.Errorf("%w: connector belum punya kontrak data", ErrValidation)
	}
	var wrap struct {
		Fields []entity.ContractField `json:"fields"`
	}
	if err := json.Unmarshal([]byte(c.ContractJSON), &wrap); err != nil {
		return nil, err
	}
	if len(wrap.Fields) == 0 {
		return nil, fmt.Errorf("%w: kontrak kosong", ErrValidation)
	}
	return wrap.Fields, nil
}

// ---------- import batch ----------

func (s *BrokerService) ListImports(connectorID string) ([]entity.ImportBatch, error) {
	q := s.db.Model(&entity.ImportBatch{})
	if connectorID != "" {
		q = q.Where("connector_id = ?", connectorID)
	}
	var list []entity.ImportBatch
	return list, q.Order("imported_at DESC").Limit(100).Find(&list).Error
}

// SavePreview — simpan hasil parse sbg batch (applied=false).
func (s *BrokerService) SavePreview(b *entity.ImportBatch) error {
	return s.db.Create(b).Error
}

// GetImport — utk halaman preview/apply.
func (s *BrokerService) GetImport(id string) (*entity.ImportBatch, error) {
	var b entity.ImportBatch
	if err := s.db.Where("id = ?", id).First(&b).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, ErrNotFound
		}
		return nil, err
	}
	return &b, nil
}

// MarkApplied — catat hasil apply.
func (s *BrokerService) MarkApplied(batchID string, achievementID string) error {
	return s.db.Model(&entity.ImportBatch{}).Where("id = ?", batchID).
		Updates(map[string]interface{}{"applied": true, "achievement_id": achievementID}).Error
}

// TouchSync — update last run info connector.
func (s *BrokerService) TouchSync(connectorID, info string) error {
	return s.db.Model(&entity.DataSourceConnector{}).Where("id = ?", connectorID).
		Updates(map[string]interface{}{"last_sync_at": gorm.Expr("NOW()"), "last_run_info": info}).Error
}
