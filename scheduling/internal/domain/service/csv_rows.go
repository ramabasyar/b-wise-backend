package service

import (
	"encoding/csv"
	"fmt"
	"io"
	"strings"
)

// ReadCSV — baca seluruh file CSV menjadi slice map[header]nilai.
// Header = baris pertama (case-insensitive, spasi di-trim). Baris kosong dilompati.
func ReadCSV(r io.Reader) ([]map[string]string, error) {
	cr := csv.NewReader(r)
	cr.TrimLeadingSpace = true
	cr.FieldsPerRecord = -1 // toleran kolom tak seragam
	header := map[string]int{}
	rows := []map[string]string{}
	i := 0
	for {
		rec, err := cr.Read()
		if err == io.EOF {
			break
		}
		if err != nil {
			return nil, fmt.Errorf("baris %d: %w", i+1, err)
		}
		if i == 0 {
			for c, h := range rec {
				header[strings.ToLower(strings.TrimSpace(h))] = c
			}
			i++
			continue
		}
		i++
		if len(rec) == 0 || strings.TrimSpace(strings.Join(rec, "")) == "" {
			continue
		}
		row := map[string]string{}
		for h, c := range header {
			if c < len(rec) {
				row[h] = strings.TrimSpace(rec[c])
			}
		}
		rows = append(rows, row)
	}
	return rows, nil
}
