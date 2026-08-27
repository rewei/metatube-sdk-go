package route

import (
	"fmt"
	"net/http"
	pkgurl "net/url"
	"os"

	"github.com/gin-gonic/gin"

	"github.com/metatube-community/metatube-sdk-go/engine"
	"github.com/metatube-community/metatube-sdk-go/errors"
	"github.com/metatube-community/metatube-sdk-go/model"
	"github.com/metatube-community/metatube-sdk-go/provider/kutikomiya"
)

type searchType uint8

const (
	actorSearchType searchType = iota
	movieSearchType
)

type searchQuery struct {
	Q        string `form:"q" binding:"required"`
	Provider string `form:"provider"`
	Fallback bool   `form:"fallback"`
}

func getSearch(app *engine.Engine, typ searchType) gin.HandlerFunc {
	return func(c *gin.Context) {
		query := &searchQuery{
			Fallback: true,
		}
		if err := c.ShouldBindQuery(query); err != nil {
			abortWithStatusMessage(c, http.StatusBadRequest, err)
			return
		}

		isValidURL := true
		if _, err := pkgurl.ParseRequestURI(query.Q); err != nil {
			isValidURL = false
		}

		searchAll := query.Provider == ""

		var (
			results any
			err     error
		)
		switch typ {
		case actorSearchType:
			if isValidURL {
				results, err = app.GetActorInfoByURL(query.Q, true)
			} else if searchAll {
				results, err = app.SearchActorAll(query.Q, query.Fallback)
			} else {
				results, err = app.SearchActor(query.Q, query.Provider, query.Fallback)
			}
		case movieSearchType:
			if isValidURL {
				results, err = app.GetMovieInfoByURL(query.Q, true)
			} else if searchAll {
				results, err = app.SearchMovieAll(query.Q, query.Fallback)
			} else {
				results, err = app.SearchMovie(query.Q, query.Provider, query.Fallback)
			}
		default:
			panic("invalid search type")
		}
		// Async learning: if KUTIKOMIYA returned nothing, try MINNANO in background.
		if typ == actorSearchType && err != nil && !isValidURL {
			go learnSlugFromMinnano(query.Q)
		}
		if err != nil {
			abortWithError(c, err)
			return
		}

		resultsLength := 1

		switch v := results.(type) {
		case *model.ActorInfo:
			results = []*model.ActorSearchResult{v.ToSearchResult()}
		case *model.MovieInfo:
			results = []*model.MovieSearchResult{v.ToSearchResult()}
		case []*model.ActorSearchResult:
			resultsLength = len(v)
		case []*model.MovieSearchResult:
			resultsLength = len(v)
		default:
			panic("unexpected search results type")
		}
		if resultsLength == 0 {
			abortWithError(c, errors.FromCode(http.StatusNotFound))
			return
		}

		c.JSON(http.StatusOK, &responseMessage{Data: results})
	}
}

func learnSlugFromMinnano(name string) {
	mainName, aliases, _ := queryMinnano(name)
	if mainName == "" {
		return
	}
	slug := kutikomiya.LookupSlug(mainName)
	if slug == "" {
		for _, alias := range aliases {
			slug = kutikomiya.LookupSlug(alias)
			if slug != "" {
				break
			}
		}
	}
	if slug != "" {
		entries := map[string]string{mainName: slug}
		if name != mainName && name != "" {
			entries[name] = slug
		}
		for _, alias := range aliases {
			if alias != "" && alias != mainName {
				entries[alias] = slug
			}
		}
		if err := kutikomiya.SaveSlugs(entries); err != nil {
			fmt.Fprintf(os.Stderr, "learnSlugFromMinnano error: %v\n", err)
		}
	}
}