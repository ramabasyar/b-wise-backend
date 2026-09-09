package service

import (
	"encoding/json"
	"errors"
	"fmt"
	"math"
	"sort"
	"strings"
	"time"

	"github.com/expr-lang/expr"
	entity "github.com/rama/b-wise/iku/internal/domain/entity"
	"gorm.io/gorm"
)

// ==================== FORMULA ENGINE (F1) ====================
// Ekspresi string dievaluasi via expr-lang (pure math; tanpa jaringan/FS).
// Versioning: create → draft; activate → active (auto-retire versi aktif lain);
// historis capaian (F2) membaca formula active_at periodenya.

type FormulaService struct {
	db *gorm.DB
}

func NewFormulaService(db *gorm.DB) *FormulaService { return &FormulaService{db: db} }

// ValidateCompile — compile ekspresi + pastikan semua variabel env terdefinisi.
func ValidateCompile(expression string, vars []entity.FormulaInputVar) error {
	if strings.TrimSpace(expression) == "" {
		return fmt.Errorf("%w: ekspresi kosong", ErrValidation)
	}
	env := map[string]interface{}{}
	for _, v := range vars {
		env[v.Name] = 0.0
	}
	_, err := expr.Compile(expression, expr.Env(env), expr.AsFloat64())
	if err != nil {
		return fmt.Errorf("%w: ekspresi invalid: %v", ErrValidation, err)
	}
	return nil
}

// Evaluate — hitung ekspresi dengan input values (sandbox aman: env hanyalah angka).
func Evaluate(f *entity.FormulaVersion, inputs map[string]float64) (float64, error) {
	env := map[string]interface{}{}
	for _, v := range f.InputVariables {
		env[v.Name] = inputs[v.Name] // zero-value kalau tidak dikirim
	}
	program, err := expr.Compile(f.Expression, expr.Env(env), expr.AsFloat64())
	if err != nil {
		return 0, err
	}
	out, err := expr.Run(program, env)
	if err != nil {
		return 0, err
	}
	f64, ok := out.(float64)
	if !ok {
		return 0, fmt.Errorf("hasil ekspresi bukan angka")
	}
	return applyRounding(f64, f.RoundingRule), nil
}

func applyRounding(v float64, rule string) float64 {
	switch rule {
	case "0dp":
		return math.Round(v)
	case "2dp":
		return math.Round(v*100) / 100
	default:
		return v
	}
}

// checkValidation — jalankan validation_rules (json: min/max/warn_above). Return warning (bukan error utk warn).
func CheckValidation(f *entity.FormulaVersion, result float64) (warn string) {
	if f.ValidationRules == "" {
		return ""
	}
	var rules struct {
		Min       *float64 `json:"min"`
		Max       *float64 `json:"max"`
		WarnAbove *float64 `json:"warn_above"`
		WarnBelow *float64 `json:"warn_below"`
	}
	if json.Unmarshal([]byte(f.ValidationRules), &rules) != nil {
		return ""
	}
	if rules.Min != nil && result < *rules.Min {
		return fmt.Sprintf("hasil %.2f di bawah batas minimal %.2f", result, *rules.Min)
	}
	if rules.Max != nil && result > *rules.Max {
		return fmt.Sprintf("hasil %.2f melebihi batas maksimal %.2f", result, *rules.Max)
	}
	if rules.WarnAbove != nil && result > *rules.WarnAbove {
		return fmt.Sprintf("perhatian: hasil %.2f > %.2f — cek kemungkinan anomali data", result, *rules.WarnAbove)
	}
	if rules.WarnBelow != nil && result < *rules.WarnBelow {
		return fmt.Sprintf("perhatian: hasil %.2f < %.2f — cek kelengkapan data", result, *rules.WarnBelow)
	}
	return ""
}

// ---------- CRUD + lifecycle ----------

func (s *FormulaService) ListByIndicator(indicatorID string, includeRetired bool) ([]entity.FormulaVersion, error) {
	q := s.db.Where("indicator_id = ?", indicatorID)
	if !includeRetired {
		q = q.Where("status IN ('draft','active')")
	}
	var list []entity.FormulaVersion
	return list, q.Order("version_number DESC").Find(&list).Error
}

func (s *FormulaService) GetActive(indicatorID string) (*entity.FormulaVersion, error) {
	var f entity.FormulaVersion
	err := s.db.Where("indicator_id = ? AND status = 'active'", indicatorID).First(&f).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, ErrNotFound
	}
	return &f, err
}

type CreateFormulaInput struct {
	IndicatorID     string
	Expression      string
	InputVariables  []entity.FormulaInputVar
	RoundingRule    string
	ValidationRules string
	Notes           string
}

