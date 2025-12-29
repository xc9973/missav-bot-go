package push

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/rs/zerolog/log"
	"github.com/user/missav-bot-go/internal/model"
	"github.com/user/missav-bot-go/internal/store"
	"golang.org/x/time/rate"
)

// TelegramClient defines the interface for sending Telegram messages
type TelegramClient interface {
	SendMessage(chatID int64, text string) error
	SendMarkdown(chatID int64, text string) error
	SendPhoto(chatID int64, photoURL string, caption string) error
	SendVideo(chatID int64, videoURL string, thumbURL string, caption string) error
}

// Service handles pushing video notifications to subscribers
type Service struct {
	store    store.Store
	telegram TelegramClient
	limiter  *rate.Limiter // Telegram rate limit: max 30 msg/sec globally
}

// NewService creates a new push service
func NewService(store store.Store, telegram TelegramClient) *Service {
	return &Service{
		store:    store,
		telegram: telegram,
		// Telegram rate limit: 30 messages per second globally
		limiter: rate.NewLimiter(rate.Limit(30), 1),
	}
}

// MatchesSubscription checks if a video matches a subscription
// Returns true if:
// - ALL type subscription: always matches
// - ACTRESS type subscription: video.actresses contains subscription.keyword (case-insensitive)
// - TAG type subscription: video.tags contains subscription.keyword (case-insensitive)
func MatchesSubscription(video *model.Video, sub *model.Subscription) bool {
	if video == nil || sub == nil {
		return false
	}

	switch sub.Type {
	case model.SubTypeAll:
		return true
	case model.SubTypeActress:
		return strings.Contains(
			strings.ToLower(video.Actresses),
			strings.ToLower(sub.Keyword),
		)
	case model.SubTypeTag:
		return strings.Contains(
			strings.ToLower(video.Tags),
			strings.ToLower(sub.Keyword),
		)
	default:
		return false
	}
}

// PushUnpushedVideos fetches all unpushed videos and pushes them to matching subscribers
func (s *Service) PushUnpushedVideos(ctx context.Context) error {
	videos, err := s.store.GetUnpushedVideos(ctx)
	if err != nil {
		return fmt.Errorf("failed to get unpushed videos: %w", err)
	}

	log.Info().Int("count", len(videos)).Msg("Found unpushed videos")

	for _, video := range videos {
		if err := s.PushVideoToSubscribers(ctx, video); err != nil {
			log.Error().Err(err).Str("code", video.Code).Msg("Failed to push video to subscribers")
			continue
		}

		// Mark video as pushed after successful push to all subscribers
		if err := s.store.MarkAsPushed(ctx, video.ID); err != nil {
			log.Error().Err(err).Str("code", video.Code).Msg("Failed to mark video as pushed")
		}
	}

	return nil
}

// PushVideoToSubscribers pushes a video to all matching subscribers
func (s *Service) PushVideoToSubscribers(ctx context.Context, video *model.Video) error {
	subs, err := s.store.GetMatchingSubscriptions(ctx, video)
	if err != nil {
		return fmt.Errorf("failed to get matching subscriptions: %w", err)
	}

	log.Info().
		Str("code", video.Code).
		Int("subscribers", len(subs)).
		Msg("Pushing video to subscribers")

	// Track which chats we've already pushed to (for deduplication)
	pushedChats := make(map[int64]bool)

	for _, sub := range subs {
		// Skip if we've already pushed to this chat
		if pushedChats[sub.ChatID] {
			continue
		}

		if err := s.PushVideoToChat(ctx, video, sub.ChatID); err != nil {
			log.Error().
				Err(err).
				Str("code", video.Code).
				Int64("chatID", sub.ChatID).
				Msg("Failed to push video to chat")
			continue
		}

		pushedChats[sub.ChatID] = true

		// Add 1 second delay between messages to same chat (Requirement 5.10)
		time.Sleep(1 * time.Second)
	}

	return nil
}


// PushVideoToChat pushes a video to a specific chat
// It checks for duplicates before pushing and records the push result
func (s *Service) PushVideoToChat(ctx context.Context, video *model.Video, chatID int64) error {
	// Check if already pushed (Requirement 5.3)
	hasPushed, err := s.store.HasPushed(ctx, video.ID, chatID)
	if err != nil {
		return fmt.Errorf("failed to check push history: %w", err)
	}

	if hasPushed {
		log.Debug().
			Str("code", video.Code).
			Int64("chatID", chatID).
			Msg("Video already pushed to chat, skipping")
		return nil
	}

	// Wait for rate limiter (Requirement 5.9)
	if err := s.limiter.Wait(ctx); err != nil {
		return fmt.Errorf("rate limiter error: %w", err)
	}

	// Format the message
	message := FormatVideoMessage(video)

	// Try to send with media
	var sendErr error
	var messageID int

	// Try video first if preview URL exists (Requirement 5.8)
	if video.PreviewURL != "" {
		sendErr = s.telegram.SendVideo(chatID, video.PreviewURL, video.CoverURL, message)
	}

	// Fallback to photo if video fails or no preview URL (Requirement 5.7)
	if sendErr != nil || video.PreviewURL == "" {
		if video.CoverURL != "" {
			sendErr = s.telegram.SendPhoto(chatID, video.CoverURL, message)
		} else {
			// No media, send text only
			sendErr = s.telegram.SendMarkdown(chatID, message)
		}
	}

	// Record the push result
	record := &model.PushRecord{
		VideoID:   video.ID,
		ChatID:    chatID,
		PushedAt:  time.Now(),
		MessageID: messageID,
	}

	if sendErr != nil {
		// Record failed push (Requirement 5.5)
		record.Status = model.PushStatusFailed
		record.FailReason = sendErr.Error()
		log.Error().
			Err(sendErr).
			Str("code", video.Code).
			Int64("chatID", chatID).
			Msg("Failed to send message")
	} else {
		// Record successful push (Requirement 5.4)
		record.Status = model.PushStatusSuccess
		log.Info().
			Str("code", video.Code).
			Int64("chatID", chatID).
			Msg("Successfully pushed video")
	}

	if err := s.store.RecordPush(ctx, record); err != nil {
		log.Error().Err(err).Msg("Failed to record push")
	}

	return sendErr
}

// FindMatchingSubscriptions finds all subscriptions that match a video
// This is a helper function that can be used for testing
func (s *Service) FindMatchingSubscriptions(ctx context.Context, video *model.Video) ([]*model.Subscription, error) {
	return s.store.GetMatchingSubscriptions(ctx, video)
}
