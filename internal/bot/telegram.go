package bot

import (
	"fmt"

	tgbotapi "github.com/go-telegram-bot-api/telegram-bot-api/v5"
)

// Client wraps the Telegram Bot API for sending messages
type Client struct {
	api *tgbotapi.BotAPI
}

// NewClient creates a new Telegram client with the given bot token
func NewClient(token string) (*Client, error) {
	api, err := tgbotapi.NewBotAPI(token)
	if err != nil {
		return nil, fmt.Errorf("failed to create bot API: %w", err)
	}

	return &Client{api: api}, nil
}

// GetAPI returns the underlying bot API for advanced operations
func (c *Client) GetAPI() *tgbotapi.BotAPI {
	return c.api
}

// GetUpdates returns a channel for receiving updates from Telegram
func (c *Client) GetUpdates() tgbotapi.UpdatesChannel {
	u := tgbotapi.NewUpdate(0)
	u.Timeout = 60
	return c.api.GetUpdatesChan(u)
}

// StopReceivingUpdates stops the update channel
func (c *Client) StopReceivingUpdates() {
	c.api.StopReceivingUpdates()
}

// SendMessage sends a plain text message to a chat
func (c *Client) SendMessage(chatID int64, text string) error {
	msg := tgbotapi.NewMessage(chatID, text)
	_, err := c.api.Send(msg)
	if err != nil {
		return fmt.Errorf("failed to send message: %w", err)
	}
	return nil
}

// SendMarkdown sends a message with MarkdownV2 formatting to a chat
func (c *Client) SendMarkdown(chatID int64, text string) error {
	msg := tgbotapi.NewMessage(chatID, text)
	msg.ParseMode = tgbotapi.ModeMarkdownV2
	_, err := c.api.Send(msg)
	if err != nil {
		return fmt.Errorf("failed to send markdown message: %w", err)
	}
	return nil
}


// SendPhoto sends a photo with caption to a chat
// The photoURL can be a URL or a file_id
func (c *Client) SendPhoto(chatID int64, photoURL string, caption string) error {
	photo := tgbotapi.NewPhoto(chatID, tgbotapi.FileURL(photoURL))
	photo.Caption = caption
	photo.ParseMode = tgbotapi.ModeMarkdownV2
	_, err := c.api.Send(photo)
	if err != nil {
		return fmt.Errorf("failed to send photo: %w", err)
	}
	return nil
}

// SendVideo sends a video with thumbnail and caption to a chat
// The videoURL and thumbURL can be URLs or file_ids
func (c *Client) SendVideo(chatID int64, videoURL string, thumbURL string, caption string) error {
	video := tgbotapi.NewVideo(chatID, tgbotapi.FileURL(videoURL))
	video.Caption = caption
	video.ParseMode = tgbotapi.ModeMarkdownV2
	if thumbURL != "" {
		video.Thumb = tgbotapi.FileURL(thumbURL)
	}
	_, err := c.api.Send(video)
	if err != nil {
		return fmt.Errorf("failed to send video: %w", err)
	}
	return nil
}

// SendMessageWithReply sends a message as a reply to another message
func (c *Client) SendMessageWithReply(chatID int64, text string, replyToMessageID int) error {
	msg := tgbotapi.NewMessage(chatID, text)
	msg.ReplyToMessageID = replyToMessageID
	_, err := c.api.Send(msg)
	if err != nil {
		return fmt.Errorf("failed to send reply message: %w", err)
	}
	return nil
}

// SendMarkdownWithReply sends a markdown message as a reply to another message
func (c *Client) SendMarkdownWithReply(chatID int64, text string, replyToMessageID int) error {
	msg := tgbotapi.NewMessage(chatID, text)
	msg.ParseMode = tgbotapi.ModeMarkdownV2
	msg.ReplyToMessageID = replyToMessageID
	_, err := c.api.Send(msg)
	if err != nil {
		return fmt.Errorf("failed to send markdown reply: %w", err)
	}
	return nil
}
