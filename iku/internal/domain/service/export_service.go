package service

import (
	"bytes"
	"fmt"
	"time"

	"github.com/rama/b-wise/iku/internal/assets"
	entity "github.com/rama/b-wise/iku/internal/domain/entity"
	"github.com/signintech/gopdf"
	"github.com/xuri/excelize/v2"
	"gorm.io/gorm"
)

// ==================== EXPORT SERVICE (F7) ====================
// Excel (excelize) + PDF (gopdf + Roboto embedded — Apache 2.0).
// Sumber data: DashboardService (rollup institusi) + query capaian langsung.

type ExportService struct {
	db   *gorm.DB
	dash *DashboardService
	cmp  *ComparisonService
}

func NewExportService(db *gorm.DB, dash *DashboardService, cmp *ComparisonService) *ExportService {
	return &ExportService{db: db, dash: dash, cmp: cmp}
}

const (
	hdrFill    = "FF1B5E20" // binawan-700
	redFill    = "FFFDE7E7"
	yellowFill = "FFFFF8E1"
	greenFill  = "FFE8F5E9"
)

// ---------- EXCEL ----------

// DashboardExcel — laporan ringkasan 12 IKU (institusi, rollup fakultas) + tren.
func (s *ExportService) resolvePeriod(periodID string) string {
	if periodID == "" {
		var per entity.Period
		if err := s.dash.LatestPeriod(&per); err == nil {
			return per.ID
		}
	}
	return periodID
}

func (s *ExportService) childIDs() []string {
	var ids []string
	_ = s.db.Raw(`SELECT id FROM branches`).Scan(&ids).Error // unit anak = branch (fakultas)
	return ids
}

