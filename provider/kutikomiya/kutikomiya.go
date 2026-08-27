package kutikomiya

import (
	"encoding/json"
	"fmt"
	"net/url"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"sync"

	"golang.org/x/text/language"

	"github.com/metatube-community/metatube-sdk-go/common/curlfetch"
	"github.com/metatube-community/metatube-sdk-go/common/parser"
	"github.com/metatube-community/metatube-sdk-go/model"
	"github.com/metatube-community/metatube-sdk-go/provider"
	"github.com/metatube-community/metatube-sdk-go/provider/internal/scraper"
)

var (
	_ provider.ActorProvider = (*Kutikomiya)(nil)
	_ provider.ActorSearcher = (*Kutikomiya)(nil)
)

const (
	Name     = "KUTIKOMIYA"
	Priority = 1000 + 1
)

const (
	baseURL  = "https://kutikomiya.jp"
	actorURL = "https://kutikomiya.jp/av-idol/%s/"
	imageURL = "https://img.kutikomiya.jp/thumbnail/%s/W365xH450/%s001.jpg"
)

// Precompiled regexes
var (
	reName        = regexp.MustCompile(`<h1>([^（]+)`)
	reBirthday   = regexp.MustCompile(`生年月日：\s*<b>(\d+/\d+/\d+)`)
	reBirthplace = regexp.MustCompile(`出身地：\s*<a[^>]*>([^<]+)</a>`)
	reBloodType  = regexp.MustCompile(`血液型：\s*<a[^>]*>([^<]+)</a>`)
	reHeight     = regexp.MustCompile(`身長：\s*(\d+)cm`)
	reMeasure    = regexp.MustCompile(`3サイズ:\s*<b>B(\d+):W(\d+):H(\d+)`)
	reCupSize    = regexp.MustCompile(`B\d+:W\d+:H\d+cm</b>\s*\(<a[^>]*>([^<]+)カップ`)
	reHobby      = regexp.MustCompile(`趣味：\s*([^<]+)`)
	reSkill      = regexp.MustCompile(`特技：\s*([^<]+)`)
	reAlias      = regexp.MustCompile(`別名：<b>([^<]+)`)
	reAltAlias   = regexp.MustCompile(`別名：</span><p>([^<]+)`)
)

var (
	_slugDir string
)

func SetSlugDir(dir string) {
	_slugDir = dir
}

func LookupSlug(name string) string {
	return _slugMap[name]
}

func SaveSlugs(entries map[string]string) error {
	_slugMu.Lock()
	defer _slugMu.Unlock()
	// Always update in-memory maps, even if slugDir is not set.
	for name, slug := range entries {
		_slugMap[name] = slug
		_slugReverse[slug] = name
	}
	if _slugDir == "" {
		return nil
	}
	filePath := filepath.Join(_slugDir, "gfriends_slug.json")
	data, err := os.ReadFile(filePath)
	m := make(map[string]string)
	if err == nil {
		json.Unmarshal(data, &m)
	}
	for name, slug := range entries {
		m[name] = slug
	}
	// Sort by slug first, then by name, so aliases of the same actor are grouped.
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	sort.SliceStable(keys, func(i, j int) bool {
		if m[keys[i]] != m[keys[j]] {
			return m[keys[i]] < m[keys[j]]
		}
		return keys[i] < keys[j]
	})
	var buf strings.Builder
	buf.WriteString("{\n")
	for i, k := range keys {
		if i > 0 {
			buf.WriteString(",\n")
		}
		buf.WriteString(fmt.Sprintf("  %q: %q", k, m[k]))
	}
	buf.WriteString("\n}\n")
	return os.WriteFile(filePath, []byte(buf.String()), 0644)
}

var (
	_slugMap     = make(map[string]string)
	_slugReverse = make(map[string]string)
	_slugMu      sync.Mutex
)

type Kutikomiya struct {
	*scraper.Scraper
}

func New() *Kutikomiya {
	k := &Kutikomiya{
		Scraper: scraper.NewDefaultScraper(
			Name, baseURL, Priority,
			language.Japanese,
			scraper.WithDisableCookies(),
		),
	}
	k.loadSlugFile()
	return k
}

func (k *Kutikomiya) loadSlugFile() {
	dirs := []string{_slugDir, os.Getenv("PROVIDER_CONFIG_DIR"), "."}
	for _, dir := range dirs {
		if dir == "" {
			continue
		}
		filePath := filepath.Join(dir, "gfriends_slug.json")
		data, err := os.ReadFile(filePath)
		if err != nil {
			continue
		}
		m := make(map[string]string)
		if err := json.Unmarshal(data, &m); err != nil {
			continue
		}
		_slugMap = m
		for name, slug := range m {
			_slugReverse[slug] = name
		}
		return
	}
}

