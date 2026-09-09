package config

import (
	"fmt"
	"log"
	"os"
	"path/filepath"
	"time"

	"github.com/joho/godotenv"
	"github.com/spf13/viper"
)

// Config holds application configuration
type Config struct {
	Server   ServerConfig
	Database DatabaseConfig
	Redis    RedisConfig
	JWT      JWTConfig
	SSO      SSOConfig
	Logger   LoggerConfig
	CORS     CORSConfig
}

// ServerConfig holds server configuration
type ServerConfig struct {
	Port              int64         `mapstructure:"port"`
	Host              string        `mapstructure:"host"`
	ReadTimeout       time.Duration `mapstructure:"read_timeout"`
	WriteTimeout      time.Duration `mapstructure:"write_timeout"`
	ShutdownTimeout   time.Duration `mapstructure:"shutdown_timeout"`
	MaxBodySize       int64         `mapstructure:"max_body_size"`
}

// DatabaseConfig holds database configuration
type DatabaseConfig struct {
	Host            string        `mapstructure:"host"`
	Port            int           `mapstructure:"port"`
	User            string        `mapstructure:"user"`
	Password        string        `mapstructure:"password"`
	DBName          string        `mapstructure:"dbname"`
	SSLMode         string        `mapstructure:"sslmode"`
	MaxOpenConns    int           `mapstructure:"max_open_conns"`
	MaxIdleConns    int           `mapstructure:"max_idle_conns"`
	ConnMaxLifetime time.Duration `mapstructure:"conn_max_lifetime"`
}

// RedisConfig holds Redis configuration
type RedisConfig struct {
	Host         string `mapstructure:"host"`
	Port         int    `mapstructure:"port"`
	Password     string `mapstructure:"password"`
	DB           int    `mapstructure:"db"`
	PoolSize     int    `mapstructure:"pool_size"`
	MinIdleConns int    `mapstructure:"min_idle_conns"`
}

// JWTConfig holds JWT configuration
type JWTConfig struct {
	Secret          string        `mapstructure:"secret"`
	AccessExpiration time.Duration `mapstructure:"access_expiration"`
}

// SSOConfig holds SSO client configuration
type SSOConfig struct {
	URL                      string        `mapstructure:"url"`
	ServiceAccountEmail      string        `mapstructure:"service_account_email"`
	ServiceAccountPassword   string        `mapstructure:"service_account_password"`
	CacheEnabled             bool          `mapstructure:"cache_enabled"`
	CacheTTL                 time.Duration `mapstructure:"cache_ttl"`
	PermissionServiceURL     string        `mapstructure:"permission_service_url"`
	HumanCapitalURL          string        `mapstructure:"human_capital_url"` // Fase B: resolusi approver org API
	ServiceName              string        `mapstructure:"service_name"`
	ServiceClientID          string        `mapstructure:"service_client_id"`
	SuperAdminIDs            []string      `mapstructure:"super_admin_ids"`
	ServiceClientSecret      string        `mapstructure:"service_client_secret"`
}

// LoggerConfig holds logger configuration
type LoggerConfig struct {
	Level  string `mapstructure:"level"`
	Format string `mapstructure:"format"`
}

// CORSConfig holds CORS configuration
type CORSConfig struct {
	AllowedOrigins   []string `mapstructure:"allowed_origins"`
	AllowCredentials bool     `mapstructure:"allow_credentials"`
	MaxAge           int      `mapstructure:"max_age"`
}

// Load loads configuration from file and environment variables
func Load(configPath string) (*Config, error) {
	// Load .env file (optional, won't error if not found)
	if err := godotenv.Load(); err != nil {
		log.Printf("Warning: .env file not found or error loading: %v", err)
	}

	// If configPath is relative, make it absolute relative to the current working directory
	if !filepath.IsAbs(configPath) {
		// Get current working directory
		cwd, err := os.Getwd()
		if err == nil {
			// CWD should be service directory when running binary
			configPath = filepath.Join(cwd, configPath)
			log.Printf("[config] Resolved config path from CWD: %s", configPath)
		}
	}

	viper.SetConfigFile(configPath)
	viper.SetConfigType("yaml")

	// Set defaults
	setDefaults()

	// Read config file
	if err := viper.ReadInConfig(); err != nil {
		return nil, fmt.Errorf("failed to read config file: %w", err)
	}

	// Bind environment variables
	bindEnvVars()

	var cfg Config
	if err := viper.Unmarshal(&cfg); err != nil {
		return nil, fmt.Errorf("failed to unmarshal config: %w", err)
	}

	// Validate required configuration
	if err := validate(&cfg); err != nil {
		return nil, fmt.Errorf("config validation failed: %w", err)
	}

	return &cfg, nil
}

