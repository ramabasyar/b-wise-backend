package service

import (
	"errors"
	"fmt"
	"reflect"
	"strings"

	"gorm.io/gorm"
)

var ErrNotFound = errors.New("data tidak ditemukan")
var ErrValidation = errors.New("validasi gagal")

// CrudService — CRUD generik untuk master data F0.
type CrudService[T any] struct{ db *gorm.DB }

func NewCrud[T any](db *gorm.DB) *CrudService[T] { return &CrudService[T]{db: db} }

func (s *CrudService[T]) List(preloads ...string) ([]T, error) {
	var list []T
	q := s.db
	for _, p := range preloads {
		q = q.Preload(p)
	}
	return list, q.Order("updated_at DESC").Limit(1000).Find(&list).Error
}

func (s *CrudService[T]) Get(id string, preloads ...string) (*T, error) {
	var m T
	q := s.db
	for _, p := range preloads {
		q = q.Preload(p)
	}
	if err := q.Where("id = ?", id).First(&m).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, ErrNotFound
		}
		return nil, err
	}
	return &m, nil
}

// Update — patch sebagian field (whitelist ditentukan handler via DTO).
func (s *CrudService[T]) Update(id string, patch map[string]any) (*T, error) {
	var m T
	if err := s.db.Where("id = ?", id).First(&m).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, ErrNotFound
		}
		return nil, err
	}
	if len(patch) == 0 {
		return nil, fmt.Errorf("%w: tidak ada field yang diubah", ErrValidation)
	}
	if err := s.db.Model(&m).Updates(patch).Error; err != nil {
		return nil, err
	}
	return s.Get(id)
}

// CreateRow — insert baris baru.
func (s *CrudService[T]) CreateRow(m *T) error { return s.db.Create(m).Error }

// DB — akses koneksi (dipakai handler untuk query khusus kecil).
func (s *CrudService[T]) DB() *gorm.DB { return s.db }

func (s *CrudService[T]) Delete(id string) error {
	res := s.db.Delete(new(T), "id = ?", id)
	if res.RowsAffected == 0 {
		return ErrNotFound
	}
	return res.Error
}

// ImportResult — hasil import CSV (idempotent upsert by unique code).
type ImportResult struct {
	Resource string   `json:"resource"`
	Total    int      `json:"total"`
	Created  int      `json:"created"`
	Updated  int      `json:"updated"`
	Errors   []string `json:"errors,omitempty"`
}

// UpsertByKey — insert bila belum ada (byField=unique code), update bila ada.
// fillTarget: pointer row hasil parse; onExist: patch field yang boleh berubah.
func (s *CrudService[T]) UpsertByKey(m *T, byField string, key any, onExist func(existing *T)) (created bool, err error) {
	var ex T
	q := s.db.Where(byField+" = ?", key).First(&ex)
	if errors.Is(q.Error, gorm.ErrRecordNotFound) {
		return true, s.db.Create(m).Error
	}
	if q.Error != nil {
		return false, q.Error
	}
	if onExist != nil {
		onExist(&ex)
	}
	return false, s.db.Save(&ex).Error
}

// ParseCSV — baris CSV ([]map kolom→nilai) menjadi struct via tag `csv:"..."`.
// Baris pertama dianggap header. Kolom kosong dilewati. Bool: true/1/ya/y.
func ParseCSV(rows []map[string]string, out any) error {
	rv := reflect.ValueOf(out)
	if rv.Kind() != reflect.Pointer || rv.Elem().Kind() != reflect.Struct {
		return fmt.Errorf("target harus pointer struct")
	}
	st := rv.Elem()
	t := st.Type()
	for i := 0; i < t.NumField(); i++ {
		f := t.Field(i)
		tag := f.Tag.Get("csv")
		if tag == "" {
			tag = f.Tag.Get("json") // fallback: kolom CSV = nama field JSON
		}
		if tag == "" || tag == "-" {
			continue
		}
		if idx := strings.Index(tag, ","); idx >= 0 {
			tag = tag[:idx]
		}
		col := tag
		def := ""
		_ = def
		val, ok := "", false
		for _, r := range rows {
			if v, exist := r[col]; exist && strings.TrimSpace(v) != "" {
				val, ok = strings.TrimSpace(v), true
				break
			}
		}
		if !ok || val == "" {
			val = def
		}
		fv := st.Field(i)
		if !fv.CanSet() {
			continue
		}
		switch fv.Kind() {
		case reflect.String:
			fv.SetString(val)
		case reflect.Int:
			var n int
			fmt.Sscanf(val, "%d", &n)
			fv.SetInt(int64(n))
		case reflect.Bool:
			switch strings.ToLower(val) {
			case "true", "1", "ya", "y", "yes":
				fv.SetBool(true)
			}
		case reflect.Pointer:
			if fv.Type().Elem().Kind() == reflect.String && val != "" {
				pv := reflect.New(fv.Type().Elem())
				pv.Elem().SetString(val)
				fv.Set(pv)
			}
		}
	}
	return nil
}

// DefaultTimeSlots — template standar Binawan: Senin–Jumat, 5 slot/hari.
func DefaultTimeSlots() []any {
	var out []any
	type mk struct {
		Day, Order  int
		Label, S, E string
	}
	rows := []mk{}
	for d := 1; d <= 5; d++ {
		names := []string{"Senin", "Selasa", "Rabu", "Kamis", "Jumat"}
		rows = append(rows,
			mk{d, 1, names[d-1] + " 07:30-09:10", "07:30", "09:10"},
			mk{d, 2, names[d-1] + " 09:20-11:00", "09:20", "11:00"},
			mk{d, 3, names[d-1] + " 11:10-12:50", "11:10", "12:50"},
			mk{d, 4, names[d-1] + " 13:30-15:10", "13:30", "15:10"},
			mk{d, 5, names[d-1] + " 15:20-17:00", "15:20", "17:00"},
		)
	}
	for _, r := range rows {
		out = append(out, map[string]any{
			"label": r.Label, "day": r.Day, "start_time": r.S, "end_time": r.E, "order": r.Order, "is_active": true,
		})
	}
	return out
}
