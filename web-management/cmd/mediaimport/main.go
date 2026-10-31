package main

import (
	"context"
	"encoding/json"
	"flag"
	"log"
	"os"
	"path/filepath"
	"strings"

	"github.com/rama/b-wise/web-management/internal/adapter/config"
	"github.com/rama/b-wise/web-management/internal/adapter/database"
	"github.com/rama/b-wise/web-management/internal/adapter/storage"
	"github.com/rama/b-wise/web-management/internal/domain/service"
)

type mapEntry struct {
	URL    string `json:"url"`
	Thumb  string `json:"thumb,omitempty"`
	Medium string `json:"medium,omitempty"`
	Large  string `json:"large,omitempty"`
	Width  int    `json:"width"`
	Height int    `json:"height"`
}

func contentTypeOf(ext string) string {
	switch strings.ToLower(ext) {
	case ".webp":
		return "image/webp"
	case ".jpg", ".jpeg":
		return "image/jpeg"
	case ".png":
		return "image/png"
	case ".gif":
		return "image/gif"
	}
	return ""
}

func humanize(name string) string {
	base := strings.TrimSuffix(name, filepath.Ext(name))
	base = strings.NewReplacer("-", " ", "_", " ").Replace(base)
	return strings.TrimSpace(base)
}

func main() {
	dir := flag.String("dir", "", "direktori src/assets/Images web (wajib)")
	out := flag.String("out", "legacy-media-map.json", "file keluaran peta URL")
	dry := flag.Bool("dry-run", false, "hitung saja tanpa upload")
	actor := flag.String("actor", "legacy-import", "identitas actor di media_assets")
	flag.Parse()
	if *dir == "" {
		log.Fatal("-dir wajib diisi")
	}
	cfg, err := config.LoadEnv("configs/config.yaml")
	if err != nil {
		log.Fatalf("config: %v", err)
	}
	db, err := database.New(cfg.Database)
	if err != nil {
		log.Fatalf("db: %v", err)
	}
	defer database.Close(db)
	if cfg.MinIO.Endpoint == "" || cfg.MinIO.AccessKey == "" {
		log.Fatal("MinIO belum dikonfigurasi (MINIO_ENDPOINT/ACCESS)")
	}
	mio, err := storage.New(storage.Config{
		Endpoint: cfg.MinIO.Endpoint, AccessKey: cfg.MinIO.AccessKey, SecretKey: cfg.MinIO.SecretKey,
		Bucket: cfg.MinIO.Bucket, UseSSL: cfg.MinIO.UseSSL, PublicBase: cfg.MinIO.PublicBase,
	})
	if err != nil {
		log.Fatalf("minio: %v", err)
	}
	ctx := context.Background()
	if err := mio.EnsureBucket(ctx); err != nil {
		log.Fatalf("ensure bucket: %v", err)
	}
	svc := service.NewMediaService(db, mio)

	result := map[string]mapEntry{}
	created, existing, failed := 0, 0, 0
	var totalBytes int64
	var failures []string
	err = filepath.Walk(*dir, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		if info.IsDir() {
			return nil
		}
		ct := contentTypeOf(filepath.Ext(path))
		if ct == "" {
			return nil // svg/ico/dll tetap di repo
		}
		rel := strings.TrimPrefix(strings.TrimPrefix(path, *dir), "/")
		raw, err := os.ReadFile(path)
		if err != nil {
			failed++
			failures = append(failures, rel+": "+err.Error())
			return nil
		}
		if *dry {
			created++
			totalBytes += int64(len(raw))
			return nil
		}
		asset, wasNew, err := svc.ImportLegacy(ctx, raw, filepath.Base(path), ct, rel, humanize(filepath.Base(path)), *actor)
		if err != nil {
			failed++
			failures = append(failures, rel+": "+err.Error())
			return nil
		}
		if wasNew {
			created++
			totalBytes += int64(len(raw))
		} else {
			existing++
		}
		vm := asset.VariantsMap()
		e := mapEntry{URL: asset.URL, Width: asset.Width, Height: asset.Height}
		if v, ok := vm["thumb"]; ok {
			e.Thumb = v.URL
		}
		if v, ok := vm["medium"]; ok {
			e.Medium = v.URL
		}
		if v, ok := vm["large"]; ok {
			e.Large = v.URL
		}
		result[rel] = e
		if (created+existing)%50 == 0 {
			log.Printf("progres: %d diproses...", created+existing)
		}
		return nil
	})
	if err != nil {
		log.Fatalf("walk: %v", err)
	}
	if !*dry {
		buf, _ := json.MarshalIndent(result, "", "  ")
		if err := os.WriteFile(*out, buf, 0644); err != nil {
			log.Fatalf("tulis peta: %v", err)
		}
	}
	log.Printf("selesai: baru=%d sudah-ada=%d gagal=%d bytes=%d peta=%s", created, existing, failed, totalBytes, *out)
	for _, f := range failures {
		log.Printf("GAGAL: %s", f)
	}
	if failed > 0 {
		os.Exit(2)
	}
}
