// Package storage — abstraksi penyimpanan evidence (blueprint: filesystem lokal
// dulu, swap ke MinIO via konfigurasi S3_*, tanpa refactor).
package storage

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"

	"github.com/aws/aws-sdk-go-v2/aws"
	awsconfig "github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/credentials"
	"github.com/aws/aws-sdk-go-v2/service/s3"
)

var ErrNotFound = errors.New("file tidak ditemukan")

type Driver string

const (
	DriverLocal Driver = "local"
	DriverS3    Driver = "s3"
)

type FileObject struct {
	Content  []byte
	FileName string
	FileType string
	Size     int64
}

type Storage interface {
	Put(ctx context.Context, key string, obj FileObject) error
	Get(ctx context.Context, key string) (*FileObject, error)
	Delete(ctx context.Context, key string) error
	Driver() Driver
}

// ==================== LOCAL (dev) ====================

type LocalStorage struct{ baseDir string }

func NewLocalStorage(baseDir string) (*LocalStorage, error) {
	if baseDir == "" {
		baseDir = "./data/evidence"
	}
	if err := os.MkdirAll(baseDir, 0o750); err != nil {
		return nil, err
	}
	return &LocalStorage{baseDir: baseDir}, nil
}

func (l *LocalStorage) safe(key string) (string, error) {
	clean := filepath.Clean(key)
	if strings.HasPrefix(clean, "..") || filepath.IsAbs(clean) {
		return "", fmt.Errorf("key invalid: %s", key)
	}
	return filepath.Join(l.baseDir, clean), nil
}

func (l *LocalStorage) Put(_ context.Context, key string, obj FileObject) error {
	path, err := l.safe(key)
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o750); err != nil {
		return err
	}
	return os.WriteFile(path, obj.Content, 0o640)
}

func (l *LocalStorage) Get(_ context.Context, key string) (*FileObject, error) {
	path, err := l.safe(key)
	if err != nil {
		return nil, err
	}
	data, err := os.ReadFile(path)
	if os.IsNotExist(err) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, err
	}
	return &FileObject{Content: data, FileName: filepath.Base(path), Size: int64(len(data))}, nil
}

func (l *LocalStorage) Delete(_ context.Context, key string) error {
	path, err := l.safe(key)
	if err != nil {
		return err
	}
	if err := os.Remove(path); err != nil && !os.IsNotExist(err) {
		return err
	}
	return nil
}

func (l *LocalStorage) Driver() Driver { return DriverLocal }

// ==================== S3 / MinIO ====================

type S3Config struct {
	Endpoint  string // host:port (tanpa scheme) — kosong = AWS default
	AccessKey string
	SecretKey string
	Bucket    string
	Region    string
	UseSSL    bool
}

type S3Storage struct {
	client *s3.Client
	bucket string
}

func NewS3Storage(ctx context.Context, cfg S3Config) (*S3Storage, error) {
	if cfg.Bucket == "" {
		return nil, fmt.Errorf("S3 bucket kosong")
	}
	awsCfg, err := awsconfig.LoadDefaultConfig(ctx,
		awsconfig.WithRegion(orDefault(cfg.Region, "us-east-1")),
		awsconfig.WithCredentialsProvider(credentials.NewStaticCredentialsProvider(cfg.AccessKey, cfg.SecretKey, "")),
	)
	if err != nil {
		return nil, err
	}
	client := s3.NewFromConfig(awsCfg, func(o *s3.Options) {
		if cfg.Endpoint != "" {
			o.BaseEndpoint = aws.String(endpointURL(cfg.Endpoint, cfg.UseSSL))
			o.UsePathStyle = true // MinIO
		}
	})
	return &S3Storage{client: client, bucket: cfg.Bucket}, nil
}

func (s *S3Storage) Put(ctx context.Context, key string, obj FileObject) error {
	_, err := s.client.PutObject(ctx, &s3.PutObjectInput{
		Bucket: aws.String(s.bucket), Key: aws.String(key),
		Body: bytes.NewReader(obj.Content), ContentType: aws.String(orDefault(obj.FileType, "application/octet-stream")),
	})
	return err
}

func (s *S3Storage) Get(ctx context.Context, key string) (*FileObject, error) {
	out, err := s.client.GetObject(ctx, &s3.GetObjectInput{Bucket: aws.String(s.bucket), Key: aws.String(key)})
	if err != nil {
		return nil, ErrNotFound
	}
	defer out.Body.Close()
	data, err := io.ReadAll(out.Body)
	if err != nil {
		return nil, err
	}
	return &FileObject{Content: data, Size: int64(len(data))}, nil
}

func (s *S3Storage) Delete(ctx context.Context, key string) error {
	_, err := s.client.DeleteObject(ctx, &s3.DeleteObjectInput{Bucket: aws.String(s.bucket), Key: aws.String(key)})
	return err
}

func (s *S3Storage) Driver() Driver { return DriverS3 }

func orDefault(v, def string) string {
	if v != "" {
		return v
	}
	return def
}
func endpointURL(ep string, ssl bool) string {
	if strings.HasPrefix(ep, "http") {
		return ep
	}
	if ssl {
		return "https://" + ep
	}
	return "http://" + ep
}
