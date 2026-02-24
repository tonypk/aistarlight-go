package config

import (
	"time"

	"github.com/spf13/viper"
)

type Config struct {
	Server    ServerConfig
	Database  DatabaseConfig
	Redis     RedisConfig
	JWT       JWTConfig
	OpenAI    OpenAIConfig
	GCS       GCSConfig
	OCR       OCRConfig
	CORS      CORSConfig
	Rate      RateConfig
	Log       LogConfig
	UploadDir string
}

type ServerConfig struct {
	Host string
	Port int
	Mode string // debug, release, test
}

type DatabaseConfig struct {
	URL         string
	MaxConns    int32
	MinConns    int32
	MaxLifetime time.Duration
}

type RedisConfig struct {
	URL string
}

type JWTConfig struct {
	Secret             string
	ExpiryHours        int
	RefreshExpiryHours int
}

type OpenAIConfig struct {
	APIKey         string
	Model          string
	EmbeddingModel string
}

type GCSConfig struct {
	Bucket          string
	CredentialsFile string
}

type OCRConfig struct {
	ServiceURL string
}

type CORSConfig struct {
	Origins []string
}

type RateConfig struct {
	RPS   int
	Burst int
}

type LogConfig struct {
	Level  string
	Format string // text, json
}

func Load() (*Config, error) {
	v := viper.New()

	v.SetConfigFile(".env")
	v.SetConfigType("env")
	v.AutomaticEnv()

	// Defaults
	v.SetDefault("SERVER_HOST", "0.0.0.0")
	v.SetDefault("SERVER_PORT", 8000)
	v.SetDefault("GIN_MODE", "debug")
	v.SetDefault("DATABASE_MAX_CONNS", 50)
	v.SetDefault("DATABASE_MIN_CONNS", 10)
	v.SetDefault("DATABASE_MAX_LIFETIME", "1h")
	v.SetDefault("JWT_EXPIRY_HOURS", 24)
	v.SetDefault("JWT_REFRESH_EXPIRY_HOURS", 168)
	v.SetDefault("OPENAI_MODEL", "gpt-4.1-mini")
	v.SetDefault("OPENAI_EMBEDDING_MODEL", "text-embedding-3-small")
	v.SetDefault("OCR_SERVICE_URL", "http://localhost:8010")
	v.SetDefault("CORS_ORIGINS", "http://localhost:5173")
	v.SetDefault("RATE_LIMIT_RPS", 10)
	v.SetDefault("RATE_LIMIT_BURST", 20)
	v.SetDefault("LOG_LEVEL", "info")
	v.SetDefault("LOG_FORMAT", "text")
	v.SetDefault("UPLOAD_DIR", "/tmp/aistarlight-uploads")

	// Try reading .env, ignore if not found
	_ = v.ReadInConfig()

	maxLifetime, err := time.ParseDuration(v.GetString("DATABASE_MAX_LIFETIME"))
	if err != nil {
		maxLifetime = time.Hour
	}

	cfg := &Config{
		Server: ServerConfig{
			Host: v.GetString("SERVER_HOST"),
			Port: v.GetInt("SERVER_PORT"),
			Mode: v.GetString("GIN_MODE"),
		},
		Database: DatabaseConfig{
			URL:         v.GetString("DATABASE_URL"),
			MaxConns:    v.GetInt32("DATABASE_MAX_CONNS"),
			MinConns:    v.GetInt32("DATABASE_MIN_CONNS"),
			MaxLifetime: maxLifetime,
		},
		Redis: RedisConfig{
			URL: v.GetString("REDIS_URL"),
		},
		JWT: JWTConfig{
			Secret:             v.GetString("JWT_SECRET"),
			ExpiryHours:        v.GetInt("JWT_EXPIRY_HOURS"),
			RefreshExpiryHours: v.GetInt("JWT_REFRESH_EXPIRY_HOURS"),
		},
		OpenAI: OpenAIConfig{
			APIKey:         v.GetString("OPENAI_API_KEY"),
			Model:          v.GetString("OPENAI_MODEL"),
			EmbeddingModel: v.GetString("OPENAI_EMBEDDING_MODEL"),
		},
		GCS: GCSConfig{
			Bucket:          v.GetString("GCS_BUCKET"),
			CredentialsFile: v.GetString("GCS_CREDENTIALS_FILE"),
		},
		OCR: OCRConfig{
			ServiceURL: v.GetString("OCR_SERVICE_URL"),
		},
		CORS: CORSConfig{
			Origins: v.GetStringSlice("CORS_ORIGINS"),
		},
		Rate: RateConfig{
			RPS:   v.GetInt("RATE_LIMIT_RPS"),
			Burst: v.GetInt("RATE_LIMIT_BURST"),
		},
		Log: LogConfig{
			Level:  v.GetString("LOG_LEVEL"),
			Format: v.GetString("LOG_FORMAT"),
		},
		UploadDir: v.GetString("UPLOAD_DIR"),
	}

	return cfg, nil
}
