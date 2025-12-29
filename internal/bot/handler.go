package bot

import (
	"context"
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/rs/zerolog/log"
	tgbotapi "github.com/go-telegram-bot-api/telegram-bot-api/v5"
	"github.com/user/missav-bot-go/internal/crawler"
	"github.com/user/missav-bot-go/internal/model"
	"github.com/user/missav-bot-go/internal/push"
	"github.com/user/missav-bot-go/internal/store"
)

// Handler handles Telegram bot commands
type Handler struct {
	store       store.Store
	crawler     crawler.Crawler
	pushService *push.Service
	telegram    *Client
	startTime   time.Time
}

// NewHandler creates a new command handler
func NewHandler(store store.Store, crawler crawler.Crawler, pushService *push.Service, telegram *Client) *Handler {
	return &Handler{
		store:       store,
		crawler:     crawler,
		pushService: pushService,
		telegram:    telegram,
		startTime:   time.Now(),
	}
}

// HandleUpdate processes an incoming Telegram update
func (h *Handler) HandleUpdate(ctx context.Context, update tgbotapi.Update) {
	if update.Message == nil {
		return
	}

	msg := update.Message
	chatID := msg.Chat.ID
	chatType := msg.Chat.Type

	// Handle commands
	if msg.IsCommand() {
		h.handleCommand(ctx, msg)
		return
	}

	// Auto-subscribe group chats on first message (Requirement 3.12)
	if chatType == "group" || chatType == "supergroup" {
		h.autoSubscribeGroup(ctx, chatID, chatType)
	}
}


// handleCommand routes commands to their respective handlers
func (h *Handler) handleCommand(ctx context.Context, msg *tgbotapi.Message) {
	chatID := msg.Chat.ID
	chatType := msg.Chat.Type
	command := msg.Command()
	args := strings.TrimSpace(msg.CommandArguments())

	log.Info().
		Int64("chatID", chatID).
		Str("command", command).
		Str("args", args).
		Msg("Received command")

	switch command {
	case "start", "help":
		h.handleStart(ctx, chatID)
	case "subscribe":
		h.handleSubscribe(ctx, chatID, chatType, args)
	case "unsubscribe":
		h.handleUnsubscribe(ctx, chatID, args)
	case "list":
		h.handleList(ctx, chatID)
	case "search":
		h.handleSearch(ctx, chatID, args)
	case "latest":
		h.handleLatest(ctx, chatID, args)
	case "crawl":
		h.handleCrawl(ctx, chatID, chatType, args)
	case "status":
		h.handleStatus(ctx, chatID)
	default:
		h.sendError(chatID, "未知命令。使用 /help 查看可用命令。")
	}
}

// handleStart handles /start and /help commands (Requirement 3.1)
func (h *Handler) handleStart(ctx context.Context, chatID int64) {
	helpText := `🤖 *MissAV 机器人帮助*

*订阅命令:*
/subscribe \- 订阅所有新视频
/subscribe 演员名 \- 订阅特定演员
/subscribe \#标签 \- 订阅特定标签
/unsubscribe \- 取消所有订阅
/unsubscribe 关键词 \- 取消特定订阅
/list \- 查看我的订阅

*搜索命令:*
/search 关键词 \- 搜索视频（最多10条）
/latest \[页码\] \- 查看最新视频

*管理命令:*
/crawl actor/code/search 关键词 \- 手动爬取
/status \- 查看机器人状态

_提示: 在群组中，机器人会自动订阅所有视频_`

	if err := h.telegram.SendMarkdown(chatID, helpText); err != nil {
		log.Error().Err(err).Int64("chatID", chatID).Msg("Failed to send help message")
	}
}


// DetermineSubscriptionType determines the subscription type based on the argument
// - Empty string → ALL type subscription
// - String starting with "#" → TAG type subscription with keyword (without #)
// - Other string → ACTRESS type subscription with keyword
// This function is exported for property testing (Property 4)
func DetermineSubscriptionType(args string) (model.SubscriptionType, string) {
	args = strings.TrimSpace(args)
	
	if args == "" {
		return model.SubTypeAll, ""
	}
	
	if strings.HasPrefix(args, "#") {
		keyword := strings.TrimPrefix(args, "#")
		return model.SubTypeTag, keyword
	}
	
	return model.SubTypeActress, args
}

