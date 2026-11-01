package config

import (
	"os"
	"testing"
)

func TestLoadConfig(t *testing.T) {
	// Create a temp config file
	configContent := `
server:
  port: 9090
  host: "127.0.0.1"

database:
  host: "localhost"
  port: 5432
  user: "testuser"
  password: "testpass"
  dbname: "testdb"
  sslmode: "disable"

redis:
  host: "localhost"
  port: 6379

jwt:
  secret: "test-jwt-secret"
  access_expiration: 1h

sso:
  url: "http://localhost:8080"
  service_account_email: "service@test.com"
  service_account_password: "testpass"
  cache_enabled: true
  cache_ttl: 10m

logger:
  level: "debug"
  format: "json"
`
	tmpFile, err := os.CreateTemp("", "config-*.yaml")
	if err != nil {
		t.Fatal(err)
	}
	defer os.Remove(tmpFile.Name())

	tmpFile.WriteString(configContent)
	tmpFile.Close()

	cfg, err := Load(tmpFile.Name())
	if err != nil {
		t.Fatalf("Load failed: %v", err)
	}

	if cfg.Server.Port != 9090 {
		t.Errorf("Expected server port 9090, got %d", cfg.Server.Port)
	}
	if cfg.Database.User != "testuser" {
		t.Errorf("Expected db user 'testuser', got '%s'", cfg.Database.User)
	}
	if cfg.JWT.Secret != "test-jwt-secret" {
		t.Errorf("Expected jwt secret 'test-jwt-secret', got '%s'", cfg.JWT.Secret)
	}
	if cfg.SSO.URL != "http://localhost:8080" {
		t.Errorf("Expected SSO URL 'http://localhost:8080', got '%s'", cfg.SSO.URL)
	}
	if cfg.SSO.CacheEnabled != true {
		t.Error("Expected SSO cache enabled")
	}
	if cfg.Logger.Level != "debug" {
		t.Errorf("Expected logger level 'debug', got '%s'", cfg.Logger.Level)
	}
}

func TestLoadConfig_FileNotFound(t *testing.T) {
	_, err := Load("nonexistent-config.yaml")
	if err == nil {
		t.Error("Expected error for missing config file, got nil")
	}
}