func (s *ExportService) DashboardExcel(periodID string) (*excelize.File, error) {
	periodID = s.resolvePeriod(periodID)
	childIDs := s.childIDs()
	sum, err := s.dash.Build("institution", periodID, childIDs)
	if err != nil {
		return nil, err
	}
	unitRows, _ := s.dash.UnitComparison(periodID, childIDs)
	trendRows, _ := s.dash.Trend("institution", time.Now().Year())

	f := excelize.NewFile()
	styles := map[string]int{}
	for name, fill := range map[string]string{"ok": greenFill, "warn": yellowFill, "bad": redFill} {
		st, _ := f.NewStyle(&excelize.Style{Fill: excelize.Fill{Type: "pattern", Color: []string{fill}, Pattern: 1}})
		styles[name] = st
	}
	hstyle, _ := f.NewStyle(&excelize.Style{
		Fill: excelize.Fill{Type: "pattern", Color: []string{hdrFill}, Pattern: 1},
		Font: &excelize.Font{Color: "FFFFFFFF", Bold: true}, Alignment: &excelize.Alignment{Vertical: "center"},
	})

	// Sheet 1: Ringkasan
	f.SetSheetName("Sheet1", "Ringkasan")
	f.SetCellValue("Ringkasan", "A1", "LAPORAN KINERJA IKU — Universitas Binawan")
	f.SetCellValue("Ringkasan", "A2", "Periode: "+sum.PeriodLabel+" | Diekspor: "+time.Now().Format("02-01-2006 15:04"))
	hdrs := []string{"Kode", "Indikator", "Sifat", "Nilai", "Target", "Pencapaian %", "Status Warna", "Status Capaian"}
	for i, h := range hdrs {
		cell, _ := excelize.CoordinatesToCellName(i+1, 4)
		f.SetCellValue("Ringkasan", cell, h)
		f.SetCellStyle("Ringkasan", cell, cell, hstyle)
	}
	for r, ind := range sum.Indicators {
		row := 5 + r
		f.SetCellValue("Ringkasan", fmt.Sprintf("A%d", row), ind.IkuCode)
		f.SetCellValue("Ringkasan", fmt.Sprintf("B%d", row), ind.Name)
		f.SetCellValue("Ringkasan", fmt.Sprintf("C%d", row), ind.Nature)
		f.SetCellValue("Ringkasan", fmt.Sprintf("D%d", row), ind.Value)
		f.SetCellValue("Ringkasan", fmt.Sprintf("E%d", row), ind.Target)
		f.SetCellValue("Ringkasan", fmt.Sprintf("F%d", row), ind.Pct)
		f.SetCellValue("Ringkasan", fmt.Sprintf("G%d", row), ind.Color)
		f.SetCellValue("Ringkasan", fmt.Sprintf("H%d", row), ind.Status)
		key := map[string]string{"red": "bad", "yellow": "warn", "green": "ok"}[ind.Color]
		if st, okk := styles[key]; okk {
			f.SetCellStyle("Ringkasan", fmt.Sprintf("A%d", row), fmt.Sprintf("H%d", row), st)
		}
	}
	f.SetColWidth("Ringkasan", "A", "A", 9)
	f.SetColWidth("Ringkasan", "B", "B", 52)
	for _, col := range []string{"C", "D", "E", "F", "G", "H"} {
		f.SetColWidth("Ringkasan", col, col, 16)
	}

	// Sheet 2: Perbandingan Unit
	f.NewSheet("Per Unit")
	for i, h := range []string{"IKU", "Indikator", "Unit", "Nilai", "Target", "%", "Warna", "Status"} {
		cell, _ := excelize.CoordinatesToCellName(i+1, 1)
		f.SetCellValue("Per Unit", cell, h)
		f.SetCellStyle("Per Unit", cell, cell, hstyle)
	}
	for r, m := range unitRows {
		row := 2 + r
		setMapRow(f, "Per Unit", row, m, "iku_code", "name", "unit_name", "value", "target", "pct", "color", "status")
	}
	f.SetColWidth("Per Unit", "B", "B", 46)
	f.SetColWidth("Per Unit", "C", "C", 30)

	// Sheet 3: Tren
	f.NewSheet("Tren")
	for i, h := range []string{"IKU", "Indikator", "Periode", "Nilai", "Target", "%", "Warna"} {
		cell, _ := excelize.CoordinatesToCellName(i+1, 1)
		f.SetCellValue("Tren", cell, h)
		f.SetCellStyle("Tren", cell, cell, hstyle)
	}
	for r, m := range trendRows {
		setMapRow(f, "Tren", 2+r, m, "iku_code", "name", "period_label", "value", "target", "pct", "color")
	}
	f.SetColWidth("Tren", "B", "B", 46)
	return f, nil
}

func setMapRow(f *excelize.File, sheet string, row int, m map[string]interface{}, keys ...string) {
	for i, k := range keys {
		cell, _ := excelize.CoordinatesToCellName(i+1, row)
		if v, ok := m[k]; ok {
			f.SetCellValue(sheet, cell, v)
		}
	}
}

