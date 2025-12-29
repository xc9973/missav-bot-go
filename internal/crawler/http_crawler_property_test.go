package crawler

import (
	"testing"
	"time"

	"github.com/leanovate/gopter"
	"github.com/leanovate/gopter/gen"
	"github.com/leanovate/gopter/prop"
)

// Property 1: Rate Limiting Enforcement
// *For any* sequence of crawl requests, the time between consecutive requests
// SHALL be at least 1/rate_limit seconds (e.g., 2 seconds for 0.5 req/sec).
// **Validates: Requirements 1.2**
func TestProperty_RateLimitingEnforcement(t *testing.T) {
	parameters := gopter.DefaultTestParameters()
	parameters.MinSuccessfulTests = 100
	properties := gopter.NewProperties(parameters)

	// Generator for high rate limits (10-100 req/sec) to make tests fast
	rateLimitGen := gen.Float64Range(10.0, 100.0)

	// Property: Time between consecutive limiter waits respects rate limit
	properties.Property("rate limiter enforces minimum interval", prop.ForAll(
		func(rateLimit float64) bool {
			cfg := &CrawlerConfig{
				RateLimit:  rateLimit,
				Timeout:    30,
				MaxRetries: 3,
				UserAgent:  "test",
			}

			crawler, err := NewHTTPCrawler(cfg)
			if err != nil {
				return false
			}
			defer crawler.Close()

			limiter := crawler.GetLimiter()

			// Expected minimum interval between requests
			expectedMinInterval := time.Duration(float64(time.Second) / rateLimit)

			// Perform multiple consecutive waits and measure intervals
			const numRequests = 3
			timestamps := make([]time.Time, numRequests)

			for i := 0; i < numRequests; i++ {
				// Reserve a token (non-blocking check of when we can proceed)
				reservation := limiter.Reserve()
				if !reservation.OK() {
					return false
				}
				// Wait for the reservation
				time.Sleep(reservation.Delay())
				timestamps[i] = time.Now()
			}

			// Check intervals between consecutive requests (skip first which may be immediate)
			for i := 2; i < numRequests; i++ {
				interval := timestamps[i].Sub(timestamps[i-1])
				// Allow 20% tolerance for timing variations
				minAllowed := time.Duration(float64(expectedMinInterval) * 0.8)
				if interval < minAllowed {
					return false
				}
			}

			return true
		},
		rateLimitGen,
	))

	// Property: Rate limiter configuration matches crawler config
	properties.Property("rate limiter uses configured rate", prop.ForAll(
		func(rateLimit float64) bool {
			cfg := &CrawlerConfig{
				RateLimit:  rateLimit,
				Timeout:    30,
				MaxRetries: 3,
				UserAgent:  "test",
			}

			crawler, err := NewHTTPCrawler(cfg)
			if err != nil {
				return false
			}
			defer crawler.Close()

			// Verify the configured rate matches
			configuredRate := crawler.GetRateLimit()
			return configuredRate == rateLimit
		},
		rateLimitGen,
	))

	// Property: Token bucket allows burst of 1 (single token)
	properties.Property("rate limiter has burst size of 1", prop.ForAll(
		func(rateLimit float64) bool {
			cfg := &CrawlerConfig{
				RateLimit:  rateLimit,
				Timeout:    30,
				MaxRetries: 3,
				UserAgent:  "test",
			}

			crawler, err := NewHTTPCrawler(cfg)
			if err != nil {
				return false
			}
			defer crawler.Close()

			limiter := crawler.GetLimiter()

			// Burst should be 1 - only one immediate request allowed
			return limiter.Burst() == 1
		},
		rateLimitGen,
	))

	properties.TestingRun(t)
}
