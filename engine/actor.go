package engine

import (
	goerr "errors"
	"fmt"
	"sort"
	"sync"
	"time"

	"golang.org/x/text/language"
	"gorm.io/gorm/clause"

	"github.com/metatube-community/metatube-sdk-go/collection/sets"
	"github.com/metatube-community/metatube-sdk-go/collection/slices"
	"github.com/metatube-community/metatube-sdk-go/common/comparer"
	"github.com/metatube-community/metatube-sdk-go/common/parser"
	"github.com/metatube-community/metatube-sdk-go/engine/providerid"
	"github.com/metatube-community/metatube-sdk-go/model"
	mt "github.com/metatube-community/metatube-sdk-go/provider"
	"github.com/metatube-community/metatube-sdk-go/provider/gfriends"
)

func (e *Engine) searchActorFromDB(keyword string, provider mt.Provider) (results []*model.ActorSearchResult, err error) {
	var infos []*model.ActorInfo
	if err = e.db.
		Where("provider = ? AND name = ? COLLATE NOCASE",
			provider.Name(), keyword).
		Find(&infos).Error; err == nil {
		for _, info := range infos {
			if !info.IsValid() {
				continue
			}
			results = append(results, info.ToSearchResult())
		}
	}
	return
}

func (e *Engine) searchActor(keyword string, provider mt.Provider, fallback bool) ([]*model.ActorSearchResult, error) {
	innerSearch := func(keyword string) (results []*model.ActorSearchResult, err error) {
		if provider.Name() == gfriends.Name {
			return provider.(mt.ActorSearcher).SearchActor(keyword)
		}
		if searcher, ok := provider.(mt.ActorSearcher); ok {
			defer func() {
				if err != nil || len(results) == 0 {
					return // ignore error or empty.
				}
				const minSimilarity = 0.3
				ps := new(slices.WeightedSlice[*model.ActorSearchResult, float64])
				for _, result := range results {
					if similarity := comparer.Compare(result.Name, keyword); similarity >= minSimilarity {
						ps.Append(result, similarity)
					}
				}
				results = ps.SortFunc(sort.Stable).Slice() // replace results.
			}()
			if fallback {
				defer func() {
					if innerResults, innerErr := e.searchActorFromDB(keyword, provider);
					// ignore DB query error.
					innerErr == nil && len(innerResults) > 0 {
						// overwrite error.
						err = nil
						// update results.
						asr := sets.NewOrderedSetWithHash(func(v *model.ActorSearchResult) string { return v.Provider + v.ID })
// unlike movie searching, we want search results go first
					// than DB data here, so we add DB results after search results.
					asr.Add(results...)
					asr.Add(innerResults...)
						results = asr.AsSlice()
					}
				}()
			}
			return searcher.SearchActor(keyword)
		}
		// All providers should implement the ActorSearcher interface.
		return nil, mt.ErrInfoNotFound
	}
	names := parser.ParseActorNames(keyword)
	if len(names) == 0 {
		return nil, mt.ErrInvalidKeyword
	}
	var (
		results []*model.ActorSearchResult
		errors  []error
	)
	for _, name := range names {
		innerResults, innerErr := innerSearch(name)
		if innerErr != nil &&
			// ignore InfoNotFound error.
			!goerr.Is(innerErr, mt.ErrInfoNotFound) {
			// add error to chain and handle it later.
			errors = append(errors, innerErr)
			continue
		}
		results = append(results, innerResults...)
	}
	if len(results) == 0 {
		if len(errors) > 0 {
			return nil, fmt.Errorf("search errors: %v", errors)
		}
		return nil, mt.ErrInfoNotFound
	}
	return results, nil
}

func (e *Engine) SearchActor(keyword, name string, fallback bool) ([]*model.ActorSearchResult, error) {
	provider, err := e.GetActorProviderByName(name)
	if err != nil {
		return nil, err
	}
	return e.searchActor(keyword, provider, fallback)
}

func (e *Engine) SearchActorAll(keyword string, fallback bool) (results []*model.ActorSearchResult, err error) {
	var fastProvider mt.ActorProvider
	var fastPriority float64 = -1
	for _, p := range e.actorProviders.Iterator() {
		if p.Priority() > fastPriority {
			fastProvider = p
			fastPriority = p.Priority()
		}
	}
	if fastProvider != nil {
		if k, e2 := e.SearchActor(keyword, fastProvider.Name(), fallback); e2 == nil && len(k) > 0 {
			return k, nil
		}
	}
	var (
		mu sync.Mutex
		wg sync.WaitGroup
	)
	for _, provider := range e.actorProviders.Iterator() {
		if fastProvider != nil && provider.Name() == fastProvider.Name() {
			continue
		}
		if provider.Priority() < 1000 {
			continue
		}
		wg.Add(1)
		go func(provider mt.ActorProvider) {
			defer wg.Done()
			start := time.Now()
			if innerResults, innerErr := e.searchActor(keyword, provider, fallback); innerErr == nil {
				e.logger.Printf("Search actor %q with %s: %d results in %v", keyword, provider.Name(), len(innerResults), time.Since(start))
				for _, result := range innerResults {
					if result.IsValid() {
						mu.Lock()
						results = append(results, result)
						mu.Unlock()
					}
				}
			} else {
				e.logger.Printf("Search actor %q with %s: %v in %v", keyword, provider.Name(), innerErr, time.Since(start))
			}
		}(provider)
	}
	wg.Wait()
	// Deduplicate by provider+ID.
	seen := make(map[string]bool)
	deduped := make([]*model.ActorSearchResult, 0, len(results))
	for _, r := range results {
		key := r.Provider + r.ID
		if seen[key] {
			continue
		}
		seen[key] = true
		deduped = append(deduped, r)
	}
	results = deduped
	// Precompute priorities for sort.
	priorities := make(map[string]float64)
	for _, r := range results {
		if p, err := e.GetActorProviderByName(r.Provider); err == nil {
			priorities[r.Provider] = p.Priority()
		}
	}
	sort.SliceStable(results, func(i, j int) bool {
		return priorities[results[i].Provider] > priorities[results[j].Provider]
	})
	return
}

