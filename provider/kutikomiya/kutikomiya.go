package kutikomiya

import (
	"encoding/json"
	"fmt"
	"net/url"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"

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
	reAlias      = regexp.MustCompile(`別名：</span><p>([^<]+)`)
)

var (
	_slugDir string
)

func SetSlugDir(dir string) {
	_slugDir = dir
}

type Kutikomiya struct {
	*scraper.Scraper
	slugMap     map[string]string
	slugReverse map[string]string // prebuilt reverse: slug -> name
}

func New() *Kutikomiya {
	k := &Kutikomiya{
		Scraper: scraper.NewDefaultScraper(
			Name, baseURL, Priority,
			language.Japanese,
			scraper.WithDisableCookies(),
		),
		slugMap:     make(map[string]string),
		slugReverse: make(map[string]string),
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
		k.slugMap = m
		// Build reverse lookup
		for name, slug := range m {
			k.slugReverse[slug] = name
		}
		return
	}
}

func (k *Kutikomiya) GetActorInfoByID(id string) (*model.ActorInfo, error) {
	slug, ok := k.slugMap[id]
	if !ok || slug == "" {
		// Try reverse lookup: id might be a slug.
		if name, found := k.slugReverse[id]; found {
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
	if name, found := k.slugReverse[id]; found {
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
		if name, found := k.slugReverse[slug]; found {
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
	for _, m := range reAlias.FindAllStringSubmatch(html, -1) {
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
	slug, ok := k.slugMap[keyword]
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
