package config

import (
	"net/url"
	"strconv"
	"strings"
	"time"

	"github.com/spf13/viper"
)

type Config struct {
	Server     ServerConfig
	Database   DatabaseConfig
	Redis      RedisConfig
	JWT        JWTConfig
	OpenAI     OpenAIConfig
	GCS        GCSConfig
	OCR        OCRConfig
	CORS       CORSConfig
	Rate       RateConfig
	Log        LogConfig
	QBO        QBOConfig
	Encryption EncryptionConfig
	Telegram   TelegramConfig
	UploadDir  string
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

// AsynqAddr returns the host:port address extracted from the Redis URL.
// If the URL is already in host:port format, it is returned as-is.
func (c RedisConfig) AsynqAddr() string {
	u, err := url.Parse(c.URL)
	if err != nil || u.Host == "" {
		return c.URL
	}
	return u.Host
}

// AsynqDB returns the database number from the Redis URL path (e.g. /0 → 0).
func (c RedisConfig) AsynqDB() int {
	u, err := url.Parse(c.URL)
	if err != nil || u.Path == "" || u.Path == "/" {
		return 0
	}
	db, err := strconv.Atoi(u.Path[1:]) // strip leading "/"
	if err != nil {
		return 0
	}
	return db
}

// AsynqPassword returns the password from the Redis URL, if any.
func (c RedisConfig) AsynqPassword() string {
	u, err := url.Parse(c.URL)
	if err != nil || u.User == nil {
		return ""
	}
	pw, _ := u.User.Password()
	return pw
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

type QBOConfig struct {
	ClientID     string
	ClientSecret string
	RedirectURL  string
	Scopes       string
	BaseURL      string // https://quickbooks.api.intuit.com (production) or https://sandbox-quickbooks.api.intuit.com
	RateLimit    int    // requests per minute (QBO limit: 500)
	MaxConcur    int    // max concurrent requests (QBO limit: 10)
}

type EncryptionConfig struct {
	Key string // 32-byte hex-encoded AES-256 key
}

type TelegramConfig struct {
	BotToken    string
	BotUsername string
	Projects    []string // configurable project tags (comma-separated BOT_PROJECTS env)
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
	v.SetDefault("QBO_BASE_URL", "https://sandbox-quickbooks.api.intuit.com")
	v.SetDefault("QBO_SCOPES", "com.intuit.quickbooks.accounting")
	v.SetDefault("QBO_RATE_LIMIT", 500)
	v.SetDefault("QBO_MAX_CONCUR", 10)

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
		QBO: QBOConfig{
			ClientID:     v.GetString("QBO_CLIENT_ID"),
			ClientSecret: v.GetString("QBO_CLIENT_SECRET"),
			RedirectURL:  v.GetString("QBO_REDIRECT_URL"),
			Scopes:       v.GetString("QBO_SCOPES"),
			BaseURL:      v.GetString("QBO_BASE_URL"),
			RateLimit:    v.GetInt("QBO_RATE_LIMIT"),
			MaxConcur:    v.GetInt("QBO_MAX_CONCUR"),
		},
		Encryption: EncryptionConfig{
			Key: v.GetString("ENCRYPTION_KEY"),
		},
		Telegram: TelegramConfig{
			BotToken:    v.GetString("TELEGRAM_BOT_TOKEN"),
			BotUsername: v.GetString("TELEGRAM_BOT_USERNAME"),
			Projects:    parseProjects(v.GetString("BOT_PROJECTS")),
		},
		UploadDir: v.GetString("UPLOAD_DIR"),
	}

	return cfg, nil
}

// parseProjects splits a comma-separated BOT_PROJECTS string into a list of project names.
func parseProjects(s string) []string {
	if s == "" {
		return nil
	}
	var projects []string
	for _, p := range strings.Split(s, ",") {
		p = strings.TrimSpace(p)
		if p != "" {
			projects = append(projects, p)
		}
	}
	return projects
}
