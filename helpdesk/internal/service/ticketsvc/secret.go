package ticketsvc

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"fmt"
	"strings"
	"time"

	"github.com/rama/b-wise/helpdesk/internal/domain/entity"
)

// ===== Kanal rahasia & PPKPT (RH-02..RH-08, blueprint 6.3) =====

const encPrefix = "enc:v1:"

// SetSecretKey - kunci induk enkripsi (dari env, TIDAK di DB; akses DB saja
// tidak cukup untuk mendekripsi - RH-04).
func (s *Service) SetSecretKey(key string) {
	s.secretKey = []byte(key)
}

func (s *Service) hasSecretKey() bool {
	return len(s.secretKey) > 0
}

func (s *Service) ticketKey(ticketID string) []byte {
	h := sha256.Sum256(append(s.secretKey, []byte(ticketID)...))
	return h[:]
}

func (s *Service) sealText(ticketID, plain string) string {
	if !s.hasSecretKey() || plain == "" || strings.HasPrefix(plain, encPrefix) {
		return plain
	}
	block, err := aes.NewCipher(s.ticketKey(ticketID))
	if err != nil {
		return plain
	}
	gcm, _ := cipher.NewGCM(block)
	nonce := make([]byte, gcm.NonceSize())
	rand.Read(nonce)
	ct := gcm.Seal(nonce, nonce, []byte(plain), nil)
	return encPrefix + base64.StdEncoding.EncodeToString(ct)
}

func (s *Service) openText(ticketID, stored string) string {
	if !strings.HasPrefix(stored, encPrefix) {
		return stored
	}
	raw, err := base64.StdEncoding.DecodeString(strings.TrimPrefix(stored, encPrefix))
	if err != nil {
		return "[tidak terbaca]"
	}
	block, err := aes.NewCipher(s.ticketKey(ticketID))
	if err != nil {
		return "[tidak terbaca]"
	}
	gcm, _ := cipher.NewGCM(block)
	if len(raw) < gcm.NonceSize() {
		return "[tidak terbaca]"
	}
	pt, err := gcm.Open(nil, raw[:gcm.NonceSize()], raw[gcm.NonceSize():], nil)
	if err != nil {
		return "[tidak terbaca]"
	}
	return string(pt)
}

// hashTrackCode - kode lacak disimpan ter-hash (kebocoran DB tidak membocorkan kode).
func hashTrackCode(code string) string {
	h := sha256.Sum256([]byte("ppkpt:" + code))
	return hex.EncodeToString(h[:])
}

func newTrackCode() string {
	raw := make([]byte, 6)
	rand.Read(raw)
	return strings.ToUpper(hex.EncodeToString(raw))
}

// SecretReportView - tampilan aman pelacak: status & tahap TANPA isi/identitas (RH-05).
type SecretReportView struct {
	Number string `json:"number"`
	Status string `json:"status"`
	Stage  string `json:"stage"`
	At     string `json:"at"`
}

// CreateSecretReport - laporan rahasia (RH-03/RH-08): boleh anonim; isi terenkripsi;
// kode lacak dikembalikan SEKALI kepada pelapor.
func (s *Service) CreateSecretReport(p CreateParams, anonim bool) (*entity.Ticket, string, error) {
	if !s.hasSecretKey() {
		return nil, "", fmt.Errorf("%w: kunci enkripsi belum dikonfigurasi", ErrValidation)
	}
	if strings.TrimSpace(p.Subject) == "" {
		return nil, "", fmt.Errorf("%w: isi laporan wajib", ErrValidation)
	}
	requester := p.RequesterID
	if anonim {
		requester = "anon"
	}
	teamID := ""
	flow := ""
	if p.ServiceID != "" {
		var svc entity.Service
		if err := s.db.Where("id = ? AND status = ?", p.ServiceID, "published").First(&svc).Error; err != nil {
			return nil, "", fmt.Errorf("%w: layanan tidak tersedia", ErrNotFound)
		}
		teamID = svc.TeamID
		flow = svc.Flow
	} else {
		var fo entity.Team
		if err := s.db.Where("is_front_office = ? AND is_active = ?", true, true).First(&fo).Error; err != nil {
			return nil, "", fmt.Errorf("%w: tim front office belum ada", ErrNotFound)
		}
		teamID = fo.ID
	}
	number, err := s.nextNumber()
	if err != nil {
		return nil, "", err
	}
	code := newTrackCode()
	idRaw := make([]byte, 16)
	rand.Read(idRaw)
	t := entity.Ticket{
		Base:            entity.Base{ID: hex.EncodeToString(idRaw)},
		Number:          number,
		Type:            "complaint",
		ServiceID:       p.ServiceID,
		TeamID:          teamID,
		RequesterUserID: requester,
		Channel:         "rahasia",
		Subject:         "Laporan rahasia",
		Answers:         "{}",
		Status:          "baru",
		Confidentiality: "rahasia",
		Flow:            flow,
		TrackCodeHash:   hashTrackCode(code),
	}
	if flow == "ppkpt" {
		t.PenangananStage = "lapor_diterima"
	}
	t.Subject = s.sealText(t.ID, p.Subject)
	if err := s.db.Create(&t).Error; err != nil {
		return nil, "", err
	}
	body := p.Subject
	if strings.TrimSpace(p.Answers) != "" && p.Answers != "{}" {
		body += "\n" + p.Answers
	}
	_ = s.addSealedEvent(t.ID, "laporan", body)
	if !anonim && p.RequesterID != "" {
		s.notifyInApp(p.RequesterID, "status_tiket", &t, "Laporan rahasia diterima", "Laporan Anda tercatat. Simpan kode lacak untuk memantau pembaruan.")
	}
	return &t, code, nil
}

