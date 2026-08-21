package configurable

import (
	"github.com/metatube-community/metatube-sdk-go/provider"
)

// RegisterAllFromDir loads all YAML config files from the given directory
// and registers them as providers. Call this before engine.New().
func RegisterAllFromDir(dir string) error {
	configs, err := LoadConfigsFromDir(dir)
	if err != nil {
		return err
	}
	for _, cfg := range configs {
		if cfg.Movie != nil {
			name := cfg.Name
			provider.Register(name, func() provider.MovieProvider {
				return NewMovieProvider(cfg)
			})
		}
		if cfg.Actor != nil {
			name := cfg.Name
			provider.Register(name, func() provider.ActorProvider {
				return NewActorProvider(cfg)
			})
		}
	}
	return nil
}