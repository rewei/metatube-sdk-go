package configurable

import (
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"

	"golang.org/x/text/language"
	"gopkg.in/yaml.v3"
)

type ProviderConfig struct {
	Name     string  `yaml:"name"`
	BaseURL  string  `yaml:"base_url"`
	Priority float64 `yaml:"priority"`
	Language string  `yaml:"language"`
	Movie    *MovieConfig `yaml:"movie,omitempty"`
	Actor    *ActorConfig `yaml:"actor,omitempty"`
}

type MovieConfig struct {
	URL     string      `yaml:"url"`
	IDRegex string      `yaml:"id_regex,omitempty"`
	Fields  MovieFields `yaml:"fields"`
	Search  *SearchConfig `yaml:"search,omitempty"`
}

type MovieFields struct {
	Title           string `yaml:"title"`
	Number          string `yaml:"number,omitempty"`
	Cover           string `yaml:"cover,omitempty"`
	BigCover        string `yaml:"big_cover,omitempty"`
	Thumb           string `yaml:"thumb,omitempty"`
	BigThumb        string `yaml:"big_thumb,omitempty"`
	PreviewVideo    string `yaml:"preview_video,omitempty"`
	PreviewVideoHLS string `yaml:"preview_video_hls,omitempty"`
	PreviewImages   string `yaml:"preview_images,omitempty"`
	Summary         string `yaml:"summary,omitempty"`
	Director        string `yaml:"director,omitempty"`
	Actors          string `yaml:"actors,omitempty"`
	Maker           string `yaml:"maker,omitempty"`
	Label           string `yaml:"label,omitempty"`
	Series          string `yaml:"series,omitempty"`
	Genres          string `yaml:"genres,omitempty"`
	ReleaseDate     string `yaml:"release_date,omitempty"`
	Runtime         string `yaml:"runtime,omitempty"`
	Score           string `yaml:"score,omitempty"`
}

type ActorConfig struct {
	URL     string      `yaml:"url"`
	IDRegex string      `yaml:"id_regex,omitempty"`
	Fields  ActorFields `yaml:"fields"`
	Search  *SearchConfig `yaml:"search,omitempty"`
}

type ActorFields struct {
	Name         string `yaml:"name"`
	Images       string `yaml:"images,omitempty"`
	Summary      string `yaml:"summary,omitempty"`
	Birthday     string `yaml:"birthday,omitempty"`
	Height       string `yaml:"height,omitempty"`
	Measurements string `yaml:"measurements,omitempty"`
	BloodType    string `yaml:"blood_type,omitempty"`
	CupSize      string `yaml:"cup_size,omitempty"`
	Nationality  string `yaml:"nationality,omitempty"`
	Aliases      string `yaml:"aliases,omitempty"`
	Hobby        string `yaml:"hobby,omitempty"`
	Skill        string `yaml:"skill,omitempty"`
	DebutDate    string `yaml:"debut_date,omitempty"`
}

type SearchConfig struct {
	URL     string         `yaml:"url"`
	Results string         `yaml:"results"`
	Fields  SearchFields   `yaml:"fields"`
}

type SearchFields struct {
	ID          string `yaml:"id"`
	Number      string `yaml:"number,omitempty"`
	Title       string `yaml:"title"`
	Cover       string `yaml:"cover,omitempty"`
	Thumb       string `yaml:"thumb,omitempty"`
	Actors      string `yaml:"actors,omitempty"`
	ReleaseDate string `yaml:"release_date,omitempty"`
	Score       string `yaml:"score,omitempty"`
	Homepage    string `yaml:"homepage,omitempty"`
}

func LoadConfigFromFile(path string) (*ProviderConfig, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	cfg := &ProviderConfig{Language: "ja"}
	if err := yaml.Unmarshal(data, cfg); err != nil {
		return nil, fmt.Errorf("parse %s: %w", path, err)
	}
	if cfg.Name == "" {
		return nil, fmt.Errorf("%s: name is required", path)
	}
	if cfg.BaseURL == "" {
		return nil, fmt.Errorf("%s: base_url is required", path)
	}
	if cfg.Movie == nil && cfg.Actor == nil {
		return nil, fmt.Errorf("%s: at least one of movie or actor config is required", path)
	}
	return cfg, nil
}

func LoadConfigsFromDir(dir string) ([]*ProviderConfig, error) {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return nil, err
	}
	var configs []*ProviderConfig
	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}
		ext := strings.ToLower(filepath.Ext(entry.Name()))
		if ext != ".yaml" && ext != ".yml" {
			continue
		}
		cfg, err := LoadConfigFromFile(filepath.Join(dir, entry.Name()))
		if err != nil {
			return nil, fmt.Errorf("load %s: %w", entry.Name(), err)
		}
		configs = append(configs, cfg)
	}
	return configs, nil
}

func (cfg *ProviderConfig) ParseLanguage() language.Tag {
	switch strings.ToLower(cfg.Language) {
	case "ja", "jp", "japanese":
		return language.Japanese
	case "en", "english":
		return language.English
	case "zh", "cn", "chinese":
		return language.Chinese
	case "ko", "korean":
		return language.Korean
	default:
		return language.Japanese
	}
}

func (cfg *ProviderConfig) CompileIDRegex() (*regexp.Regexp, error) {
	if cfg.Movie != nil && cfg.Movie.IDRegex != "" {
		return regexp.Compile(cfg.Movie.IDRegex)
	}
	if cfg.Actor != nil && cfg.Actor.IDRegex != "" {
		return regexp.Compile(cfg.Actor.IDRegex)
	}
	return nil, nil
}