package service

import (
	"bytes"
	"fmt"
	"strings"
	"time"

	entity "github.com/rama/b-wise/scheduling/internal/domain/entity"
	"github.com/xuri/excelize/v2"
)

// ==================== EXPORT & CALENDAR (F4) ====================

const (
	xHdrFill = "FF14532D"
	xOkFill  = "FFEEF6F0"
)

var dayName = map[int]string{1: "Senin", 2: "Selasa", 3: "Rabu", 4: "Kamis", 5: "Jumat", 6: "Sabtu", 7: "Minggu"}

// ExportXLSX — jadwal per semester. view: master|dosen|rombel|ruang.
// Sumber: versi published (fallback draft bila belum publish).
func (s *SolveService) ExportXLSX(termID, view string) (*excelize.File, string, error) {
	entries, err := s.PublishedEntries(termID)
	if err != nil || len(entries) == 0 {
		entries, err = s.Entries(termID)
		if err != nil {
			return nil, "", err
		}
	}
	// enrich labels
	roomCodes := map[string]string{}
	{
		var rooms []entity.Room
		s.db.Find(&rooms)
		for _, r := range rooms {
			roomCodes[r.ID] = r.Code
		}
	}
	groupCodes := map[string]string{}
	lectNames := map[string]string{}
	{
		var gs []entity.ClassGroup
		s.db.Find(&gs)
		for _, g := range gs {
			groupCodes[g.ID] = g.Code
		}
		var ls []entity.Lecturer
		s.db.Find(&ls)
		for _, l := range ls {
			lectNames[l.ID] = l.Name
		}
	}
	var offerings []entity.Offering
	s.db.Preload("Course").Preload("Lecturer").Preload("ClassGroup").Where("term_id = ?", termID).Find(&offerings)
	offByID := map[string]entity.Offering{}
	for _, o := range offerings {
		offByID[o.ID] = o
	}

	f := excelize.NewFile()
	hstyle, _ := f.NewStyle(&excelize.Style{
		Fill: excelize.Fill{Type: "pattern", Color: []string{xHdrFill}, Pattern: 1},
		Font: &excelize.Font{Color: "FFFFFFFF", Bold: true},
	})
	sheet := "Jadwal"
	f.SetSheetName("Sheet1", sheet)

	headers := []string{"Hari", "Jam", "Kode MK", "Mata Kuliah", "SKS", "Rombel", "Dosen", "Ruang"}
	for i, h := range headers {
		cell, _ := excelize.CoordinatesToCellName(i+1, 1)
		f.SetCellValue(sheet, cell, h)
		f.SetCellStyle(sheet, cell, cell, hstyle)
	}
	row := 2
	for _, e := range entries {
		set := func(col int, v any) {
			cell, _ := excelize.CoordinatesToCellName(col, row)
			f.SetCellValue(sheet, cell, v)
		}
		o := offByID[e.OfferingID]
		cname, ccode, sks := "", e.CourseCode, 0
		if o.Course != nil {
			cname, ccode, sks = o.Course.Name, o.Course.Code, o.Course.Sks
		}
		lect := lectNames[e.LecturerID]
		if lect == "" && o.Lecturer != nil {
			lect = o.Lecturer.Name
		}
		set(1, dayName[e.Day])
		set(2, fmt.Sprintf("%s-%s", e.StartTime, e.EndTime))
		set(3, ccode)
		set(4, cname)
		set(5, sks)
		set(6, groupCodes[e.GroupID])
		set(7, lect)
		set(8, roomCodes[e.RoomID])
		row++
	}
	for i := range headers {
		col, _ := excelize.ColumnNumberToName(i + 1)
		f.SetColWidth(sheet, col, col, 18)
	}

	// ringkasan per view
	switch view {
	case "dosen", "rombel", "ruang":
		key := func(e entity.TimetableEntry) string {
			switch view {
			case "dosen":
				return lectNames[e.LecturerID]
			case "rombel":
				return groupCodes[e.GroupID]
			default:
				return roomCodes[e.RoomID]
			}
		}
		grouped := map[string][]entity.TimetableEntry{}
		var order []string
		for _, e := range entries {
			k := key(e)
			if _, ok := grouped[k]; !ok {
				order = append(order, k)
			}
			grouped[k] = append(grouped[k], e)
		}
		for _, k := range order {
			if k == "" {
				continue
			}
			safe := strings.ReplaceAll(strings.ReplaceAll(k, "/", "-"), ":", "-")
			if len(safe) > 28 {
				safe = safe[:28]
			}
			const sh = ""
			_ = sh
			f.NewSheet(safe)
			for i, h := range headers {
				cell, _ := excelize.CoordinatesToCellName(i+1, 1)
				f.SetCellValue(safe, cell, h)
				f.SetCellStyle(safe, cell, cell, hstyle)
			}
			r := 2
			for _, e := range grouped[k] {
				o := offByID[e.OfferingID]
				cname := ""
				if o.Course != nil {
					cname = o.Course.Name
				}
				lect := lectNames[e.LecturerID]
				if lect == "" && o.Lecturer != nil {
					lect = o.Lecturer.Name
				}
				vals := []any{dayName[e.Day], fmt.Sprintf("%s-%s", e.StartTime, e.EndTime), e.CourseCode, cname, groupCodes[e.GroupID], lect, roomCodes[e.RoomID]}
				for i, v := range vals {
					cell, _ := excelize.CoordinatesToCellName(i+1, r)
					f.SetCellValue(safe, cell, v)
				}
				r++
			}
			for i := range headers[:len(headers)-1] {
				col, _ := excelize.ColumnNumberToName(i + 1)
				f.SetColWidth(safe, col, col, 18)
			}
		}
	}
	title := fmt.Sprintf("jadwal-%s-%s.xlsx", view, time.Now().Format("20060102"))
	return f, title, nil
}

