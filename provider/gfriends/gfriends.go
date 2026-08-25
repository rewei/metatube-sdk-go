package gfriends

import (
	"encoding/json"
	"fmt"
	"net/url"
	"path"
	"slices"
	"time"

	"golang.org/x/text/language"

	"github.com/metatube-community/metatube-sdk-go/collection/maps"
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
		Homepage: "",
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

// ResetCache clears the filetree cache so it will be re-fetched on next query.
func ResetCache() {
	_fileTree.single.Reset()
}

type fileTree struct {
	single *singledo.Single

	Content *maps.OrderedMap[string, *maps.OrderedMap[string, string]] `json:"Content"`
}

func newFileTree(wait time.Duration) *fileTree {
	return &fileTree{
		single:  singledo.NewSingle(wait),
		Content: maps.NewOrderedMap[string, *maps.OrderedMap[string, string]](),
	}
}

func (ft *fileTree) query(s string) (images []string, err error) {
	ft.single.Do(func() (any, error) {
		err = ft.update()
		return nil, nil
	})
	for co, am := range ft.Content.Iterator() {
		for n, p := range am.Iterator() {
			if n[:len(n)-len(path.Ext(n))] == s {
				if u, e := url.Parse(fmt.Sprintf(contentURL, co, p)); e == nil {
					images = append(images, u.String())
				}
			}
		}
	}
	slices.Reverse(images)
	return
}

func (ft *fileTree) update() error {
	resp, err := _fetcher.Fetch(jsonURL)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	return json.NewDecoder(resp.Body).Decode(ft)
}

func init() {
	provider.Register(Name, New)
}