package push

import (
	"context"
	"sync"
	"testing"
	"time"

	"github.com/leanovate/gopter"
	"github.com/leanovate/gopter/gen"
	"github.com/leanovate/gopter/prop"
	"github.com/user/missav-bot-go/internal/model"
	"github.com/user/missav-bot-go/internal/store"
)

// MockStore implements a minimal in-memory store for testing push deduplication
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
	m.mu.Lock()
	defer m.mu.Unlock()
	for _, v := range m.videos {
		if v.Code == code {
			return v, nil
		}
	}
	return nil, nil
}

func (m *MockStore) GetUnpushedVideos(ctx context.Context) ([]*model.Video, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	var result []*model.Video
	for _, v := range m.videos {
		if !v.Pushed {
			result = append(result, v)
		}
	}
	return result, nil
}

func (m *MockStore) MarkAsPushed(ctx context.Context, videoID uint) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	if v, ok := m.videos[videoID]; ok {
		v.Pushed = true
	}
	return nil
}

func (m *MockStore) SearchVideos(ctx context.Context, keyword string, limit int) ([]*model.Video, error) {
	return nil, nil
}

func (m *MockStore) GetLatestVideos(ctx context.Context, limit, offset int) ([]*model.Video, error) {
	return nil, nil
}

func (m *MockStore) CountVideos(ctx context.Context) (int64, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	return int64(len(m.videos)), nil
}

func (m *MockStore) ExistsByCode(ctx context.Context, code string) (bool, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	for _, v := range m.videos {
		if v.Code == code {
			return true, nil
		}
	}
	return false, nil
}

func (m *MockStore) CreateSubscription(ctx context.Context, sub *model.Subscription) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.subscriptions = append(m.subscriptions, sub)
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
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.subscriptions, nil
}

func (m *MockStore) GetMatchingSubscriptions(ctx context.Context, video *model.Video) ([]*model.Subscription, error) {
	return nil, nil
}

func (m *MockStore) RecordPush(ctx context.Context, record *model.PushRecord) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.pushRecords = append(m.pushRecords, record)
	return nil
}

func (m *MockStore) HasPushed(ctx context.Context, videoID uint, chatID int64) (bool, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	for _, r := range m.pushRecords {
		if r.VideoID == videoID && r.ChatID == chatID && r.Status == model.PushStatusSuccess {
			return true, nil
		}
	}
	return false, nil
}

func (m *MockStore) Ping(ctx context.Context) error {
	return nil
}

func (m *MockStore) Close() error {
	return nil
}

// CountSuccessPushes counts successful push records for a video-chat pair
func (m *MockStore) CountSuccessPushes(videoID uint, chatID int64) int {
	m.mu.Lock()
	defer m.mu.Unlock()
	count := 0
	for _, r := range m.pushRecords {
		if r.VideoID == videoID && r.ChatID == chatID && r.Status == model.PushStatusSuccess {
			count++
		}
	}
	return count
}

// MockTelegramClient implements TelegramClient for testing
type MockTelegramClient struct {
	mu       sync.Mutex
	messages []string
}

func NewMockTelegramClient() *MockTelegramClient {
	return &MockTelegramClient{
		messages: make([]string, 0),
	}
}

func (m *MockTelegramClient) SendMessage(chatID int64, text string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.messages = append(m.messages, text)
	return nil
}

func (m *MockTelegramClient) SendMarkdown(chatID int64, text string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.messages = append(m.messages, text)
	return nil
}

func (m *MockTelegramClient) SendPhoto(chatID int64, photoURL string, caption string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.messages = append(m.messages, caption)
	return nil
}

func (m *MockTelegramClient) SendVideo(chatID int64, videoURL string, thumbURL string, caption string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.messages = append(m.messages, caption)
	return nil
}

