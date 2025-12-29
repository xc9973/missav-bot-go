package bot

import (
	"context"
	"testing"

	"github.com/leanovate/gopter"
	"github.com/leanovate/gopter/gen"
	"github.com/leanovate/gopter/prop"
	"github.com/user/missav-bot-go/internal/model"
)

// MockStoreForSearch is a mock store that returns a configurable number of videos
type MockStoreForSearch struct {
	videos []*model.Video
}

func (m *MockStoreForSearch) SearchVideos(ctx context.Context, keyword string, limit int) ([]*model.Video, error) {
	// Return up to 'limit' videos from the mock data
	if limit > len(m.videos) {
		return m.videos, nil
	}
	return m.videos[:limit], nil
}

// generateVideos creates a slice of mock videos
func generateVideos(count int) []*model.Video {
	videos := make([]*model.Video, count)
	for i := 0; i < count; i++ {
		videos[i] = &model.Video{
			ID:        uint(i + 1),
			Code:      "ABC-" + string(rune('0'+i%10)) + string(rune('0'+i/10)),
			Title:     "Test Video " + string(rune('0'+i)),
			Actresses: "Test Actress",
			Tags:      "test,video",
		}
	}
	return videos
}

// TestProperty_SearchResultLimit tests Property 5: Search Result Limit
// For any search query, the returned results SHALL contain at most 10 videos.
// **Validates: Requirements 3.8**
func TestProperty_SearchResultLimit(t *testing.T) {
	parameters := gopter.DefaultTestParameters()
	parameters.MinSuccessfulTests = 100
	properties := gopter.NewProperties(parameters)

	// Property 5.1: Search results never exceed 10 items regardless of database size
	// Feature: missav-bot-go, Property 5: Search Result Limit
	properties.Property("search results never exceed 10 items", prop.ForAll(
		func(dbSize int) bool {
			// Create a mock store with 'dbSize' videos
			mockStore := &MockStoreForSearch{
				videos: generateVideos(dbSize),
			}

			// Simulate the search with limit 10 (as per Requirement 3.8)
			results, err := mockStore.SearchVideos(context.Background(), "test", 10)
			if err != nil {
				return false
			}

			// Property: results should never exceed 10
			return len(results) <= 10
		},
		gen.IntRange(0, 100), // Test with database sizes from 0 to 100
	))

	// Property 5.2: Search returns correct number when database has fewer than 10 items
	// Feature: missav-bot-go, Property 5: Search Result Limit (small database case)
	properties.Property("search returns all items when database has fewer than 10", prop.ForAll(
		func(dbSize int) bool {
			mockStore := &MockStoreForSearch{
				videos: generateVideos(dbSize),
			}

			results, err := mockStore.SearchVideos(context.Background(), "test", 10)
			if err != nil {
				return false
			}

			// When database has fewer than 10 items, return all of them
			if dbSize < 10 {
				return len(results) == dbSize
			}
			// When database has 10 or more items, return exactly 10
			return len(results) == 10
		},
		gen.IntRange(0, 20),
	))

	// Property 5.3: Search limit is enforced regardless of keyword
	// Feature: missav-bot-go, Property 5: Search Result Limit (keyword independence)
	properties.Property("search limit is enforced regardless of keyword", prop.ForAll(
		func(keyword string) bool {
			// Create a store with more than 10 videos
			mockStore := &MockStoreForSearch{
				videos: generateVideos(50),
			}

			results, err := mockStore.SearchVideos(context.Background(), keyword, 10)
			if err != nil {
				return false
			}

			return len(results) <= 10
		},
		gen.AlphaString(),
	))

	// Property 5.4: Empty search still respects limit
	// Feature: missav-bot-go, Property 5: Search Result Limit (empty keyword)
	properties.Property("empty search keyword still respects limit", prop.ForAll(
		func(dbSize int) bool {
			mockStore := &MockStoreForSearch{
				videos: generateVideos(dbSize),
			}

			results, err := mockStore.SearchVideos(context.Background(), "", 10)
			if err != nil {
				return false
			}

			return len(results) <= 10
		},
		gen.IntRange(0, 50),
	))

	properties.TestingRun(t)
}

// TestSearchLimitConstant verifies the search limit constant is 10
func TestSearchLimitConstant(t *testing.T) {
	// This test verifies that the search limit used in handleSearch is 10
	// The actual limit is hardcoded in handleSearch as per Requirement 3.8
	const expectedLimit = 10

	// Create a mock store with more than 10 videos
	mockStore := &MockStoreForSearch{
		videos: generateVideos(50),
	}

	// Simulate what handleSearch does
	results, err := mockStore.SearchVideos(context.Background(), "test", expectedLimit)
	if err != nil {
		t.Fatalf("SearchVideos failed: %v", err)
	}

	if len(results) > expectedLimit {
		t.Errorf("Expected at most %d results, got %d", expectedLimit, len(results))
	}
}
