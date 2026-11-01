package ticketsvc

// ==================== LP-08: Deteksi Masalah Berulang (F4) ====================
// V1 statistik: tiket selesai dikelompokkan per tim + awalan subjek
// ternormalisasi; berulang >= ambang = kandidat masalah sistemik (problem
// management: cari akar, bukan tangani tiket satu-satu).

// RecurringProblems - pola tiket berulang (jendela hari, ambang jumlah).
func (s *Service) RecurringProblems(days, minCount int) ([]map[string]any, error) {
	if days <= 0 || days > 365 {
		days = 60
	}
	if minCount <= 0 {
		minCount = 3
	}
	sql := `SELECT COALESCE(t.name, '-') AS team_name,
		lower(left(tk.subject, 40)) AS subject_gist,
		count(*)::int AS count,
		max(tk.updated_at)::text AS last_at
		FROM tickets tk LEFT JOIN teams t ON t.id = tk.team_id
		WHERE tk.status = ? AND tk.updated_at >= now() - make_interval(days => ?)
		GROUP BY 1, 2 HAVING count(*) >= ? ORDER BY count DESC LIMIT 25`
	var rows []map[string]any
	if err := s.db.Raw(sql, "selesai", days, minCount).Scan(&rows).Error; err != nil {
		return nil, err
	}
	return rows, nil
}
