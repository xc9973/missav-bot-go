package crawler

import (
	"context"
	"fmt"
	"sync"
	"time"

	"github.com/go-rod/rod"
	"github.com/go-rod/rod/lib/launcher"
	"github.com/go-rod/rod/lib/proto"
	"github.com/rs/zerolog/log"
)

const (
	// DefaultWaitTimeout is the maximum time to wait for page elements
	DefaultWaitTimeout = 15 * time.Second
	// DefaultPageLoadTimeout is the maximum time to wait for page load
	DefaultPageLoadTimeout = 30 * time.Second
)

// Browser wraps rod browser for headless browsing with instance reuse
type Browser struct {
	browser  *rod.Browser
	launcher *launcher.Launcher
	mu       sync.Mutex
	closed   bool
}

// BrowserConfig holds configuration for the browser
type BrowserConfig struct {
	// Headless indicates if browser should run in headless mode
	Headless bool
	// UserAgent is the browser user agent string
	UserAgent string
	// ProxyURL is the proxy server URL
	ProxyURL string
}

// DefaultBrowserConfig returns default browser configuration
func DefaultBrowserConfig() *BrowserConfig {
	return &BrowserConfig{
		Headless:  true,
		UserAgent: "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/120.0.0.0 Safari/537.36",
	}
}

// NewBrowser creates a new Browser instance with default configuration
func NewBrowser() (*Browser, error) {
	return NewBrowserWithConfig(DefaultBrowserConfig())
}

// NewBrowserWithConfig creates a new Browser instance with custom configuration
func NewBrowserWithConfig(cfg *BrowserConfig) (*Browser, error) {
	if cfg == nil {
		cfg = DefaultBrowserConfig()
	}

	// Create launcher with configuration
	l := launcher.New().
		Headless(cfg.Headless).
		Set("disable-gpu").
		Set("no-sandbox").
		Set("disable-dev-shm-usage").
		Set("disable-extensions").
		Set("disable-background-networking").
		Set("disable-sync").
		Set("disable-translate").
		Set("metrics-recording-only").
		Set("mute-audio").
		Set("no-first-run").
		Set("safebrowsing-disable-auto-update")

	// Configure proxy if provided
	if cfg.ProxyURL != "" {
		l = l.Proxy(cfg.ProxyURL)
	}

	// Launch browser
	controlURL, err := l.Launch()
	if err != nil {
		return nil, fmt.Errorf("failed to launch browser: %w", err)
	}

	// Connect to browser
	browser := rod.New().ControlURL(controlURL)
	if err := browser.Connect(); err != nil {
		return nil, fmt.Errorf("failed to connect to browser: %w", err)
	}

	// Set default user agent
	if cfg.UserAgent != "" {
		// User agent will be set per page
	}

	return &Browser{
		browser:  browser,
		launcher: l,
		closed:   false,
	}, nil
}

// FetchRenderedHTML fetches a page and waits for JavaScript rendering
// waitSelector is the CSS selector to wait for (max 15 seconds)
func (b *Browser) FetchRenderedHTML(ctx context.Context, url string, waitSelector string) (string, error) {
	b.mu.Lock()
	defer b.mu.Unlock()

	if b.closed {
		return "", fmt.Errorf("browser is closed")
	}

	// Create a new page
	page, err := b.browser.Page(proto.TargetCreateTarget{URL: "about:blank"})
	if err != nil {
		return "", fmt.Errorf("failed to create page: %w", err)
	}
	defer page.Close()

	// Set page timeout (longer for Cloudflare challenge)
	page = page.Timeout(60 * time.Second)

	// Set user agent
	err = page.SetUserAgent(&proto.NetworkSetUserAgentOverride{
		UserAgent: "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/120.0.0.0 Safari/537.36",
	})
	if err != nil {
		// Non-fatal error, continue
	}

	log.Info().Str("url", url).Msg("Browser navigating to URL")

	// Navigate to URL
	if err := page.Navigate(url); err != nil {
		return "", fmt.Errorf("failed to navigate to %s: %w", url, err)
	}

	// Wait for page to load
	if err := page.WaitLoad(); err != nil {
		return "", fmt.Errorf("failed to wait for page load: %w", err)
	}

	log.Info().Msg("Page loaded, waiting for Cloudflare challenge...")

	// Wait longer for Cloudflare challenge to complete (5 seconds)
	time.Sleep(5 * time.Second)

	// Wait for specific selector if provided (max 20 seconds for content after Cloudflare)
	if waitSelector != "" {
		waitCtx, cancel := context.WithTimeout(ctx, 20*time.Second)
		defer cancel()

		// Try to find the element with timeout
		page = page.Context(waitCtx)
		elem, err := page.Element(waitSelector)
		if err != nil {
			log.Warn().Str("selector", waitSelector).Err(err).Msg("Selector not found, continuing anyway")
			// Selector not found within timeout, but continue anyway
			// The page might still have useful content
		} else {
			log.Info().Str("selector", waitSelector).Msg("Found target element")
			_ = elem // Use elem to avoid unused variable warning
		}
	}

	// Additional wait for dynamic content
	time.Sleep(3 * time.Second)

	// Get rendered HTML
	html, err := page.HTML()
	if err != nil {
		return "", fmt.Errorf("failed to get HTML: %w", err)
	}

	log.Info().Int("htmlLength", len(html)).Msg("Browser got HTML")

	return html, nil
}

