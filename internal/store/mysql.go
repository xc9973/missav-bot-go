package store

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/user/missav-bot-go/internal/config"
	"github.com/user/missav-bot-go/internal/model"
	"gorm.io/driver/mysql"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
	"gorm.io/gorm/logger"
)

// MySQLStore implements Store interface using MySQL database
type MySQLStore struct {
	db *gorm.DB
}

// NewMySQLStore creates a new MySQL store instance
func NewMySQLStore(cfg *config.DBConfig) (*MySQLStore, error) {
	dsn := fmt.Sprintf("%s:%s@tcp(%s:%d)/%s?charset=utf8mb4&parseTime=True&loc=Local",
		cfg.User, cfg.Password, cfg.Host, cfg.Port, cfg.Database)

	gormConfig := &gorm.Config{
		Logger: logger.Default.LogMode(logger.Silent),
	}

	db, err := gorm.Open(mysql.Open(dsn), gormConfig)
	if err != nil {
		return nil, fmt.Errorf("failed to connect to database: %w", err)
	}

	sqlDB, err := db.DB()
	if err != nil {
		return nil, fmt.Errorf("failed to get underlying sql.DB: %w", err)
	}

	// Configure connection pool
	sqlDB.SetMaxOpenConns(cfg.MaxConns)
	sqlDB.SetMaxIdleConns(cfg.MaxConns / 2)
	sqlDB.SetConnMaxLifetime(time.Hour)

	// Auto migrate tables
	if err := db.AutoMigrate(&model.Video{}, &model.Subscription{}, &model.PushRecord{}); err != nil {
		return nil, fmt.Errorf("failed to migrate database: %w", err)
	}

	return &MySQLStore{db: db}, nil
}


// SaveVideo saves a single video to the database
// Returns error if video already exists (duplicate code)
func (s *MySQLStore) SaveVideo(ctx context.Context, video *model.Video) error {
	// Ensure new videos have pushed=false
	video.Pushed = false
	
	result := s.db.WithContext(ctx).Clauses(clause.OnConflict{
		Columns:   []clause.Column{{Name: "code"}},
		DoNothing: true,
	}).Create(video)
	
	if result.Error != nil {
		return fmt.Errorf("failed to save video: %w", result.Error)
	}
	
	return nil
}

// SaveVideos saves multiple videos in batch
// Returns count of saved videos, duplicates, and any error
func (s *MySQLStore) SaveVideos(ctx context.Context, videos []*model.Video) (saved int, duplicates int, err error) {
	if len(videos) == 0 {
		return 0, 0, nil
	}

	// Ensure all new videos have pushed=false
	for _, v := range videos {
		v.Pushed = false
	}

	result := s.db.WithContext(ctx).Clauses(clause.OnConflict{
		Columns:   []clause.Column{{Name: "code"}},
		DoNothing: true,
	}).CreateInBatches(videos, 100)

	if result.Error != nil {
		return 0, 0, fmt.Errorf("failed to save videos: %w", result.Error)
	}

	saved = int(result.RowsAffected)
	duplicates = len(videos) - saved
	return saved, duplicates, nil
}

// GetVideoByCode retrieves a video by its code
func (s *MySQLStore) GetVideoByCode(ctx context.Context, code string) (*model.Video, error) {
	var video model.Video
	result := s.db.WithContext(ctx).Where("code = ?", code).First(&video)
	if result.Error != nil {
		if errors.Is(result.Error, gorm.ErrRecordNotFound) {
			return nil, nil
		}
		return nil, fmt.Errorf("failed to get video by code: %w", result.Error)
	}
	return &video, nil
}

// GetUnpushedVideos retrieves all videos that haven't been pushed yet
// Ordered by created_at DESC
func (s *MySQLStore) GetUnpushedVideos(ctx context.Context) ([]*model.Video, error) {
	var videos []*model.Video
	result := s.db.WithContext(ctx).
		Where("pushed = ?", false).
		Order("created_at DESC").
		Find(&videos)
	if result.Error != nil {
		return nil, fmt.Errorf("failed to get unpushed videos: %w", result.Error)
	}
	return videos, nil
}

