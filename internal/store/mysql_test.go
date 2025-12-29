package store

import (
	"context"
	"fmt"
	"os"
	"testing"
	"time"

	"github.com/leanovate/gopter"
	"github.com/leanovate/gopter/gen"
	"github.com/leanovate/gopter/prop"
	"github.com/user/missav-bot-go/internal/config"
	"github.com/user/missav-bot-go/internal/model"
	"gorm.io/driver/mysql"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

// testStore is a helper to create a test store with a real MySQL database
func setupTestStore(t *testing.T) (*MySQLStore, func()) {
	// Use environment variables or defaults for test database
	host := os.Getenv("TEST_DB_HOST")
	if host == "" {
		host = "localhost"
	}
	port := 3306
	user := os.Getenv("TEST_DB_USER")
	if user == "" {
		user = "root"
	}
	password := os.Getenv("TEST_DB_PASSWORD")
	if password == "" {
		password = "root"
	}
	database := os.Getenv("TEST_DB_NAME")
	if database == "" {
		database = "missav_bot_test"
	}

	cfg := &config.DBConfig{
		Host:     host,
		Port:     port,
		User:     user,
		Password: password,
		Database: database,
		MaxConns: 5,
	}

	// First connect without database to create it if needed
	dsn := fmt.Sprintf("%s:%s@tcp(%s:%d)/?charset=utf8mb4&parseTime=True&loc=Local",
		cfg.User, cfg.Password, cfg.Host, cfg.Port)
	
	db, err := gorm.Open(mysql.Open(dsn), &gorm.Config{
		Logger: logger.Default.LogMode(logger.Silent),
	})
	if err != nil {
		t.Skipf("Skipping test: cannot connect to MySQL: %v", err)
	}
	
	// Create test database
	db.Exec(fmt.Sprintf("CREATE DATABASE IF NOT EXISTS %s", database))
	
	sqlDB, _ := db.DB()
	sqlDB.Close()

	// Now connect to the test database
	store, err := NewMySQLStore(cfg)
	if err != nil {
		t.Skipf("Skipping test: cannot create store: %v", err)
	}

	// Cleanup function
	cleanup := func() {
		// Clean up tables
		store.db.Exec("DELETE FROM push_records")
		store.db.Exec("DELETE FROM subscriptions")
		store.db.Exec("DELETE FROM videos")
		store.Close()
	}

	return store, cleanup
}


// genVideoCode generates valid video codes in format ABC-123
func genVideoCode() gopter.Gen {
	return gen.RegexMatch(`[A-Z]{2,5}-[0-9]{3,5}`)
}

// genVideo generates a random video with a given code
func genVideo(code string) *model.Video {
	return &model.Video{
		Code:      code,
		Title:     "Test Video " + code,
		Actresses: "Test Actress",
		Tags:      "test,video",
		Duration:  120,
		CoverURL:  "https://example.com/cover.jpg",
		DetailURL: "https://example.com/detail/" + code,
	}
}

// Feature: missav-bot-go, Property 6: Video Deduplication
// For any video with a given code, saving it multiple times SHALL result in exactly one record in the database.
// Validates: Requirements 4.2
func TestProperty_VideoDeduplication(t *testing.T) {
	store, cleanup := setupTestStore(t)
	defer cleanup()

	parameters := gopter.DefaultTestParameters()
	parameters.MinSuccessfulTests = 100
	properties := gopter.NewProperties(parameters)

	properties.Property("saving video multiple times results in exactly one record", prop.ForAll(
		func(code string, saveCount int) bool {
			ctx := context.Background()
			
			// Clean up any existing video with this code
			store.db.Where("code = ?", code).Delete(&model.Video{})
			
			// Save the video multiple times
			for i := 0; i < saveCount; i++ {
				video := genVideo(code)
				_ = store.SaveVideo(ctx, video)
			}
			
			// Count videos with this code
			var count int64
			store.db.Model(&model.Video{}).Where("code = ?", code).Count(&count)
			
			// Clean up
			store.db.Where("code = ?", code).Delete(&model.Video{})
			
			return count == 1
		},
		genVideoCode(),
		gen.IntRange(2, 5), // Save 2-5 times
	))

	properties.TestingRun(t)
}


// Feature: missav-bot-go, Property 7: New Video Pushed Flag
// For any newly saved video, the `pushed` flag SHALL be `false`.
// Validates: Requirements 4.3
func TestProperty_NewVideoPushedFlag(t *testing.T) {
	store, cleanup := setupTestStore(t)
	defer cleanup()

	parameters := gopter.DefaultTestParameters()
	parameters.MinSuccessfulTests = 100
	properties := gopter.NewProperties(parameters)

	properties.Property("newly saved video has pushed=false", prop.ForAll(
		func(code string) bool {
			ctx := context.Background()
			
			// Clean up any existing video with this code
			store.db.Where("code = ?", code).Delete(&model.Video{})
			
			// Create video with pushed=true (to verify it gets set to false)
			video := genVideo(code)
			video.Pushed = true // Intentionally set to true
			
			err := store.SaveVideo(ctx, video)
			if err != nil {
				return false
			}
			
			// Retrieve the video and check pushed flag
			savedVideo, err := store.GetVideoByCode(ctx, code)
			if err != nil || savedVideo == nil {
				return false
			}
			
			result := savedVideo.Pushed == false
			
			// Clean up
			store.db.Where("code = ?", code).Delete(&model.Video{})
			
			return result
		},
		genVideoCode(),
	))

	properties.TestingRun(t)
}


// Feature: missav-bot-go, Property 8: Unpushed Videos Ordering
// For any query of unpushed videos, the results SHALL be ordered by `created_at` in descending order.
// Validates: Requirements 4.4
func TestProperty_UnpushedVideosOrdering(t *testing.T) {
	store, cleanup := setupTestStore(t)
	defer cleanup()

	parameters := gopter.DefaultTestParameters()
	parameters.MinSuccessfulTests = 100
	properties := gopter.NewProperties(parameters)

	properties.Property("unpushed videos are ordered by created_at DESC", prop.ForAll(
		func(codes []string) bool {
			ctx := context.Background()
			
			if len(codes) < 2 {
				return true // Need at least 2 videos to test ordering
			}
			
			// Clean up any existing videos with these codes
			for _, code := range codes {
				store.db.Where("code = ?", code).Delete(&model.Video{})
			}
			
			// Save videos with small delays to ensure different created_at times
			for _, code := range codes {
				video := genVideo(code)
				_ = store.SaveVideo(ctx, video)
				time.Sleep(10 * time.Millisecond) // Small delay to ensure different timestamps
			}
			
			// Get unpushed videos
			unpushed, err := store.GetUnpushedVideos(ctx)
			if err != nil {
				return false
			}
			
			// Filter to only our test videos
			var testVideos []*model.Video
			codeSet := make(map[string]bool)
			for _, code := range codes {
				codeSet[code] = true
			}
			for _, v := range unpushed {
				if codeSet[v.Code] {
					testVideos = append(testVideos, v)
				}
			}
			
			// Verify ordering: each video's created_at should be >= next video's created_at
			for i := 0; i < len(testVideos)-1; i++ {
				if testVideos[i].CreatedAt.Before(testVideos[i+1].CreatedAt) {
					// Clean up
					for _, code := range codes {
						store.db.Where("code = ?", code).Delete(&model.Video{})
					}
					return false
				}
			}
			
			// Clean up
			for _, code := range codes {
				store.db.Where("code = ?", code).Delete(&model.Video{})
			}
			
			return true
		},
		gen.SliceOfN(5, genVideoCode()).SuchThat(func(codes []string) bool {
			// Ensure all codes are unique
			seen := make(map[string]bool)
			for _, code := range codes {
				if seen[code] {
					return false
				}
				seen[code] = true
			}
			return len(codes) >= 2
		}),
	))

	properties.TestingRun(t)
}
