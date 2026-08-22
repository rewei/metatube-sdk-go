package minnano

import (
	"encoding/json"
	"fmt"
	"net/url"
	"regexp"
	"sort"
	"strconv"
	"strings"

	"github.com/gocolly/colly/v2"
	"golang.org/x/text/language"

	"github.com/metatube-community/metatube-sdk-go/common/parser"
	"github.com/metatube-community/metatube-sdk-go/model"
	"github.com/metatube-community/metatube-sdk-go/provider"
	"github.com/metatube-community/metatube-sdk-go/provider/internal/scraper"
)

var (
	_ provider.ActorProvider = (*Minnano)(nil)
	_ provider.ActorSearcher = (*Minnano)(nil)
)

const (
	Name     = "MINNANO"
	Priority = 500
)

const (
	baseURL   = "https://www.minnano-av.com/"
	actorURL  = "https://www.minnano-av.com/actress%s.html"
	searchURL = "https://www.minnano-av.com/search_result.php?search_scope=actress&search_word=%s&search=Go"
)

type Minnano struct {
	*scraper.Scraper
}

func New() *Minnano {
	return &Minnano{scraper.NewDefaultScraper(
		Name, baseURL, Priority,
		language.Japanese,
		scraper.WithDisableCookies(),
	)}
}

func (m *Minnano) GetActorInfoByID(id string) (info *model.ActorInfo, err error) {
	return m.GetActorInfoByURL(fmt.Sprintf(actorURL, id))
}

func (m *Minnano) ParseActorIDFromURL(rawURL string) (id string, err error) {
	homepage, err := url.Parse(rawURL)
	if err != nil {
		return
	}
	re := regexp.MustCompile(`actress(\d+)\.html`)
	matches := re.FindStringSubmatch(homepage.Path)
	if len(matches) < 2 {
		// Try matching on the full URL string (handles cases with query params).
		matches = re.FindStringSubmatch(rawURL)
	}
	if len(matches) >= 2 {
		id = matches[1]
	}
	return
}

func (m *Minnano) GetActorInfoByURL(rawURL string) (info *model.ActorInfo, err error) {
	id, err := m.ParseActorIDFromURL(rawURL)
	if err != nil || id == "" {
		return nil, provider.ErrInvalidURL
	}

	info = &model.ActorInfo{
		ID:       id,
		Provider: m.Name(),
		Homepage: rawURL,
		Aliases:  []string{},
		Images:   []string{},
	}

	c := m.ClonedCollector()

	// Parse JSON-LD structured data.
	c.OnXML(`//script[@type="application/ld+json"]`, func(e *colly.XMLElement) {
		var ld struct {
			Name          string `json:"name"`
			AlternateName string `json:"alternateName"`
			AdditionalName string `json:"additionalName"`
			Image         string `json:"image"`
			BirthDate     string `json:"birthDate"`
			BirthPlace    struct {
				Name string `json:"name"`
			} `json:"birthPlace"`
			Affiliation struct {
				Name string `json:"name"`
			} `json:"affiliation"`
			Description string `json:"description"`
		}
		if err := json.Unmarshal([]byte(e.Text), &ld); err != nil {
			return
		}
		info.Name = ld.Name
		if ld.AlternateName != "" {
			info.Aliases = append(info.Aliases, ld.AlternateName)
		}
		if ld.AdditionalName != "" {
			info.Aliases = append(info.Aliases, ld.AdditionalName)
		}
		if ld.Image != "" {
			info.Images = append(info.Images, ld.Image)
		}
		if ld.BirthDate != "" {
			info.Birthday = parser.ParseDate(ld.BirthDate)
		}
		if ld.BirthPlace.Name != "" {
			info.Nationality = ld.BirthPlace.Name
		}
		if ld.Description != "" {
			info.Summary = ld.Description
		}
	})

	// Parse size/measurements: T167 / B85(Eカップ) / W60 / H87 / S
	c.OnXML(`//td[span="サイズ"]/p`, func(e *colly.XMLElement) {
		text := strings.TrimSpace(e.Text)
		re := regexp.MustCompile(`T(\d+)`)
		if match := re.FindStringSubmatch(text); len(match) >= 2 {
			if h, err := strconv.Atoi(match[1]); err == nil {
				info.Height = h
			}
		}
		re = regexp.MustCompile(`B(\d+)\(([^)]+)\)`)
		if match := re.FindStringSubmatch(text); len(match) >= 3 {
			info.CupSize = match[2]
			info.Measurements = fmt.Sprintf("B%s", match[1])
		}
		re = regexp.MustCompile(`W(\d+)`)
		if match := re.FindStringSubmatch(text); len(match) >= 2 {
			if info.Measurements != "" {
				info.Measurements += fmt.Sprintf(" / W%s", match[1])
			}
		}
		re = regexp.MustCompile(`H(\d+)`)
		if match := re.FindStringSubmatch(text); len(match) >= 2 {
			if info.Measurements != "" {
				info.Measurements += fmt.Sprintf(" / H%s", match[1])
			}
		}
	})

	// Parse blood type.
	c.OnXML(`//td[span="血液型"]/p`, func(e *colly.XMLElement) {
		info.BloodType = strings.TrimSpace(e.Text)
	})

	// Parse birthplace.
	c.OnXML(`//td[span="出身地"]/p`, func(e *colly.XMLElement) {
		text := strings.TrimSpace(e.Text)
		if text != "" {
			info.Nationality = text
		}
	})

	err = c.Visit(rawURL)
	return
}

