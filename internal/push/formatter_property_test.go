package push

import (
	"strings"
	"testing"

	"github.com/leanovate/gopter"
	"github.com/leanovate/gopter/gen"
	"github.com/leanovate/gopter/prop"
	"github.com/user/missav-bot-go/internal/model"
)

// Property 11: Message Format Completeness
// *For any* video with non-empty fields, the formatted message SHALL contain: video code, and detail URL.
// If present, it SHALL also contain: actresses, tags, duration.
// **Validates: Requirements 5.6**
func TestProperty_MessageFormatCompleteness(t *testing.T) {
	parameters := gopter.DefaultTestParameters()
	parameters.MinSuccessfulTests = 100
	properties := gopter.NewProperties(parameters)

	// Generator for video code (required field)
	codeGen := gen.RegexMatch(`[A-Z]{2,5}-[0-9]{3,5}`)

	// Generator for non-empty strings (for optional fields)
	nonEmptyStringGen := gen.AlphaString().SuchThat(func(s string) bool {
		return len(s) > 0
	})

	// Generator for URL-like strings
	urlGen := gen.RegexMatch(`https://example\.com/video/[a-z0-9]+`)

	// Generator for duration (positive integer)
	durationGen := gen.IntRange(1, 7200) // 1 second to 2 hours

	// Property: Formatted message always contains video code when code is non-empty
	properties.Property("message contains video code", prop.ForAll(
		func(code string, detailURL string) bool {
			video := &model.Video{
				Code:      code,
				DetailURL: detailURL,
			}
			message := FormatVideoMessage(video)
			// The code should appear in the message (escaped)
			return strings.Contains(message, EscapeMarkdown(code))
		},
		codeGen,
		urlGen,
	))

	// Property: Formatted message contains detail URL when present
	properties.Property("message contains detail URL when present", prop.ForAll(
		func(code string, detailURL string) bool {
			video := &model.Video{
				Code:      code,
				DetailURL: detailURL,
			}
			message := FormatVideoMessage(video)
			return strings.Contains(message, EscapeMarkdown(detailURL))
		},
		codeGen,
		urlGen,
	))

	// Property: Formatted message contains actresses when present
	properties.Property("message contains actresses when present", prop.ForAll(
		func(code string, actresses string) bool {
			video := &model.Video{
				Code:      code,
				Actresses: actresses,
			}
			message := FormatVideoMessage(video)
			return strings.Contains(message, EscapeMarkdown(actresses))
		},
		codeGen,
		nonEmptyStringGen,
	))

	// Property: Formatted message contains tags when present
	properties.Property("message contains tags when present", prop.ForAll(
		func(code string, tags string) bool {
			video := &model.Video{
				Code: code,
				Tags: tags,
			}
			message := FormatVideoMessage(video)
			return strings.Contains(message, EscapeMarkdown(tags))
		},
		codeGen,
		nonEmptyStringGen,
	))

	// Property: Formatted message contains duration when positive
	properties.Property("message contains duration when positive", prop.ForAll(
		func(code string, duration int) bool {
			video := &model.Video{
				Code:     code,
				Duration: duration,
			}
			message := FormatVideoMessage(video)
			// Check that the duration icon appears in the message
			return strings.Contains(message, "⏱")
		},
		codeGen,
		durationGen,
	))

	// Property: Formatted message does not contain actresses section when empty
	properties.Property("message omits actresses when empty", prop.ForAll(
		func(code string) bool {
			video := &model.Video{
				Code:      code,
				Actresses: "",
			}
			message := FormatVideoMessage(video)
			return !strings.Contains(message, "👩")
		},
		codeGen,
	))

	// Property: Formatted message does not contain tags section when empty
	properties.Property("message omits tags when empty", prop.ForAll(
		func(code string) bool {
			video := &model.Video{
				Code: code,
				Tags: "",
			}
			message := FormatVideoMessage(video)
			return !strings.Contains(message, "🏷")
		},
		codeGen,
	))

	// Property: Formatted message does not contain duration section when zero
	properties.Property("message omits duration when zero", prop.ForAll(
		func(code string) bool {
			video := &model.Video{
				Code:     code,
				Duration: 0,
			}
			message := FormatVideoMessage(video)
			return !strings.Contains(message, "⏱")
		},
		codeGen,
	))

	// Property: nil video returns empty string
	properties.Property("nil video returns empty string", prop.ForAll(
		func(_ int) bool {
			message := FormatVideoMessage(nil)
			return message == ""
		},
		gen.Int(),
	))

	properties.TestingRun(t)
}

// TestProperty_EscapeMarkdown tests the Markdown escaping function
func TestProperty_EscapeMarkdown(t *testing.T) {
	parameters := gopter.DefaultTestParameters()
	parameters.MinSuccessfulTests = 100
	properties := gopter.NewProperties(parameters)

	// Property: EscapeMarkdown is idempotent for already escaped strings
	// Note: This is NOT true because escaping twice would double-escape
	// Instead, we test that special characters are escaped

	// Property: All special characters are escaped
	properties.Property("special characters are escaped", prop.ForAll(
		func(text string) bool {
			result := EscapeMarkdown(text)
			// Check that none of the special characters appear unescaped
			specialChars := []string{"_", "*", "[", "]", "(", ")", "~", "`", ">", "#", "+", "-", "=", "|", "{", "}", ".", "!"}
			for _, char := range specialChars {
				// Count occurrences in original
				origCount := strings.Count(text, char)
				// Count escaped occurrences in result
				escapedCount := strings.Count(result, "\\"+char)
				if origCount != escapedCount {
					return false
				}
			}
			return true
		},
		gen.AnyString(),
	))

	properties.TestingRun(t)
}
