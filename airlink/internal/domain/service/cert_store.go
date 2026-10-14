package service

import (
	"context"
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"database/sql"
	"encoding/base64"
	"fmt"
	"io"
	"time"

	_ "github.com/jackc/pgx/v5/stdlib"
)

type StoredCert struct {
	ID        int64      `json:"id"`
	Username  string     `json:"username"`
	Device    string     `json:"device_name"`
	Serial    string     `json:"cert_serial"`
	IssuedAt  time.Time  `json:"issued_at"`
	ExpiresAt time.Time  `json:"expires_at"`
	RevokedAt *time.Time `json:"revoked_at,omitempty"`
}

type CertStore struct {
	db   *sql.DB
	aead cipher.AEAD
}

func NewCertStore(dsn, keyBase64 string) (*CertStore, error) {
	key, err := base64.StdEncoding.DecodeString(keyBase64)
	if err != nil || len(key) != 32 {
		return nil, fmt.Errorf("AIRLINK_ENCRYPTION_KEY harus base64 dari 32 byte")
	}
	block, err := aes.NewCipher(key)
	if err != nil {
		return nil, err
	}
	aead, err := cipher.NewGCM(block)
	if err != nil {
		return nil, err
	}
	db, err := sql.Open("pgx", dsn)
	if err != nil {
		return nil, err
	}
	db.SetMaxOpenConns(5)
	db.SetConnMaxLifetime(5 * time.Minute)
	if err := db.Ping(); err != nil {
		db.Close()
		return nil, err
	}
	return &CertStore{db: db, aead: aead}, nil
}

func (s *CertStore) Close() error { return s.db.Close() }

func (s *CertStore) seal(data []byte, aad string) ([]byte, error) {
	nonce := make([]byte, s.aead.NonceSize())
	if _, err := io.ReadFull(rand.Reader, nonce); err != nil {
		return nil, err
	}
	return s.aead.Seal(nonce, nonce, data, []byte(aad)), nil
}

func (s *CertStore) open(data []byte, aad string) ([]byte, error) {
	n := s.aead.NonceSize()
	if len(data) < n {
		return nil, fmt.Errorf("paket terenkripsi tidak lengkap")
	}
	return s.aead.Open(nil, data[:n], data[n:], []byte(aad))
}

func (s *CertStore) Exists(ctx context.Context, username, device string) (bool, error) {
	var exists bool
	err := s.db.QueryRowContext(ctx, `SELECT EXISTS(SELECT 1 FROM wifi_certificates WHERE username=$1 AND device_name=$2 AND revoked_at IS NULL)`, username, device).Scan(&exists)
	return exists, err
}

func (s *CertStore) Save(ctx context.Context, username, device, serial string, expiresAt time.Time, packageData []byte, password, actor string) (int64, error) {
	aad := username + "/" + device
	packageCipher, err := s.seal(packageData, aad+":p12")
	if err != nil {
		return 0, err
	}
	passCipher, err := s.seal([]byte(password), aad+":password")
	if err != nil {
		return 0, err
	}
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return 0, err
	}
	defer tx.Rollback()
	var id int64
	err = tx.QueryRowContext(ctx, `INSERT INTO wifi_certificates(username,device_name,cert_serial,p12_ciphertext,p12_password_ciphertext,expires_at)
		VALUES($1,$2,$3,$4,$5,$6) RETURNING id`, username, device, serial, packageCipher, passCipher, expiresAt).Scan(&id)
	if err != nil {
		return 0, err
	}
	if _, err = tx.ExecContext(ctx, `INSERT INTO wifi_certificate_events(certificate_id,actor,action) VALUES($1,$2,'issue')`, id, actor); err != nil {
		return 0, err
	}
	return id, tx.Commit()
}

func (s *CertStore) List(ctx context.Context, username string, limit, offset int) ([]StoredCert, error) {
	if limit < 1 || limit > 100 {
		limit = 50
	}
	if offset < 0 {
		offset = 0
	}
	var rows *sql.Rows
	var err error
	query := `SELECT id,username,device_name,cert_serial,issued_at,expires_at,revoked_at FROM wifi_certificates`
	if username != "" {
		rows, err = s.db.QueryContext(ctx, query+` WHERE username=$1 ORDER BY issued_at DESC LIMIT $2 OFFSET $3`, username, limit, offset)
	} else {
		rows, err = s.db.QueryContext(ctx, query+` ORDER BY issued_at DESC LIMIT $1 OFFSET $2`, limit, offset)
	}
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	items := make([]StoredCert, 0)
	for rows.Next() {
		var c StoredCert
		if err = rows.Scan(&c.ID, &c.Username, &c.Device, &c.Serial, &c.IssuedAt, &c.ExpiresAt, &c.RevokedAt); err != nil {
			return nil, err
		}
		items = append(items, c)
	}
	return items, rows.Err()
}

// Package mengaudit unduhan sebelum menyerahkan paket dan password ke handler.
func (s *CertStore) Package(ctx context.Context, username, device, actor string) ([]byte, string, error) {
	var id int64
	var ciphertext, passCipher []byte
	err := s.db.QueryRowContext(ctx, `SELECT id,p12_ciphertext,p12_password_ciphertext FROM wifi_certificates
		WHERE username=$1 AND device_name=$2 AND revoked_at IS NULL ORDER BY id DESC LIMIT 1`, username, device).Scan(&id, &ciphertext, &passCipher)
	if err != nil {
		return nil, "", err
	}
	aad := username + "/" + device
	packageData, err := s.open(ciphertext, aad+":p12")
	if err != nil {
		return nil, "", err
	}
	password, err := s.open(passCipher, aad+":password")
	if err != nil {
		return nil, "", err
	}
	if _, err = s.db.ExecContext(ctx, `INSERT INTO wifi_certificate_events(certificate_id,actor,action) VALUES($1,$2,'download')`, id, actor); err != nil {
		return nil, "", err
	}
	return packageData, string(password), nil
}
