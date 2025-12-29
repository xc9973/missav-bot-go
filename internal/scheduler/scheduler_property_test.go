package scheduler

import (
	"context"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/leanovate/gopter"
	"github.com/leanovate/gopter/gen"
	"github.com/leanovate/gopter/prop"
	"github.com/user/missav-bot-go/internal/config"
	"github.com/user/missav-bot-go/internal/model"
	"github.com/user/missav-bot-go/internal/push"
	"github.com/user/missav-bot-go/internal/store"
)

// MockCrawler implements crawler.Crawler for testing
type MockCrawler struct {
	mu           sync.Mutex
	crawlCount   int32
	concurrent   int32
	maxConcurrent int32
	crawlDelay   time.Duration
}

func NewMockCrawler(crawlDelay time.Duration) *MockCrawler {
	return &MockCrawler{
		crawlDelay: crawlDelay,
	}
}

func (m *MockCrawler) CrawlNewVideos(ctx context.Context, pages int) ([]*model.Video, error) {
	// Track concurrent executions
	current := atomic.AddInt32(&m.concurrent, 1)
	defer atomic.AddInt32(&m.concurrent, -1)

	// Update max concurrent
	m.mu.Lock()
	if current > m.maxConcurrent {
		m.maxConcurrent = current
	}
	m.mu.Unlock()

	atomic.AddInt32(&m.crawlCount, 1)

	// Simulate crawl work
	time.Sleep(m.crawlDelay)

	return []*model.Video{}, nil
}

func (m *MockCrawler) CrawlVideoDetail(ctx context.Context, detailURL string) (*model.Video, error) {
	return nil, nil
}

func (m *MockCrawler) CrawlByActor(ctx context.Context, actorName string, limit int) ([]*model.Video, error) {
	return nil, nil
}

func (m *MockCrawler) CrawlByCode(ctx context.Context, code string) (*model.Video, error) {
	return nil, nil
}

func (m *MockCrawler) CrawlByKeyword(ctx context.Context, keyword string, limit int) ([]*model.Video, error) {
	return nil, nil
}

func (m *MockCrawler) Close() error {
	return nil
}

func (m *MockCrawler) GetCrawlCount() int32 {
	return atomic.LoadInt32(&m.crawlCount)
}

func (m *MockCrawler) GetMaxConcurrent() int32 {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.maxConcurrent
}


// MockStore implements store.Store for testing
type MockStore struct {
	mu            sync.Mutex
	videos        map[uint]*model.Video
	subscriptions []*model.Subscription
	pushRecords   []*model.PushRecord
}

func NewMockStore() *MockStore {
	return &MockStore{
		videos:        make(map[uint]*model.Video),
		subscriptions: make([]*model.Subscription, 0),
		pushRecords:   make([]*model.PushRecord, 0),
	}
}

func (m *MockStore) SaveVideo(ctx context.Context, video *model.Video) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.videos[video.ID] = video
	return nil
}

func (m *MockStore) SaveVideos(ctx context.Context, videos []*model.Video) (saved int, duplicates int, err error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	for _, v := range videos {
		m.videos[v.ID] = v
	}
	return len(videos), 0, nil
}

func (m *MockStore) GetVideoByCode(ctx context.Context, code string) (*model.Video, error) {
	return nil, nil
}

func (m *MockStore) GetUnpushedVideos(ctx context.Context) ([]*model.Video, error) {
	return []*model.Video{}, nil
}

func (m *MockStore) MarkAsPushed(ctx context.Context, videoID uint) error {
	return nil
}

func (m *MockStore) SearchVideos(ctx context.Context, keyword string, limit int) ([]*model.Video, error) {
	return nil, nil
}

func (m *MockStore) GetLatestVideos(ctx context.Context, limit, offset int) ([]*model.Video, error) {
	return nil, nil
}

func (m *MockStore) CountVideos(ctx context.Context) (int64, error) {
	return 0, nil
}

func (m *MockStore) ExistsByCode(ctx context.Context, code string) (bool, error) {
	return false, nil
}

func (m *MockStore) CreateSubscription(ctx context.Context, sub *model.Subscription) error {
	return nil
}

func (m *MockStore) DeleteSubscription(ctx context.Context, chatID int64, subType string, keyword string) error {
	return nil
}

