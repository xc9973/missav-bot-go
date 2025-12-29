package config

import (
	"fmt"
	"time"

	"github.com/kelseyhightower/envconfig"
)

// Config holds all application configuration
type Config struct {
	Bot     BotConfig
	DB      DBConfig
	Crawler CrawlerConfig
	Server  ServerConfig
}

// BotConfig holds Telegram bot configuration
type BotConfig struct {
	Token         string `envconfig:"BOT_TOKEN" required:"true"`
	Username      string `envconfig:"BOT_USERNAME" default:"MissavBot"`
	DefaultChatID int64  `envconfig:"BOT_CHAT_ID" default:"0"`
}

// DBConfig holds database configuration
type DBConfig struct {
	Host     string `envconfig:"DB_HOST" default:"localhost"`
	Port     int    `envconfig:"DB_PORT" default:"3306"`
	User     string `envconfig:"DB_USER" default:"root"`
	Password string `envconfig:"DB_PASSWORD" required:"true"`
	Database string `envconfig:"DB_NAME" default:"missav_bot"`
	MaxConns int    `envconfig:"DB_MAX_CONNS" default:"10"`
}

// CrawlerConfig holds crawler configuration
type CrawlerConfig struct {
	Enabled      bool          `envconfig:"CRAWLER_ENABLED" default:"true"`
	Interval     time.Duration `envconfig:"CRAWLER_INTERVAL" default:"15m"`
	InitialPages int           `envconfig:"CRAWLER_INITIAL_PAGES" default:"2"`
	RateLimit    float64       `envconfig:"CRAWLER_RATE_LIMIT" default:"0.5"`
	Timeout      time.Duration `envconfig:"CRAWLER_TIMEOUT" default:"30s"`
	MaxRetries   int           `envconfig:"CRAWLER_MAX_RETRIES" default:"3"`
	Concurrency  int           `envconfig:"CRAWLER_CONCURRENCY" default:"3"`
	UserAgent    string        `envconfig:"CRAWLER_USER_AGENT"`
	ProxyURL     string        `envconfig:"CRAWLER_PROXY_URL"`
}

// ServerConfig holds HTTP server configuration
type ServerConfig struct {
	Port int `envconfig:"SERVER_PORT" default:"8080"`
}


// DSN returns the MySQL data source name
func (c *DBConfig) DSN() string {
	return fmt.Sprintf("%s:%s@tcp(%s:%d)/%s?charset=utf8mb4&parseTime=True&loc=Local",
		c.User, c.Password, c.Host, c.Port, c.Database)
}

// Load loads configuration from environment variables
func Load() (*Config, error) {
	var cfg Config

	if err := envconfig.Process("", &cfg.Bot); err != nil {
		return nil, fmt.Errorf("failed to load bot config: %w", err)
	}

	if err := envconfig.Process("", &cfg.DB); err != nil {
		return nil, fmt.Errorf("failed to load db config: %w", err)
	}

	if err := envconfig.Process("", &cfg.Crawler); err != nil {
		return nil, fmt.Errorf("failed to load crawler config: %w", err)
	}

	if err := envconfig.Process("", &cfg.Server); err != nil {
		return nil, fmt.Errorf("failed to load server config: %w", err)
	}

	return &cfg, nil
}

// Validate validates the configuration
func (c *Config) Validate() error {
	if c.Bot.Token == "" {
		return fmt.Errorf("BOT_TOKEN is required")
	}
	if c.DB.Password == "" {
		return fmt.Errorf("DB_PASSWORD is required")
	}
	if c.Crawler.RateLimit <= 0 {
		return fmt.Errorf("CRAWLER_RATE_LIMIT must be positive")
	}
	if c.Crawler.Concurrency <= 0 {
		return fmt.Errorf("CRAWLER_CONCURRENCY must be positive")
	}
	if c.Server.Port <= 0 || c.Server.Port > 65535 {
		return fmt.Errorf("SERVER_PORT must be between 1 and 65535")
	}
	return nil
}
