package crawler

import (
	"context"
	"fmt"
	"io"
	"math"
	"math/rand"
	"net/http"
	"net/http/cookiejar"
	"net/url"
	"strings"
	"sync"
	"time"

	"github.com/rs/zerolog/log"
	"github.com/user/missav-bot-go/internal/model"
	"golang.org/x/time/rate"
)

const (
	newVideosPath = "/new"
	actressesPath = "/actresses/"
	searchPath    = "/search/"
	// Cookie 有效期 10 分钟
	cookieExpireDuration = 10 * time.Minute
)

// HTTPCrawler implements the Crawler interface using HTTP requests
type HTTPCrawler struct {
	client         *http.Client
	limiter        *rate.Limiter
	config         *CrawlerConfig
	parser         *Parser
	browser        *Browser
	browserMu      sync.Mutex
	cookieInitTime time.Time
	cookieMu       sync.Mutex
}

// NewHTTPCrawler creates a new HTTP crawler instance
func NewHTTPCrawler(cfg *CrawlerConfig) (*HTTPCrawler, error) {
	if cfg == nil {
		cfg = DefaultCrawlerConfig()
	}

	// Create cookie jar for session management
	jar, err := cookiejar.New(nil)
	if err != nil {
		return nil, fmt.Errorf("failed to create cookie jar: %w", err)
	}

	// Create HTTP client with connection pooling
	transport := &http.Transport{
		MaxIdleConns:        100,
		MaxIdleConnsPerHost: 10,
		IdleConnTimeout:     90 * time.Second,
	}

	// Configure proxy if provided
	if cfg.ProxyURL != "" {
		proxyURL, err := url.Parse(cfg.ProxyURL)
		if err != nil {
			return nil, fmt.Errorf("invalid proxy URL: %w", err)
		}
		transport.Proxy = http.ProxyURL(proxyURL)
	}

	client := &http.Client{
		Transport: transport,
		Timeout:   time.Duration(cfg.Timeout) * time.Second,
		Jar:       jar, // Enable cookie handling
	}

	// Create rate limiter using token bucket algorithm
	// rate.Limit is events per second
	limiter := rate.NewLimiter(rate.Limit(cfg.RateLimit), 1)

	return &HTTPCrawler{
		client:  client,
		limiter: limiter,
		config:  cfg,
		parser:  NewParser(),
	}, nil
}

// initCookies initializes cookies by making warmup requests to establish a session
// This is required because the website needs multiple requests to establish a valid session
func (c *HTTPCrawler) initCookies(ctx context.Context) {
	c.cookieMu.Lock()
	defer c.cookieMu.Unlock()

	// Check if cookies are still valid
	if !c.cookieInitTime.IsZero() && time.Since(c.cookieInitTime) < cookieExpireDuration {
		log.Debug().
			Dur("age", time.Since(c.cookieInitTime)).
			Msg("Cookies still valid, skipping initialization")
		return
	}

	if c.cookieInitTime.IsZero() {
		log.Info().Msg("Initializing cookies (warming up session)...")
	} else {
		log.Info().
			Time("lastInit", c.cookieInitTime).
			Msg("Cookies expired, re-initializing...")
	}

	// Make 3 warmup requests to establish session (like Java version)
	warmupURL := BaseURL + newVideosPath + "?page=2"
	for i := 1; i <= 3; i++ {
		select {
		case <-ctx.Done():
			return
		default:
		}

		_, err := c.fetch(ctx, warmupURL)
		if err != nil {
			log.Warn().Err(err).Int("attempt", i).Msg("Cookie warmup request failed")
		} else {
			log.Debug().Int("attempt", i).Msg("Cookie warmup request completed")
		}

		if i < 3 {
			time.Sleep(1 * time.Second)
		}
	}

	c.cookieInitTime = time.Now()
	log.Info().Msg("Cookie initialization completed")
}