// LoadEnv loads config with optional environment file
func LoadEnv(configPath string, envPath ...string) (*Config, error) {
	// Load .env file if provided
	if len(envPath) > 0 && envPath[0] != "" {
		viper.SetConfigFile(envPath[0])
		viper.SetConfigType("env")
		_ = viper.ReadInConfig()
	}

	// Automatically read from .env in current directory
	if _, err := os.Stat(".env"); err == nil {
		viper.SetConfigFile(".env")
		viper.SetConfigType("env")
		_ = viper.ReadInConfig()
	}

	return Load(configPath)
}

// validate validates required configuration
func validate(cfg *Config) error {
	// JWT secret is required for authentication
	if cfg.JWT.Secret == "" {
		// Set a default for development
		cfg.JWT.Secret = "dev-secret-change-in-production"
	}
	return nil
}

// setDefaults sets default configuration values
func setDefaults() {
	viper.SetDefault("server.port", 8081)
	viper.SetDefault("server.host", "0.0.0.0")
	viper.SetDefault("server.read_timeout", 30*time.Second)
	viper.SetDefault("server.write_timeout", 30*time.Second)
	viper.SetDefault("server.shutdown_timeout", 10*time.Second)
	viper.SetDefault("server.max_body_size", int64(10<<20)) // 10MB

	viper.SetDefault("database.host", "localhost")
	viper.SetDefault("database.port", 5432)
	viper.SetDefault("database.user", "postgres")
	viper.SetDefault("database.password", "postgres")
	viper.SetDefault("database.dbname", "kit_db")
	viper.SetDefault("database.sslmode", "disable")
	viper.SetDefault("database.max_open_conns", 100)
	viper.SetDefault("database.max_idle_conns", 10)
	viper.SetDefault("database.conn_max_lifetime", time.Hour)

	viper.SetDefault("redis.host", "localhost")
	viper.SetDefault("redis.port", 6379)
	viper.SetDefault("redis.db", 0)
	viper.SetDefault("redis.pool_size", 10)
	viper.SetDefault("redis.min_idle_conns", 5)

	viper.SetDefault("jwt.access_expiration", time.Hour)

	viper.SetDefault("sso.url", "http://localhost:8080")
	viper.SetDefault("sso.cache_enabled", true)
	viper.SetDefault("sso.cache_ttl", 15*time.Minute)

	viper.SetDefault("logger.level", "info")
	viper.SetDefault("logger.format", "json")

	viper.SetDefault("cors.allowed_origins", []string{"*"})
	viper.SetDefault("cors.allow_credentials", true)
	viper.SetDefault("cors.max_age", 86400)
}

// bindEnvVars binds environment variables
func bindEnvVars() {
	// Server
	viper.BindEnv("server.port", "SERVER_PORT")
	viper.BindEnv("server.host", "SERVER_HOST")
	viper.BindEnv("server.max_body_size", "SERVER_MAX_BODY_SIZE")

	// Database
	viper.BindEnv("database.host", "DB_HOST")
	viper.BindEnv("database.port", "DB_PORT")
	viper.BindEnv("database.user", "DB_USER")
	viper.BindEnv("database.password", "DB_PASSWORD")
	viper.BindEnv("database.dbname", "DB_NAME")
	viper.BindEnv("database.sslmode", "DB_SSLMODE")

	// Redis
	viper.BindEnv("redis.host", "REDIS_HOST")
	viper.BindEnv("redis.port", "REDIS_PORT")
	viper.BindEnv("redis.password", "REDIS_PASSWORD")
	viper.BindEnv("redis.db", "REDIS_DB")

	// JWT
	viper.BindEnv("jwt.secret", "JWT_SECRET")
	viper.BindEnv("jwt.access_expiration", "JWT_ACCESS_EXPIRATION")

	// SSO
	viper.BindEnv("sso.url", "SSO_URL")
	viper.BindEnv("sso.human_capital_url", "HUMAN_CAPITAL_URL")
	viper.BindEnv("sso.service_account_email", "SSO_SERVICE_ACCOUNT_EMAIL")
	viper.BindEnv("sso.service_account_password", "SSO_SERVICE_ACCOUNT_PASSWORD")
	viper.BindEnv("sso.cache_enabled", "SSO_CACHE_ENABLED")
	viper.BindEnv("sso.cache_ttl", "SSO_CACHE_TTL")
	viper.BindEnv("sso.permission_service_url", "PERMISSION_SERVICE_URL")
	viper.BindEnv("sso.service_name", "SERVICE_NAME")
	viper.BindEnv("sso.service_client_id", "SERVICE_CLIENT_ID")
	viper.BindEnv("sso.service_client_secret", "SERVICE_CLIENT_SECRET")
	viper.BindEnv("sso.super_admin_ids", "SUPER_ADMIN_IDS")

	// Logger
	viper.BindEnv("logger.level", "LOGGER_LEVEL")
	viper.BindEnv("logger.format", "LOGGER_FORMAT")

	// CORS
	viper.BindEnv("cors.allowed_origins", "CORS_ALLOWED_ORIGINS")
	viper.BindEnv("cors.allow_credentials", "CORS_ALLOW_CREDENTIALS")
	viper.BindEnv("cors.max_age", "CORS_MAX_AGE")
}
