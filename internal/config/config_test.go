package config

import (
	"os"
	"testing"
	"time"
)

func TestLoad_WithRequiredEnvVars(t *testing.T) {
	// Set required environment variables
	os.Setenv("BOT_TOKEN", "test-token-123")
	os.Setenv("DB_PASSWORD", "test-password")
	defer func() {
		os.Unsetenv("BOT_TOKEN")
		os.Unsetenv("DB_PASSWORD")
	}()

	cfg, err := Load()
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}

	if cfg.Bot.Token != "test-token-123" {
		t.Errorf("Bot.Token = %v, want %v", cfg.Bot.Token, "test-token-123")
	}
	if cfg.DB.Password != "test-password" {
		t.Errorf("DB.Password = %v, want %v", cfg.DB.Password, "test-password")
	}
}

func TestLoad_DefaultValues(t *testing.T) {
	os.Setenv("BOT_TOKEN", "test-token")
	os.Setenv("DB_PASSWORD", "test-pass")
	defer func() {
		os.Unsetenv("BOT_TOKEN")
		os.Unsetenv("DB_PASSWORD")
	}()

	cfg, err := Load()
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}

	// Test Bot defaults
	if cfg.Bot.Username != "MissavBot" {
		t.Errorf("Bot.Username = %v, want %v", cfg.Bot.Username, "MissavBot")
	}
	if cfg.Bot.DefaultChatID != 0 {
		t.Errorf("Bot.DefaultChatID = %v, want %v", cfg.Bot.DefaultChatID, 0)
	}

	// Test DB defaults
	if cfg.DB.Host != "localhost" {
		t.Errorf("DB.Host = %v, want %v", cfg.DB.Host, "localhost")
	}
	if cfg.DB.Port != 3306 {
		t.Errorf("DB.Port = %v, want %v", cfg.DB.Port, 3306)
	}
	if cfg.DB.User != "root" {
		t.Errorf("DB.User = %v, want %v", cfg.DB.User, "root")
	}
	if cfg.DB.Database != "missav_bot" {
		t.Errorf("DB.Database = %v, want %v", cfg.DB.Database, "missav_bot")
	}
	if cfg.DB.MaxConns != 10 {
		t.Errorf("DB.MaxConns = %v, want %v", cfg.DB.MaxConns, 10)
	}

	// Test Crawler defaults
	if cfg.Crawler.Enabled != true {
		t.Errorf("Crawler.Enabled = %v, want %v", cfg.Crawler.Enabled, true)
	}
	if cfg.Crawler.Interval != 15*time.Minute {
		t.Errorf("Crawler.Interval = %v, want %v", cfg.Crawler.Interval, 15*time.Minute)
	}
	if cfg.Crawler.InitialPages != 2 {
		t.Errorf("Crawler.InitialPages = %v, want %v", cfg.Crawler.InitialPages, 2)
	}
	if cfg.Crawler.RateLimit != 0.5 {
		t.Errorf("Crawler.RateLimit = %v, want %v", cfg.Crawler.RateLimit, 0.5)
	}
	if cfg.Crawler.Timeout != 30*time.Second {
		t.Errorf("Crawler.Timeout = %v, want %v", cfg.Crawler.Timeout, 30*time.Second)
	}
	if cfg.Crawler.MaxRetries != 3 {
		t.Errorf("Crawler.MaxRetries = %v, want %v", cfg.Crawler.MaxRetries, 3)
	}
	if cfg.Crawler.Concurrency != 3 {
		t.Errorf("Crawler.Concurrency = %v, want %v", cfg.Crawler.Concurrency, 3)
	}

	// Test Server defaults
	if cfg.Server.Port != 8080 {
		t.Errorf("Server.Port = %v, want %v", cfg.Server.Port, 8080)
	}
}


func TestLoad_MissingBotToken(t *testing.T) {
	// Clear any existing env vars
	os.Unsetenv("BOT_TOKEN")
	os.Setenv("DB_PASSWORD", "test-pass")
	defer os.Unsetenv("DB_PASSWORD")

	_, err := Load()
	if err == nil {
		t.Error("Load() expected error for missing BOT_TOKEN, got nil")
	}
}

func TestLoad_MissingDBPassword(t *testing.T) {
	os.Setenv("BOT_TOKEN", "test-token")
	os.Unsetenv("DB_PASSWORD")
	defer os.Unsetenv("BOT_TOKEN")

	_, err := Load()
	if err == nil {
		t.Error("Load() expected error for missing DB_PASSWORD, got nil")
	}
}

func TestConfig_Validate(t *testing.T) {
	tests := []struct {
		name    string
		cfg     Config
		wantErr bool
	}{
		{
			name: "valid config",
			cfg: Config{
				Bot:     BotConfig{Token: "token"},
				DB:      DBConfig{Password: "pass"},
				Crawler: CrawlerConfig{RateLimit: 0.5, Concurrency: 3},
				Server:  ServerConfig{Port: 8080},
			},
			wantErr: false,
		},
		{
			name: "missing bot token",
			cfg: Config{
				Bot:     BotConfig{Token: ""},
				DB:      DBConfig{Password: "pass"},
				Crawler: CrawlerConfig{RateLimit: 0.5, Concurrency: 3},
				Server:  ServerConfig{Port: 8080},
			},
			wantErr: true,
		},
		{
			name: "missing db password",
			cfg: Config{
				Bot:     BotConfig{Token: "token"},
				DB:      DBConfig{Password: ""},
				Crawler: CrawlerConfig{RateLimit: 0.5, Concurrency: 3},
				Server:  ServerConfig{Port: 8080},
			},
			wantErr: true,
		},
		{
			name: "invalid rate limit",
			cfg: Config{
				Bot:     BotConfig{Token: "token"},
				DB:      DBConfig{Password: "pass"},
				Crawler: CrawlerConfig{RateLimit: 0, Concurrency: 3},
				Server:  ServerConfig{Port: 8080},
			},
			wantErr: true,
		},
		{
			name: "invalid concurrency",
			cfg: Config{
				Bot:     BotConfig{Token: "token"},
				DB:      DBConfig{Password: "pass"},
				Crawler: CrawlerConfig{RateLimit: 0.5, Concurrency: 0},
				Server:  ServerConfig{Port: 8080},
			},
			wantErr: true,
		},
		{
			name: "invalid port",
			cfg: Config{
				Bot:     BotConfig{Token: "token"},
				DB:      DBConfig{Password: "pass"},
				Crawler: CrawlerConfig{RateLimit: 0.5, Concurrency: 3},
				Server:  ServerConfig{Port: 0},
			},
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := tt.cfg.Validate()
			if (err != nil) != tt.wantErr {
				t.Errorf("Validate() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}

func TestDBConfig_DSN(t *testing.T) {
	cfg := DBConfig{
		Host:     "localhost",
		Port:     3306,
		User:     "root",
		Password: "secret",
		Database: "testdb",
	}

	expected := "root:secret@tcp(localhost:3306)/testdb?charset=utf8mb4&parseTime=True&loc=Local"
	if got := cfg.DSN(); got != expected {
		t.Errorf("DSN() = %v, want %v", got, expected)
	}
}
