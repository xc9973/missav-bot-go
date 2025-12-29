package main

import (
	"context"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/rs/zerolog"
	"github.com/rs/zerolog/log"
	"github.com/user/missav-bot-go/internal/bot"
	"github.com/user/missav-bot-go/internal/config"
	"github.com/user/missav-bot-go/internal/crawler"
	"github.com/user/missav-bot-go/internal/push"
	"github.com/user/missav-bot-go/internal/scheduler"
	"github.com/user/missav-bot-go/internal/server"
	"github.com/user/missav-bot-go/internal/store"
)

const (
	// ShutdownTimeout is the maximum time to wait for graceful shutdown
	ShutdownTimeout = 30 * time.Second
)

func main() {
	// Initialize structured JSON logging (Requirement 8.5)
	zerolog.TimeFieldFormat = zerolog.TimeFormatUnix
	log.Logger = zerolog.New(os.Stdout).With().Timestamp().Caller().Logger()

	// Load configuration (Requirement 7.1, 7.2, 7.3)
	cfg, err := config.Load()
	if err != nil {
		log.Fatal().Err(err).Msg("Failed to load configuration")
	}

	// Validate configuration (Requirement 7.5, 7.6)
	if err := cfg.Validate(); err != nil {
		log.Fatal().Err(err).Msg("Invalid configuration")
	}

	log.Info().Msg("Configuration loaded successfully")

	// Create root context for graceful shutdown
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// Initialize MySQL store (Requirement 4.1, 4.7)
	mysqlStore, err := store.NewMySQLStore(&cfg.DB)
	if err != nil {
		log.Fatal().Err(err).Msg("Failed to connect to database")
	}
	log.Info().Msg("Database connection established")

	// Initialize HTTP crawler
	crawlerCfg := &crawler.CrawlerConfig{
		Enabled:      cfg.Crawler.Enabled,
		RateLimit:    cfg.Crawler.RateLimit,
		Timeout:      int(cfg.Crawler.Timeout.Seconds()),
		MaxRetries:   cfg.Crawler.MaxRetries,
		Concurrency:  cfg.Crawler.Concurrency,
		UserAgent:    cfg.Crawler.UserAgent,
		ProxyURL:     cfg.Crawler.ProxyURL,
		InitialPages: cfg.Crawler.InitialPages,
	}
	httpCrawler, err := crawler.NewHTTPCrawler(crawlerCfg)
	if err != nil {
		log.Fatal().Err(err).Msg("Failed to create crawler")
	}
	log.Info().Msg("Crawler initialized")

	// Initialize Telegram client (Requirement 3.1)
	telegramClient, err := bot.NewClient(cfg.Bot.Token)
	if err != nil {
		log.Fatal().Err(err).Msg("Failed to create Telegram client")
	}
	log.Info().Msg("Telegram client initialized")

	// Initialize push service (Requirement 5.1)
	pushService := push.NewService(mysqlStore, telegramClient)
	log.Info().Msg("Push service initialized")

	// Initialize bot handler (Requirement 3.1)
	botHandler := bot.NewHandler(mysqlStore, httpCrawler, pushService, telegramClient)
	log.Info().Msg("Bot handler initialized")

	// Initialize scheduler (Requirement 6.1, 6.2)
	sched := scheduler.NewScheduler(httpCrawler, mysqlStore, pushService, &cfg.Crawler)

	// Initialize HTTP server (Requirement 8.1)
	httpServer := server.NewServer(mysqlStore)

	// Setup signal handling for graceful shutdown (Requirement 9.1)
	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, syscall.SIGINT, syscall.SIGTERM)

	// Start HTTP server in goroutine
	go func() {
		log.Info().Int("port", cfg.Server.Port).Msg("Starting HTTP server")
		if err := httpServer.Start(cfg.Server.Port); err != nil && err != http.ErrServerClosed {
			log.Error().Err(err).Msg("HTTP server error")
		}
	}()

	// Start scheduler (Requirement 6.1)
	sched.Start(ctx)
	log.Info().Msg("Scheduler started")

	// Start Telegram bot polling in goroutine
	go func() {
		log.Info().Msg("Starting Telegram bot polling")
		updates := telegramClient.GetUpdates()
		for update := range updates {
			botHandler.HandleUpdate(ctx, update)
		}
	}()

	log.Info().Msg("MissAV Bot started successfully")

	// Wait for shutdown signal
	sig := <-sigCh
	log.Info().Str("signal", sig.String()).Msg("Received shutdown signal")

	// Create shutdown context with timeout (Requirement 9.2)
	shutdownCtx, shutdownCancel := context.WithTimeout(context.Background(), ShutdownTimeout)
	defer shutdownCancel()

	// Graceful shutdown sequence
	log.Info().Msg("Starting graceful shutdown...")

	// 1. Stop scheduler from triggering new tasks (Requirement 9.1)
	sched.Stop()
	log.Info().Msg("Scheduler stopped")

	// 2. Stop Telegram bot polling (Requirement 9.3)
	telegramClient.StopReceivingUpdates()
	log.Info().Msg("Telegram bot polling stopped")

	// 3. Stop HTTP server
	if err := httpServer.Stop(shutdownCtx); err != nil {
		log.Error().Err(err).Msg("Error stopping HTTP server")
	} else {
		log.Info().Msg("HTTP server stopped")
	}

	// 4. Close crawler (closes headless browser instances) (Requirement 9.5)
	if err := httpCrawler.Close(); err != nil {
		log.Error().Err(err).Msg("Error closing crawler")
	} else {
		log.Info().Msg("Crawler closed")
	}

	// 5. Close database connection pool (Requirement 9.4)
	if err := mysqlStore.Close(); err != nil {
		log.Error().Err(err).Msg("Error closing database connection")
	} else {
		log.Info().Msg("Database connection closed")
	}

	// Cancel root context
	cancel()

	// Check if shutdown completed within timeout (Requirement 9.6)
	select {
	case <-shutdownCtx.Done():
		if shutdownCtx.Err() == context.DeadlineExceeded {
			log.Warn().Msg("Shutdown timeout exceeded, forcing exit")
		}
	default:
		log.Info().Msg("Graceful shutdown completed")
	}
}
