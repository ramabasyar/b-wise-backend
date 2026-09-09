package jwt

import (
	"time"

	"github.com/golang-jwt/jwt/v5"
	"github.com/rama/b-wise/permission-service/internal/adapter/config"
)

// Claims represents JWT claims with user info and roles
type Claims struct {
	UserID string   `json:"user_id"`
	Email  string   `json:"email"`
	Roles  []string `json:"roles"`
	jwt.RegisteredClaims
}

// GenerateToken generates a JWT token for a user
func GenerateToken(userID, email string, roles []string, cfg config.JWTConfig) (string, error) {
	// Set expiration time
	expirationTime := time.Now().Add(24 * time.Hour) // Default 24 hours

	// Create claims
	claims := &Claims{
		UserID: userID,
		Email:  email,
		Roles:  roles,
		RegisteredClaims: jwt.RegisteredClaims{
			ExpiresAt: jwt.NewNumericDate(expirationTime),
			IssuedAt:  jwt.NewNumericDate(time.Now()),
			NotBefore: jwt.NewNumericDate(time.Now()),
			Issuer:    "starter-kit",
		},
	}

	// Create token with claims
	token := jwt.NewWithClaims(jwt.SigningMethodHS256, claims)

	// Sign token with secret
	tokenString, err := token.SignedString([]byte(cfg.Secret))
	if err != nil {
		return "", err
	}

	return tokenString, nil
}

// ValidateToken validates a JWT token
func ValidateToken(tokenString string, secret string) (*Claims, error) {
	token, err := jwt.ParseWithClaims(tokenString, &Claims{}, func(token *jwt.Token) (interface{}, error) {
		// Validate signing method
		if _, ok := token.Method.(*jwt.SigningMethodHMAC); !ok {
			return nil, jwt.ErrSignatureInvalid
		}
		return []byte(secret), nil
	})

	if err != nil {
		return nil, err
	}

	// Extract and validate claims
	if claims, ok := token.Claims.(*Claims); ok && token.Valid {
		return claims, nil
	}

	return nil, jwt.ErrTokenInvalidClaims
}

// ExtractUserID extracts user ID from token
func ExtractUserID(tokenString, secret string) (string, error) {
	claims, err := ValidateToken(tokenString, secret)
	if err != nil {
		return "", err
	}
	return claims.UserID, nil
}

// ExtractRoles extracts roles from token
func ExtractRoles(tokenString, secret string) ([]string, error) {
	claims, err := ValidateToken(tokenString, secret)
	if err != nil {
		return nil, err
	}
	return claims.Roles, nil
}
