package ticketsvc

import (
	"encoding/json"
	"fmt"
	"strings"

	"github.com/rama/b-wise/helpdesk/internal/domain/entity"
)

// ===== Formulir layanan (KT-05/KT-06, blueprint 6.4) =====

// FormField - satu isian formulir; kondisi tampil KT-05 (F2).
type FormField struct {
	Key      string   `json:"key"`
	Label    string   `json:"label"`
	Type     string   `json:"type"` // teks | paragraf | angka | tanggal | pilihan | multi_pilihan
	Required bool     `json:"required"`
	Options  []string `json:"options,omitempty"`
	VisibleIf *FormCondition `json:"visible_if,omitempty"`
}

// FormCondition - field hanya tampil bila kondisi terpenuhi.
type FormCondition struct {
	Field string `json:"field"`
	Op    string `json:"op"`  // eq | neq | filled
	Value string `json:"value,omitempty"`
}

type formSchema struct {
	Fields []FormField `json:"fields"`
}

func condMet(c *FormCondition, answers map[string]interface{}) bool {
	if c == nil || c.Field == "" {
		return true
	}
	v, ok := answers[c.Field]
	switch c.Op {
	case "neq":
		return ok && fmt.Sprint(v) != c.Value
	case "filled":
		return ok && strings.TrimSpace(fmt.Sprint(v)) != ""
	default: // eq
		if list, isList := v.([]interface{}); isList {
			for _, item := range list {
				if fmt.Sprint(item) == c.Value {
					return true
				}
			}
			return false
		}
		return ok && fmt.Sprint(v) == c.Value
	}
}

// validateAnswers - wajib-isi hanya untuk field yang tampil menurut kondisi (KT-05).
func validateAnswers(schemaJSON, answersJSON string) error {
	var sc formSchema
	if err := json.Unmarshal([]byte(schemaJSON), &sc); err != nil || len(sc.Fields) == 0 {
		return nil // tanpa formulir = tanpa validasi
	}
	answers := map[string]interface{}{}
	if strings.TrimSpace(answersJSON) != "" {
		if err := json.Unmarshal([]byte(answersJSON), &answers); err != nil {
			return fmt.Errorf("%w: format jawaban tidak valid", ErrValidation)
		}
	}
	for _, f := range sc.Fields {
		if !condMet(f.VisibleIf, answers) {
			continue
		}
		if !f.Required {
			continue
		}
		v, ok := answers[f.Key]
		if !ok || strings.TrimSpace(fmt.Sprint(v)) == "" || fmt.Sprint(v) == "[]" {
			return fmt.Errorf("%w: isian wajib belum lengkap: %s", ErrValidation, f.Label)
		}
	}
	return nil
}

// FormPublished - versi terbit terbaru layanan (KT-06: terbit tidak diubah).
func (s *Service) FormPublished(serviceID string) *entity.ServiceForm {
	var f entity.ServiceForm
	if err := s.db.Where("service_id = ? AND published_at IS NOT NULL", serviceID).
		Order("version DESC").First(&f).Error; err != nil {
		return nil
	}
	return &f
}

// FormDraft - draf terbaru layanan (bisa diubah; versi berikutnya).
func (s *Service) FormDraft(serviceID string) *entity.ServiceForm {
	var f entity.ServiceForm
	if err := s.db.Where("service_id = ? AND published_at IS NULL", serviceID).
		Order("version DESC").First(&f).Error; err != nil {
		return nil
	}
	return &f
}

// SaveFormDraft - simpan/ubah draf = versi baru; versi terbit tidak disentuh (KT-06).
func (s *Service) SaveFormDraft(userID, serviceID, schema string) (*entity.ServiceForm, error) {
	svc, err := s.serviceOfTeam(userID, serviceID)
	if err != nil {
		return nil, err
	}
	var sc formSchema
	if strings.TrimSpace(schema) == "" {
		sc = formSchema{Fields: []FormField{}}
	} else if err := json.Unmarshal([]byte(schema), &sc); err != nil {
		return nil, fmt.Errorf("%w: skema formulir tidak valid", ErrValidation)
	}
	for i := range sc.Fields {
		if sc.Fields[i].Key == "" {
			sc.Fields[i].Key = slugify(sc.Fields[i].Label)
		}
		if sc.Fields[i].Type == "" {
			sc.Fields[i].Type = "teks"
		}
	}
	clean, _ := json.Marshal(sc)
	var maxVer int
	s.db.Model(&entity.ServiceForm{}).Where("service_id = ?", svc.ID).
		Select("COALESCE(MAX(version),0)").Scan(&maxVer)
	s.db.Where("service_id = ? AND published_at IS NULL", svc.ID).
		Delete(&entity.ServiceForm{})
	f := entity.ServiceForm{ServiceID: svc.ID, Version: maxVer + 1, Schema: string(clean)}
	if err := s.db.Create(&f).Error; err != nil {
		return nil, err
	}
	return &f, nil
}

// PublishForm - bekukan draf terbaru sebagai versi terbit (KT-06).
func (s *Service) PublishForm(userID, serviceID string) (*entity.ServiceForm, error) {
	svc, err := s.serviceOfTeam(userID, serviceID)
	if err != nil {
		return nil, err
	}
	draft := s.FormDraft(svc.ID)
	if draft == nil {
		return nil, fmt.Errorf("%w: belum ada draf untuk diterbitkan", ErrValidation)
	}
	now := s.now()
	if err := s.db.Model(draft).Update("published_at", now).Error; err != nil {
		return nil, err
	}
	draft.PublishedAt = &now
	return draft, nil
}

// serviceOfTeam - layanan milik tim user (guard admin unit).
func (s *Service) serviceOfTeam(userID, serviceID string) (*entity.Service, error) {
	team, err := s.TeamOf(userID)
	if err != nil {
		return nil, err
	}
	var svc entity.Service
	if err := s.db.Where("id = ? AND team_id = ?", serviceID, team.ID).First(&svc).Error; err != nil {
		return nil, ErrNotFound
	}
	return &svc, nil
}