// MarkAsPushed marks a video as pushed atomically
func (s *MySQLStore) MarkAsPushed(ctx context.Context, videoID uint) error {
	result := s.db.WithContext(ctx).
		Model(&model.Video{}).
		Where("id = ?", videoID).
		Update("pushed", true)
	if result.Error != nil {
		return fmt.Errorf("failed to mark video as pushed: %w", result.Error)
	}
	return nil
}

// SearchVideos searches videos by keyword in code, title, actresses, or tags
func (s *MySQLStore) SearchVideos(ctx context.Context, keyword string, limit int) ([]*model.Video, error) {
	var videos []*model.Video
	searchPattern := "%" + keyword + "%"
	result := s.db.WithContext(ctx).
		Where("code LIKE ? OR title LIKE ? OR actresses LIKE ? OR tags LIKE ?",
			searchPattern, searchPattern, searchPattern, searchPattern).
		Order("created_at DESC").
		Limit(limit).
		Find(&videos)
	if result.Error != nil {
		return nil, fmt.Errorf("failed to search videos: %w", result.Error)
	}
	return videos, nil
}

// GetLatestVideos retrieves the latest videos with pagination
func (s *MySQLStore) GetLatestVideos(ctx context.Context, limit, offset int) ([]*model.Video, error) {
	var videos []*model.Video
	result := s.db.WithContext(ctx).
		Order("created_at DESC").
		Limit(limit).
		Offset(offset).
		Find(&videos)
	if result.Error != nil {
		return nil, fmt.Errorf("failed to get latest videos: %w", result.Error)
	}
	return videos, nil
}

// CountVideos returns the total count of videos
func (s *MySQLStore) CountVideos(ctx context.Context) (int64, error) {
	var count int64
	result := s.db.WithContext(ctx).Model(&model.Video{}).Count(&count)
	if result.Error != nil {
		return 0, fmt.Errorf("failed to count videos: %w", result.Error)
	}
	return count, nil
}

// ExistsByCode checks if a video with the given code exists
func (s *MySQLStore) ExistsByCode(ctx context.Context, code string) (bool, error) {
	var count int64
	result := s.db.WithContext(ctx).Model(&model.Video{}).Where("code = ?", code).Count(&count)
	if result.Error != nil {
		return false, fmt.Errorf("failed to check video existence: %w", result.Error)
	}
	return count > 0, nil
}


// CreateSubscription creates a new subscription
func (s *MySQLStore) CreateSubscription(ctx context.Context, sub *model.Subscription) error {
	// Check if subscription already exists
	var existing model.Subscription
	result := s.db.WithContext(ctx).
		Where("chat_id = ? AND type = ? AND keyword = ?", sub.ChatID, sub.Type, sub.Keyword).
		First(&existing)
	
	if result.Error == nil {
		// Subscription already exists, update enabled status
		return s.db.WithContext(ctx).
			Model(&existing).
			Update("enabled", true).Error
	}
	
	if !errors.Is(result.Error, gorm.ErrRecordNotFound) {
		return fmt.Errorf("failed to check existing subscription: %w", result.Error)
	}
	
	// Create new subscription
	if err := s.db.WithContext(ctx).Create(sub).Error; err != nil {
		return fmt.Errorf("failed to create subscription: %w", err)
	}
	return nil
}

// DeleteSubscription deletes a specific subscription
func (s *MySQLStore) DeleteSubscription(ctx context.Context, chatID int64, subType string, keyword string) error {
	result := s.db.WithContext(ctx).
		Where("chat_id = ? AND type = ? AND keyword = ?", chatID, subType, keyword).
		Delete(&model.Subscription{})
	if result.Error != nil {
		return fmt.Errorf("failed to delete subscription: %w", result.Error)
	}
	return nil
}