// Property 10: Push Deduplication
// *For any* (video_id, chat_id) pair, pushing multiple times SHALL result in at most one SUCCESS push record.
// **Validates: Requirements 5.3**
func TestProperty_PushDeduplication(t *testing.T) {
	parameters := gopter.DefaultTestParameters()
	parameters.MinSuccessfulTests = 100
	properties := gopter.NewProperties(parameters)

	// Generator for video ID
	videoIDGen := gen.UIntRange(1, 10000)

	// Generator for chat ID
	chatIDGen := gen.Int64Range(1, 1000000)

	// Generator for push count (how many times to attempt push)
	pushCountGen := gen.IntRange(2, 5)

	// Property: Multiple pushes to same video-chat pair result in at most one SUCCESS record
	properties.Property("at most one SUCCESS push per video-chat pair", prop.ForAll(
		func(videoID uint, chatID int64, pushCount int) bool {
			// Create fresh mock store and client for each test
			mockStore := NewMockStore()
			mockTelegram := NewMockTelegramClient()
			service := NewService(mockStore, mockTelegram)

			// Create a test video
			video := &model.Video{
				ID:        videoID,
				Code:      "TEST-001",
				Title:     "Test Video",
				DetailURL: "https://example.com/test",
			}
			mockStore.SaveVideo(context.Background(), video)

			// Attempt to push multiple times
			ctx := context.Background()
			for i := 0; i < pushCount; i++ {
				_ = service.PushVideoToChat(ctx, video, chatID)
			}

			// Count SUCCESS push records
			successCount := mockStore.CountSuccessPushes(videoID, chatID)

			// Should have at most 1 SUCCESS record
			return successCount <= 1
		},
		videoIDGen,
		chatIDGen,
		pushCountGen,
	))

	// Property: First push creates exactly one SUCCESS record
	properties.Property("first push creates exactly one SUCCESS record", prop.ForAll(
		func(videoID uint, chatID int64) bool {
			mockStore := NewMockStore()
			mockTelegram := NewMockTelegramClient()
			service := NewService(mockStore, mockTelegram)

			video := &model.Video{
				ID:        videoID,
				Code:      "TEST-002",
				Title:     "Test Video",
				DetailURL: "https://example.com/test",
			}
			mockStore.SaveVideo(context.Background(), video)

			// Push once
			ctx := context.Background()
			_ = service.PushVideoToChat(ctx, video, chatID)

			// Should have exactly 1 SUCCESS record
			successCount := mockStore.CountSuccessPushes(videoID, chatID)
			return successCount == 1
		},
		videoIDGen,
		chatIDGen,
	))

	// Property: Second push to same pair is skipped (no new record)
	properties.Property("second push is skipped", prop.ForAll(
		func(videoID uint, chatID int64) bool {
			mockStore := NewMockStore()
			mockTelegram := NewMockTelegramClient()
			service := NewService(mockStore, mockTelegram)

			video := &model.Video{
				ID:        videoID,
				Code:      "TEST-003",
				Title:     "Test Video",
				DetailURL: "https://example.com/test",
			}
			mockStore.SaveVideo(context.Background(), video)

			ctx := context.Background()
			
			// First push
			_ = service.PushVideoToChat(ctx, video, chatID)
			recordsAfterFirst := len(mockStore.pushRecords)

			// Second push
			_ = service.PushVideoToChat(ctx, video, chatID)
			recordsAfterSecond := len(mockStore.pushRecords)

			// No new records should be created
			return recordsAfterSecond == recordsAfterFirst
		},
		videoIDGen,
		chatIDGen,
	))

	// Property: Different chat IDs can each have one SUCCESS record
	properties.Property("different chats can each have one SUCCESS", prop.ForAll(
		func(videoID uint, chatID1 int64, chatID2 int64) bool {
			if chatID1 == chatID2 {
				return true // Skip if same chat ID
			}

			mockStore := NewMockStore()
			mockTelegram := NewMockTelegramClient()
			service := NewService(mockStore, mockTelegram)

			video := &model.Video{
				ID:        videoID,
				Code:      "TEST-004",
				Title:     "Test Video",
				DetailURL: "https://example.com/test",
			}
			mockStore.SaveVideo(context.Background(), video)

			ctx := context.Background()
			
			// Push to both chats
			_ = service.PushVideoToChat(ctx, video, chatID1)
			_ = service.PushVideoToChat(ctx, video, chatID2)

			// Each chat should have exactly 1 SUCCESS record
			success1 := mockStore.CountSuccessPushes(videoID, chatID1)
			success2 := mockStore.CountSuccessPushes(videoID, chatID2)

			return success1 == 1 && success2 == 1
		},
		videoIDGen,
		chatIDGen,
		chatIDGen,
	))

	// Property: HasPushed returns true after successful push
	properties.Property("HasPushed returns true after push", prop.ForAll(
		func(videoID uint, chatID int64) bool {
			mockStore := NewMockStore()
			mockTelegram := NewMockTelegramClient()
			service := NewService(mockStore, mockTelegram)

			video := &model.Video{
				ID:        videoID,
				Code:      "TEST-005",
				Title:     "Test Video",
				DetailURL: "https://example.com/test",
			}
			mockStore.SaveVideo(context.Background(), video)

			ctx := context.Background()
			
			// Before push
			hasPushedBefore, _ := mockStore.HasPushed(ctx, videoID, chatID)
			
			// Push
			_ = service.PushVideoToChat(ctx, video, chatID)
			
			// After push
			hasPushedAfter, _ := mockStore.HasPushed(ctx, videoID, chatID)

			return !hasPushedBefore && hasPushedAfter
		},
		videoIDGen,
		chatIDGen,
	))

	properties.TestingRun(t)
}

// Ensure MockStore implements the store.Store interface
var _ store.Store = (*MockStore)(nil)

// Suppress unused variable warning
var _ = time.Now