// handleSubscribe handles /subscribe command (Requirements 3.2, 3.3, 3.4)
func (h *Handler) handleSubscribe(ctx context.Context, chatID int64, chatType string, args string) {
	subType, keyword := DetermineSubscriptionType(args)

	sub := &model.Subscription{
		ChatID:   chatID,
		ChatType: chatType,
		Type:     subType,
		Keyword:  keyword,
		Enabled:  true,
	}

	if err := h.store.CreateSubscription(ctx, sub); err != nil {
		log.Error().Err(err).Int64("chatID", chatID).Msg("Failed to create subscription")
		h.sendError(chatID, "创建订阅失败，请重试。")
		return
	}

	var message string
	switch subType {
	case model.SubTypeAll:
		message = "✅ 已订阅所有新视频！"
	case model.SubTypeActress:
		message = fmt.Sprintf("✅ 已订阅演员: %s", keyword)
	case model.SubTypeTag:
		message = fmt.Sprintf("✅ 已订阅标签: #%s", keyword)
	}

	if err := h.telegram.SendMessage(chatID, message); err != nil {
		log.Error().Err(err).Int64("chatID", chatID).Msg("Failed to send subscription confirmation")
	}
}

// handleUnsubscribe handles /unsubscribe command (Requirements 3.5, 3.6)
func (h *Handler) handleUnsubscribe(ctx context.Context, chatID int64, args string) {
	args = strings.TrimSpace(args)

	if args == "" {
		// Unsubscribe from all (Requirement 3.5)
		if err := h.store.DeleteAllSubscriptions(ctx, chatID); err != nil {
			log.Error().Err(err).Int64("chatID", chatID).Msg("Failed to delete all subscriptions")
			h.sendError(chatID, "取消订阅失败，请重试。")
			return
		}
		if err := h.telegram.SendMessage(chatID, "✅ 已取消所有订阅。"); err != nil {
			log.Error().Err(err).Int64("chatID", chatID).Msg("Failed to send unsubscribe confirmation")
		}
		return
	}

	// Unsubscribe from specific keyword (Requirement 3.6)
	subType, keyword := DetermineSubscriptionType(args)
	if err := h.store.DeleteSubscription(ctx, chatID, string(subType), keyword); err != nil {
		log.Error().Err(err).Int64("chatID", chatID).Str("keyword", keyword).Msg("Failed to delete subscription")
		h.sendError(chatID, "取消订阅失败，请重试。")
		return
	}

	message := fmt.Sprintf("✅ 已取消订阅: %s", args)
	if err := h.telegram.SendMessage(chatID, message); err != nil {
		log.Error().Err(err).Int64("chatID", chatID).Msg("Failed to send unsubscribe confirmation")
	}
}


// handleList handles /list command (Requirement 3.7)
func (h *Handler) handleList(ctx context.Context, chatID int64) {
	subs, err := h.store.GetSubscriptions(ctx, chatID)
	if err != nil {
		log.Error().Err(err).Int64("chatID", chatID).Msg("Failed to get subscriptions")
		h.sendError(chatID, "获取订阅列表失败，请重试。")
		return
	}

	if len(subs) == 0 {
		if err := h.telegram.SendMessage(chatID, "📭 你还没有任何订阅。\n使用 /subscribe 开始接收通知。"); err != nil {
			log.Error().Err(err).Int64("chatID", chatID).Msg("Failed to send empty list message")
		}
		return
	}

	var lines []string
	lines = append(lines, "📋 *我的订阅:*\n")
	for i, sub := range subs {
		var line string
		switch sub.Type {
		case model.SubTypeAll:
			line = fmt.Sprintf("%d\\. 🌐 所有视频", i+1)
		case model.SubTypeActress:
			line = fmt.Sprintf("%d\\. 👩 演员: %s", i+1, push.EscapeMarkdown(sub.Keyword))
		case model.SubTypeTag:
			line = fmt.Sprintf("%d\\. 🏷 标签: \\#%s", i+1, push.EscapeMarkdown(sub.Keyword))
		}
		lines = append(lines, line)
	}

	if err := h.telegram.SendMarkdown(chatID, strings.Join(lines, "\n")); err != nil {
		log.Error().Err(err).Int64("chatID", chatID).Msg("Failed to send subscription list")
	}
}

