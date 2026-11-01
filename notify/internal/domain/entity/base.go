package entity

import (
	"crypto/rand"
	"encoding/hex"
	"time"

	"gorm.io/gorm"
)

// Base — field umum semua entity notify. BeforeCreate menghasilkan ID hex 32
// (jebakan starter kit: tanpa hook, INSERT kedua gagal duplicate PK ID kosong).
type Base struct {
	ID        string    `json:"id" gorm:"primaryKey;type:varchar(36)"`
	CreatedAt time.Time `json:"created_at" gorm:"autoCreateTime"`
	UpdatedAt time.Time `json:"updated_at" gorm:"autoUpdateTime"`
}

func (b *Base) BeforeCreate(tx *gorm.DB) error {
	if b.ID == "" {
		raw := make([]byte, 16)
		_, _ = rand.Read(raw)
		b.ID = hex.EncodeToString(raw)
	}
	return nil
}
