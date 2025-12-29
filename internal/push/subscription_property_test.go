package push

import (
	"strings"
	"testing"

	"github.com/leanovate/gopter"
	"github.com/leanovate/gopter/gen"
	"github.com/leanovate/gopter/prop"
	"github.com/user/missav-bot-go/internal/model"
)

// Property 9: Subscription Matching Logic
// *For any* video and subscription:
// - ALL type subscription → always matches
// - ACTRESS type subscription → matches if video.actresses contains subscription.keyword (case-insensitive)
// - TAG type subscription → matches if video.tags contains subscription.keyword (case-insensitive)
// **Validates: Requirements 5.1, 5.2**
func TestProperty_SubscriptionMatchingLogic(t *testing.T) {
	parameters := gopter.DefaultTestParameters()
	parameters.MinSuccessfulTests = 100
	properties := gopter.NewProperties(parameters)

	// Generator for video code
	codeGen := gen.RegexMatch(`[A-Z]{2,5}-[0-9]{3,5}`)

	// Generator for non-empty strings
	nonEmptyStringGen := gen.AlphaString().SuchThat(func(s string) bool {
		return len(s) > 0
	})

	// Generator for chat ID
	chatIDGen := gen.Int64Range(1, 1000000)

	// Property: ALL type subscription always matches any video
	properties.Property("ALL type subscription always matches", prop.ForAll(
		func(code string, actresses string, tags string, chatID int64) bool {
			video := &model.Video{
				Code:      code,
				Actresses: actresses,
				Tags:      tags,
			}
			sub := &model.Subscription{
				ChatID: chatID,
				Type:   model.SubTypeAll,
			}
			return MatchesSubscription(video, sub)
		},
		codeGen,
		gen.AnyString(),
		gen.AnyString(),
		chatIDGen,
	))

	// Property: ACTRESS type subscription matches when actresses contains keyword (case-insensitive)
	properties.Property("ACTRESS type matches when actresses contains keyword", prop.ForAll(
		func(code string, keyword string, prefix string, suffix string, chatID int64) bool {
			// Build actresses string that contains the keyword
			actresses := prefix + keyword + suffix
			video := &model.Video{
				Code:      code,
				Actresses: actresses,
			}
			sub := &model.Subscription{
				ChatID:  chatID,
				Type:    model.SubTypeActress,
				Keyword: keyword,
			}
			return MatchesSubscription(video, sub)
		},
		codeGen,
		nonEmptyStringGen,
		gen.AlphaString(),
		gen.AlphaString(),
		chatIDGen,
	))

	// Property: ACTRESS type subscription matches case-insensitively
	properties.Property("ACTRESS type matches case-insensitively", prop.ForAll(
		func(code string, keyword string, chatID int64) bool {
			// Use uppercase keyword in video, lowercase in subscription
			video := &model.Video{
				Code:      code,
				Actresses: strings.ToUpper(keyword),
			}
			sub := &model.Subscription{
				ChatID:  chatID,
				Type:    model.SubTypeActress,
				Keyword: strings.ToLower(keyword),
			}
			return MatchesSubscription(video, sub)
		},
		codeGen,
		nonEmptyStringGen,
		chatIDGen,
	))

	// Property: ACTRESS type subscription does not match when keyword not in actresses
	properties.Property("ACTRESS type does not match when keyword absent", prop.ForAll(
		func(code string, actresses string, keyword string, chatID int64) bool {
			// Ensure keyword is not in actresses
			if strings.Contains(strings.ToLower(actresses), strings.ToLower(keyword)) {
				return true // Skip this case
			}
			video := &model.Video{
				Code:      code,
				Actresses: actresses,
			}
			sub := &model.Subscription{
				ChatID:  chatID,
				Type:    model.SubTypeActress,
				Keyword: keyword,
			}
			return !MatchesSubscription(video, sub)
		},
		codeGen,
		gen.AlphaString(),
		nonEmptyStringGen,
		chatIDGen,
	))

	// Property: TAG type subscription matches when tags contains keyword (case-insensitive)
	properties.Property("TAG type matches when tags contains keyword", prop.ForAll(
		func(code string, keyword string, prefix string, suffix string, chatID int64) bool {
			// Build tags string that contains the keyword
			tags := prefix + keyword + suffix
			video := &model.Video{
				Code: code,
				Tags: tags,
			}
			sub := &model.Subscription{
				ChatID:  chatID,
				Type:    model.SubTypeTag,
				Keyword: keyword,
			}
			return MatchesSubscription(video, sub)
		},
		codeGen,
		nonEmptyStringGen,
		gen.AlphaString(),
		gen.AlphaString(),
		chatIDGen,
	))

	// Property: TAG type subscription matches case-insensitively
	properties.Property("TAG type matches case-insensitively", prop.ForAll(
		func(code string, keyword string, chatID int64) bool {
			// Use uppercase keyword in video, lowercase in subscription
			video := &model.Video{
				Code: code,
				Tags: strings.ToUpper(keyword),
			}
			sub := &model.Subscription{
				ChatID:  chatID,
				Type:    model.SubTypeTag,
				Keyword: strings.ToLower(keyword),
			}
			return MatchesSubscription(video, sub)
		},
		codeGen,
		nonEmptyStringGen,
		chatIDGen,
	))

	// Property: TAG type subscription does not match when keyword not in tags
	properties.Property("TAG type does not match when keyword absent", prop.ForAll(
		func(code string, tags string, keyword string, chatID int64) bool {
			// Ensure keyword is not in tags
			if strings.Contains(strings.ToLower(tags), strings.ToLower(keyword)) {
				return true // Skip this case
			}
			video := &model.Video{
				Code: code,
				Tags: tags,
			}
			sub := &model.Subscription{
				ChatID:  chatID,
				Type:    model.SubTypeTag,
				Keyword: keyword,
			}
			return !MatchesSubscription(video, sub)
		},
		codeGen,
		gen.AlphaString(),
		nonEmptyStringGen,
		chatIDGen,
	))

	// Property: nil video never matches
	properties.Property("nil video never matches", prop.ForAll(
		func(chatID int64) bool {
			sub := &model.Subscription{
				ChatID: chatID,
				Type:   model.SubTypeAll,
			}
			return !MatchesSubscription(nil, sub)
		},
		chatIDGen,
	))

	// Property: nil subscription never matches
	properties.Property("nil subscription never matches", prop.ForAll(
		func(code string) bool {
			video := &model.Video{
				Code: code,
			}
			return !MatchesSubscription(video, nil)
		},
		codeGen,
	))

	properties.TestingRun(t)
}
