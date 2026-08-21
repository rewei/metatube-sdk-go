package configurable

import (
	"io"
	"net/url"
	"path"
	"regexp"
	"strings"

	"github.com/antchfx/htmlquery"
	"golang.org/x/net/html"

	"github.com/metatube-community/metatube-sdk-go/common/fetch"
	"github.com/metatube-community/metatube-sdk-go/common/parser"
	"github.com/metatube-community/metatube-sdk-go/model"
	"github.com/metatube-community/metatube-sdk-go/provider"
	"github.com/metatube-community/metatube-sdk-go/provider/internal/scraper"
)

var (
	_ provider.MovieProvider = (*movieProvider)(nil)
	_ provider.ActorProvider = (*actorProvider)(nil)
)

type movieProvider struct {
	*scraper.Scraper
	cfg     *ProviderConfig
	movie   *MovieConfig
	idRegex *regexp.Regexp
}

func NewMovieProvider(cfg *ProviderConfig) *movieProvider {
	lang := cfg.ParseLanguage()
	s := scraper.NewDefaultScraper(cfg.Name, cfg.BaseURL, cfg.Priority, lang)
	p := &movieProvider{
		Scraper: s,
		cfg:     cfg,
		movie:   cfg.Movie,
	}
	if cfg.Movie.IDRegex != "" {
		p.idRegex = regexp.MustCompile(cfg.Movie.IDRegex)
	}
	return p
}

func (p *movieProvider) NormalizeMovieID(id string) string {
	return id
}

func (p *movieProvider) ParseMovieIDFromURL(rawURL string) (string, error) {
	u, err := url.Parse(rawURL)
	if err != nil {
		return "", err
	}
	base := path.Base(u.Path)
	if p.idRegex != nil {
		m := p.idRegex.FindStringSubmatch(base)
		if len(m) > 1 {
			return m[1], nil
		}
	}
	return base, nil
}

func (p *movieProvider) GetMovieInfoByID(id string) (*model.MovieInfo, error) {
	movieURL := strings.ReplaceAll(p.movie.URL, "{id}", id)
	return p.GetMovieInfoByURL(movieURL)
}

func (p *movieProvider) GetMovieInfoByURL(rawURL string) (*model.MovieInfo, error) {
	id, err := p.ParseMovieIDFromURL(rawURL)
	if err != nil {
		return nil, err
	}

	info := &model.MovieInfo{
		ID:            id,
		Number:        id,
		Provider:      p.Name(),
		Homepage:      rawURL,
		Actors:        []string{},
		PreviewImages: []string{},
		Genres:        []string{},
	}

	doc, err := p.fetchHTML(rawURL)
	if err != nil {
		return nil, err
	}

	p.extractMovieFields(doc, info)
	return info, nil
}

func (p *movieProvider) extractMovieFields(doc *html.Node, info *model.MovieInfo) {
	f := &p.movie.Fields

	if v := evalXPathText(doc, f.Title); v != "" {
		info.Title = v
	}
	if v := evalXPathText(doc, f.Number); v != "" {
		info.Number = v
	}
	if v := evalXPathAttr(doc, f.Cover, "src"); v != "" {
		info.CoverURL = absURL(p.cfg.BaseURL, v)
	}
	if v := evalXPathAttr(doc, f.BigCover, "src", "href"); v != "" {
		info.BigCoverURL = absURL(p.cfg.BaseURL, v)
	}
	if v := evalXPathAttr(doc, f.Thumb, "src"); v != "" {
		info.ThumbURL = absURL(p.cfg.BaseURL, v)
	}
	if v := evalXPathAttr(doc, f.BigThumb, "src", "href"); v != "" {
		info.BigThumbURL = absURL(p.cfg.BaseURL, v)
	}
	if v := evalXPathAttr(doc, f.PreviewVideo, "src"); v != "" {
		info.PreviewVideoURL = absURL(p.cfg.BaseURL, v)
	}
	if v := evalXPathAttr(doc, f.PreviewVideoHLS, "src"); v != "" {
		info.PreviewVideoHLSURL = absURL(p.cfg.BaseURL, v)
	}
	if v := evalXPathMultiAttr(doc, f.PreviewImages, "src"); len(v) > 0 {
		info.PreviewImages = makeURLAbs(p.cfg.BaseURL, v)
	}
	if v := evalXPathText(doc, f.Summary); v != "" {
		info.Summary = v
	}
	if v := evalXPathText(doc, f.Director); v != "" {
		info.Director = v
	}
	if v := evalXPathMultiText(doc, f.Actors); len(v) > 0 {
		info.Actors = v
	}
	if v := evalXPathText(doc, f.Maker); v != "" {
		info.Maker = v
	}
	if v := evalXPathText(doc, f.Label); v != "" {
		info.Label = v
	}
	if v := evalXPathText(doc, f.Series); v != "" {
		info.Series = v
	}
	if v := evalXPathMultiText(doc, f.Genres); len(v) > 0 {
		info.Genres = v
	}
	if v := evalXPathText(doc, f.ReleaseDate); v != "" {
		info.ReleaseDate = parser.ParseDate(v)
	}
	if v := evalXPathText(doc, f.Runtime); v != "" {
		info.Runtime = parser.ParseRuntime(v)
	}
	if v := evalXPathText(doc, f.Score); v != "" {
		info.Score = parser.ParseScore(v)
	}
}

