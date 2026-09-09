package main

import (
	"fmt"

	"github.com/rama/b-wise/human-capital/internal/adapter/config"
	"github.com/rama/b-wise/human-capital/pkg/jwt"
)

// Example: How to generate JWT token in your microservice
// This is for reference - DO NOT include in production code

func ExampleGenerateJWT() {
	// Load config (in real code, this comes from config.yaml)
	cfg := config.JWTConfig{
		Secret: "your-super-secret-jwt-key-change-this-in-production",
	}

	// User data from your database or SSO
	userID := "user-123"
	email := "user@example.com"
	roles := []string{"user", "moderator"}

	// Generate JWT token
	token, err := jwt.GenerateToken(userID, email, roles, cfg)
	if err != nil {
		fmt.Printf("Error generating token: %v\n", err)
		return
	}

	fmt.Printf("Generated JWT Token: %s\n", token)
	fmt.Printf("Token contains: user_id=%s, email=%s, roles=%v\n", userID, email, roles)
}

// Example: How to validate JWT token
func ExampleValidateJWT() {
	tokenString := "YOUR_JWT_TOKEN_HERE"
	secret := "your-super-secret-jwt-key-change-this-in-production"

	// Validate token
	claims, err := jwt.ValidateToken(tokenString, secret)
	if err != nil {
		fmt.Printf("Invalid token: %v\n", err)
		return
	}

	fmt.Printf("Valid token! User ID: %s, Email: %s, Roles: %v\n",
		claims.UserID, claims.Email, claims.Roles)
}

// Example: In your auth handler (login endpoint)
func ExampleLoginHandler() {
	// After user authentication, generate token
	/*
	   func (h *AuthHandler) Login(c *gin.Context) {
	       // 1. Validate credentials (username/password)
	       // 2. Get user from database
	       // 3. Get user roles
	       // 4. Generate JWT token

	       userID := user.ID
	       email := user.Email
	       roles := []string{"user"} // or get from database

	       token, err := jwt.GenerateToken(userID, email, roles, h.cfg.JWT)
	       if err != nil {
	           c.JSON(500, gin.H{"error": "Failed to generate token"})
	           return
	       }

	       c.JSON(200, gin.H{
	           "token": token,
	           "user": gin.H{
	               "id":    userID,
	               "email": email,
	               "roles": roles,
	           },
	       })
	   }
	*/
}
