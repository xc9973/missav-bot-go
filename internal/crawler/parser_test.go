package crawler

import (
	"testing"
)

func TestExtractCode(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		expected string
	}{
		{"uppercase code", "ABC-123", "ABC-123"},
		{"lowercase code", "abc-123", "ABC-123"},
		{"mixed case code", "AbC-456", "ABC-456"},
		{"code in text", "Video ABC-123 is great", "ABC-123"},
		{"code in URL", "https://missav.ai/abc-789", "ABC-789"},
		{"empty string", "", ""},
		{"no code", "no code here", ""},
		{"multiple codes", "ABC-123 and DEF-456", "ABC-123"}, // returns first match
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := ExtractCode(tt.input)
			if result != tt.expected {
				t.Errorf("ExtractCode(%q) = %q, want %q", tt.input, result, tt.expected)
			}
		})
	}
}

func TestNormalizeCode(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		expected string
	}{
		{"uppercase", "ABC-123", "ABC-123"},
		{"lowercase", "abc-123", "ABC-123"},
		{"mixed case", "AbC-456", "ABC-456"},
		{"empty string", "", ""},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := NormalizeCode(tt.input)
			if result != tt.expected {
				t.Errorf("NormalizeCode(%q) = %q, want %q", tt.input, result, tt.expected)
			}
		})
	}
}

func TestIsValidCode(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		expected bool
	}{
		{"valid uppercase", "ABC-123", true},
		{"valid long code", "ABCD-12345", true},
		{"lowercase invalid", "abc-123", false},
		{"mixed case invalid", "AbC-123", false},
		{"no hyphen", "ABC123", false},
		{"empty string", "", false},
		{"only letters", "ABC-", false},
		{"only numbers", "-123", false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := IsValidCode(tt.input)
			if result != tt.expected {
				t.Errorf("IsValidCode(%q) = %v, want %v", tt.input, result, tt.expected)
			}
		})
	}
}

func TestExtractDuration(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		expected int
	}{
		{"minutes with 分", "120分", 120},
		{"minutes with space", "90 分", 90},
		{"minutes with 分钟", "60分钟", 60},
		{"plain number", "45", 45},
		{"empty string", "", 0},
		{"no number", "no duration", 0},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := ExtractDuration(tt.input)
			if result != tt.expected {
				t.Errorf("ExtractDuration(%q) = %d, want %d", tt.input, result, tt.expected)
			}
		})
	}
}

func TestExtractCodeFromURL(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		expected string
	}{
		{"simple URL", "https://missav.ai/abc-123", "ABC-123"},
		{"URL with query", "https://missav.ai/abc-123?page=1", "ABC-123"},
		{"URL with fragment", "https://missav.ai/abc-123#section", "ABC-123"},
		{"URL with trailing slash", "https://missav.ai/abc-123/", "ABC-123"},
		{"empty string", "", ""},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := extractCodeFromURL(tt.input)
			if result != tt.expected {
				t.Errorf("extractCodeFromURL(%q) = %q, want %q", tt.input, result, tt.expected)
			}
		})
	}
}

func TestParseVideoList(t *testing.T) {
	parser := NewParser()

	// Test with simple HTML containing video cards
	html := `
	<html>
	<body>
		<div class="video-card">
			<a href="/abc-123">
				<img src="https://example.com/cover1.jpg">
				<h3>ABC-123 Video Title</h3>
			</a>
			<span class="duration">120分</span>
		</div>
		<div class="video-card">
			<a href="/def-456">
				<img src="https://example.com/cover2.jpg">
				<h3>DEF-456 Another Video</h3>
			</a>
			<span class="duration">90分</span>
		</div>
	</body>
	</html>
	`

	videos, err := parser.ParseVideoList(html)
	if err != nil {
		t.Fatalf("ParseVideoList failed: %v", err)
	}

	if len(videos) != 2 {
		t.Errorf("Expected 2 videos, got %d", len(videos))
	}

	if len(videos) > 0 {
		if videos[0].Code != "ABC-123" {
			t.Errorf("Expected first video code ABC-123, got %s", videos[0].Code)
		}
	}
}

func TestParseVideoDetail(t *testing.T) {
	parser := NewParser()

	html := `
	<html>
	<head>
		<meta property="og:image" content="https://example.com/cover.jpg">
	</head>
	<body>
		<h1>ABC-123 Amazing Video Title</h1>
		<a href="/actress/actress1">Actress One</a>
		<a href="/actress/actress2">Actress Two</a>
		<a href="/tag/tag1">Tag One</a>
		<a href="/genre/genre1">Genre One</a>
		<span class="duration">120分钟</span>
		<video data-src="https://example.com/preview.mp4"></video>
	</body>
	</html>
	`

	video, err := parser.ParseVideoDetail(html, "https://missav.ai/abc-123")
	if err != nil {
		t.Fatalf("ParseVideoDetail failed: %v", err)
	}

	if video.Code != "ABC-123" {
		t.Errorf("Expected code ABC-123, got %s", video.Code)
	}

	if video.Title != "ABC-123 Amazing Video Title" {
		t.Errorf("Expected title 'ABC-123 Amazing Video Title', got %s", video.Title)
	}

	if video.CoverURL != "https://example.com/cover.jpg" {
		t.Errorf("Expected cover URL, got %s", video.CoverURL)
	}

	if video.Duration != 120 {
		t.Errorf("Expected duration 120, got %d", video.Duration)
	}

	if video.PreviewURL != "https://example.com/preview.mp4" {
		t.Errorf("Expected preview URL, got %s", video.PreviewURL)
	}
}

func TestParseVideoListWithJSON(t *testing.T) {
	parser := NewParser()

	// Test with JSON data in script tag
	html := `
	<html>
	<body>
		<script>
			window.videos = [
				{"dvd_id": "abc-123", "title": "Video 1"},
				{"dvd_id": "def-456", "title": "Video 2"}
			];
		</script>
	</body>
	</html>
	`

	videos, err := parser.ParseVideoList(html)
	if err != nil {
		t.Fatalf("ParseVideoList failed: %v", err)
	}

	if len(videos) != 2 {
		t.Errorf("Expected 2 videos from JSON, got %d", len(videos))
	}

	if len(videos) > 0 && videos[0].Code != "ABC-123" {
		t.Errorf("Expected first video code ABC-123, got %s", videos[0].Code)
	}
}