func (p *movieProvider) SearchMovie(keyword string) ([]*model.MovieSearchResult, error) {
	if p.movie.Search == nil {
		return nil, provider.ErrInfoNotFound
	}
	s := p.movie.Search
	searchURL := strings.ReplaceAll(s.URL, "{keyword}", url.QueryEscape(keyword))

	doc, err := p.fetchHTML(searchURL)
	if err != nil {
		return nil, err
	}

	nodes := htmlquery.Find(doc, s.Results)
	if len(nodes) == 0 {
		return nil, provider.ErrInfoNotFound
	}

	var results []*model.MovieSearchResult
	for _, node := range nodes {
		result := &model.MovieSearchResult{
			Provider: p.Name(),
		}
		p.extractSearchFields(node, result)
		if result.IsValid() {
			results = append(results, result)
		}
	}
	if len(results) == 0 {
		return nil, provider.ErrInfoNotFound
	}
	return results, nil
}

func (p *movieProvider) extractSearchFields(node *html.Node, result *model.MovieSearchResult) {
	f := &p.movie.Search.Fields

	if v := evalXPathText(node, f.ID); v != "" {
		result.ID = v
	}
	if v := evalXPathText(node, f.Number); v != "" {
		result.Number = v
	}
	if v := evalXPathText(node, f.Title); v != "" {
		result.Title = v
	}
	if v := evalXPathAttr(node, f.Cover, "src"); v != "" {
		result.CoverURL = absURL(p.cfg.BaseURL, v)
	}
	if v := evalXPathAttr(node, f.Thumb, "src"); v != "" {
		result.ThumbURL = absURL(p.cfg.BaseURL, v)
	}
	if v := evalXPathMultiText(node, f.Actors); len(v) > 0 {
		result.Actors = v
	}
	if v := evalXPathText(node, f.ReleaseDate); v != "" {
		result.ReleaseDate = parser.ParseDate(v)
	}
	if v := evalXPathText(node, f.Score); v != "" {
		result.Score = parser.ParseScore(v)
	}
	if v := evalXPathAttr(node, f.Homepage, "href"); v != "" {
		result.Homepage = absURL(p.cfg.BaseURL, v)
	}
	if result.Homepage == "" {
		result.Homepage = p.cfg.BaseURL
	}
}

func (p *movieProvider) NormalizeMovieKeyword(keyword string) string {
	return keyword
}

func (p *movieProvider) fetchHTML(rawURL string) (*html.Node, error) {
	resp, err := fetch.Get(rawURL,
		fetch.WithRandomUserAgent(),
		fetch.WithRaiseForStatus(true),
	)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	data, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}
	doc, err := htmlquery.Parse(strings.NewReader(string(data)))
	if err != nil {
		return nil, err
	}
	return doc, nil
}

func (p *actorProvider) fetchHTML(rawURL string) (*html.Node, error) {
	resp, err := fetch.Get(rawURL,
		fetch.WithRandomUserAgent(),
		fetch.WithRaiseForStatus(true),
	)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	data, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}
	doc, err := htmlquery.Parse(strings.NewReader(string(data)))
	if err != nil {
		return nil, err
	}
	return doc, nil
}

// --- Actor Provider ---

type actorProvider struct {
	*scraper.Scraper
	cfg     *ProviderConfig
	actor   *ActorConfig
	idRegex *regexp.Regexp
}

func NewActorProvider(cfg *ProviderConfig) *actorProvider {
	lang := cfg.ParseLanguage()
	s := scraper.NewDefaultScraper(cfg.Name, cfg.BaseURL, cfg.Priority, lang)
	p := &actorProvider{
		Scraper: s,
		cfg:     cfg,
		actor:   cfg.Actor,
	}
	if cfg.Actor.IDRegex != "" {
		p.idRegex = regexp.MustCompile(cfg.Actor.IDRegex)
	}
	return p
}

func (p *actorProvider) NormalizeActorID(id string) string {
	return id
}

func (p *actorProvider) ParseActorIDFromURL(rawURL string) (string, error) {
	u, err := url.Parse(rawURL)
	if err != nil {
		return "", err
	}
	base := path.Base(u.Path)
	if p.idRegex != nil {
		m := p.idRegex.FindStringSubmatch(base)
		if len(m) > 1 {
			return m[1], nil
		}
	}
	return base, nil
}