func (m *Minnano) SearchActor(keyword string) (results []*model.ActorSearchResult, err error) {
	searchURL := fmt.Sprintf(searchURL, url.QueryEscape(keyword))

	c := m.ClonedCollector()

	// Check if redirected to an actress page (single result).
	c.OnResponse(func(r *colly.Response) {
		if r.StatusCode == 200 {
			finalURL := r.Request.URL.String()
			id, parseErr := m.ParseActorIDFromURL(finalURL)
			if parseErr == nil && id != "" {
				// Try to extract image from the actress page JSON-LD.
				var images []string
				re := regexp.MustCompile(`"image"\s*:\s*"([^"]+)"`)
				if match := re.FindSubmatch(r.Body); len(match) >= 2 {
					imgURL := string(match[1])
					if !strings.HasPrefix(imgURL, "http") {
						if u, err := url.Parse(finalURL); err == nil {
							base, _ := url.Parse(u.Scheme + "://" + u.Host)
							imgURL = base.ResolveReference(&url.URL{Path: imgURL}).String()
						}
					}
					images = append(images, imgURL)
				}
				results = []*model.ActorSearchResult{{
					ID:       id,
					Name:     keyword,
					Provider: m.Name(),
					Homepage: finalURL,
					Images:   images,
				}}
			}
		}
	})

	// Parse search results page (multiple results).
	var searchResults []*model.ActorSearchResult
	c.OnXML(`//a[contains(@href, "actress")]`, func(e *colly.XMLElement) {
		href := e.Attr("href")
		// Skip non-actress links (e.g. ranking, list, detail).
		if !strings.HasPrefix(href, "actress") || strings.HasPrefix(href, "actress_list") {
			return
		}
		// Skip detail links.
		cls := e.Attr("class")
		if cls != "" && strings.Contains(cls, "detail") {
			return
		}
		// Get name from img alt, then title, then text content.
		title := e.ChildAttr("img", "alt")
		if title == "" {
			title = e.Attr("title")
		}
		if title == "" {
			title = strings.TrimSpace(e.Text)
		}
		// Skip non-name links (e.g., "AV女優", "AV作品を見る", empty).
		if title == "" || title == "AV女優" || title == "AV作品を見る" {
			return
		}
		// Get image from child img src.
		imgSrc := e.ChildAttr("img", "src")
		id, parseErr := m.ParseActorIDFromURL(href)
		if parseErr != nil || id == "" {
			return
		}
		absURL := e.Request.AbsoluteURL(href)
		var images []string
		if imgSrc != "" {
			images = append(images, e.Request.AbsoluteURL(imgSrc))
		}
		searchResults = append(searchResults, &model.ActorSearchResult{
			ID:       id,
			Name:     title,
			Provider: m.Name(),
			Homepage: absURL,
			Images:   images,
		})
	})

	err = c.Visit(searchURL)
	if err != nil {
		return
	}

	// If we got a redirect result, use it directly.
	if len(results) > 0 {
		return
	}

	// Sort search results: exact matches first, then deduplicate.
	results = searchResults
	sort.SliceStable(results, func(i, j int) bool {
		// Exact matches come before partial matches.
		if results[i].Name == keyword && results[j].Name != keyword {
			return true
		}
		return false
	})
	seen := make(map[string]bool)
	filtered := make([]*model.ActorSearchResult, 0, len(results))
	for _, r := range results {
		if seen[r.ID] {
			continue
		}
		seen[r.ID] = true
		filtered = append(filtered, r)
	}
	results = filtered
	return
}

func init() {
	provider.Register(Name, New)
}