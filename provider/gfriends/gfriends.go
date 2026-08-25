package gfriends

import (
	"encoding/json"
	"fmt"
	"net/url"
	"slices"
	"time"

	"golang.org/x/text/language"

	"github.com/metatube-community/metatube-sdk-go/common/fetch"
	"github.com/metatube-community/metatube-sdk-go/common/singledo"
	"github.com/metatube-community/metatube-sdk-go/model"
	"github.com/metatube-community/metatube-sdk-go/provider"
	"github.com/metatube-community/metatube-sdk-go/provider/internal/scraper"
)

var (
	_ provider.ActorProvider = (*Gfriends)(nil)
	_ provider.ActorSearcher = (*Gfriends)(nil)
)

const (
	Name     = "Gfriends"
	Priority = 1000 - 1
)

const (
	baseURL    = "https://raw.githubusercontent.com/rewei/avatars/master"
	contentURL = "https://raw.githubusercontent.com/rewei/avatars/master/Content/%s/%s"
	jsonURL    = "https://raw.githubusercontent.com/rewei/avatars/master/Filetree.json"
)

type Gfriends struct {
	*scraper.Scraper
}

func New() *Gfriends {
	return &Gfriends{scraper.NewDefaultScraper(
		Name, baseURL, Priority,
		language.Japanese,
		scraper.WithDisableCookies(),
	)}
}

func (gf *Gfriends) GetActorInfoByID(id string) (*model.ActorInfo, error) {
	images, err := _fileTree.query(id)
	if len(images) == 0 {
		if err != nil {
			return nil, err
		}
		return nil, provider.ErrInfoNotFound
	}
	return &model.ActorInfo{
		ID:       id,
		Name:     id,
		Provider: gf.Name(),
		Homepage: baseURL,
		Aliases:  []string{},
		Images:   images,
	}, nil
}

func (gf *Gfriends) ParseActorIDFromURL(rawURL string) (string, error) {
	homepage, err := url.Parse(rawURL)
	if err != nil {
		return "", err
	}
	return homepage.Query().Get("id"), nil
}

func (gf *Gfriends) GetActorInfoByURL(u string) (*model.ActorInfo, error) {
	id, err := gf.ParseActorIDFromURL(u)
	if err != nil {
		return nil, err
	}
	return gf.GetActorInfoByID(id)
}

func (gf *Gfriends) SearchActor(keyword string) (results []*model.ActorSearchResult, err error) {
	var info *model.ActorInfo
	if info, err = gf.GetActorInfoByID(keyword); err == nil && info.IsValid() {
		results = []*model.ActorSearchResult{info.ToSearchResult()}
	}
	return
}

var (
	_fileTree = newFileTree(2 * time.Hour)
	_fetcher  = fetch.Default(nil)
)

func ResetCache() {
	_fileTree.single.Reset()
}

type fileTree struct {
	single *singledo.Single

	// index: actor name → image URLs (built on update)
	index map[string][]string
}

func newFileTree(wait time.Duration) *fileTree {
	return &fileTree{
		single: singledo.NewSingle(wait),
		index:  make(map[string][]string),
	}
}

func (ft *fileTree) query(s string) (images []string, err error) {
	// single.Do returns the cached result. If update failed previously,
	// the error is returned and NOT cached (so next call retries).
	v, e, _ := ft.single.Do(func() (any, error) {
		if err := ft.update(); err != nil {
			return nil, err
		}
		return ft.index, nil
	})
	if e != nil {
		return nil, e
	}
	idx, ok := v.(map[string][]string)
	if !ok {
		return nil, nil
	}
	images = idx[s]
	slices.Reverse(images)
	return
}

func (ft *fileTree) update() error {
	resp, err := _fetcher.Fetch(jsonURL)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	var raw struct {
		Content map[string]map[string]string `json:"Content"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&raw); err != nil {
		return err
	}

	// Build index: name (without extension) → image URLs
	idx := make(map[string][]string)
	for category, files := range raw.Content {
		for name, p := range files {
			idx[name] = append(idx[name], fmt.Sprintf(contentURL, category, p))
		}
	}
	// Reverse each list for descending order.
	for k := range idx {
		slices.Reverse(idx[k])
	}
	ft.index = idx
	return nil
}

func init() {
	provider.Register(Name, New)
}