// AchievementsExcel — detail capaian (filter indikator/periode), utk audit & review.
func (s *ExportService) AchievementsExcel(indicatorID, periodID string) (*excelize.File, error) {
	q := s.db.Preload("Indicator").Preload("Period").Order("indicator_id, unit_id")
	if indicatorID != "" {
		q = q.Where("indicator_id = ?", indicatorID)
	}
	if periodID != "" {
		q = q.Where("period_id = ?", periodID)
	}
	var achs []entity.AchievementRecord
	if err := q.Limit(2000).Find(&achs).Error; err != nil {
		return nil, err
	}
	f := excelize.NewFile()
	sheet := "Capaian"
	f.SetSheetName("Sheet1", sheet)
	hstyle, _ := f.NewStyle(&excelize.Style{
		Fill: excelize.Fill{Type: "pattern", Color: []string{hdrFill}, Pattern: 1},
		Font: &excelize.Font{Color: "FFFFFFFF", Bold: true},
	})
	for i, h := range []string{"IKU", "Indikator", "Unit", "Periode", "Raw Data", "Nilai", "Target", "%", "Warna", "Status", "Sumber", "Formula v", "Diajukan", "Catatan Review"} {
		cell, _ := excelize.CoordinatesToCellName(i+1, 1)
		f.SetCellValue(sheet, cell, h)
		f.SetCellStyle(sheet, cell, cell, hstyle)
	}
	for r, a := range achs {
		row := 2 + r
		raw := ""
		for k, v := range a.RawData {
			raw += fmt.Sprintf("%s=%v; ", k, v)
		}
		unit := a.UnitID
		if a.UnitType != "institution" {
			unit = string([]rune(a.UnitID)[:8]) + " (" + a.UnitType + ")"
		}
		f.SetCellValue(sheet, fmt.Sprintf("A%d", row), a.Indicator.IkuCode)
		f.SetCellValue(sheet, fmt.Sprintf("B%d", row), a.Indicator.Name)
		f.SetCellValue(sheet, fmt.Sprintf("C%d", row), unit)
		f.SetCellValue(sheet, fmt.Sprintf("D%d", row), a.Period.Label)
		f.SetCellValue(sheet, fmt.Sprintf("E%d", row), raw)
		f.SetCellValue(sheet, fmt.Sprintf("F%d", row), a.CalculatedValue)
		f.SetCellValue(sheet, fmt.Sprintf("G%d", row), a.TargetValue)
		f.SetCellValue(sheet, fmt.Sprintf("H%d", row), a.AchievementPct)
		f.SetCellValue(sheet, fmt.Sprintf("I%d", row), a.ThresholdColor)
		f.SetCellValue(sheet, fmt.Sprintf("J%d", row), a.Status)
		f.SetCellValue(sheet, fmt.Sprintf("K%d", row), a.Source)
		f.SetCellValue(sheet, fmt.Sprintf("L%d", row), stringPtr(a.FormulaID))
		if a.SubmittedAt != nil {
			f.SetCellValue(sheet, fmt.Sprintf("M%d", row), a.SubmittedAt.Format("02-01-2006"))
		}
		f.SetCellValue(sheet, fmt.Sprintf("N%d", row), a.ReviewNotes)
	}
	for _, w := range []struct {
		c string
		w float64
	}{{"B", 46}, {"C", 24}, {"D", 14}, {"E", 40}, {"N", 30}} {
		f.SetColWidth(sheet, w.c, w.c, w.w)
	}
	return f, nil
}

// ---------- PDF ----------

