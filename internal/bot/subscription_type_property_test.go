package bot

import (
	"strings"
	"testing"

	"github.com/leanovate/gopter"
	"github.com/leanovate/gopter/gen"
	"github.com/leanovate/gopter/prop"
	"github.com/user/missav-bot-go/internal/model"
)

// TestProperty_SubscriptionTypeDetermination tests Property 4: Subscription Type Determination
// For any subscribe command argument:
// - Empty string → ALL type subscription
// - String starting with "#" → TAG type subscription with keyword (without #)
// - Other string → ACTRESS type subscription with keyword
// **Validates: Requirements 3.2, 3.3, 3.4**
func TestProperty_SubscriptionTypeDetermination(t *testing.T) {
	parameters := gopter.DefaultTestParameters()
	parameters.MinSuccessfulTests = 100
	properties := gopter.NewProperties(parameters)

	// Property 4.1: Empty string always results in ALL type
	// Feature: missav-bot-go, Property 4: Subscription Type Determination (empty string case)
	properties.Property("empty string results in ALL type subscription", prop.ForAll(
		func(whitespace string) bool {
			// Test with various whitespace-only strings
			subType, keyword := DetermineSubscriptionType(whitespace)
			return subType == model.SubTypeAll && keyword == ""
		},
		gen.OneConstOf("", " ", "  ", "\t", "\n", " \t\n "),
	))

	// Property 4.2: String starting with "#" results in TAG type
	// Feature: missav-bot-go, Property 4: Subscription Type Determination (tag case)
	properties.Property("string starting with # results in TAG type subscription", prop.ForAll(
		func(tagName string) bool {
			input := "#" + tagName
			subType, keyword := DetermineSubscriptionType(input)
			return subType == model.SubTypeTag && keyword == tagName
		},
		gen.AlphaString().SuchThat(func(s string) bool { return len(s) > 0 }),
	))

	// Property 4.3: Non-empty string not starting with "#" results in ACTRESS type
	// Feature: missav-bot-go, Property 4: Subscription Type Determination (actress case)
	properties.Property("non-empty string not starting with # results in ACTRESS type subscription", prop.ForAll(
		func(actressName string) bool {
			// Ensure the string doesn't start with # and is not empty/whitespace
			if strings.TrimSpace(actressName) == "" || strings.HasPrefix(strings.TrimSpace(actressName), "#") {
				return true // Skip invalid inputs
			}
			subType, keyword := DetermineSubscriptionType(actressName)
			return subType == model.SubTypeActress && keyword == actressName
		},
		gen.AlphaString().SuchThat(func(s string) bool {
			return len(s) > 0 && !strings.HasPrefix(s, "#")
		}),
	))

	// Property 4.4: Keyword extraction is correct for TAG type
	// Feature: missav-bot-go, Property 4: Subscription Type Determination (keyword extraction)
	properties.Property("TAG type keyword does not include the # prefix", prop.ForAll(
		func(tagName string) bool {
			input := "#" + tagName
			_, keyword := DetermineSubscriptionType(input)
			return !strings.HasPrefix(keyword, "#") && keyword == tagName
		},
		gen.AlphaString().SuchThat(func(s string) bool { return len(s) > 0 }),
	))

	// Property 4.5: Unicode actress names are handled correctly
	// Feature: missav-bot-go, Property 4: Subscription Type Determination (unicode support)
	properties.Property("unicode actress names result in ACTRESS type", prop.ForAll(
		func(name string) bool {
			subType, keyword := DetermineSubscriptionType(name)
			return subType == model.SubTypeActress && keyword == name
		},
		gen.OneConstOf("三上悠亜", "橋本ありな", "明日花キララ", "波多野結衣"),
	))

	// Property 4.6: Unicode tag names are handled correctly
	// Feature: missav-bot-go, Property 4: Subscription Type Determination (unicode tag support)
	properties.Property("unicode tag names result in TAG type with correct keyword", prop.ForAll(
		func(tag string) bool {
			input := "#" + tag
			subType, keyword := DetermineSubscriptionType(input)
			return subType == model.SubTypeTag && keyword == tag
		},
		gen.OneConstOf("巨乳", "美乳", "素人", "人妻"),
	))

	properties.TestingRun(t)
}
