// Package storage — adapter MinIO utk gambar KB helpdesk (bucket helpdesk-kb).
// Pola mengikuti web-management; rute penyajian tetap lewat API service
// (browser tidak dapat menjangkau IP internal MinIO langsung).
package storage

import (
	"context"
	"fmt"
	"io"

	"github.com/minio/minio-go/v7"
	"github.com/minio/minio-go/v7/pkg/credentials"
)

type Config struct {
	Endpoint  string // host:port tanpa scheme
	AccessKey string
	SecretKey string
	Bucket    string
	UseSSL    bool
}

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

// EnsureBucket — buat bucket bila belum ada (privat; diunduh lewat service).
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
	return nil
}

// Put — simpan objek (key = path di bucket).
func (m *MinIO) Put(ctx context.Context, key string, r io.Reader, size int64, contentType string) error {
	_, err := m.cli.PutObject(ctx, m.cfg.Bucket, key, r, size, minio.PutObjectOptions{ContentType: contentType})
	return err
}

// Get — buka objek utk distream (caller menutup body); *minio.Object
// memenuhi io.ReadSeekCloser sehingga bisa langsung dipakai http.ServeContent.
func (m *MinIO) Get(ctx context.Context, key string) (*minio.Object, error) {
	obj, err := m.cli.GetObject(ctx, m.cfg.Bucket, key, minio.GetObjectOptions{})
	if err != nil {
		return nil, err
	}
	// Stat utk memastikan objek benar-benar ada (GetObject lazy).
	if _, err := obj.Stat(); err != nil {
		obj.Close()
		return nil, fmt.Errorf("objek tidak ada: %w", err)
	}
	return obj, nil
}