// addSealedEvent - event ber-isi terenkripsi utk tiket rahasia.
func (s *Service) addSealedEvent(ticketID, kind, body string) error {
	ev := entity.TicketEvent{TicketID: ticketID, Kind: kind, Visibility: "internal", Body: s.sealText(ticketID, body), Meta: "{}"}
	return s.db.Create(&ev).Error
}

// SecretTrack - lacak via kode: status+tahap saja, tanpa isi (RH-05).
func (s *Service) SecretTrack(code string) (*SecretReportView, error) {
	var t entity.Ticket
	if err := s.db.Where("track_code_hash = ?", hashTrackCode(strings.TrimSpace(code))).First(&t).Error; err != nil {
		return nil, fmt.Errorf("%w: kode lacak tidak dikenal", ErrNotFound)
	}
	_ = s.db.Create(&entity.TicketAccessLog{TicketID: t.ID, Action: "track", Detail: "pelacakan via kode"}).Error
	return &SecretReportView{Number: t.Number, Status: t.Status, Stage: t.PenangananStage, At: t.UpdatedAt.Format("2006-01-02 15:04")}, nil
}

// OpenSecret - dekripsi teks tiket (ekspor utk handler; hanya utk anggota tim).
func (s *Service) OpenSecret(ticketID, stored string) string {
	return s.openText(ticketID, stored)
}

// LogSecretUser - catat akses user (ekspor utk handler).
func (s *Service) LogSecretUser(ticketID, userID, action string) {
	_ = s.logAccessUser(ticketID, userID, action)
}

// logAccessUser - catat akses tiket rahasia oleh user (RH-06).
func (s *Service) logAccessUser(ticketID, userID, action string) error {
	return s.db.Create(&entity.TicketAccessLog{TicketID: ticketID, UserID: userID, Action: action}).Error
}

// ppkptStages - alur penanganan Satgas (RH-02).
var ppkptStages = map[string]bool{"lapor_diterima": true, "pemeriksaan": true, "kesimpulan_rekomendasi": true, "pemulihan": true, "selesai": true}

// SetStage - transisi tahap penanganan PPKPT; hanya anggota tim pemilik.
func (s *Service) SetStage(ticketID, userID, stage string) (*entity.Ticket, error) {
	if !ppkptStages[stage] {
		return nil, fmt.Errorf("%w: tahap tidak dikenal", ErrValidation)
	}
	var t entity.Ticket
	if err := s.db.Where("id = ?", ticketID).First(&t).Error; err != nil {
		return nil, ErrNotFound
	}
	if !s.IsMember(t.TeamID, userID) {
		return nil, ErrForbidden
	}
	if err := s.db.Model(&t).Update("penanganan_stage", stage).Error; err != nil {
		return nil, err
	}
	if stage == "selesai" {
		_ = s.db.Model(&t).Updates(map[string]interface{}{"status": "selesai", "resolved_at": s.now()}).Error
	}
	_ = s.addSealedEvent(ticketID, "tahap", "tahap penanganan: "+stage)
	_ = s.logAccessUser(ticketID, userID, "stage:"+stage)
	_ = s.db.Where("id = ?", ticketID).First(&t).Error
	return &t, nil
}

// AccessLogs - jejak akses tiket utk audit tim (RH-06).
func (s *Service) AccessLogs(ticketID, userID string) ([]entity.TicketAccessLog, error) {
	var t entity.Ticket
	if err := s.db.Where("id = ?", ticketID).First(&t).Error; err != nil {
		return nil, ErrNotFound
	}
	if !s.IsMember(t.TeamID, userID) {
		return nil, ErrForbidden
	}
	var logs []entity.TicketAccessLog
	err := s.db.Where("ticket_id = ?", ticketID).Order("created_at DESC").Limit(100).Find(&logs).Error
	return logs, err
}

// SecretAggregate - statistik rahasia TANPA identitas (RH-07).
type SecretAggregate struct {
	Total    int64            `json:"total"`
	PerStage map[string]int64 `json:"per_stage"`
	PerBulan []struct {
		Bulan string `json:"bulan"`
		N     int64  `json:"n"`
	} `json:"per_bulan"`
}

func (s *Service) SecretAggregate(from, to time.Time) (*SecretAggregate, error) {
	a := &SecretAggregate{PerStage: map[string]int64{}}
	s.db.Model(&entity.Ticket{}).Where("confidentiality = ? AND created_at >= ? AND created_at <= ?", "rahasia", from, to).Count(&a.Total)
	rows := []struct {
		Stage string
		N     int64
	}{}
	s.db.Model(&entity.Ticket{}).Select("penanganan_stage as stage, count(*) as n").
		Where("confidentiality = ? AND created_at >= ? AND created_at <= ?", "rahasia", from, to).
		Group("penanganan_stage").Scan(&rows)
	for _, r := range rows {
		k := r.Stage
		if k == "" {
			k = "(umum)"
		}
		a.PerStage[k] = r.N
	}
	s.db.Raw("SELECT to_char(created_at, 'YYYY-MM') AS bulan, count(*) AS n FROM tickets WHERE confidentiality = 'rahasia' AND created_at >= ? AND created_at <= ? GROUP BY 1 ORDER BY 1", from, to).Scan(&a.PerBulan)
	return a, nil
}
