// Package importer — import terkelola Excel/CSV utk capaian IKU (F5).
// Alur: parse file → validasi vs kontrak (kolom wajib, tipe) → PREVIEW
// (baris valid/error, tanpa menyentuh capaian) → APPLY (bentuk capaian
// draft dgn source=import + ImportBatch audit).
package importer

import (
	"encoding/csv"
	"fmt"
	"io"
	"mime/multipart"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/xuri/excelize/v2"

	entity "github.com/rama/b-wise/iku/internal/domain/entity"
)

type RowData map[string]float64

type ParsedRow struct {
	LineNumber int      `json:"line"`    // 1-based di sheet/file (termasuk header)
	UnitID     string   `json:"unit_id"` // jika kolom unit ada
	Data       RowData  `json:"data"`
	Errors     []string `json:"errors,omitempty"`
}

type ParseResult struct {
	Headers    []string    `json:"headers"`
	Rows       []ParsedRow `json:"rows"`
	ValidCount int         `json:"valid_count"`
	ErrorCount int         `json:"error_count"`
}

// ParseFile — deteksi ekstensi → parse → validasi kontrak.
// contractFields: mapping kolom file → variabel. requiredUnit: kolom unit wajib (multi-unit) atau "" (single unit).
func ParseFile(fh *multipart.FileHeader, contractFields []entity.ContractField, requireUnitCol bool) (*ParseResult, error) {
	ext := strings.ToLower(filepath.Ext(fh.Filename))
	if ext != ".xlsx" && ext != ".xls" && ext != ".csv" {
		return nil, fmt.Errorf("format tidak didukung: %s (gunakan .xlsx atau .csv)", ext)
	}
	f, err := fh.Open()
	if err != nil {
		return nil, err
	}
	defer f.Close()

	var headers []string
	var rawRows [][]string
	if ext == ".csv" {
		r := csv.NewReader(f)
		r.TrimLeadingSpace = true
		headers, err = r.Read()
		if err != nil {
			return nil, fmt.Errorf("baca header CSV: %w", err)
		}
		for {
			row, err := r.Read()
			if err == io.EOF {
				break
			}
			if err != nil {
				return nil, err
			}
			rawRows = append(rawRows, row)
		}
	} else {
		xl, err := excelize.OpenReader(f)
		if err != nil {
			return nil, fmt.Errorf("buka Excel: %w", err)
		}
		defer xl.Close()
		sheets := xl.GetSheetList()
		if len(sheets) == 0 {
			return nil, fmt.Errorf("file Excel kosong")
		}
		rows, err := xl.GetRows(sheets[0])
		if err != nil {
			return nil, err
		}
		if len(rows) == 0 {
			return nil, fmt.Errorf("sheet kosong")
		}
		headers = rows[0]
		rawRows = rows[1:]
	}

	// normalisasi header
	norm := func(s string) string {
		return strings.ToLower(strings.Join(strings.Fields(strings.TrimSpace(s)), "_"))
	}
	headerIdx := map[string]int{}
	for i, h := range headers {
		headerIdx[norm(h)] = i
	}

	// cek kolom kontrak
	var missing []string
	for _, cf := range contractFields {
		if _, ok := headerIdx[norm(cf.Name)]; !ok && cf.Required {
			missing = append(missing, cf.Name)
		}
	}
	if len(missing) > 0 {
		return nil, fmt.Errorf("kolom wajib tidak ditemukan: %s (header file: %s)", strings.Join(missing, ", "), strings.Join(headers, ", "))
	}
	unitCol := -1
	if idx, ok := headerIdx["unit_id"]; ok {
		unitCol = idx
	} else if idx, ok := headerIdx["unit"]; ok {
		unitCol = idx
	} else if requireUnitCol {
		return nil, fmt.Errorf("kolom unit_id/unit wajib untuk import multi-unit")
	}

	res := &ParseResult{Headers: headers}
	for i, row := range rawRows {
		if len(row) == 0 || (len(row) == 1 && strings.TrimSpace(row[0]) == "") {
			continue // skip baris kosong
		}
		pr := ParsedRow{LineNumber: i + 2, Data: RowData{}} // +2: header + 1-based
		if unitCol >= 0 && unitCol < len(row) {
			pr.UnitID = strings.TrimSpace(row[unitCol])
		}
		for _, cf := range contractFields {
			idx, ok := headerIdx[norm(cf.Name)]
			if !ok {
				continue // kolom opsional tidak ada
			}
			if idx >= len(row) {
				if cf.Required {
					pr.Errors = append(pr.Errors, fmt.Sprintf("kolom %s kosong", cf.Name))
				}
				continue
			}
			val := strings.TrimSpace(row[idx])
			if val == "" {
				if cf.Required {
					pr.Errors = append(pr.Errors, fmt.Sprintf("%s wajib diisi", cf.Name))
				}
				continue
			}
			n, err := strconv.ParseFloat(strings.ReplaceAll(val, ",", "."), 64)
			if err != nil {
				pr.Errors = append(pr.Errors, fmt.Sprintf("%s bukan angka: %q", cf.Name, val))
				continue
			}
			if cf.Target != "" {
				pr.Data[cf.Target] = n
			} else {
				pr.Data[cf.Name] = n
			}
		}
		if len(pr.Errors) > 0 {
			res.ErrorCount++
		} else {
			res.ValidCount++
		}
		res.Rows = append(res.Rows, pr)
	}
	return res, nil
}

// TemplateHeader — baris header template utk download
func TemplateHeader(fields []entity.ContractField, withUnit bool) []string {
	h := []string{}
	if withUnit {
		h = append(h, "unit_id")
	}
	for _, f := range fields {
		h = append(h, f.Name)
	}
	return h
}

var _ = time.Now