// handleSearch handles /search command (Requirement 3.8)
// Returns at most 10 results (Property 5)
func (h *Handler) handleSearch(ctx context.Context, chatID int64, keyword string) {
	if keyword == "" {
		h.sendError(chatID, "请提供搜索关键词。例如: /search ABC-123")
		return
	}

	// Limit to 10 results (Requirement 3.8, Property 5)
	videos, err := h.store.SearchVideos(ctx, keyword, 10)
	if err != nil {
		log.Error().Err(err).Int64("chatID", chatID).Str("keyword", keyword).Msg("Failed to search videos")
		h.sendError(chatID, "搜索失败，请重试。")
		return
	}

	if len(videos) == 0 {
		if err := h.telegram.SendMessage(chatID, fmt.Sprintf("🔍 未找到相关视频: %s", keyword)); err != nil {
			log.Error().Err(err).Int64("chatID", chatID).Msg("Failed to send no results message")
		}
		return
	}

	var lines []string
	lines = append(lines, fmt.Sprintf("🔍 *搜索结果: %s*\n", push.EscapeMarkdown(keyword)))
	for i, video := range videos {
		line := fmt.Sprintf("%d\\. *%s*", i+1, push.EscapeMarkdown(video.Code))
		if video.Title != "" {
			// Truncate title if too long
			title := video.Title
			if len(title) > 50 {
				title = title[:47] + "..."
			}
			line += fmt.Sprintf("\n   %s", push.EscapeMarkdown(title))
		}
		lines = append(lines, line)
	}

	if err := h.telegram.SendMarkdown(chatID, strings.Join(lines, "\n")); err != nil {
		log.Error().Err(err).Int64("chatID", chatID).Msg("Failed to send search results")
	}
}


// handleLatest handles /latest command (Requirement 3.9)
func (h *Handler) handleLatest(ctx context.Context, chatID int64, args string) {
	page := 1
	if args != "" {
		var err error
		page, err = strconv.Atoi(args)
		if err != nil || page < 1 {
			page = 1
		}
	}

	limit := 5
	offset := (page - 1) * limit

	videos, err := h.store.GetLatestVideos(ctx, limit, offset)
	if err != nil {
		log.Error().Err(err).Int64("chatID", chatID).Msg("Failed to get latest videos")
		h.sendError(chatID, "获取最新视频失败，请重试。")
		return
	}

	if len(videos) == 0 {
		if err := h.telegram.SendMessage(chatID, "📭 暂无视频。"); err != nil {
			log.Error().Err(err).Int64("chatID", chatID).Msg("Failed to send no videos message")
		}
		return
	}

	var lines []string
	lines = append(lines, fmt.Sprintf("📺 *最新视频（第 %d 页）*\n", page))
	for i, video := range videos {
		line := fmt.Sprintf("%d\\. *%s*", i+1, push.EscapeMarkdown(video.Code))
		if video.Actresses != "" {
			line += fmt.Sprintf(" \\- %s", push.EscapeMarkdown(video.Actresses))
		}
		if video.DetailURL != "" {
			line += fmt.Sprintf("\n   🔗 %s", push.EscapeMarkdown(video.DetailURL))
		}
		lines = append(lines, line)
	}

	// Add pagination hint
	if len(videos) == limit {
		lines = append(lines, fmt.Sprintf("\n_使用 /latest %d 查看下一页_", page+1))
	}

	if err := h.telegram.SendMarkdown(chatID, strings.Join(lines, "\n")); err != nil {
		log.Error().Err(err).Int64("chatID", chatID).Msg("Failed to send latest videos")
	}
}

