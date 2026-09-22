// Package storage — adapter MinIO utk media library BWM (bucket bwm-media).
package storage

import (
	"context"
	"fmt"
	"io"
	"strings"

	"github.com/minio/minio-go/v7"
	"github.com/minio/minio-go/v7/pkg/credentials"
)

type Config struct {
	Endpoint   string // host:port (tanpa scheme)
	AccessKey  string
	SecretKey  string
	Bucket     string
	UseSSL     bool
	PublicBase string // base URL publik utk menyusun URL file (mis. http://localhost:9000); kosong = dari endpoint
}

// MinIO — client wrapper tipis: ensure bucket, put, delete, public URL.
type MinIO struct {
	cli *minio.Client
	cfg Config
}

func New(cfg Config) (*MinIO, error) {
	cli, err := minio.New(cfg.Endpoint, &minio.Options{
		Creds:  credentials.NewStaticV4(cfg.AccessKey, cfg.SecretKey, ""),
		Secure: cfg.UseSSL,
	})
	if err != nil {
		return nil, err
	}
	return &MinIO{cli: cli, cfg: cfg}, nil
}

// EnsureBucket — buat bucket bila belum ada + policy public-read (download)
// supaya web publik bisa memuat gambar langsung tanpa presign.
func (m *MinIO) EnsureBucket(ctx context.Context) error {
	ok, err := m.cli.BucketExists(ctx, m.cfg.Bucket)
	if err != nil {
		return err
	}
	if !ok {
		if err := m.cli.MakeBucket(ctx, m.cfg.Bucket, minio.MakeBucketOptions{}); err != nil {
			return err
		}
	}
	policy := fmt.Sprintf(`{"Version":"2012-10-17","Statement":[{"Effect":"Allow","Principal":{"AWS":["*"]},"Action":["s3:GetObject"],"Resource":["arn:aws:s3:::%s/*"]}]}`, m.cfg.Bucket)
	return m.cli.SetBucketPolicy(ctx, m.cfg.Bucket, policy)
}

// Put — upload objek; key = path di bucket.
func (m *MinIO) Put(ctx context.Context, key string, r io.Reader, size int64, contentType string) error {
	_, err := m.cli.PutObject(ctx, m.cfg.Bucket, key, r, size, minio.PutObjectOptions{ContentType: contentType})
	return err
}

// Delete — hapus objek (best-effort; object hilang ≠ fatal).
func (m *MinIO) Delete(ctx context.Context, key string) error {
	return m.cli.RemoveObject(ctx, m.cfg.Bucket, key, minio.RemoveObjectOptions{})
}

// PublicURL — URL publik utk object key: {base}/{bucket}/{key}.
func (m *MinIO) PublicURL(key string) string {
	base := strings.TrimRight(m.cfg.PublicBase, "/")
	if base == "" {
		scheme := "http"
		if m.cfg.UseSSL {
			scheme = "https"
		}
		base = scheme + "://" + m.cfg.Endpoint
	}
	return base + "/" + m.cfg.Bucket + "/" + key
}

func (m *MinIO) Bucket() string { return m.cfg.Bucket }
