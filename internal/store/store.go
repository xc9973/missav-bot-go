package store

import (
	"context"

	"github.com/user/missav-bot-go/internal/model"
)

// Store defines the interface for data persistence operations
type Store interface {
	// Video operations
	SaveVideo(ctx context.Context, video *model.Video) error
	SaveVideos(ctx context.Context, videos []*model.Video) (saved int, duplicates int, err error)
	GetVideoByCode(ctx context.Context, code string) (*model.Video, error)
	GetUnpushedVideos(ctx context.Context) ([]*model.Video, error)
	MarkAsPushed(ctx context.Context, videoID uint) error
	SearchVideos(ctx context.Context, keyword string, limit int) ([]*model.Video, error)
	GetLatestVideos(ctx context.Context, limit, offset int) ([]*model.Video, error)
	CountVideos(ctx context.Context) (int64, error)
	ExistsByCode(ctx context.Context, code string) (bool, error)

	// Subscription operations
	CreateSubscription(ctx context.Context, sub *model.Subscription) error
	DeleteSubscription(ctx context.Context, chatID int64, subType string, keyword string) error
	DeleteAllSubscriptions(ctx context.Context, chatID int64) error
	GetSubscriptions(ctx context.Context, chatID int64) ([]*model.Subscription, error)
	GetAllSubscriptions(ctx context.Context) ([]*model.Subscription, error)
	GetMatchingSubscriptions(ctx context.Context, video *model.Video) ([]*model.Subscription, error)

	// PushRecord operations
	RecordPush(ctx context.Context, record *model.PushRecord) error
	HasPushed(ctx context.Context, videoID uint, chatID int64) (bool, error)

	// Health check
	Ping(ctx context.Context) error
	Close() error
}
