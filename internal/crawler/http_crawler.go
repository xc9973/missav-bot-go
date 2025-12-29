package crawler

import (
	"context"
	"fmt"
	"io"
	"math"
	"net/http"
	"net/url"
	"strings"
	"sync"
	"time"

	"github.com/user/missav-bot-go/internal/model"
	"golang.org/x/time/rate"
)

const (
	newVideosPath = "/new"
	actressesPath = "/actresses/"
	searchPath    = "/search/"
)

// HTTPCrawler implements the Crawler interface using HTTP requests
type HTTPCrawler struct {
	client    *http.Client
	limiter   *rate.Limiter
	config    *CrawlerConfig
	parser    *Parser
	browser   *Browser
	browserMu sync.Mutex
}

// NewHTTPCrawler creates a new HTTP crawler instance
func NewHTTPCrawler(cfg *CrawlerConfig) (*HTTPCrawler, error) {
	if cfg == nil {
		cfg = DefaultCrawlerConfig()
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

// CrawlNewVideos crawls the latest video list
func (c *HTTPCrawler) CrawlNewVideos(ctx context.Context, pages int) ([]*model.Video, error) {
	var allVideos []*model.Video

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

		html, err := c.fetchWithRetry(ctx, pageURL)
		if err != nil {
			// If first page fails with HTTP, try headless browser
			if page == 1 {
				videos, browserErr := c.crawlWithBrowser(ctx, pageURL)
				if browserErr == nil && len(videos) > 0 {
					allVideos = append(allVideos, videos...)
					continue
				}
			}
			continue
		}

		videos, err := c.parser.ParseVideoList(html)
		if err != nil {
			continue
		}

		// If first page returns empty, try headless browser
		if page == 1 && len(videos) == 0 {
			browserVideos, browserErr := c.crawlWithBrowser(ctx, pageURL)
			if browserErr == nil && len(browserVideos) > 0 {
				videos = browserVideos
			}
		}

		allVideos = append(allVideos, videos...)
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

		html, err := c.fetchWithRetry(ctx, pageURL)
		if err != nil {
			break
		}

		videos, err := c.parser.ParseVideoList(html)
		if err != nil || len(videos) == 0 {
			break
		}

		allVideos = append(allVideos, videos...)

		if len(allVideos) >= limit {
			allVideos = allVideos[:limit]
			break
		}

		page++
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
			browserVideos, browserErr := c.crawlWithBrowser(ctx, pageURL)
			if browserErr == nil && len(browserVideos) > 0 {
				videos = browserVideos
			}
		}

		if len(videos) == 0 {
			break
		}

		allVideos = append(allVideos, videos...)

		if len(allVideos) >= limit {
			allVideos = allVideos[:limit]
			break
		}

		page++
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

	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("HTTP status %d", resp.StatusCode)
	}

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", fmt.Errorf("read body error: %w", err)
	}

	return string(body), nil
}

// crawlWithBrowser uses headless browser to crawl a page
func (c *HTTPCrawler) crawlWithBrowser(ctx context.Context, pageURL string) ([]*model.Video, error) {
	browser, err := c.getBrowser()
	if err != nil {
		return nil, err
	}

	html, err := browser.FetchRenderedHTML(ctx, pageURL, "div.group")
	if err != nil {
		return nil, err
	}

	return c.parser.ParseVideoList(html)
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
