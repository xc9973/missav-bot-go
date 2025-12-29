package crawler

import (
	"regexp"
	"strconv"
	"strings"

	"github.com/PuerkitoBio/goquery"
	"github.com/rs/zerolog/log"
	"github.com/user/missav-bot-go/internal/model"
)

const (
	BaseURL = "https://missav.ai"
)

var (
	// CODE_PATTERN matches video codes like ABC-123, abc-123
	codePattern = regexp.MustCompile(`(?i)([A-Z]+-\d+)`)
	// DURATION_PATTERN matches duration in minutes like "120分" or "120 分"
	durationPattern = regexp.MustCompile(`(\d+)\s*分`)
)

// Parser handles HTML parsing for video data extraction
type Parser struct{}

// NewParser creates a new Parser instance
func NewParser() *Parser {
	return &Parser{}
}

// ParseVideoList parses HTML and extracts a list of videos from a listing page
func (p *Parser) ParseVideoList(html string) ([]*model.Video, error) {
	doc, err := goquery.NewDocumentFromReader(strings.NewReader(html))
	if err != nil {
		return nil, err
	}

	var videos []*model.Video

	// Log page title for debugging
	title := doc.Find("title").Text()
	log.Info().Str("pageTitle", title).Int("htmlLen", len(html)).Msg("Parsing video list")

	// Try to extract from JSON in script tags first (for client-rendered pages)
	jsonVideos := p.extractVideosFromJSON(doc)
	if len(jsonVideos) > 0 {
		log.Info().Int("count", len(jsonVideos)).Msg("Extracted videos from JSON")
		return jsonVideos, nil
	}

	// Fallback to HTML parsing
	// Try multiple selectors for video cards
	selectors := []string{
		"div.video-card",
		"article.video",
		"div[class*=thumbnail]",
		"div.group",
	}

	var videoCards *goquery.Selection
	for _, selector := range selectors {
		cards := doc.Find(selector)
		if cards.Length() > 0 {
			log.Info().Str("selector", selector).Int("count", cards.Length()).Msg("Found video cards")
			videoCards = cards
			break
		}
	}

	// If no cards found, try to find links with video codes
	if videoCards == nil || videoCards.Length() == 0 {
		log.Info().Msg("No video cards found, trying to find links with video codes")
		doc.Find("a[href]").Each(func(i int, s *goquery.Selection) {
			href, exists := s.Attr("href")
			if !exists {
				return
			}
			if codePattern.MatchString(href) {
				video := p.parseVideoFromLink(s)
				if video != nil && video.Code != "" {
					videos = append(videos, video)
				}
			}
		})
		log.Info().Int("count", len(videos)).Msg("Found videos from links")
		return videos, nil
	}

	// Parse each video card
	videoCards.Each(func(i int, card *goquery.Selection) {
		video := p.parseVideoCard(card)
		if video != nil && video.Code != "" {
			videos = append(videos, video)
		}
	})

	log.Info().Int("count", len(videos)).Msg("Parsed videos from cards")
	return videos, nil
}

// ParseVideoDetail parses HTML and extracts detailed video information
func (p *Parser) ParseVideoDetail(html string, detailURL string) (*model.Video, error) {
	doc, err := goquery.NewDocumentFromReader(strings.NewReader(html))
	if err != nil {
		return nil, err
	}

	video := &model.Video{
		DetailURL: detailURL,
	}

	// Extract title
	titleEl := doc.Find("h1, .video-title, [class*=title]").First()
	if titleEl.Length() > 0 {
		video.Title = strings.TrimSpace(titleEl.Text())
	}

	// Extract code from title or URL
	video.Code = ExtractCode(video.Title)
	if video.Code == "" {
		video.Code = ExtractCode(detailURL)
	}

	// Extract actresses
	var actresses []string
	doc.Find("a[href*=actress], a[href*=actor], .actress").Each(func(i int, s *goquery.Selection) {
		text := strings.TrimSpace(s.Text())
		if text != "" {
			actresses = append(actresses, text)
		}
	})
	if len(actresses) > 0 {
		video.Actresses = strings.Join(actresses, ", ")
	}

	// Extract tags
	var tags []string
	doc.Find("a[href*=tag], a[href*=genre], .tag").Each(func(i int, s *goquery.Selection) {
		text := strings.TrimSpace(s.Text())
		if text != "" {
			tags = append(tags, text)
		}
	})
	if len(tags) > 0 {
		video.Tags = strings.Join(tags, ", ")
	}

	// Extract cover image
	coverEl := doc.Find("meta[property='og:image'], img.cover, .video-cover img").First()
	if coverEl.Length() > 0 {
		if content, exists := coverEl.Attr("content"); exists && content != "" {
			video.CoverURL = content
		} else if src, exists := coverEl.Attr("src"); exists && src != "" {
			video.CoverURL = src
		}
	}

	// Extract preview video URL
	videoEl := doc.Find("video").First()
	if videoEl.Length() > 0 {
		previewURL := p.extractVideoURL(videoEl)
		if previewURL != "" {
			video.PreviewURL = previewURL
		}
	}

	// If no preview found, try to extract from script tags
	if video.PreviewURL == "" {
		video.PreviewURL = p.extractPreviewFromScripts(doc)
	}

	// Extract duration
	durationEl := doc.Find(".duration, [class*=duration], span:contains(分钟), span:contains(分)").First()
	if durationEl.Length() > 0 {
		video.Duration = ExtractDuration(durationEl.Text())
	}

	return video, nil
}