// FetchRenderedHTMLWithWait fetches a page with custom wait time
func (b *Browser) FetchRenderedHTMLWithWait(ctx context.Context, url string, waitSelector string, waitTime time.Duration) (string, error) {
	b.mu.Lock()
	defer b.mu.Unlock()

	if b.closed {
		return "", fmt.Errorf("browser is closed")
	}

	// Create a new page
	page, err := b.browser.Page(proto.TargetCreateTarget{URL: "about:blank"})
	if err != nil {
		return "", fmt.Errorf("failed to create page: %w", err)
	}
	defer page.Close()

	// Set page timeout
	page = page.Timeout(DefaultPageLoadTimeout)

	// Navigate to URL
	if err := page.Navigate(url); err != nil {
		return "", fmt.Errorf("failed to navigate to %s: %w", url, err)
	}

	// Wait for page to load
	if err := page.WaitLoad(); err != nil {
		return "", fmt.Errorf("failed to wait for page load: %w", err)
	}

	// Wait for specific selector if provided
	if waitSelector != "" {
		waitCtx, cancel := context.WithTimeout(ctx, waitTime)
		defer cancel()

		page = page.Context(waitCtx)
		_, err := page.Element(waitSelector)
		if err != nil {
			// Selector not found, continue anyway
		}
	}

	// Get rendered HTML
	html, err := page.HTML()
	if err != nil {
		return "", fmt.Errorf("failed to get HTML: %w", err)
	}

	return html, nil
}

// ExecuteScript executes JavaScript on a page and returns the result
func (b *Browser) ExecuteScript(ctx context.Context, url string, script string) (interface{}, error) {
	b.mu.Lock()
	defer b.mu.Unlock()

	if b.closed {
		return nil, fmt.Errorf("browser is closed")
	}

	// Create a new page
	page, err := b.browser.Page(proto.TargetCreateTarget{URL: "about:blank"})
	if err != nil {
		return nil, fmt.Errorf("failed to create page: %w", err)
	}
	defer page.Close()

	// Set page timeout
	page = page.Timeout(DefaultPageLoadTimeout)

	// Navigate to URL
	if err := page.Navigate(url); err != nil {
		return nil, fmt.Errorf("failed to navigate to %s: %w", url, err)
	}

	// Wait for page to load
	if err := page.WaitLoad(); err != nil {
		return nil, fmt.Errorf("failed to wait for page load: %w", err)
	}

	// Execute script
	result, err := page.Eval(script)
	if err != nil {
		return nil, fmt.Errorf("failed to execute script: %w", err)
	}

	return result.Value.Val(), nil
}

// Close closes the browser and releases all resources gracefully
func (b *Browser) Close() error {
	b.mu.Lock()
	defer b.mu.Unlock()

	if b.closed {
		return nil
	}

	b.closed = true

	var errs []error

	// Close browser
	if b.browser != nil {
		if err := b.browser.Close(); err != nil {
			errs = append(errs, fmt.Errorf("failed to close browser: %w", err))
		}
	}

	// Cleanup launcher
	if b.launcher != nil {
		b.launcher.Cleanup()
	}

	if len(errs) > 0 {
		return errs[0]
	}

	return nil
}

// IsClosed returns whether the browser has been closed
func (b *Browser) IsClosed() bool {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.closed
}

// Reconnect attempts to reconnect to the browser if connection was lost
func (b *Browser) Reconnect() error {
	b.mu.Lock()
	defer b.mu.Unlock()

	if b.closed {
		return fmt.Errorf("browser is closed, cannot reconnect")
	}

	// Try to reconnect
	if err := b.browser.Connect(); err != nil {
		return fmt.Errorf("failed to reconnect: %w", err)
	}

	return nil
}
