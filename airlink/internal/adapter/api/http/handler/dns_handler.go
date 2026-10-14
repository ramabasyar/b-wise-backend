package handler

import (
	"database/sql"
	"errors"
	"net/http"
	"strconv"
	"time"

	"airlink/internal/domain/service"
	"github.com/gin-gonic/gin"
)

type DNSHandler struct {
	dns    *service.DNSService
	radius *service.RadiusService
}

func NewDNSHandler(dns *service.DNSService, radius *service.RadiusService) *DNSHandler {
	return &DNSHandler{dns: dns, radius: radius}
}
func (h *DNSHandler) unavailable(c *gin.Context) bool {
	if h.dns == nil {
		c.JSON(503, gin.H{"error": "DNS database unavailable"})
		return true
	}
	return false
}

func (h *DNSHandler) List(c *gin.Context) {
	if h.unavailable(c) {
		return
	}
	data, err := h.dns.ListRecords(c.Request.Context())
	if err != nil {
		c.JSON(500, gin.H{"error": "gagal membaca DNS records"})
		return
	}
	c.JSON(200, gin.H{"data": data})
}

func (h *DNSHandler) Save(c *gin.Context) {
	if h.unavailable(c) {
		return
	}
	var r service.DNSRecord
	if err := c.ShouldBindJSON(&r); err != nil {
		c.JSON(400, gin.H{"error": "payload tidak valid"})
		return
	}
	if id := c.Param("id"); id != "" {
		n, err := strconv.ParseInt(id, 10, 64)
		if err != nil || n < 1 {
			c.JSON(400, gin.H{"error": "id tidak valid"})
			return
		}
		r.ID = n
	} else {
		r.ID = 0
	}
	if err := h.dns.SaveRecord(c.Request.Context(), &r, actorID(c)); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			c.JSON(404, gin.H{"error": "record tidak ditemukan"})
			return
		}
		if err.Error() == "nama harus subdomain valid dari binawan.ac.id" || err.Error() == "jenis record hanya A, AAAA, CNAME, atau TXT" || err.Error() == "TTL harus 60–86400 detik" || err.Error() == "IPv4 tidak valid" || err.Error() == "IPv6 tidak valid" || err.Error() == "target CNAME tidak valid" || err.Error() == "isi TXT tidak valid" || err.Error() == "CNAME tidak boleh digabung dengan tipe record lain pada nama yang sama" {
			c.JSON(400, gin.H{"error": err.Error()})
			return
		}
		c.JSON(409, gin.H{"error": "record gagal disimpan; periksa duplikat atau konflik tipe"})
		return
	}
	if err := h.dns.Sync(c.Request.Context()); err != nil {
		c.JSON(202, gin.H{"data": r, "warning": "Record tersimpan tetapi DNS belum tersinkron. Tekan Sinkronkan Ulang."})
		return
	}
	c.JSON(200, gin.H{"data": r})
}

func (h *DNSHandler) Delete(c *gin.Context) {
	if h.unavailable(c) {
		return
	}
	id, err := strconv.ParseInt(c.Param("id"), 10, 64)
	if err != nil || id < 1 {
		c.JSON(400, gin.H{"error": "id tidak valid"})
		return
	}
	if err := h.dns.DeleteRecord(c.Request.Context(), id, actorID(c)); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			c.JSON(404, gin.H{"error": "record tidak ditemukan"})
		} else {
			c.JSON(500, gin.H{"error": "hapus record gagal"})
		}
		return
	}
	if err := h.dns.Sync(c.Request.Context()); err != nil {
		c.JSON(202, gin.H{"warning": "Record terhapus tetapi DNS belum tersinkron. Tekan Sinkronkan Ulang."})
		return
	}
	c.JSON(200, gin.H{"status": "ok"})
}

func (h *DNSHandler) Sync(c *gin.Context) {
	if h.unavailable(c) {
		return
	}
	if err := h.dns.Sync(c.Request.Context()); err != nil {
		c.JSON(502, gin.H{"error": "sinkronisasi DNS gagal"})
		return
	}
	c.JSON(200, gin.H{"status": "ok"})
}

func activitySince(c *gin.Context) time.Time {
	days, _ := strconv.Atoi(c.DefaultQuery("days", "7"))
	if days < 1 || days > 90 {
		days = 7
	}
	return time.Now().AddDate(0, 0, -days)
}
func activityLimit(c *gin.Context) int {
	n, _ := strconv.Atoi(c.DefaultQuery("limit", "100"))
	if n < 1 || n > 200 {
		return 100
	}
	return n
}

func (h *DNSHandler) Queries(c *gin.Context) {
	if h.unavailable(c) {
		return
	}
	rows, err := h.dns.Queries(c.Request.Context(), activitySince(c), activityLimit(c))
	if err != nil {
		c.JSON(500, gin.H{"error": "gagal membaca query DNS"})
		return
	}
	if h.radius != nil {
		for i := range rows {
			rows[i].Username, _ = h.radius.FindUserForIPAt(rows[i].SourceIP, rows[i].Time)
		}
	}
	c.JSON(http.StatusOK, gin.H{"data": rows, "note": "Query DNS bukan bukti situs dikunjungi; DoH dan resolver lain tidak tercatat."})
}

func (h *DNSHandler) Flows(c *gin.Context) {
	if h.unavailable(c) {
		return
	}
	rows, err := h.dns.Flows(c.Request.Context(), activitySince(c), activityLimit(c))
	if err != nil {
		c.JSON(500, gin.H{"error": "gagal membaca NetFlow"})
		return
	}
	if h.radius != nil {
		for i := range rows {
			rows[i].Username, _ = h.radius.FindUserForIPAt(rows[i].SourceIP, rows[i].Time)
		}
	}
	c.JSON(http.StatusOK, gin.H{"data": rows, "note": "Flow menunjukkan IP tujuan, port dan volume, bukan URL atau isi trafik."})
}

func (h *DNSHandler) Summary(c *gin.Context) {
	if h.unavailable(c) {
		return
	}
	data, err := h.dns.Summary(c.Request.Context(), activitySince(c))
	if err != nil {
		c.JSON(500, gin.H{"error": "gagal memuat ringkasan aktivitas"})
		return
	}
	c.JSON(200, gin.H{"data": data})
}