// parseVideoCard extracts video information from a card element
func (p *Parser) parseVideoCard(card *goquery.Selection) *model.Video {
	video := &model.Video{}

	// Extract link
	link := card.Find("a[href*=missav]").First()
	if link.Length() == 0 {
		link = card.Find("a").First()
	}
	if link.Length() > 0 {
		if href, exists := link.Attr("href"); exists {
			video.DetailURL = p.normalizeURL(href)
		}
	}

	// Extract title
	titleEl := card.Find("h3, h4, .title, [class*=title]").First()
	if titleEl.Length() > 0 {
		video.Title = strings.TrimSpace(titleEl.Text())
	}

	// Extract code from title or URL
	video.Code = ExtractCode(video.Title)
	if video.Code == "" && video.DetailURL != "" {
		video.Code = ExtractCode(video.DetailURL)
	}
	// Fallback: extract from URL path
	if video.Code == "" && video.DetailURL != "" {
		video.Code = extractCodeFromURL(video.DetailURL)
	}

	// Extract cover image
	img := card.Find("img").First()
	if img.Length() > 0 {
		video.CoverURL = p.extractImageURL(img)
	}

	// Extract duration
	durationEl := card.Find(".duration, [class*=duration], span:contains(分)").First()
	if durationEl.Length() > 0 {
		video.Duration = ExtractDuration(durationEl.Text())
	}

	return video
}

// parseVideoFromLink extracts video information from a link element
func (p *Parser) parseVideoFromLink(link *goquery.Selection) *model.Video {
	video := &model.Video{}

	if href, exists := link.Attr("href"); exists {
		video.DetailURL = p.normalizeURL(href)
		video.Code = ExtractCode(href)
		if video.Code == "" {
			video.Code = extractCodeFromURL(href)
		}
	}

	// Try to find image within the link
	img := link.Find("img").First()
	if img.Length() > 0 {
		video.CoverURL = p.extractImageURL(img)
	}

	return video
}

// extractVideosFromJSON tries to extract video data from JSON in script tags
func (p *Parser) extractVideosFromJSON(doc *goquery.Document) []*model.Video {
	var videos []*model.Video

	// Pattern to match dvd_id in JSON
	dvdIDPattern := regexp.MustCompile(`"dvd_id"\s*:\s*"([^"]+)"`)
	uuidPattern := regexp.MustCompile(`"uuid"\s*:\s*"([^"]+)"`)

	doc.Find("script").Each(func(i int, s *goquery.Selection) {
		scriptContent := s.Text()

		// Check if script contains video data
		if !strings.Contains(scriptContent, "dvd_id") && !strings.Contains(scriptContent, "uuid") {
			return
		}

		// Extract dvd_id values
		matches := dvdIDPattern.FindAllStringSubmatch(scriptContent, -1)
		for _, match := range matches {
			if len(match) > 1 {
				code := NormalizeCode(match[1])
				video := &model.Video{
					Code:      code,
					DetailURL: BaseURL + "/" + strings.ToLower(match[1]),
				}
				videos = append(videos, video)
			}
		}

		// If no dvd_id found, try uuid
		if len(videos) == 0 {
			matches = uuidPattern.FindAllStringSubmatch(scriptContent, -1)
			for _, match := range matches {
				if len(match) > 1 {
					code := NormalizeCode(match[1])
					video := &model.Video{
						Code:      code,
						DetailURL: BaseURL + "/" + match[1],
					}
					videos = append(videos, video)
				}
			}
		}
	})

	return videos
}