func (m *MockStore) DeleteAllSubscriptions(ctx context.Context, chatID int64) error {
	return nil
}

func (m *MockStore) GetSubscriptions(ctx context.Context, chatID int64) ([]*model.Subscription, error) {
	return nil, nil
}

func (m *MockStore) GetAllSubscriptions(ctx context.Context) ([]*model.Subscription, error) {
	return nil, nil
}

func (m *MockStore) GetMatchingSubscriptions(ctx context.Context, video *model.Video) ([]*model.Subscription, error) {
	return nil, nil
}

func (m *MockStore) RecordPush(ctx context.Context, record *model.PushRecord) error {
	return nil
}

func (m *MockStore) HasPushed(ctx context.Context, videoID uint, chatID int64) (bool, error) {
	return false, nil
}

func (m *MockStore) Ping(ctx context.Context) error {
	return nil
}

func (m *MockStore) Close() error {
	return nil
}

// MockTelegramClient implements push.TelegramClient for testing
type MockTelegramClient struct{}

func (m *MockTelegramClient) SendMessage(chatID int64, text string) error {
	return nil
}

func (m *MockTelegramClient) SendMarkdown(chatID int64, text string) error {
	return nil
}

func (m *MockTelegramClient) SendPhoto(chatID int64, photoURL string, caption string) error {
	return nil
}

func (m *MockTelegramClient) SendVideo(chatID int64, videoURL string, thumbURL string, caption string) error {
	return nil
}

// Ensure MockStore implements the store.Store interface
var _ store.Store = (*MockStore)(nil)


