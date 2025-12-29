package crawler

import (
	"regexp"
	"strings"
	"testing"

	"github.com/leanovate/gopter"
	"github.com/leanovate/gopter/gen"
	"github.com/leanovate/gopter/prop"
)

// Property 2: Video Code Normalization
// *For any* extracted video code string, the result SHALL be uppercase and match the pattern [A-Z]+-\d+.
// **Validates: Requirements 1.4**
func TestProperty_VideoCodeNormalization(t *testing.T) {
	parameters := gopter.DefaultTestParameters()
	parameters.MinSuccessfulTests = 100
	properties := gopter.NewProperties(parameters)

	// Generator for valid video codes (letters-digits format)
	codeGen := gen.RegexMatch(`[a-zA-Z]{2,5}-[0-9]{1,5}`)

	// Property: NormalizeCode always returns uppercase
	properties.Property("NormalizeCode returns uppercase", prop.ForAll(
		func(code string) bool {
			result := NormalizeCode(code)
			return result == strings.ToUpper(result)
		},
		codeGen,
	))

	// Property: ExtractCode from valid code returns uppercase matching pattern
	validCodePattern := regexp.MustCompile(`^[A-Z]+-\d+$`)
	properties.Property("ExtractCode returns uppercase matching pattern", prop.ForAll(
		func(code string) bool {
			result := ExtractCode(code)
			if result == "" {
				// Empty result is acceptable if input doesn't match pattern
				return true
			}
			// Result must be uppercase and match pattern
			return result == strings.ToUpper(result) && validCodePattern.MatchString(result)
		},
		codeGen,
	))

	// Property: ExtractCode from text containing code returns uppercase
	properties.Property("ExtractCode from text returns uppercase", prop.ForAll(
		func(prefix, code, suffix string) bool {
			text := prefix + code + suffix
			result := ExtractCode(text)
			if result == "" {
				return true
			}
			return result == strings.ToUpper(result) && validCodePattern.MatchString(result)
		},
		gen.AlphaString(),
		codeGen,
		gen.AlphaString(),
	))

	// Property: NormalizeCode is idempotent
	properties.Property("NormalizeCode is idempotent", prop.ForAll(
		func(code string) bool {
			once := NormalizeCode(code)
			twice := NormalizeCode(once)
			return once == twice
		},
		codeGen,
	))

	// Property: ExtractCode then NormalizeCode equals ExtractCode
	properties.Property("ExtractCode result is already normalized", prop.ForAll(
		func(code string) bool {
			extracted := ExtractCode(code)
			if extracted == "" {
				return true
			}
			normalized := NormalizeCode(extracted)
			return extracted == normalized
		},
		codeGen,
	))

	properties.TestingRun(t)
}
