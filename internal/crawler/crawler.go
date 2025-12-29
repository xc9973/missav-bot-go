package crawler

import (
	"context"

	"github.com/user/missav-bot-go/internal/model"
)

// Crawler defines the interface for crawling video data
type Crawler interface {
	// CrawlNewVideos crawls the latest video list
	CrawlNewVideos(ctx context.Context, pages int) ([]*model.Video, error)

	// CrawlVideoDetail crawls video details from a detail URL
	CrawlVideoDetail(ctx context.Context, detailURL string) (*model.Video, error)

	// CrawlByActor crawls videos by actor name
	CrawlByActor(ctx context.Context, actorName string, limit int) ([]*model.Video, error)

	// CrawlByCode crawls a video by its code
	CrawlByCode(ctx context.Context, code string) (*model.Video, error)

	// CrawlByKeyword searches and crawls videos by keyword
	CrawlByKeyword(ctx context.Context, keyword string, limit int) ([]*model.Video, error)

	// Close releases crawler resources
	Close() error
}

// CrawlerConfig holds configuration for the crawler
type CrawlerConfig struct {
	// Enabled indicates if crawling is enabled
	Enabled bool
	// RateLimit is the maximum requests per second
	RateLimit float64
	// Timeout is the HTTP request timeout
	Timeout int // seconds
	// MaxRetries is the maximum number of retry attempts
	MaxRetries int
	// Concurrency is the number of concurrent workers
	Concurrency int
	// UserAgent is the HTTP User-Agent header
	UserAgent string
	// ProxyURL is the proxy server URL (HTTP or SOCKS5)
	ProxyURL string
	// InitialPages is the number of pages to crawl initially
	InitialPages int
}

// DefaultCrawlerConfig returns default crawler configuration
func DefaultCrawlerConfig() *CrawlerConfig {
	return &CrawlerConfig{
		Enabled:      true,
		RateLimit:    0.5, // 0.5 req/sec = 1 request per 2 seconds
		Timeout:      30,
		MaxRetries:   3,
		Concurrency:  3,
		UserAgent:    "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/120.0.0.0 Safari/537.36",
		InitialPages: 2,
	}
}