// CrawlNewVideos crawls the latest video list
func (c *HTTPCrawler) CrawlNewVideos(ctx context.Context, pages int) ([]*model.Video, error) {
	var allVideos []*model.Video

	// Initialize cookies before crawling
	c.initCookies(ctx)

	for page := 1; page <= pages; page++ {
		select {
		case <-ctx.Done():
			return allVideos, ctx.Err()
		default:
		}

		pageURL := BaseURL + newVideosPath
		if page > 1 {
			pageURL = fmt.Sprintf("%s?page=%d", pageURL, page)
		}

		log.Info().Str("url", pageURL).Int("page", page).Msg("Crawling new videos page")

		html, err := c.fetchWithRetry(ctx, pageURL)
		if err != nil {
			log.Warn().Err(err).Str("url", pageURL).Msg("HTTP fetch failed, trying browser")
			// If first page fails with HTTP, try headless browser
			if page == 1 {
				videos, browserErr := c.crawlWithBrowser(ctx, pageURL)
				if browserErr == nil && len(videos) > 0 {
					log.Info().Int("count", len(videos)).Msg("Browser crawl succeeded")
					allVideos = append(allVideos, videos...)
					continue
				}
				log.Warn().Err(browserErr).Msg("Browser crawl also failed")
			}
			continue
		}

		videos, err := c.parser.ParseVideoList(html)
		if err != nil {
			log.Warn().Err(err).Msg("Failed to parse video list")
			continue
		}

		log.Info().Int("count", len(videos)).Int("page", page).Msg("Parsed videos from HTTP response")

		// If first page returns empty, try headless browser
		if page == 1 && len(videos) == 0 {
			log.Warn().Msg("HTTP returned 0 videos, trying browser fallback")
			browserVideos, browserErr := c.crawlWithBrowser(ctx, pageURL)
			if browserErr == nil && len(browserVideos) > 0 {
				log.Info().Int("count", len(browserVideos)).Msg("Browser fallback succeeded")
				videos = browserVideos
			} else {
				log.Warn().Err(browserErr).Msg("Browser fallback also returned 0 videos")
			}
		}

		allVideos = append(allVideos, videos...)

		// Add delay between pages (like Java version)
		if page < pages {
			time.Sleep(2 * time.Second)
		}
	}

	return allVideos, nil
}

// CrawlVideoDetail crawls video details from a detail URL
func (c *HTTPCrawler) CrawlVideoDetail(ctx context.Context, detailURL string) (*model.Video, error) {
	html, err := c.fetchWithRetry(ctx, detailURL)
	if err != nil {
		// Fallback to headless browser
		return c.crawlDetailWithBrowser(ctx, detailURL)
	}

	return c.parser.ParseVideoDetail(html, detailURL)
}

// CrawlByActor crawls videos by actor name
func (c *HTTPCrawler) CrawlByActor(ctx context.Context, actorName string, limit int) ([]*model.Video, error) {
	var allVideos []*model.Video
	page := 1
	// Estimate max pages needed (assuming ~12 videos per page)
	maxPages := (limit + 11) / 12

	// Initialize cookies before crawling
	c.initCookies(ctx)

	encodedName := url.PathEscape(actorName)

	for page <= maxPages {
		select {
		case <-ctx.Done():
			return allVideos, ctx.Err()
		default:
		}

		pageURL := BaseURL + actressesPath + encodedName
		if page > 1 {
			pageURL = fmt.Sprintf("%s?page=%d", pageURL, page)
		}

		log.Info().Str("url", pageURL).Str("actor", actorName).Int("page", page).Msg("Crawling actor videos")

		html, err := c.fetchWithRetry(ctx, pageURL)
		if err != nil {
			break
		}

		videos, err := c.parser.ParseVideoList(html)
		if err != nil || len(videos) == 0 {
			break
		}

		allVideos = append(allVideos, videos...)
		log.Info().Int("pageCount", len(videos)).Int("total", len(allVideos)).Msg("Parsed actor videos")

		if len(allVideos) >= limit {
			allVideos = allVideos[:limit]
			break
		}

		page++
		time.Sleep(2 * time.Second)
	}

	return allVideos, nil
}

