package handler

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"net/http"
	"os"

	"github.com/gin-gonic/gin"

	"github.com/rama/b-wise/library/pkg/response"
)

const visitorAppFilename = "library_visitor.exe"

// visitorAppPath — lokasi file aplikasi kiosk Windows (library_visitor.exe).
// File di-deploy terpisah dari binary (drop-in update tanpa rebuild service).
// Default relatif CWD service; override via env LIBRARY_VISITOR_APP_PATH.
func visitorAppPath() string {
	if p := os.Getenv("LIBRARY_VISITOR_APP_PATH"); p != "" {
		return p
	}
	return "static/" + visitorAppFilename
}

// DownloadVisitorApp — GET /api/library/v1/downloads/visitor-app
// Guard: library.kiosks.manage. Mengembalikan file exe aplikasi kiosk
// beserta header X-Content-Sha256 untuk verifikasi integritas.
// 404 bila file belum diunggah ke server.
func (h *LibraryHandler) DownloadVisitorApp(c *gin.Context) {
	path := visitorAppPath()
	st, err := os.Stat(path)
	if err != nil || st.IsDir() {
		response.NotFound(c, "file aplikasi kiosk belum tersedia di server (minta admin mengunggah "+visitorAppFilename+")")
		return
	}
	data, err := os.ReadFile(path)
	if err != nil {
		response.Error(c, err)
		return
	}
	digest := sha256.Sum256(data)
	c.Header("X-Content-Sha256", hex.EncodeToString(digest[:]))
	c.DataFromReader(
		http.StatusOK,
		int64(len(data)),
		"application/octet-stream",
		bytes.NewReader(data),
		map[string]string{"Content-Disposition": "attachment; filename=\"" + visitorAppFilename + "\""},
	)
}