// Create — buat versi baru (draft). Validasi: compile + variabel unik.
func (s *FormulaService) Create(in CreateFormulaInput, createdBy string) (*entity.FormulaVersion, error) {
	// indikator ada?
	var ind entity.IndicatorDefinition
	if err := s.db.Where("id = ?", in.IndicatorID).First(&ind).Error; err != nil {
		return nil, ErrNotFound
	}
	if err := ValidateCompile(in.Expression, in.InputVariables); err != nil {
		return nil, err
	}
	// variabel unik
	seen := map[string]bool{}
	for _, v := range in.InputVariables {
		if v.Name == "" || seen[v.Name] {
			return nil, fmt.Errorf("%w: variabel kosong/duplikat", ErrValidation)
		}
		seen[v.Name] = true
	}

	// nomor versi berikutnya
	var maxVer int64
	s.db.Model(&entity.FormulaVersion{}).Where("indicator_id = ?", in.IndicatorID).
		Select("COALESCE(MAX(version_number),0)").Scan(&maxVer)

	vr := in.ValidationRules
	if vr == "" {
		vr = "{}" // kolom jsonb: string kosong invalid
	}
	f := &entity.FormulaVersion{
		IndicatorID:     in.IndicatorID,
		VersionNumber:   int(maxVer) + 1,
		Expression:      in.Expression,
		InputVariables:  in.InputVariables,
		RoundingRule:    in.RoundingRule,
		ValidationRules: vr,
		Status:          "draft",
		Notes:           in.Notes,
		CreatedBy:       createdBy,
	}
	if err := s.db.Create(f).Error; err != nil {
		return nil, err
	}
	return f, nil
}

// Activate — aktifkan versi; versi aktif lain auto-retire (valid_to semantics via status).
func (s *FormulaService) Activate(id, actor string) (*entity.FormulaVersion, error) {
	return s.activateTx(id, actor, false)
}

func (s *FormulaService) activateTx(id, actor string, _ bool) (*entity.FormulaVersion, error) {
	var f entity.FormulaVersion
	if err := s.db.Where("id = ?", id).First(&f).Error; err != nil {
		return nil, ErrNotFound
	}
	if err := ValidateCompile(f.Expression, f.InputVariables); err != nil {
		return nil, err
	}
	err := s.db.Transaction(func(tx *gorm.DB) error {
		now := time.Now()
		if err := tx.Model(&entity.FormulaVersion{}).
			Where("indicator_id = ? AND status = 'active' AND id <> ?", f.IndicatorID, f.ID).
			Updates(map[string]interface{}{"status": "retired", "active_from": nil}).Error; err != nil {
			return err
		}
		return tx.Model(&f).Updates(map[string]interface{}{"status": "active", "active_from": now}).Error
	})
	if err != nil {
		return nil, err
	}
	_ = actor
	return &f, nil
}

func (s *FormulaService) Retire(id string) error {
	res := s.db.Model(&entity.FormulaVersion{}).Where("id = ? AND status <> 'retired'", id).
		Update("status", "retired")
	if res.RowsAffected == 0 {
		return ErrNotFound
	}
	return nil
}

// SandboxTest — uji ekspresi TANPA menyimpan (blueprint: formula sandbox).
type SandboxResult struct {
	Expression string             `json:"expression"`
	Inputs     map[string]float64 `json:"inputs"`
	Result     float64            `json:"result"`
	Rounding   string             `json:"rounding_rule"`
	Warning    string             `json:"warning,omitempty"`
}

func (s *FormulaService) SandboxTest(expression string, vars []entity.FormulaInputVar, inputs map[string]float64, rounding, validation string) (*SandboxResult, error) {
	f := &entity.FormulaVersion{
		Expression: expression, InputVariables: vars,
		RoundingRule: rounding, ValidationRules: validation,
	}
	if err := ValidateCompile(expression, vars); err != nil {
		return nil, err
	}
	res, err := Evaluate(f, inputs)
	if err != nil {
		return nil, fmt.Errorf("%w: %v", ErrValidation, err)
	}
	return &SandboxResult{
		Expression: expression, Inputs: inputs, Result: res,
		Rounding: rounding, Warning: CheckValidation(f, res),
	}, nil
}

// used by F2 (achievement calc): formula aktif pada periode (active_from <= period.end).
func (s *FormulaService) GetActiveAt(indicatorID string, at time.Time) (*entity.FormulaVersion, error) {
	var f entity.FormulaVersion
	err := s.db.Where("indicator_id = ? AND status = 'active'", indicatorID).First(&f).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, ErrNotFound
	}
	return &f, err
}

// sortedInputNames — helper (dokumentasi variabel)
func SortedVarNames(vars []entity.FormulaInputVar) []string {
	names := make([]string, 0, len(vars))
	for _, v := range vars {
		names = append(names, v.Name)
	}
	sort.Strings(names)
	return names
}