// ReportPDF — laporan 1 halaman A4: ringkasan 12 IKU institusi.
func (s *ExportService) ReportPDF(periodID string) ([]byte, error) {
	periodID = s.resolvePeriod(periodID)
	childIDs := s.childIDs()
	sum, err := s.dash.Build("institution", periodID, childIDs)
	if err != nil {
		return nil, err
	}

	pdf := gopdf.GoPdf{}
	pdf.Start(gopdf.Config{PageSize: *gopdf.PageSizeA4})
	if err := pdf.AddTTFFontData("roboto", assets.RobotoRegular); err != nil {
		return nil, err
	}
	if err := pdf.AddTTFFontData("roboto-b", assets.RobotoBold); err != nil {
		return nil, err
	}
	pdf.SetMargins(40, 40, 40, 40)
	pdf.AddPage()

	// header
	if err := pdf.SetFont("roboto-b", "", 16); err != nil {
		return nil, err
	}
	pdf.SetXY(40, 40)
	_ = pdf.Cell(nil, "LAPORAN KINERJA IKU")
	if err := pdf.SetFont("roboto", "", 10); err != nil {
		return nil, err
	}
	pdf.SetXY(40, 60)
	_ = pdf.Cell(nil, "Universitas Binawan — Periode "+sum.PeriodLabel)
	pdf.SetXY(40, 74)
	_ = pdf.Cell(nil, "Diekspor: "+time.Now().Format("02-01-2006 15:04")+" WIB | Sumber: B-Wise Performance Center")

	// tabel header
	const (
		x0   = 40.0
		rowH = 18.0
	)
	cols := []float64{0, 55, 330, 395, 445, 495} // x-offsets: kode, nama, nilai, target, pct, warna
	y := 95.0
	pdf.SetXY(x0, y)
	pdf.SetFillColor(27, 94, 32)
	pdf.RectFromUpperLeftWithStyle(x0, y, 515, rowH, "f")
	pdf.SetFont("roboto-b", "", 9)
	pdf.SetTextColor(255, 255, 255)
	for i, h := range []string{"Kode", "Indikator", "Nilai", "Target", "%", "Warna"} {
		pdf.SetXY(cols[i]+3, y+5)
		_ = pdf.Cell(nil, h)
	}
	y += rowH
	pdf.SetTextColor(30, 30, 30)
	pdf.SetFont("roboto", "", 8)
	for _, ind := range sum.Indicators {
		if y > 780 { // page break
			pdf.AddPage()
			y = 40
		}
		val, tgt, pct, col := "-", "-", "-", stringOrNil(ind.Color)
		if ind.Value != nil {
			val = fmt.Sprintf("%.4f", *ind.Value)
		}
		if ind.Target != nil {
			tgt = fmt.Sprintf("%.4f", *ind.Target)
		}
		if ind.Pct != nil {
			pct = fmt.Sprintf("%.1f%%", *ind.Pct)
		}
		if col == "" {
			col = "n/a"
		}
		var fillR, fillG, fillB = 255, 255, 255
		switch ind.Color {
		case "red":
			fillR, fillG, fillB = 253, 231, 231
		case "yellow":
			fillR, fillG, fillB = 255, 248, 225
		case "green":
			fillR, fillG, fillB = 232, 245, 233
		}
		pdf.SetFillColor(uint8(fillR), uint8(fillG), uint8(fillB))
		pdf.RectFromUpperLeftWithStyle(x0, y, 515, rowH, "f")
		pdf.SetXY(cols[0]+3, y+5)
		_ = pdf.Cell(nil, ind.IkuCode)
		pdf.SetXY(cols[1]+3, y+5)
		_ = pdf.Cell(nil, truncate(ind.Name, 58))
		pdf.SetXY(cols[2]+3, y+5)
		_ = pdf.Cell(nil, val)
		pdf.SetXY(cols[3]+3, y+5)
		_ = pdf.Cell(nil, tgt)
		pdf.SetXY(cols[4]+3, y+5)
		_ = pdf.Cell(nil, pct)
		pdf.SetXY(cols[5]+3, y+5)
		_ = pdf.Cell(nil, col)
		y += rowH
	}

	// footer
	pdf.SetFont("roboto", "", 7)
	pdf.SetTextColor(120, 120, 120)
	pdf.SetXY(40, 810)
	_ = pdf.Cell(nil, "Dokumen ini dihasilkan otomatis oleh B-Wise Performance Center. Nilai historis dihitung dengan formula versi saat pencatatan (immutable).")

	var buf bytes.Buffer
	if err := pdf.Write(&buf); err != nil {
		return nil, err
	}
	return buf.Bytes(), nil
}

func stringOrNil(s string) string { return s }

func truncate(s string, n int) string {
	r := []rune(s)
	if len(r) <= n {
		return s
	}
	return string(r[:n-1]) + "…"
}

func stringPtr(s *string) string {
	if s == nil {
		return ""
	}
	return *s
}