// BuildICS — feed kalender standar (Google Calendar/Apple). scope: room|group|lecturer.
func (s *SolveService) BuildICS(termID, scope, scopeID string) (string, error) {
	entries, err := s.PublishedEntries(termID)
	if err != nil || len(entries) == 0 {
		entries, err = s.Entries(termID) // fallback draft (preload lengkap utk view)
		if err != nil {
			return "", err
		}
	}
	var term entity.Term
	s.db.Where("id = ?", termID).First(&term)

	var rooms []entity.Room
	s.db.Find(&rooms)
	roomByID := map[string]entity.Room{}
	for _, r := range rooms {
		roomByID[r.ID] = r
	}
	var gs []entity.ClassGroup
	s.db.Find(&gs)
	groupByID := map[string]entity.ClassGroup{}
	for _, g := range gs {
		groupByID[g.ID] = g
	}

	var b bytes.Buffer
	b.WriteString("BEGIN:VCALENDAR\r\nVERSION:2.0\r\nPRODID:-//B-Wise Scheduling//ID\r\nCALSCALE:GREGORIAN\r\n")
	// minggu pertama semester sebagai basis repeat
	base := term.StartDate
	weeks := 16
	if term.EndDate.After(term.StartDate) {
		w := int(term.EndDate.Sub(term.StartDate).Hours() / 24 / 7)
		if w > 0 {
			weeks = w
		}
	}
	n := 0
	for _, e := range entries {
		switch scope {
		case "room":
			if e.RoomID != scopeID {
				continue
			}
		case "group":
			if e.GroupID != scopeID {
				continue
			}
		case "lecturer":
			if e.LecturerID != scopeID {
				continue
			}
		}
		// offset hari dari Senin minggu basis
		// weekday: Senin offset 0
		firstMonday := base
		wd := int(base.Weekday())
		if wd == 0 {
			wd = 7
		}
		firstMonday = base.AddDate(0, 0, 1-wd)
		start := firstMonday.AddDate(0, 0, e.Day-1)
		st, _ := time.Parse("15:04", e.StartTime)
		en, _ := time.Parse("15:04", e.EndTime)
		evStart := time.Date(start.Year(), start.Month(), start.Day(), st.Hour(), st.Minute(), 0, 0, time.Local)
		evEnd := time.Date(start.Year(), start.Month(), start.Day(), en.Hour(), en.Minute(), 0, 0, time.Local)

		room := roomByID[e.RoomID]
		group := groupByID[e.GroupID]
		gname := group.Code
		if gname == "" && group.Name != "" {
			gname = group.Name
		}
		summary := fmt.Sprintf("%s — %s", e.CourseCode, gname)
		loc := room.Code
		if room.Name != "" {
			loc = fmt.Sprintf("%s (%s)", room.Code, room.Name)
		}

		b.WriteString("BEGIN:VEVENT\r\n")
		b.WriteString(fmt.Sprintf("UID:%s-%s@bwise\r\n", e.ID, termID[:8]))
		b.WriteString(fmt.Sprintf("DTSTAMP:%s\r\n", time.Now().UTC().Format("20060102T150405Z")))
		b.WriteString(fmt.Sprintf("DTSTART:%s\r\n", evStart.Format("20060102T150405")))
		b.WriteString(fmt.Sprintf("DTEND:%s\r\n", evEnd.Format("20060102T150405")))
		b.WriteString(fmt.Sprintf("RRULE:FREQ=WEEKLY;COUNT=%d\r\n", weeks))
		b.WriteString(fmt.Sprintf("SUMMARY:%s\r\n", icsEscape(summary)))
		b.WriteString(fmt.Sprintf("LOCATION:%s\r\n", icsEscape(loc)))
		b.WriteString("END:VEVENT\r\n")
		n++
	}
	b.WriteString("END:VCALENDAR\r\n")
	if n == 0 {
		return "", ErrNoData
	}
	return b.String(), nil
}

func icsEscape(s string) string {
	s = strings.ReplaceAll(s, "\\", "\\\\")
	s = strings.ReplaceAll(s, ";", "\\;")
	s = strings.ReplaceAll(s, ",", "\\,")
	s = strings.ReplaceAll(s, "\n", "\\n")
	return s
}