// CrawlByCode crawls a video by its code
func (c *HTTPCrawler) CrawlByCode(ctx context.Context, code string) (*model.Video, error) {
	detailURL := BaseURL + "/" + strings.ToLower(code)
	return c.CrawlVideoDetail(ctx, detailURL)
}

// CrawlByKeyword searches and crawls videos by keyword
func (c *HTTPCrawler) CrawlByKeyword(ctx context.Context, keyword string, limit int) ([]*model.Video, error) {
	var allVideos []*model.Video
	page := 1
	maxPages := (limit + 11) / 12

	// Initialize cookies before crawling
	c.initCookies(ctx)

	encodedKeyword := url.PathEscape(keyword)

	for page <= maxPages {
		select {
		case <-ctx.Done():
			return allVideos, ctx.Err()
		default:
		}

		pageURL := BaseURL + searchPath + encodedKeyword
		if page > 1 {
			pageURL = fmt.Sprintf("%s?page=%d", pageURL, page)
		}

		log.Info().Str("url", pageURL).Str("keyword", keyword).Int("page", page).Msg("Crawling keyword search")

		html, err := c.fetchWithRetry(ctx, pageURL)
		if err != nil {
			// Try headless browser on first page
			if page == 1 {
				browserVideos, browserErr := c.crawlWithBrowser(ctx, pageURL)
				if browserErr == nil && len(browserVideos) > 0 {
					allVideos = append(allVideos, browserVideos...)
					page++
					continue
				}
			}
			break
		}

		videos, err := c.parser.ParseVideoList(html)
		if err != nil {
			break
		}

		// If first page returns empty, try headless browser
		if page == 1 && len(videos) == 0 {
			log.Warn().Msg("HTTP returned 0 videos for keyword search, trying browser")
			browserVideos, browserErr := c.crawlWithBrowser(ctx, pageURL)
			if browserErr == nil && len(browserVideos) > 0 {
				videos = browserVideos
			}
		}

		if len(videos) == 0 {
			break
		}

		allVideos = append(allVideos, videos...)
		log.Info().Int("pageCount", len(videos)).Int("total", len(allVideos)).Msg("Parsed keyword search videos")

		if len(allVideos) >= limit {
			allVideos = allVideos[:limit]
			break
		}

		page++
		time.Sleep(2 * time.Second)
	}

	return allVideos, nil
}

// Close releases crawler resources
func (c *HTTPCrawler) Close() error {
	c.browserMu.Lock()
	defer c.browserMu.Unlock()

	if c.browser != nil {
		return c.browser.Close()
	}
	return nil
}

// fetchWithRetry fetches a URL with rate limiting and exponential backoff retry
func (c *HTTPCrawler) fetchWithRetry(ctx context.Context, targetURL string) (string, error) {
	var lastErr error

	for attempt := 0; attempt <= c.config.MaxRetries; attempt++ {
		// Wait for rate limiter
		if err := c.limiter.Wait(ctx); err != nil {
			return "", fmt.Errorf("rate limiter error: %w", err)
		}

		html, err := c.fetch(ctx, targetURL)
		if err == nil {
			return html, nil
		}

		lastErr = err

		// Check if we should retry
		if attempt < c.config.MaxRetries {
			// Exponential backoff: 1s, 2s, 4s
			backoff := time.Duration(math.Pow(2, float64(attempt))) * time.Second
			select {
			case <-ctx.Done():
				return "", ctx.Err()
			case <-time.After(backoff):
			}
		}
	}

	return "", fmt.Errorf("max retries exceeded: %w", lastErr)
}

