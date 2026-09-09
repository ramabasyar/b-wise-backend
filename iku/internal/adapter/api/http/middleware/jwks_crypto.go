package middleware

import (
	"crypto"
	"crypto/rsa"
	"crypto/sha256"
	"encoding/base64"
	"encoding/binary"
	"fmt"
	"math/big"
)

func base64Decode(s string) ([]byte, error) {
	// Add padding if needed
	for len(s)%4 != 0 {
		s += "="
	}
	return base64.URLEncoding.DecodeString(s)
}

func base64URLDecode(s string) ([]byte, error) {
	for len(s)%4 != 0 {
		s += "="
	}
	return base64.URLEncoding.DecodeString(s)
}

func verifyRS256(payload, signature []byte, pubKey *rsa.PublicKey) error {
	hashed := sha256.Sum256(payload)
	return rsa.VerifyPKCS1v15(pubKey, crypto.SHA256, hashed[:], signature)
}

func buildRSAPublicKey(nStr, eStr string) (*rsa.PublicKey, error) {
	nBytes, err := base64URLDecode(nStr)
	if err != nil {
		return nil, fmt.Errorf("failed to decode n: %w", err)
	}

	eBytes, err := base64URLDecode(eStr)
	if err != nil {
		return nil, fmt.Errorf("failed to decode e: %w", err)
	}

	n := new(big.Int).SetBytes(nBytes)
	var e int
	if len(eBytes) < 4 {
		e = int(binary.BigEndian.Uint32(append(make([]byte, 4-len(eBytes)), eBytes...)))
	} else {
		e = int(binary.BigEndian.Uint32(eBytes))
	}

	return &rsa.PublicKey{N: n, E: e}, nil
}