func (e *Engine) getActorInfoFromDB(provider mt.ActorProvider, id string) (*model.ActorInfo, error) {
	info := &model.ActorInfo{}
	err := e.db. // Exact match here.
			Where("provider = ?", provider.Name()).
			Where("id = ? COLLATE NOCASE", id).
			First(info).Error
	return info, err
}

func (e *Engine) getActorInfoWithCallback(provider mt.ActorProvider, id string, lazy bool, callback func() (*model.ActorInfo, error)) (info *model.ActorInfo, err error) {
	defer func() {
		if err == nil && (info == nil || !info.IsValid()) {
			err = mt.ErrIncompleteMetadata
		}
	}()
	if provider.Name() == gfriends.Name {
		return provider.GetActorInfoByID(id)
	}
	defer func() {
		if err == nil && info != nil && provider.Language() == language.Japanese {
			if gInfo, gErr := e.MustGetActorProviderByName(gfriends.Name).GetActorInfoByID(id); gErr == nil && len(gInfo.Images) > 0 {
				info.Images = gInfo.Images
			}
		}
		if err == nil && info != nil && info.IsValid() {
			e.preFetchActorImages(info)
		}
	}()
	if lazy {
		if info, err = e.getActorInfoFromDB(provider, id); err == nil && info.IsValid() {
			return
		}
	}
	defer func() {
		if err == nil && info.IsValid() {
			e.db.Clauses(clause.OnConflict{
				UpdateAll: true,
			}).Create(info)
		}
	}()
	info, err = callback()
	if err != nil {
		// Fallback to gfriends for image-only result.
		if gInfo, gErr := e.MustGetActorProviderByName(gfriends.Name).GetActorInfoByID(id); gErr == nil && gInfo.IsValid() && len(gInfo.Images) > 0 {
			info = gInfo
			err = nil
		}
	}
	return
}

func (e *Engine) getActorInfoByProviderID(provider mt.ActorProvider, id string, lazy bool) (*model.ActorInfo, error) {
	if id = provider.NormalizeActorID(id); id == "" {
		return nil, mt.ErrInvalidID
	}
	return e.getActorInfoWithCallback(provider, id, lazy, func() (*model.ActorInfo, error) {
		return provider.GetActorInfoByID(id)
	})
}

func (e *Engine) GetActorInfoByProviderID(pid providerid.ProviderID, lazy bool) (*model.ActorInfo, error) {
	provider, err := e.GetActorProviderByName(pid.Provider)
	if err != nil {
		return nil, err
	}
	return e.getActorInfoByProviderID(provider, pid.ID, lazy)
}

func (e *Engine) getActorInfoByProviderURL(provider mt.ActorProvider, rawURL string, lazy bool) (*model.ActorInfo, error) {
	id, err := provider.ParseActorIDFromURL(rawURL)
	switch {
	case err != nil:
		return nil, err
	case id == "":
		return nil, mt.ErrInvalidURL
	}
	return e.getActorInfoWithCallback(provider, id, lazy, func() (*model.ActorInfo, error) {
		return provider.GetActorInfoByURL(rawURL)
	})
}

func (e *Engine) GetActorInfoByURL(rawURL string, lazy bool) (*model.ActorInfo, error) {
	provider, err := e.GetActorProviderByURL(rawURL)
	if err != nil {
		return nil, err
	}
	return e.getActorInfoByProviderURL(provider, rawURL, lazy)
}

func (e *Engine) preFetchActorImages(info *model.ActorInfo) {
	if e.imageCacheDir == "" {
		return
	}
	provider, err := e.GetActorProviderByName(gfriends.Name)
	if err != nil {
		return
	}
	var sem = make(chan struct{}, 5) // limit concurrent pre-fetches
	var wg sync.WaitGroup
	for _, url := range info.Images {
		if url == "" {
			continue
		}
		wg.Add(1)
		sem <- struct{}{}
		go func(u string) {
			defer wg.Done()
			defer func() { <-sem }()
			if _, err := e.getImageByURL(provider, u); err != nil {
				e.logger.Printf("Pre-fetch actor image failed: %s, %v", u, err)
			}
		}(url)
	}
}
