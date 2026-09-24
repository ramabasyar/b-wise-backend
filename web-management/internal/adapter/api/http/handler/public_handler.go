package handler

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"net/http"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/redis/go-redis/v9"

	"github.com/rama/b-wise/web-management/internal/domain/service"
)

// PublicHandler — API publik read-only BWM: tanpa auth, cache Redis per-URL
// (TTL 60s, di-flush saat mutasi konten) + ETag/304 utk efisiensi klien.
type PublicHandler struct {
	svc   *service.ContentService
	redis *redis.Client
}

func NewPublicHandler(svc *service.ContentService, r *redis.Client) *PublicHandler {
	return &PublicHandler{svc: svc, redis: r}
}

const pubCacheTTL = 60 * time.Second

// serve — pola umum: cek cache → fetch → simpan cache → balas dengan ETag.
func (h *PublicHandler) serve(c *gin.Context, fetch func() (any, error)) {
	ctx := context.Background()
	key := "bwm:pub:" + cacheKeyOf(c)
	if h.redis != nil {
		if b, err := h.redis.Get(ctx, key).Bytes(); err == nil && len(b) > 0 {
			h.writeWithETag(c, b)
			return
		}
	}
	data, err := fetch()
	if err != nil {
		errJSON(c, err)
		return
	}
	body, err := json.Marshal(gin.H{"success": true, "data": data})
	if err != nil {
		errJSON(c, err)
		return
	}
	if h.redis != nil {
		_ = h.redis.Set(ctx, key, body, pubCacheTTL).Err()
	}
	h.writeWithETag(c, body)
}

func (h *PublicHandler) writeWithETag(c *gin.Context, body []byte) {
	sum := sha256.Sum256(body)
	etag := `"` + hex.EncodeToString(sum[:16]) + `"`
	c.Header("ETag", etag)
	c.Header("Cache-Control", "public, max-age=60")
	if c.GetHeader("If-None-Match") == etag {
		c.Status(http.StatusNotModified)
		return
	}
	c.Data(http.StatusOK, "application/json; charset=utf-8", body)
}

func cacheKeyOf(c *gin.Context) string {
	raw := c.Request.URL.Path + "?" + c.Request.URL.RawQuery
	sum := sha256.Sum256([]byte(raw))
	return hex.EncodeToString(sum[:12])
}

// ==================== endpoints ====================

func (h *PublicHandler) Posts(c *gin.Context) {
	page, per := pageParams(c)
	typ := c.Query("type")
	locale := c.Query("locale")
	term := c.Query("term")
	q := c.Query("q")
	h.serve(c, func() (any, error) {
		items, total, err := h.svc.PublicPosts(locale, typ, term, q, page, per)
		if err != nil {
			return nil, err
		}
		return gin.H{"items": items, "total": total, "page": page, "per_page": per}, nil
	})
}

func (h *PublicHandler) PostBySlug(c *gin.Context) {
	slug := c.Param("slug")
	locale := c.Query("locale")
	h.serve(c, func() (any, error) {
		return h.svc.PublicPostBySlug(slug, locale)
	})
}

func (h *PublicHandler) Banners(c *gin.Context) {
	locale := c.Query("locale")
	h.serve(c, func() (any, error) {
		items, err := h.svc.PublicBanners(locale)
		if err != nil {
			return nil, err
		}
		return gin.H{"items": items, "total": len(items)}, nil
	})
}

func (h *PublicHandler) Events(c *gin.Context) {
	page, per := pageParams(c)
	locale := c.Query("locale")
	h.serve(c, func() (any, error) {
		items, total, err := h.svc.PublicEvents(locale, page, per)
		if err != nil {
			return nil, err
		}
		return gin.H{"items": items, "total": total, "page": page, "per_page": per}, nil
	})
}

func (h *PublicHandler) PageBySlug(c *gin.Context) {
	slug := c.Param("slug")
	locale := c.Query("locale")
	h.serve(c, func() (any, error) {
		return h.svc.PublicPageBySlug(slug, locale)
	})
}

func (h *PublicHandler) Documents(c *gin.Context) {
	h.serve(c, func() (any, error) {
		items, err := h.svc.PublicDocuments()
		if err != nil {
			return nil, err
		}
		return gin.H{"items": items, "total": len(items)}, nil
	})
}

// Terms — daftar kategori/tag (publik, + jumlah konten).
func (h *PublicHandler) Terms(c *gin.Context) {
	taxonomy := c.Query("taxonomy")
	h.serve(c, func() (any, error) {
		return h.svc.PublicTerms(taxonomy)
	})
}

// RelatedPosts — konten terkait berdasarkan overlap taksonomi.
func (h *PublicHandler) RelatedPosts(c *gin.Context) {
	slug := c.Param("slug")
	locale := c.Query("locale")
	h.serve(c, func() (any, error) {
		items, err := h.svc.PublicRelatedPosts(slug, locale, 3)
		if err != nil {
			return nil, err
		}
		return gin.H{"items": items}, nil
	})
}

// Sitemap — sitemap.xml konten tayang.
func (h *PublicHandler) Sitemap(c *gin.Context) {
	data, err := h.svc.SitemapXML()
	if err != nil {
		c.String(http.StatusInternalServerError, "sitemap error")
		return
	}
	c.Data(http.StatusOK, "application/xml; charset=utf-8", data)
}

// Feed — RSS 2.0 kabar kampus.
func (h *PublicHandler) Feed(c *gin.Context) {
	data, err := h.svc.RSSFeed()
	if err != nil {
		c.String(http.StatusInternalServerError, "feed error")
		return
	}
	c.Data(http.StatusOK, "application/rss+xml; charset=utf-8", data)
}

// RedirectLookup — GET /api/v1/public/redirects?path=/x — tujuan pemetaan utk web.
func (h *PublicHandler) RedirectLookup(c *gin.Context) {
	path := c.Query("path")
	h.serve(c, func() (any, error) {
		r, err := h.svc.LookupRedirect(path)
		if err != nil {
			return nil, err
		}
		return gin.H{"from_path": r.FromPath, "to_path": r.ToPath, "status_code": r.StatusCode}, nil
	})
}

// Preview — GET /api/v1/public/preview/:token?locale= — data draft utk web staging.
func (h *PublicHandler) Preview(c *gin.Context) {
	locale := c.DefaultQuery("locale", "id")
	h.serve(c, func() (any, error) {
		return h.svc.PreviewPost(c.Param("token"), locale)
	})
}
