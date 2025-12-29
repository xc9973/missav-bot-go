package server

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"time"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promhttp"
	"github.com/rs/zerolog/log"
	"github.com/user/missav-bot-go/internal/store"
)

// Metrics for Prometheus (Requirement 8.4)
var (
	videosTotal = prometheus.NewGauge(prometheus.GaugeOpts{
		Name: "missav_bot_videos_total",
		Help: "Total number of videos in database",
	})

	pushesTotal = prometheus.NewCounterVec(prometheus.CounterOpts{
		Name: "missav_bot_pushes_total",
		Help: "Total number of push operations",
	}, []string{"status"})

	crawlDurationSeconds = prometheus.NewHistogram(prometheus.HistogramOpts{
		Name:    "missav_bot_crawl_duration_seconds",
		Help:    "Duration of crawl operations in seconds",
		Buckets: prometheus.DefBuckets,
	})

	errorsTotal = prometheus.NewCounterVec(prometheus.CounterOpts{
		Name: "missav_bot_errors_total",
		Help: "Total number of errors",
	}, []string{"type"})
)

func init() {
	prometheus.MustRegister(videosTotal)
	prometheus.MustRegister(pushesTotal)
	prometheus.MustRegister(crawlDurationSeconds)
	prometheus.MustRegister(errorsTotal)
}

// HealthResponse represents the health check response (Requirement 8.2)
type HealthResponse struct {
	Status   string `json:"status"`
	Database string `json:"database"`
	Uptime   string `json:"uptime"`
}

// Server handles HTTP requests for health checks and metrics
type Server struct {
	store     store.Store
	router    *http.ServeMux
	server    *http.Server
	startTime time.Time
}

// NewServer creates a new HTTP server instance
func NewServer(store store.Store) *Server {
	s := &Server{
		store:     store,
		router:    http.NewServeMux(),
		startTime: time.Now(),
	}

	s.setupRoutes()
	return s
}


// setupRoutes configures the HTTP routes
func (s *Server) setupRoutes() {
	// Health check endpoint (Requirement 8.1)
	s.router.HandleFunc("/health", s.handleHealth)

	// Metrics endpoint (Requirement 8.3)
	s.router.Handle("/metrics", promhttp.Handler())
}

// Start begins listening on the specified port (Requirement 8.1)
func (s *Server) Start(port int) error {
	addr := fmt.Sprintf(":%d", port)
	s.server = &http.Server{
		Addr:         addr,
		Handler:      s.router,
		ReadTimeout:  10 * time.Second,
		WriteTimeout: 10 * time.Second,
		IdleTimeout:  60 * time.Second,
	}

	log.Info().Int("port", port).Msg("Starting HTTP server")
	return s.server.ListenAndServe()
}

// Stop gracefully shuts down the server
func (s *Server) Stop(ctx context.Context) error {
	if s.server == nil {
		return nil
	}
	log.Info().Msg("Stopping HTTP server")
	return s.server.Shutdown(ctx)
}

// handleHealth handles the /health endpoint (Requirement 8.2)
// Returns JSON with status, database connectivity, and uptime
func (s *Server) handleHealth(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()

	// Check database connectivity
	dbStatus := "healthy"
	if err := s.store.Ping(ctx); err != nil {
		dbStatus = fmt.Sprintf("unhealthy: %v", err)
	}

	// Calculate uptime
	uptime := time.Since(s.startTime).Round(time.Second).String()

	// Determine overall status
	status := "healthy"
	if dbStatus != "healthy" {
		status = "unhealthy"
	}

	response := HealthResponse{
		Status:   status,
		Database: dbStatus,
		Uptime:   uptime,
	}

	w.Header().Set("Content-Type", "application/json")
	if status != "healthy" {
		w.WriteHeader(http.StatusServiceUnavailable)
	}

	if err := json.NewEncoder(w).Encode(response); err != nil {
		log.Error().Err(err).Msg("Failed to encode health response")
		http.Error(w, "Internal server error", http.StatusInternalServerError)
	}
}

// UpdateVideoCount updates the videos_total metric
func UpdateVideoCount(count int64) {
	videosTotal.Set(float64(count))
}

// RecordPush records a push operation metric
func RecordPush(status string) {
	pushesTotal.WithLabelValues(status).Inc()
}

// RecordCrawlDuration records the duration of a crawl operation
func RecordCrawlDuration(duration time.Duration) {
	crawlDurationSeconds.Observe(duration.Seconds())
}

// RecordError records an error metric
func RecordError(errorType string) {
	errorsTotal.WithLabelValues(errorType).Inc()
}

// GetUptime returns the server uptime
func (s *Server) GetUptime() time.Duration {
	return time.Since(s.startTime)
}
