package push

import (
	"fmt"
	"strings"

	"github.com/user/missav-bot-go/internal/model"
)

// EscapeMarkdown escapes special characters for Telegram MarkdownV2 format
func EscapeMarkdown(text string) string {
	// Characters that need to be escaped in MarkdownV2:
	// _ * [ ] ( ) ~ ` > # + - = | { } . !
	specialChars := []string{"_", "*", "[", "]", "(", ")", "~", "`", ">", "#", "+", "-", "=", "|", "{", "}", ".", "!"}
	result := text
	for _, char := range specialChars {
		result = strings.ReplaceAll(result, char, "\\"+char)
	}
	return result
}

// FormatVideoMessage formats a video into a Telegram message string
// The message includes: video code, actresses (if present), tags (if present),
// duration (if present), and detail URL
func FormatVideoMessage(video *model.Video) string {
	if video == nil {
		return ""
	}

	var parts []string

	// Video code is always included (required field)
	parts = append(parts, fmt.Sprintf("🎬 *%s*", EscapeMarkdown(video.Code)))

	// Title if present
	if video.Title != "" {
		parts = append(parts, fmt.Sprintf("📝 %s", EscapeMarkdown(video.Title)))
	}

	// Actresses if present
	if video.Actresses != "" {
		parts = append(parts, fmt.Sprintf("👩 %s", EscapeMarkdown(video.Actresses)))
	}

	// Tags if present
	if video.Tags != "" {
		parts = append(parts, fmt.Sprintf("🏷 %s", EscapeMarkdown(video.Tags)))
	}

	// Duration if present (greater than 0)
	if video.Duration > 0 {
		minutes := video.Duration / 60
		seconds := video.Duration % 60
		parts = append(parts, fmt.Sprintf("⏱ %d:%02d", minutes, seconds))
	}

	// Detail URL is always included (required field)
	if video.DetailURL != "" {
		parts = append(parts, fmt.Sprintf("🔗 %s", EscapeMarkdown(video.DetailURL)))
	}

	return strings.Join(parts, "\n")
}