// handleCrawl handles /crawl command (Requirement 3.10)
func (h *Handler) handleCrawl(ctx context.Context, chatID int64, chatType string, args string) {
	if args == "" {
		h.sendError(chatID, "请指定爬取类型。例如:\n/crawl actor 三上悠亜\n/crawl code ABC-123\n/crawl search 关键词")
		return
	}

	parts := strings.SplitN(args, " ", 2)
	crawlType := strings.ToLower(parts[0])
	var keyword string
	if len(parts) > 1 {
		keyword = strings.TrimSpace(parts[1])
	}

	if keyword == "" && crawlType != "new" {
		h.sendError(chatID, "请提供爬取关键词。")
		return
	}

	// Send acknowledgment
	if err := h.telegram.SendMessage(chatID, "🔄 开始爬取... 请稍候。"); err != nil {
		log.Error().Err(err).Int64("chatID", chatID).Msg("Failed to send crawl acknowledgment")
	}

	// Execute crawl asynchronously
	go func() {
		var videos []*model.Video
		var err error

		switch crawlType {
		case "actor", "actress":
			videos, err = h.crawler.CrawlByActor(ctx, keyword, 20)
		case "code":
			video, crawlErr := h.crawler.CrawlByCode(ctx, keyword)
			if crawlErr != nil {
				err = crawlErr
			} else if video != nil {
				videos = []*model.Video{video}
			}
		case "search", "keyword":
			videos, err = h.crawler.CrawlByKeyword(ctx, keyword, 20)
		case "new":
			videos, err = h.crawler.CrawlNewVideos(ctx, 2)
		default:
			h.sendError(chatID, "未知爬取类型。可用: actor, code, search, new")
			return
		}

		if err != nil {
			log.Error().Err(err).Str("type", crawlType).Str("keyword", keyword).Msg("Crawl failed")
			h.sendError(chatID, fmt.Sprintf("❌ 爬取失败: %s", err.Error()))
			return
		}

		if len(videos) == 0 {
			if err := h.telegram.SendMessage(chatID, "📭 未找到视频。"); err != nil {
				log.Error().Err(err).Int64("chatID", chatID).Msg("Failed to send no results message")
			}
			return
		}

		// Save videos to database
		saved, duplicates, saveErr := h.store.SaveVideos(ctx, videos)
		if saveErr != nil {
			log.Error().Err(saveErr).Msg("Failed to save crawled videos")
		}

		message := fmt.Sprintf("✅ 爬取完成！\n📊 找到: %d 个视频\n💾 新增: %d 个\n🔄 重复: %d 个", len(videos), saved, duplicates)
		if err := h.telegram.SendMessage(chatID, message); err != nil {
			log.Error().Err(err).Int64("chatID", chatID).Msg("Failed to send crawl results")
		}
	}()
}


// handleStatus handles /status command (Requirement 3.11)
func (h *Handler) handleStatus(ctx context.Context, chatID int64) {
	videoCount, err := h.store.CountVideos(ctx)
	if err != nil {
		log.Error().Err(err).Msg("Failed to count videos")
		videoCount = -1
	}

	uptime := time.Since(h.startTime)
	uptimeStr := formatDuration(uptime)

	var lines []string
	lines = append(lines, "📊 *机器人状态*\n")
	lines = append(lines, fmt.Sprintf("🎬 数据库视频数: %d", videoCount))
	lines = append(lines, fmt.Sprintf("⏱ 运行时间: %s", uptimeStr))
	lines = append(lines, fmt.Sprintf("🕐 启动时间: %s", h.startTime.Format("2006\\-01\\-02 15:04:05")))

	if err := h.telegram.SendMarkdown(chatID, strings.Join(lines, "\n")); err != nil {
		log.Error().Err(err).Int64("chatID", chatID).Msg("Failed to send status")
	}
}

// autoSubscribeGroup auto-subscribes a group chat with ALL type (Requirement 3.12)
func (h *Handler) autoSubscribeGroup(ctx context.Context, chatID int64, chatType string) {
	// Check if already subscribed
	subs, err := h.store.GetSubscriptions(ctx, chatID)
	if err != nil {
		log.Error().Err(err).Int64("chatID", chatID).Msg("Failed to check existing subscriptions")
		return
	}

	// If already has subscriptions, don't auto-subscribe
	if len(subs) > 0 {
		return
	}

	// Create ALL type subscription
	sub := &model.Subscription{
		ChatID:   chatID,
		ChatType: chatType,
		Type:     model.SubTypeAll,
		Keyword:  "",
		Enabled:  true,
	}

	if err := h.store.CreateSubscription(ctx, sub); err != nil {
		log.Error().Err(err).Int64("chatID", chatID).Msg("Failed to auto-subscribe group")
		return
	}

	log.Info().Int64("chatID", chatID).Msg("Auto-subscribed group to all videos")
}

// sendError sends an error message to a chat (Requirement 3.13)
func (h *Handler) sendError(chatID int64, message string) {
	if err := h.telegram.SendMessage(chatID, "❌ "+message); err != nil {
		log.Error().Err(err).Int64("chatID", chatID).Msg("Failed to send error message")
	}
}

// formatDuration formats a duration into a human-readable string
func formatDuration(d time.Duration) string {
	days := int(d.Hours()) / 24
	hours := int(d.Hours()) % 24
	minutes := int(d.Minutes()) % 60

	if days > 0 {
		return fmt.Sprintf("%d天 %d小时 %d分钟", days, hours, minutes)
	}
	if hours > 0 {
		return fmt.Sprintf("%d小时 %d分钟", hours, minutes)
	}
	return fmt.Sprintf("%d分钟", minutes)
}
