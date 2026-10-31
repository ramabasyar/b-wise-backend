package ticketsvc

// CatalogPublic — item katalog untuk portal pemohon.
type CatalogPublic struct {
	ID           string `json:"id"`
	Slug         string `json:"slug"`
	Name         string `json:"name"`
	Description  string `json:"description"`
	TeamCode     string `json:"team_code"`
	TeamName     string `json:"team_name"`
	CategoryName string `json:"category_name"`
	TicketType   string `json:"ticket_type"`
	Requirements string `json:"requirements"`
	Procedure    string `json:"procedure"`
	FeeNote      string `json:"fee_note"`
	OutputNote   string `json:"output_note"`
}

// Catalog — seluruh layanan berstatus published dari tim aktif (pemohon, tanpa permission).
func (s *Service) Catalog() ([]CatalogPublic, error) {
	var rows []CatalogPublic
	err := s.db.Table("services").
		Select("services.id, services.slug, services.name, services.description, services.ticket_type, services.requirements, services.procedure, services.fee_note, services.output_note, teams.code as team_code, teams.name as team_name, service_categories.name as category_name").
		Joins("JOIN teams ON teams.id = services.team_id AND teams.is_active = ?", true).
		Joins("LEFT JOIN service_categories ON service_categories.id = services.category_id").
		Where("services.status = ?", "published").
		Order("service_categories.sort ASC, services.name ASC").
		Scan(&rows).Error
	return rows, err
}
