package ticketsvc

import (
	"encoding/json"
	"fmt"
	"strings"

	"github.com/rama/b-wise/helpdesk/internal/domain/entity"
)

// ===== Otomatisasi tim (KT-10/OT-05) =====

type autoCond struct {
	Field string `json:"field"`
	Op    string `json:"op"`
	Value string `json:"value"`
}

type autoAction struct {
	Type  string `json:"type"`
	Value string `json:"value"`
}

func condMatch(c autoCond, t *entity.Ticket) bool {
	var actual string
	switch c.Field {
	case "subject":
		actual = t.Subject
	case "type":
		actual = t.Type
	case "channel":
		actual = t.Channel
	case "priority":
		actual = t.Priority
	default:
		return false
	}
	if c.Op == "contains" {
		return strings.Contains(strings.ToLower(actual), strings.ToLower(c.Value))
	}
	return strings.EqualFold(actual, c.Value)
}

func (s *Service) applyAutomation(trigger string, t *entity.Ticket, actorID string) {
	var rules []entity.AutomationRule
	s.db.Where("team_id = ? AND trigger_event = ? AND is_active = ?", t.TeamID, trigger, true).Find(&rules)
	for _, r := range rules {
		var conds []autoCond
		_ = json.Unmarshal([]byte(r.Conditions), &conds)
		ok := true
		for _, c := range conds {
			if c.Value != "" && !condMatch(c, t) {
				ok = false
				break
			}
		}
		if !ok {
			continue
		}
		var acts []autoAction
		_ = json.Unmarshal([]byte(r.Actions), &acts)
		for _, a := range acts {
			switch a.Type {
			case "set_priority":
				_ = s.db.Model(t).Update("priority", a.Value).Error
				_ = s.AddSystemEvent(t.ID, "otomatisasi "+r.Name+": prioritas -> "+a.Value, "")
				t.Priority = a.Value
			case "add_note":
				_ = s.addEvent(t.ID, "note", "internal", actorID, a.Value, "")
			}
		}
	}
}

// RulesOfTeam - daftar aturan tim (kelola KT-10).
func (s *Service) RulesOfTeam(userID string) ([]entity.AutomationRule, error) {
	team, err := s.TeamOf(userID)
	if err != nil {
		return nil, err
	}
	var list []entity.AutomationRule
	err = s.db.Where("team_id = ?", team.ID).Order("created_at DESC").Find(&list).Error
	return list, err
}

// SaveRule - buat/ubah aturan (id kosong = baru) milik tim user.
func (s *Service) SaveRule(userID string, r entity.AutomationRule) (*entity.AutomationRule, error) {
	team, err := s.TeamOf(userID)
	if err != nil {
		return nil, err
	}
	if r.TriggerEvent == "" {
		r.TriggerEvent = "tiket_dibuat"
	}
	if r.TriggerEvent != "tiket_dibuat" && r.TriggerEvent != "tiket_selesai" {
		return nil, fmt.Errorf("%w: pemicu tidak dikenal", ErrValidation)
	}
	if r.Conditions == "" {
		r.Conditions = "[]"
	}
	if r.Actions == "" {
		r.Actions = "[]"
	}
	if !json.Valid([]byte(r.Conditions)) || !json.Valid([]byte(r.Actions)) {
		return nil, fmt.Errorf("%w: kondisi/aksi harus JSON valid", ErrValidation)
	}
	r.TeamID = team.ID
	if r.ID == "" {
		r.IsActive = true
		if err := s.db.Create(&r).Error; err != nil {
			return nil, err
		}
		return &r, nil
	}
	var exist entity.AutomationRule
	if err := s.db.Where("id = ? AND team_id = ?", r.ID, team.ID).First(&exist).Error; err != nil {
		return nil, ErrNotFound
	}
	if err := s.db.Model(&exist).Updates(map[string]interface{}{
		"name": r.Name, "trigger_event": r.TriggerEvent, "conditions": r.Conditions,
		"actions": r.Actions, "is_active": r.IsActive}).Error; err != nil {
		return nil, err
	}
	_ = s.db.Where("id = ?", r.ID).First(&exist).Error
	return &exist, nil
}

// DeleteRule - hapus aturan milik tim user.
func (s *Service) DeleteRule(userID, ruleID string) error {
	team, err := s.TeamOf(userID)
	if err != nil {
		return err
	}
	return s.db.Where("id = ? AND team_id = ?", ruleID, team.ID).Delete(&entity.AutomationRule{}).Error
}