// DeleteAllSubscriptions deletes all subscriptions for a chat
func (s *MySQLStore) DeleteAllSubscriptions(ctx context.Context, chatID int64) error {
	result := s.db.WithContext(ctx).
		Where("chat_id = ?", chatID).
		Delete(&model.Subscription{})
	if result.Error != nil {
		return fmt.Errorf("failed to delete all subscriptions: %w", result.Error)
	}
	return nil
}

// GetSubscriptions retrieves all subscriptions for a chat
func (s *MySQLStore) GetSubscriptions(ctx context.Context, chatID int64) ([]*model.Subscription, error) {
	var subs []*model.Subscription
	result := s.db.WithContext(ctx).
		Where("chat_id = ? AND enabled = ?", chatID, true).
		Find(&subs)
	if result.Error != nil {
		return nil, fmt.Errorf("failed to get subscriptions: %w", result.Error)
	}
	return subs, nil
}

// GetAllSubscriptions retrieves all enabled subscriptions
func (s *MySQLStore) GetAllSubscriptions(ctx context.Context) ([]*model.Subscription, error) {
	var subs []*model.Subscription
	result := s.db.WithContext(ctx).
		Where("enabled = ?", true).
		Find(&subs)
	if result.Error != nil {
		return nil, fmt.Errorf("failed to get all subscriptions: %w", result.Error)
	}
	return subs, nil
}

// GetMatchingSubscriptions finds subscriptions that match a video
func (s *MySQLStore) GetMatchingSubscriptions(ctx context.Context, video *model.Video) ([]*model.Subscription, error) {
	allSubs, err := s.GetAllSubscriptions(ctx)
	if err != nil {
		return nil, err
	}

	var matching []*model.Subscription
	for _, sub := range allSubs {
		if matchesSubscription(video, sub) {
			matching = append(matching, sub)
		}
	}
	return matching, nil
}

// matchesSubscription checks if a video matches a subscription
func matchesSubscription(video *model.Video, sub *model.Subscription) bool {
	switch sub.Type {
	case model.SubTypeAll:
		return true
	case model.SubTypeActress:
		return containsIgnoreCase(video.Actresses, sub.Keyword)
	case model.SubTypeTag:
		return containsIgnoreCase(video.Tags, sub.Keyword)
	default:
		return false
	}
}

// containsIgnoreCase checks if s contains substr (case-insensitive)
func containsIgnoreCase(s, substr string) bool {
	return strings.Contains(strings.ToLower(s), strings.ToLower(substr))
}


// RecordPush records a push operation
func (s *MySQLStore) RecordPush(ctx context.Context, record *model.PushRecord) error {
	record.PushedAt = time.Now()
	if err := s.db.WithContext(ctx).Create(record).Error; err != nil {
		return fmt.Errorf("failed to record push: %w", err)
	}
	return nil
}

// HasPushed checks if a video has been successfully pushed to a chat
func (s *MySQLStore) HasPushed(ctx context.Context, videoID uint, chatID int64) (bool, error) {
	var count int64
	result := s.db.WithContext(ctx).
		Model(&model.PushRecord{}).
		Where("video_id = ? AND chat_id = ? AND status = ?", videoID, chatID, model.PushStatusSuccess).
		Count(&count)
	if result.Error != nil {
		return false, fmt.Errorf("failed to check push status: %w", result.Error)
	}
	return count > 0, nil
}

// Ping checks database connectivity
func (s *MySQLStore) Ping(ctx context.Context) error {
	sqlDB, err := s.db.DB()
	if err != nil {
		return fmt.Errorf("failed to get underlying db: %w", err)
	}
	return sqlDB.PingContext(ctx)
}

// Close closes the database connection
func (s *MySQLStore) Close() error {
	sqlDB, err := s.db.DB()
	if err != nil {
		return fmt.Errorf("failed to get underlying db: %w", err)
	}
	return sqlDB.Close()
}

// DB returns the underlying gorm.DB instance (for testing purposes)
func (s *MySQLStore) DB() *gorm.DB {
	return s.db
}