func (k *Kutikomiya) GetActorInfoByID(id string) (*model.ActorInfo, error) {
	slug, ok := _slugMap[id]
	if !ok || slug == "" {
		// Try reverse lookup: id might be a slug.
if name, found := _slugReverse[id]; found {
			slug = id
			id = name
			ok = true
		}
	}
	if !ok || slug == "" {
		return nil, provider.ErrInfoNotFound
	}
	info, err := k.GetActorInfoByURL(fmt.Sprintf(actorURL, slug))
	if err != nil {
		return nil, err
	}
	info.ID = id
	return info, nil
}

func (k *Kutikomiya) ParseActorIDFromURL(rawURL string) (string, error) {
	homepage, err := url.Parse(rawURL)
	if err != nil {
		return "", err
	}
	id := strings.TrimPrefix(homepage.Path, "/av-idol/")
	id = strings.TrimSuffix(id, "/")
	if id == "" {
		return "", nil
	}
	if name, found := _slugReverse[id]; found {
		return name, nil
	}
	return id, nil
}

func (k *Kutikomiya) GetActorInfoByURL(rawURL string) (*model.ActorInfo, error) {
	homepage, err := url.Parse(rawURL)
	if err != nil {
		return nil, provider.ErrInvalidURL
	}
	slug := strings.TrimPrefix(homepage.Path, "/av-idol/")
	slug = strings.TrimSuffix(slug, "/")
	if slug == "" {
		return nil, provider.ErrInvalidURL
	}

	body, err := curlFetch(rawURL)
	if err != nil {
		return nil, err
	}
	html := string(body)

	info := &model.ActorInfo{
		Provider: k.Name(),
		Homepage: rawURL,
		Aliases:  []string{},
		Images:   []string{},
	}

	if match := reName.FindStringSubmatch(html); len(match) >= 2 {
		info.Name = strings.TrimSpace(match[1])
	}
	if info.Name == "" {
		if name, found := _slugReverse[slug]; found {
			info.Name = name
		}
	}
	info.ID = info.Name

	if match := reBirthday.FindStringSubmatch(html); len(match) >= 2 {
		info.Birthday = parser.ParseDate(match[1])
	}
	if match := reBirthplace.FindStringSubmatch(html); len(match) >= 2 {
		info.Nationality = strings.TrimSpace(match[1])
	}
	if match := reBloodType.FindStringSubmatch(html); len(match) >= 2 {
		bt := strings.TrimSpace(match[1])
		bt = strings.TrimSuffix(bt, "型")
		if bt != "" {
			info.BloodType = bt
		}
	}
	if match := reHeight.FindStringSubmatch(html); len(match) >= 2 {
		if h, err := strconv.Atoi(match[1]); err == nil {
			info.Height = h
		}
	}
	if match := reMeasure.FindStringSubmatch(html); len(match) >= 4 {
		info.Measurements = fmt.Sprintf("B%s / W%s / H%s", match[1], match[2], match[3])
		if cm := reCupSize.FindStringSubmatch(html); len(cm) >= 2 {
			info.CupSize = cm[1] + "カップ"
		}
	}
	if match := reHobby.FindStringSubmatch(html); len(match) >= 2 {
		hobby := strings.TrimSpace(match[1])
		if hobby != "-" && hobby != "" {
			info.Hobby = hobby
		}
	}
	if match := reSkill.FindStringSubmatch(html); len(match) >= 2 {
		skill := strings.TrimSpace(match[1])
		if skill != "-" && skill != "" {
			info.Skill = skill
		}
	}
	// Try both alias formats: 別名：<b> (old) and 別名：</span><p> (MINNANO style)
	aliasMatches := reAlias.FindAllStringSubmatch(html, -1)
	if len(aliasMatches) == 0 {
		aliasMatches = reAltAlias.FindAllStringSubmatch(html, -1)
	}
	for _, m := range aliasMatches {
		alias := strings.TrimSpace(m[1])
		if alias != "" && alias != info.Name {
			info.Aliases = append(info.Aliases, alias)
		}
	}

	imgURL := fmt.Sprintf(imageURL, slug, slug)
	info.Images = append(info.Images, imgURL)

	return info, nil
}

func (k *Kutikomiya) SearchActor(keyword string) ([]*model.ActorSearchResult, error) {
	slug, ok := _slugMap[keyword]
	if !ok || slug == "" {
		return nil, provider.ErrInfoNotFound
	}
	homepage := fmt.Sprintf(actorURL, slug)
	imgURL := fmt.Sprintf(imageURL, slug, slug)
	return []*model.ActorSearchResult{{
		ID:       keyword,
		Name:     keyword,
		Provider: k.Name(),
		Homepage: homepage,
		Images:   []string{imgURL},
	}}, nil
}

func curlFetch(rawURL string) ([]byte, error) {
	return curlfetch.Fetch(rawURL, "--tlsv1.2", "--ciphers", "DHE-RSA-AES128-GCM-SHA256")
}

func init() {
	provider.Register(Name, New)
}
