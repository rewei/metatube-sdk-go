package kutikomiya

import (
	"encoding/json"
	"fmt"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"

	"golang.org/x/text/language"

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
	baseURL   = "https://kutikomiya.jp"
	actorURL  = "https://kutikomiya.jp/av-idol/%s/"
	imageURL  = "https://img.kutikomiya.jp/thumbnail/%s/W365xH450/%s001.jpg"
	searchURL = "https://kutikomiya.jp/search/av-idol/%s/"
)

var (
	_slugDir string
)

// SetSlugDir sets the directory for the slug mapping file (gfriends_slug.json).
func SetSlugDir(dir string) {
	_slugDir = dir
}

type Kutikomiya struct {
	*scraper.Scraper
	slugMap map[string]string
}

func New() *Kutikomiya {
	k := &Kutikomiya{
		Scraper: scraper.NewDefaultScraper(
			Name, baseURL, Priority,
			language.Japanese,
			scraper.WithDisableCookies(),
		),
		slugMap: make(map[string]string),
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
		return
	}
}

func (k *Kutikomiya) GetActorInfoByID(id string) (*model.ActorInfo, error) {
	slug, ok := k.slugMap[id]
	if !ok || slug == "" {
		// Try reverse lookup: id might be a slug.
		for name, s := range k.slugMap {
			if s == id {
				slug = s
				id = name
				ok = true
				break
			}
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
	// Reverse lookup: find the Japanese name for this slug.
	for name, slug := range k.slugMap {
		if slug == id {
			return name, nil
		}
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

	// Name from title: 早坂咲重（はやさか・さきえ）
	re := regexp.MustCompile(`<h1>([^（]+)`)
	if match := re.FindStringSubmatch(html); len(match) >= 2 {
		info.Name = strings.TrimSpace(match[1])
	}
	if info.Name == "" {
		// Fallback to slug name.
		for name, s := range k.slugMap {
			if s == slug {
				info.Name = name
				break
			}
		}
	}
	// Use the Japanese name as ID (not slug), so Emby can look it up later.
	info.ID = info.Name

	// Birth date: 1995/11/26
	re = regexp.MustCompile(`生年月日：\s*<b>(\d+/\d+/\d+)`)
	if match := re.FindStringSubmatch(html); len(match) >= 2 {
		info.Birthday = parser.ParseDate(match[1])
	}

	// Birthplace: 東京都
	re = regexp.MustCompile(`出身地：\s*<a[^>]*>([^<]+)</a>`)
	if match := re.FindStringSubmatch(html); len(match) >= 2 {
		info.Nationality = strings.TrimSpace(match[1])
	}

	// Blood type: A型 → strip to just "A" (Emby plugin adds "型" automatically)
	re = regexp.MustCompile(`血液型：\s*<a[^>]*>([^<]+)</a>`)
	if match := re.FindStringSubmatch(html); len(match) >= 2 {
		bt := strings.TrimSpace(match[1])
		bt = strings.TrimSuffix(bt, "型")
		if bt != "" {
			info.BloodType = bt
		}
	}

	// Height: 160cm
	re = regexp.MustCompile(`身長：\s*(\d+)cm`)
	if match := re.FindStringSubmatch(html); len(match) >= 2 {
		if h, err := strconv.Atoi(match[1]); err == nil {
			info.Height = h
		}
	}

	// Measurements: B85:W58:H84cm
	re = regexp.MustCompile(`3サイズ:\s*<b>B(\d+):W(\d+):H(\d+)`)
	if match := re.FindStringSubmatch(html); len(match) >= 4 {
		info.Measurements = fmt.Sprintf("B%s / W%s / H%s", match[1], match[2], match[3])
		// Cup size from the same line: (Gカップ)
		re = regexp.MustCompile(`B\d+:W\d+:H\d+cm</b>\s*\(<a[^>]*>([^<]+)カップ`)
		if cm := re.FindStringSubmatch(html); len(cm) >= 2 {
			info.CupSize = cm[1] + "カップ"
		}
	}

	// Hobby
	re = regexp.MustCompile(`趣味：\s*([^<]+)`)
	if match := re.FindStringSubmatch(html); len(match) >= 2 {
		hobby := strings.TrimSpace(match[1])
		if hobby != "-" && hobby != "" {
			info.Hobby = hobby
		}
	}

	// Skill
	re = regexp.MustCompile(`特技：\s*([^<]+)`)
	if match := re.FindStringSubmatch(html); len(match) >= 2 {
		skill := strings.TrimSpace(match[1])
		if skill != "-" && skill != "" {
			info.Skill = skill
		}
	}

	// Aliases
	re = regexp.MustCompile(`別名：<b>([^<]+)`)
	if match := re.FindStringSubmatch(html); len(match) >= 2 {
		aliases := strings.Split(match[1], "、")
		for _, alias := range aliases {
			alias = strings.TrimSpace(alias)
			if alias != "" && alias != info.Name {
				info.Aliases = append(info.Aliases, alias)
			}
		}
	}

	// Image fallback (gfriends is preferred, this is used when gfriends has no image)
	imgURL := fmt.Sprintf(imageURL, slug, slug)
	info.Images = append(info.Images, imgURL)

	return info, nil
}

func (k *Kutikomiya) SearchActor(keyword string) ([]*model.ActorSearchResult, error) {
	// Look up the keyword in the slug mapping.
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
	tmpFile, err := os.CreateTemp("", "openssl-*.cnf")
	if err != nil {
		return nil, err
	}
	tmpPath := tmpFile.Name()
	tmpFile.WriteString("openssl_conf = openssl_init\n[openssl_init]\nssl_conf = ssl_sect\n[ssl_sect]\nsystem_default = system_default_sect\n[system_default_sect]\nMinProtocol = TLSv1.2\nCipherString = DEFAULT:@SECLEVEL=0\n")
	tmpFile.Close()
	defer os.Remove(tmpPath)

	cmd := exec.Command("curl", "-s", "-L",
		"--insecure",
		"--tlsv1.2",
		"--ciphers", "DHE-RSA-AES128-GCM-SHA256",
		"-H", "User-Agent: Mozilla/5.0",
		rawURL)
	cmd.Env = append(os.Environ(), "OPENSSL_CONF="+tmpPath)
	return cmd.Output()
}

func init() {
	provider.Register(Name, New)
}