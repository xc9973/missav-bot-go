package scheduler

import (
	"context"
	"sync"
	"sync/atomic"
	"time"

	"github.com/rs/zerolog/log"
	"github.com/user/missav-bot-go/internal/config"
	"github.com/user/missav-bot-go/internal/crawler"
	"github.com/user/missav-bot-go/internal/push"
	"github.com/user/missav-bot-go/internal/store"
)

// Scheduler manages periodic crawl tasks
type Scheduler struct {
	crawler     crawler.Crawler
	store       store.Store
	pushService *push.Service
	config      *config.CrawlerConfig
	running     atomic.Bool
	mu          sync.Mutex // Mutex to prevent concurrent crawl tasks (Requirement 6.3)
	stopCh      chan struct{}
	wg          sync.WaitGroup
}

// NewScheduler creates a new scheduler instance
func NewScheduler(
	crawler crawler.Crawler,
	store store.Store,
	pushService *push.Service,
	cfg *config.CrawlerConfig,
) *Scheduler {
	return &Scheduler{
		crawler:     crawler,
		store:       store,
		pushService: pushService,
		config:      cfg,
		stopCh:      make(chan struct{}),
	}
}

// Start begins the scheduler with initial delay and periodic execution
// Requirement 6.1: Execute initial crawl after configurable delay (default 5s)
// Requirement 6.2: Execute periodic crawl at configurable interval (default 15 minutes)
func (s *Scheduler) Start(ctx context.Context) {
	if !s.config.Enabled {
		log.Info().Msg("Scheduler is disabled")
		return
	}

	s.wg.Add(1)
	go s.run(ctx)
}


// run is the main scheduler loop
func (s *Scheduler) run(ctx context.Context) {
	defer s.wg.Done()

	// Initial delay before first crawl (Requirement 6.1)
	initialDelay := 5 * time.Second
	log.Info().Dur("delay", initialDelay).Msg("Scheduler starting with initial delay")

	select {
	case <-time.After(initialDelay):
		// Execute initial crawl
		s.executeCrawl(ctx)
	case <-s.stopCh:
		log.Info().Msg("Scheduler stopped during initial delay")
		return
	case <-ctx.Done():
		log.Info().Msg("Scheduler context cancelled during initial delay")
		return
	}

	// Periodic execution (Requirement 6.2)
	ticker := time.NewTicker(s.config.Interval)
	defer ticker.Stop()

	log.Info().Dur("interval", s.config.Interval).Msg("Scheduler started periodic execution")

	for {
		select {
		case <-ticker.C:
			s.executeCrawl(ctx)
		case <-s.stopCh:
			log.Info().Msg("Scheduler stopped")
			return
		case <-ctx.Done():
			log.Info().Msg("Scheduler context cancelled")
			return
		}
	}
}

// executeCrawl runs a single crawl task with mutex protection
// Requirement 6.3: Skip new triggers using mutex lock when a crawl task is running
func (s *Scheduler) executeCrawl(ctx context.Context) {
	// Try to acquire the mutex without blocking
	if !s.mu.TryLock() {
		log.Warn().Msg("Crawl task already running, skipping this trigger")
		return
	}
	defer s.mu.Unlock()

	s.running.Store(true)
	defer s.running.Store(false)

	startTime := time.Now()
	log.Info().Int("pages", s.config.InitialPages).Msg("Starting scheduled crawl")

	// Execute the crawl
	if err := s.RunOnce(ctx, s.config.InitialPages); err != nil {
		log.Error().Err(err).Msg("Scheduled crawl failed")
	}

	// Log execution time (Requirement 6.5)
	duration := time.Since(startTime)
	log.Info().
		Dur("duration", duration).
		Msg("Scheduled crawl completed")
}

// RunOnce executes a single crawl and push cycle
// Requirement 6.4: Trigger push for all unpushed videos after crawl completes
func (s *Scheduler) RunOnce(ctx context.Context, pages int) error {
	// Crawl new videos
	videos, err := s.crawler.CrawlNewVideos(ctx, pages)
	if err != nil {
		return err
	}

	log.Info().Int("count", len(videos)).Msg("Crawled videos")

	// Save videos to store
	if len(videos) > 0 {
		saved, duplicates, err := s.store.SaveVideos(ctx, videos)
		if err != nil {
			log.Error().Err(err).Msg("Failed to save videos")
		} else {
			log.Info().
				Int("saved", saved).
				Int("duplicates", duplicates).
				Msg("Videos saved to database")
		}
	}

	// Push unpushed videos to subscribers (Requirement 6.4)
	if err := s.pushService.PushUnpushedVideos(ctx); err != nil {
		log.Error().Err(err).Msg("Failed to push videos")
	}

	return nil
}

// Stop gracefully stops the scheduler
func (s *Scheduler) Stop() {
	log.Info().Msg("Stopping scheduler...")
	close(s.stopCh)
	s.wg.Wait()
	log.Info().Msg("Scheduler stopped")
}

// IsRunning returns true if a crawl task is currently running
func (s *Scheduler) IsRunning() bool {
	return s.running.Load()
}

// TryRun attempts to run a crawl task immediately
// Returns false if a task is already running
func (s *Scheduler) TryRun(ctx context.Context, pages int) bool {
	if !s.mu.TryLock() {
		return false
	}
	defer s.mu.Unlock()

	s.running.Store(true)
	defer s.running.Store(false)

	startTime := time.Now()
	log.Info().Int("pages", pages).Msg("Starting manual crawl")

	if err := s.RunOnce(ctx, pages); err != nil {
		log.Error().Err(err).Msg("Manual crawl failed")
	}

	duration := time.Since(startTime)
	log.Info().Dur("duration", duration).Msg("Manual crawl completed")

	return true
}