// Property 12: Scheduler Mutual Exclusion
// *For any* concurrent scheduler triggers, at most one crawl task SHALL be running at any time.
// **Validates: Requirements 6.3**
func TestProperty_SchedulerMutualExclusion(t *testing.T) {
	parameters := gopter.DefaultTestParameters()
	parameters.MinSuccessfulTests = 100
	properties := gopter.NewProperties(parameters)

	// Generator for number of concurrent triggers
	concurrentTriggersGen := gen.IntRange(2, 10)

	// Generator for crawl delay in milliseconds
	crawlDelayGen := gen.IntRange(10, 50)

	// Property: At most one crawl task runs at any time
	properties.Property("at most one crawl task runs concurrently", prop.ForAll(
		func(numTriggers int, crawlDelayMs int) bool {
			crawlDelay := time.Duration(crawlDelayMs) * time.Millisecond
			mockCrawler := NewMockCrawler(crawlDelay)
			mockStore := NewMockStore()
			mockTelegram := &MockTelegramClient{}
			pushService := push.NewService(mockStore, mockTelegram)

			cfg := &config.CrawlerConfig{
				Enabled:      true,
				Interval:     time.Hour, // Long interval to prevent auto-triggers
				InitialPages: 1,
			}

			scheduler := NewScheduler(mockCrawler, mockStore, pushService, cfg)

			// Launch multiple concurrent triggers
			var wg sync.WaitGroup
			ctx := context.Background()

			for i := 0; i < numTriggers; i++ {
				wg.Add(1)
				go func() {
					defer wg.Done()
					scheduler.TryRun(ctx, 1)
				}()
			}

			wg.Wait()

			// Check that max concurrent was never more than 1
			maxConcurrent := mockCrawler.GetMaxConcurrent()
			return maxConcurrent <= 1
		},
		concurrentTriggersGen,
		crawlDelayGen,
	))

	// Property: TryRun returns false when task is already running
	properties.Property("TryRun returns false when already running", prop.ForAll(
		func(crawlDelayMs int) bool {
			crawlDelay := time.Duration(crawlDelayMs) * time.Millisecond
			mockCrawler := NewMockCrawler(crawlDelay)
			mockStore := NewMockStore()
			mockTelegram := &MockTelegramClient{}
			pushService := push.NewService(mockStore, mockTelegram)

			cfg := &config.CrawlerConfig{
				Enabled:      true,
				Interval:     time.Hour,
				InitialPages: 1,
			}

			scheduler := NewScheduler(mockCrawler, mockStore, pushService, cfg)

			ctx := context.Background()
			
			// Start first task in background
			started := make(chan bool)
			done := make(chan bool)
			go func() {
				started <- true
				scheduler.TryRun(ctx, 1)
				done <- true
			}()

			<-started
			// Give the first task time to acquire the lock
			time.Sleep(5 * time.Millisecond)

			// Try to run second task while first is running
			secondResult := scheduler.TryRun(ctx, 1)

			<-done

			// Second attempt should return false (skipped)
			return !secondResult
		},
		gen.IntRange(50, 100), // Longer delay to ensure overlap
	))

	// Property: IsRunning reflects actual running state
	properties.Property("IsRunning reflects actual state", prop.ForAll(
		func(crawlDelayMs int) bool {
			crawlDelay := time.Duration(crawlDelayMs) * time.Millisecond
			mockCrawler := NewMockCrawler(crawlDelay)
			mockStore := NewMockStore()
			mockTelegram := &MockTelegramClient{}
			pushService := push.NewService(mockStore, mockTelegram)

			cfg := &config.CrawlerConfig{
				Enabled:      true,
				Interval:     time.Hour,
				InitialPages: 1,
			}

			scheduler := NewScheduler(mockCrawler, mockStore, pushService, cfg)

			// Initially not running
			if scheduler.IsRunning() {
				return false
			}

			ctx := context.Background()
			
			// Start task
			started := make(chan bool)
			done := make(chan bool)
			go func() {
				started <- true
				scheduler.TryRun(ctx, 1)
				done <- true
			}()

			<-started
			time.Sleep(5 * time.Millisecond)

			// Should be running now
			runningDuring := scheduler.IsRunning()

			<-done
			time.Sleep(5 * time.Millisecond)

			// Should not be running after completion
			runningAfter := scheduler.IsRunning()

			return runningDuring && !runningAfter
		},
		gen.IntRange(50, 100),
	))

	// Property: All triggers eventually complete (no deadlock)
	properties.Property("all triggers complete without deadlock", prop.ForAll(
		func(numTriggers int, crawlDelayMs int) bool {
			crawlDelay := time.Duration(crawlDelayMs) * time.Millisecond
			mockCrawler := NewMockCrawler(crawlDelay)
			mockStore := NewMockStore()
			mockTelegram := &MockTelegramClient{}
			pushService := push.NewService(mockStore, mockTelegram)

			cfg := &config.CrawlerConfig{
				Enabled:      true,
				Interval:     time.Hour,
				InitialPages: 1,
			}

			scheduler := NewScheduler(mockCrawler, mockStore, pushService, cfg)

			var wg sync.WaitGroup
			ctx := context.Background()
			completed := make(chan bool, numTriggers)

			for i := 0; i < numTriggers; i++ {
				wg.Add(1)
				go func() {
					defer wg.Done()
					scheduler.TryRun(ctx, 1)
					completed <- true
				}()
			}

			// Wait with timeout
			done := make(chan bool)
			go func() {
				wg.Wait()
				done <- true
			}()

			select {
			case <-done:
				return true
			case <-time.After(5 * time.Second):
				return false // Deadlock detected
			}
		},
		concurrentTriggersGen,
		crawlDelayGen,
	))

	// Property: Exactly one crawl executes per successful TryRun
	properties.Property("exactly one crawl per successful TryRun", prop.ForAll(
		func(numTriggers int, crawlDelayMs int) bool {
			crawlDelay := time.Duration(crawlDelayMs) * time.Millisecond
			mockCrawler := NewMockCrawler(crawlDelay)
			mockStore := NewMockStore()
			mockTelegram := &MockTelegramClient{}
			pushService := push.NewService(mockStore, mockTelegram)

			cfg := &config.CrawlerConfig{
				Enabled:      true,
				Interval:     time.Hour,
				InitialPages: 1,
			}

			scheduler := NewScheduler(mockCrawler, mockStore, pushService, cfg)

			var wg sync.WaitGroup
			ctx := context.Background()
			successCount := int32(0)

			for i := 0; i < numTriggers; i++ {
				wg.Add(1)
				go func() {
					defer wg.Done()
					if scheduler.TryRun(ctx, 1) {
						atomic.AddInt32(&successCount, 1)
					}
				}()
			}

			wg.Wait()

			// Number of successful TryRun calls should equal crawl count
			crawlCount := mockCrawler.GetCrawlCount()
			return crawlCount == successCount
		},
		concurrentTriggersGen,
		crawlDelayGen,
	))

	properties.TestingRun(t)
}