func (p *actorProvider) GetActorInfoByID(id string) (*model.ActorInfo, error) {
	actorURL := strings.ReplaceAll(p.actor.URL, "{id}", id)
	return p.GetActorInfoByURL(actorURL)
}

func (p *actorProvider) GetActorInfoByURL(rawURL string) (*model.ActorInfo, error) {
	id, err := p.ParseActorIDFromURL(rawURL)
	if err != nil {
		return nil, err
	}

	info := &model.ActorInfo{
		ID:       id,
		Provider: p.Name(),
		Homepage: rawURL,
		Images:   []string{},
		Aliases:  []string{},
	}

	doc, err := p.fetchHTML(rawURL)
	if err != nil {
		return nil, err
	}

	p.extractActorFields(doc, info)
	return info, nil
}

func (p *actorProvider) extractActorFields(doc *html.Node, info *model.ActorInfo) {
	f := &p.actor.Fields

	if v := evalXPathText(doc, f.Name); v != "" {
		info.Name = v
	}
	if v := evalXPathMultiAttr(doc, f.Images, "src"); len(v) > 0 {
		info.Images = makeURLAbs(p.cfg.BaseURL, v)
	}
	if v := evalXPathText(doc, f.Summary); v != "" {
		info.Summary = v
	}
	if v := evalXPathText(doc, f.Birthday); v != "" {
		info.Birthday = parser.ParseDate(v)
	}
	if v := evalXPathText(doc, f.Height); v != "" {
		info.Height = parser.ParseInt(v)
	}
	if v := evalXPathText(doc, f.Measurements); v != "" {
		info.Measurements = v
	}
	if v := evalXPathText(doc, f.BloodType); v != "" {
		info.BloodType = v
	}
	if v := evalXPathText(doc, f.CupSize); v != "" {
		info.CupSize = v
	}
	if v := evalXPathText(doc, f.Nationality); v != "" {
		info.Nationality = v
	}
	if v := evalXPathMultiText(doc, f.Aliases); len(v) > 0 {
		info.Aliases = v
	}
	if v := evalXPathText(doc, f.Hobby); v != "" {
		info.Hobby = v
	}
	if v := evalXPathText(doc, f.Skill); v != "" {
		info.Skill = v
	}
	if v := evalXPathText(doc, f.DebutDate); v != "" {
		info.DebutDate = parser.ParseDate(v)
	}
}

// --- XPath Helpers ---

func evalXPathText(node *html.Node, expr string) string {
	if expr == "" {
		return ""
	}
	n := htmlquery.FindOne(node, expr)
	if n == nil {
		return ""
	}
	return strings.TrimSpace(htmlquery.InnerText(n))
}

func evalXPathMultiText(node *html.Node, expr string) []string {
	if expr == "" {
		return nil
	}
	nodes := htmlquery.Find(node, expr)
	if len(nodes) == 0 {
		return nil
	}
	var texts []string
	for _, n := range nodes {
		if t := strings.TrimSpace(htmlquery.InnerText(n)); t != "" {
			texts = append(texts, t)
		}
	}
	return texts
}

func evalXPathAttr(node *html.Node, expr string, attrs ...string) string {
	if expr == "" {
		return ""
	}
	n := htmlquery.FindOne(node, expr)
	if n == nil {
		return ""
	}
	for _, attr := range attrs {
		if v := htmlquery.SelectAttr(n, attr); v != "" {
			return strings.TrimSpace(v)
		}
	}
	return ""
}

func evalXPathMultiAttr(node *html.Node, expr string, attr string) []string {
	if expr == "" {
		return nil
	}
	nodes := htmlquery.Find(node, expr)
	if len(nodes) == 0 {
		return nil
	}
	var values []string
	for _, n := range nodes {
		if v := strings.TrimSpace(htmlquery.SelectAttr(n, attr)); v != "" {
			values = append(values, v)
		}
	}
	return values
}

func absURL(base, raw string) string {
	if strings.HasPrefix(raw, "http://") || strings.HasPrefix(raw, "https://") {
		return raw
	}
	if strings.HasPrefix(raw, "//") {
		return "https:" + raw
	}
	baseURL := strings.TrimRight(base, "/")
	if strings.HasPrefix(raw, "/") {
		u, _ := url.Parse(baseURL)
		if u != nil {
			return u.Scheme + "://" + u.Host + raw
		}
		return baseURL + raw
	}
	return baseURL + "/" + strings.TrimLeft(raw, "/")
}

func makeURLAbs(base string, urls []string) []string {
	result := make([]string, len(urls))
	for i, u := range urls {
		result[i] = absURL(base, u)
	}
	return result
}