// fetch performs a single HTTP request
func (c *HTTPCrawler) fetch(ctx context.Context, targetURL string) (string, error) {
	// Add random delay to avoid being blocked (1-3 seconds like Java version)
	delay := time.Duration(1000+rand.Intn(2000)) * time.Millisecond
	select {
	case <-ctx.Done():
		return "", ctx.Err()
	case <-time.After(delay):
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, targetURL, nil)
	if err != nil {
		return "", fmt.Errorf("create request error: %w", err)
	}

	// Set headers
	req.Header.Set("User-Agent", c.config.UserAgent)
	req.Header.Set("Accept", "text/html,application/xhtml+xml,application/xml;q=0.9,image/avif,image/webp,image/apng,*/*;q=0.8")
	req.Header.Set("Accept-Language", "zh-CN,zh;q=0.9,en-US;q=0.8,en;q=0.7")
	req.Header.Set("Referer", BaseURL+"/")
	req.Header.Set("Connection", "keep-alive")
	req.Header.Set("Cache-Control", "max-age=0")

	resp, err := c.client.Do(req)
	if err != nil {
		return "", fmt.Errorf("request error: %w", err)
	}
	defer resp.Body.Close()

	log.Debug().
		Int("status", resp.StatusCode).
		Str("url", targetURL).
		Str("finalURL", resp.Request.URL.String()).
		Msg("HTTP response")

	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("HTTP status %d", resp.StatusCode)
	}

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", fmt.Errorf("read body error: %w", err)
	}

	html := string(body)
	log.Debug().Int("length", len(html)).Msg("Received HTML")

	return html, nil
}

// crawlWithBrowser uses headless browser to crawl a page
func (c *HTTPCrawler) crawlWithBrowser(ctx context.Context, pageURL string) ([]*model.Video, error) {
	log.Info().Str("url", pageURL).Msg("Starting browser crawl")

	browser, err := c.getBrowser()
	if err != nil {
		log.Error().Err(err).Msg("Failed to get browser instance")
		return nil, err
	}

	html, err := browser.FetchRenderedHTML(ctx, pageURL, "div.group")
	if err != nil {
		log.Error().Err(err).Msg("Browser failed to fetch HTML")
		return nil, err
	}

	log.Info().Int("htmlLength", len(html)).Msg("Browser fetched HTML")

	// Log first 500 chars for debugging
	if len(html) > 0 {
		preview := html
		if len(preview) > 500 {
			preview = preview[:500]
		}
		log.Debug().Str("preview", preview).Msg("HTML preview")
	}

	videos, err := c.parser.ParseVideoList(html)
	if err != nil {
		log.Error().Err(err).Msg("Failed to parse browser HTML")
		return nil, err
	}

	log.Info().Int("count", len(videos)).Msg("Browser crawl completed")
	return videos, nil
}

// crawlDetailWithBrowser uses headless browser to crawl video detail
func (c *HTTPCrawler) crawlDetailWithBrowser(ctx context.Context, detailURL string) (*model.Video, error) {
	browser, err := c.getBrowser()
	if err != nil {
		return nil, err
	}

	html, err := browser.FetchRenderedHTML(ctx, detailURL, "body")
	if err != nil {
		return nil, err
	}

	return c.parser.ParseVideoDetail(html, detailURL)
}

// getBrowser returns the browser instance, creating it if necessary
func (c *HTTPCrawler) getBrowser() (*Browser, error) {
	c.browserMu.Lock()
	defer c.browserMu.Unlock()

	if c.browser == nil {
		browser, err := NewBrowser()
		if err != nil {
			return nil, err
		}
		c.browser = browser
	}

	return c.browser, nil
}

// GetLimiter returns the rate limiter for testing purposes
func (c *HTTPCrawler) GetLimiter() *rate.Limiter {
	return c.limiter
}

// GetRateLimit returns the configured rate limit
func (c *HTTPCrawler) GetRateLimit() float64 {
	return c.config.RateLimit
}
