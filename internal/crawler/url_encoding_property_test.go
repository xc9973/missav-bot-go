package crawler

import (
	"net/url"
	"testing"
	"unicode/utf8"

	"github.com/leanovate/gopter"
	"github.com/leanovate/gopter/gen"
	"github.com/leanovate/gopter/prop"
)

// Property 3: Actor Name URL Encoding
// *For any* actor name string (including Unicode characters like "三上悠亚"),
// the URL-encoded result SHALL be valid UTF-8 percent-encoding.
// **Validates: Requirements 1.7**
func TestProperty_ActorNameURLEncoding(t *testing.T) {
	parameters := gopter.DefaultTestParameters()
	parameters.MinSuccessfulTests = 100
	properties := gopter.NewProperties(parameters)

	// Generator for various strings including Unicode
	stringGen := gen.AnyString()

	// Generator for Chinese characters (common actor names)
	chineseGen := gen.AnyString().SuchThat(func(s string) bool {
		// Accept strings with at least one character
		return len(s) > 0 && utf8.ValidString(s)
	})

	// Property: URL-encoded string can be decoded back
	properties.Property("URL encoding is reversible", prop.ForAll(
		func(name string) bool {
			encoded := url.PathEscape(name)
			decoded, err := url.PathUnescape(encoded)
			if err != nil {
				return false
			}
			return decoded == name
		},
		stringGen,
	))

	// Property: URL-encoded result contains only valid characters
	properties.Property("URL encoding produces valid characters", prop.ForAll(
		func(name string) bool {
			encoded := url.PathEscape(name)
			// Valid URL characters: unreserved + percent-encoded
			for _, c := range encoded {
				if !isValidURLChar(c) {
					return false
				}
			}
			return true
		},
		stringGen,
	))

	// Property: Chinese names are properly encoded
	properties.Property("Chinese names are properly encoded", prop.ForAll(
		func(name string) bool {
			if !utf8.ValidString(name) {
				return true // Skip invalid UTF-8
			}
			encoded := url.PathEscape(name)
			decoded, err := url.PathUnescape(encoded)
			if err != nil {
				return false
			}
			return decoded == name
		},
		chineseGen,
	))

	// Property: Empty string encodes to empty string
	properties.Property("empty string encodes to empty", prop.ForAll(
		func(_ int) bool {
			encoded := url.PathEscape("")
			return encoded == ""
		},
		gen.Int(),
	))

	// Property: ASCII alphanumeric characters are not encoded
	properties.Property("ASCII alphanumeric not encoded", prop.ForAll(
		func(s string) bool {
			encoded := url.PathEscape(s)
			// For pure alphanumeric, encoded should equal original
			for _, c := range s {
				if !isAlphanumeric(c) {
					return true // Skip non-alphanumeric strings
				}
			}
			return encoded == s
		},
		gen.AlphaString(),
	))

	properties.TestingRun(t)
}

// isValidURLChar checks if a character is valid in a URL-encoded string
func isValidURLChar(c rune) bool {
	// Unreserved characters (RFC 3986)
	if (c >= 'A' && c <= 'Z') || (c >= 'a' && c <= 'z') ||
		(c >= '0' && c <= '9') || c == '-' || c == '_' || c == '.' || c == '~' {
		return true
	}
	// Percent sign (for percent-encoding)
	if c == '%' {
		return true
	}
	// Hex digits (for percent-encoding)
	if (c >= '0' && c <= '9') || (c >= 'A' && c <= 'F') || (c >= 'a' && c <= 'f') {
		return true
	}
	return false
}

// isAlphanumeric checks if a character is alphanumeric
func isAlphanumeric(c rune) bool {
	return (c >= 'A' && c <= 'Z') || (c >= 'a' && c <= 'z') || (c >= '0' && c <= '9')
}