// ComparisonExcel — laporan Perbandingan YoY (F10): tahun N vs N-1 per unit.
func (s *ExportService) ComparisonExcel(year int, unitID string) (*excelize.File, error) {
	if s.cmp == nil {
		return nil, fmt.Errorf("comparison service tidak tersedia")
	}
	rows, err := s.cmp.CompareYoY(year, unitID, "")
	if err != nil {
		return nil, err
	}
	f := excelize.NewFile()
	f.SetSheetName("Sheet1", "Perbandingan YoY")
	hstyle, _ := f.NewStyle(&excelize.Style{
		Fill: excelize.Fill{Type: "pattern", Color: []string{hdrFill}, Pattern: 1},
		Font: &excelize.Font{Color: "FFFFFFFF", Bold: true}, Alignment: &excelize.Alignment{Vertical: "center"},
	})
	okStyle, _ := f.NewStyle(&excelize.Style{Fill: excelize.Fill{Type: "pattern", Color: []string{greenFill}, Pattern: 1}})
	badStyle, _ := f.NewStyle(&excelize.Style{Fill: excelize.Fill{Type: "pattern", Color: []string{redFill}, Pattern: 1}})
	flatStyle, _ := f.NewStyle(&excelize.Style{Fill: excelize.Fill{Type: "pattern", Color: []string{yellowFill}, Pattern: 1}})

	headers := []string{"IKU", "Indikator", "Unit", "Polaritas", fmt.Sprintf("Capaian %d", year-1), fmt.Sprintf("Capaian %d", year), "Delta", "Delta %", "Verdict", "Status " + fmt.Sprint(year-1), "Status " + fmt.Sprint(year), "Catatan"}
	for i, h := range headers {
		cell, _ := excelize.CoordinatesToCellName(i+1, 1)
		f.SetCellValue("Perbandingan YoY", cell, h)
		f.SetCellStyle("Perbandingan YoY", cell, cell, hstyle)
	}
	verdictLabel := map[string]string{"better": "MEMBAIK", "worse": "MEMBURUK", "flat": "STABIL", "new": "BARU", "no_curr": "TAHUN INI KOSONG"}
	unitLabel := map[string]string{"institution": "Institusi"}
	for i, uid := range s.childIDs() {
		unitLabel[uid] = fmt.Sprintf("Unit %d", i+1)
	}
	for r, row := range rows {
		xl := r + 2
		set := func(col int, v interface{}) {
			cell, _ := excelize.CoordinatesToCellName(col, xl)
			f.SetCellValue("Perbandingan YoY", cell, v)
		}
		set(1, row.IkuCode)
		set(2, row.Name)
		set(3, unitLabel[row.UnitID])
		pol := "Naik = baik"
		if row.Polarity == "lower_is_better" {
			pol = "Turun = baik"
		}
		set(4, pol)
		if row.PrevValue != nil {
			set(5, *row.PrevValue)
		} else {
			set(5, "—")
		}
		if row.CurrValue != nil {
			set(6, *row.CurrValue)
		} else {
			set(6, "—")
		}
		if row.Delta != nil {
			set(7, *row.Delta)
		}
		if row.DeltaPct != nil {
			set(8, *row.DeltaPct/100) // format percent
		}
		set(9, verdictLabel[row.Verdict])
		set(10, row.PrevStatus)
		set(11, row.CurrStatus)
		set(12, row.Note)
		styleCell := ""
		switch row.Verdict {
		case "better":
			styleCell = "C" + fmt.Sprint(xl) + ":C" + fmt.Sprint(xl)
			_ = okStyle
			// warnai kolom verdict (kolom 9 = I)
			cell, _ := excelize.CoordinatesToCellName(9, xl)
			f.SetCellStyle("Perbandingan YoY", cell, cell, okStyle)
		case "worse":
			cell, _ := excelize.CoordinatesToCellName(9, xl)
			f.SetCellStyle("Perbandingan YoY", cell, cell, badStyle)
		case "flat":
			cell, _ := excelize.CoordinatesToCellName(9, xl)
			f.SetCellStyle("Perbandingan YoY", cell, cell, flatStyle)
		}
		_ = styleCell
	}
	for i := range headers {
		col, _ := excelize.ColumnNumberToName(i + 1)
		f.SetColWidth("Perbandingan YoY", col, col, 16)
	}
	return f, nil
}
