package service

import (
	"bytes"
	"crypto/aes"
	"crypto/cipher"
	"testing"
)

func TestCertPackageEncryptionBindsOwnerAndRejectsTamper(t *testing.T) {
	block, err := aes.NewCipher(bytes.Repeat([]byte{7}, 32))
	if err != nil {
		t.Fatal(err)
	}
	aead, err := cipher.NewGCM(block)
	if err != nil {
		t.Fatal(err)
	}
	store := &CertStore{aead: aead}
	plain := []byte("example-private-package")
	sealed, err := store.seal(plain, "user/device:p12")
	if err != nil {
		t.Fatal(err)
	}
	got, err := store.open(sealed, "user/device:p12")
	if err != nil || !bytes.Equal(got, plain) {
		t.Fatalf("roundtrip failed: %v", err)
	}
	if _, err = store.open(sealed, "other/device:p12"); err == nil {
		t.Fatal("wrong owner AAD accepted")
	}
	sealed[len(sealed)-1] ^= 1
	if _, err = store.open(sealed, "user/device:p12"); err == nil {
		t.Fatal("tampered package accepted")
	}
}