// extractImageURL extracts the real image URL from an img element
func (p *Parser) extractImageURL(img *goquery.Selection) string {
	// Try multiple attributes in order of priority
	attrs := []string{"data-original", "data-lazy-src", "data-src", "srcset", "src"}

	for _, attr := range attrs {
		if url, exists := img.Attr(attr); exists && url != "" && !strings.HasPrefix(url, "data:") {
			// Handle srcset - take the first URL
			if attr == "srcset" && strings.Contains(url, " ") {
				parts := strings.Fields(url)
				if len(parts) > 0 {
					url = parts[0]
				}
			}
			// Validate URL
			if strings.HasPrefix(url, "http://") || strings.HasPrefix(url, "https://") || strings.HasPrefix(url, "/") {
				return p.normalizeURL(url)
			}
		}
	}

	return ""
}

// extractVideoURL extracts video URL from a video element
func (p *Parser) extractVideoURL(videoEl *goquery.Selection) string {
	// Try data-src first
	if url, exists := videoEl.Attr("data-src"); exists && url != "" {
		return url
	}
	// Try src
	if url, exists := videoEl.Attr("src"); exists && url != "" {
		return url
	}
	// Try source element
	source := videoEl.Find("source").First()
	if source.Length() > 0 {
		if url, exists := source.Attr("src"); exists && url != "" {
			return url
		}
		if url, exists := source.Attr("data-src"); exists && url != "" {
			return url
		}
	}
	return ""
}

// extractPreviewFromScripts tries to extract preview video URL from script tags
func (p *Parser) extractPreviewFromScripts(doc *goquery.Document) string {
	mp4Pattern := regexp.MustCompile(`(https?://[^\s"']+\.mp4)`)

	var previewURL string
	doc.Find("script").Each(func(i int, s *goquery.Selection) {
		if previewURL != "" {
			return
		}
		scriptContent := s.Text()
		if strings.Contains(scriptContent, ".mp4") || strings.Contains(scriptContent, "preview") {
			matches := mp4Pattern.FindStringSubmatch(scriptContent)
			if len(matches) > 1 {
				previewURL = matches[1]
			}
		}
	})

	return previewURL
}

// normalizeURL converts relative URLs to absolute URLs
func (p *Parser) normalizeURL(url string) string {
	if url == "" {
		return ""
	}
	if strings.HasPrefix(url, "http://") || strings.HasPrefix(url, "https://") {
		return url
	}
	if strings.HasPrefix(url, "/") {
		return BaseURL + url
	}
	return BaseURL + "/" + url
}

// ExtractCode extracts video code from text and normalizes it to uppercase
func ExtractCode(text string) string {
	if text == "" {
		return ""
	}
	matches := codePattern.FindStringSubmatch(text)
	if len(matches) > 1 {
		return NormalizeCode(matches[1])
	}
	return ""
}

// NormalizeCode normalizes a video code to uppercase format
func NormalizeCode(code string) string {
	if code == "" {
		return ""
	}
	return strings.ToUpper(code)
}

// extractCodeFromURL extracts identifier from URL path as fallback
func extractCodeFromURL(url string) string {
	if url == "" {
		return ""
	}

	// Remove query parameters and fragments
	if idx := strings.Index(url, "?"); idx != -1 {
		url = url[:idx]
	}
	if idx := strings.Index(url, "#"); idx != -1 {
		url = url[:idx]
	}

	// Remove trailing slash
	url = strings.TrimSuffix(url, "/")

	// Get the last path segment
	parts := strings.Split(url, "/")
	if len(parts) > 0 {
		lastPart := parts[len(parts)-1]
		if lastPart != "" {
			return NormalizeCode(lastPart)
		}
	}

	return ""
}

// ExtractDuration extracts duration in minutes from text
func ExtractDuration(text string) int {
	if text == "" {
		return 0
	}

	matches := durationPattern.FindStringSubmatch(text)
	if len(matches) > 1 {
		duration, err := strconv.Atoi(matches[1])
		if err == nil {
			return duration
		}
	}

	// Try to parse plain number
	numPattern := regexp.MustCompile(`\d+`)
	numMatch := numPattern.FindString(text)
	if numMatch != "" {
		duration, err := strconv.Atoi(numMatch)
		if err == nil {
			return duration
		}
	}

	return 0
}

// IsValidCode checks if a string is a valid video code format
func IsValidCode(code string) bool {
	if code == "" {
		return false
	}
	// Valid code should be uppercase and match pattern like ABC-123
	validPattern := regexp.MustCompile(`^[A-Z]+-\d+$`)
	return validPattern.MatchString(code)
}
