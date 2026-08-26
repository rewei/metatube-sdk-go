package route

import (
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"regexp"
	"strings"
	"sync"
	"time"

	"github.com/gin-gonic/gin"

	"github.com/metatube-community/metatube-sdk-go/engine"
	"github.com/metatube-community/metatube-sdk-go/provider/kutikomiya"
)

const (
	minnanoSearchURL = "https://www.minnano-av.com/search_result.php?search_scope=actress&search_word=%s&search=Go"
	minnanoActorURL  = "https://www.minnano-av.com/actress%s.html"
)

var httpClient = &http.Client{Timeout: 15 * time.Second}

type actorBatchRequest struct {
	Actors []string `json:"actors" binding:"required"`
}

type actorBatchResult struct {
	Provider string `json:"provider"`
	ID       string `json:"id"`
}

func getActorBatch(app *engine.Engine) gin.HandlerFunc {
	return func(c *gin.Context) {
		var req actorBatchRequest
		if err := c.ShouldBindJSON(&req); err != nil {
			abortWithStatusMessage(c, http.StatusBadRequest, err)
			return
		}

		results := make(map[string]*actorBatchResult)
		var (
			mu sync.Mutex
			wg sync.WaitGroup
		)

		for _, actor := range req.Actors {
			wg.Add(1)
			go func(actor string) {
				defer wg.Done()

				// 1. Try KUTIKOMIYA (fast, slug mapping).
				if k, err := app.SearchActor(actor, "KUTIKOMIYA", false); err == nil && len(k) > 0 {
					mu.Lock()
					results[actor] = &actorBatchResult{
						Provider: "KUTIKOMIYA",
						ID:       k[0].ID,
					}
					mu.Unlock()
					return
				}

				// 2. Fallback to MINNANO (slow).
				mainName, aliases, minnanoID := queryMinnano(actor)
				if minnanoID == "" {
					return // not found, skip
				}

				// 3. Try to find a slug mapping for the main name or any alias.
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
					// Found a slug mapping. Save all aliases and return KUTIKOMIYA ID.
					entries := map[string]string{mainName: slug}
					for _, alias := range aliases {
						if alias != "" && alias != mainName {
							entries[alias] = slug
						}
					}
					if err := kutikomiya.SaveSlugs(entries); err != nil {
					fmt.Fprintf(os.Stderr, "SaveSlugs error for %s: %v\n", actor, err)
				} else {
					fmt.Fprintf(os.Stderr, "SaveSlugs saved %d entries for %s: %v\n", len(entries), actor, entries)
				}

					mu.Lock()
					results[actor] = &actorBatchResult{
						Provider: "KUTIKOMIYA",
						ID:       mainName,
					}
					mu.Unlock()
				} else {
					// No slug mapping found. Return MINNANO ID directly.
					mu.Lock()
					results[actor] = &actorBatchResult{
						Provider: "MINNANO",
						ID:       minnanoID,
					}
					mu.Unlock()
				}
			}(actor)
		}
		wg.Wait()

		c.JSON(http.StatusOK, &responseMessage{
			Data: map[string]any{
				"actors": results,
			},
		})
	}
}

func queryMinnano(name string) (mainName string, aliases []string, actressID string) {
	searchURL := fmt.Sprintf(minnanoSearchURL, url.QueryEscape(name))
	req, _ := http.NewRequest("GET", searchURL, nil)
	req.Header.Set("User-Agent", "Mozilla/5.0")
	resp, err := httpClient.Do(req)
	if err != nil {
		return "", nil, ""
	}
	defer resp.Body.Close()

	finalURL := resp.Request.URL.String()
	re := regexp.MustCompile(`actress(\d+)\.html`)
	matches := re.FindStringSubmatch(finalURL)
	if len(matches) >= 2 {
		actressID = matches[1]
	} else {
		body, _ := io.ReadAll(resp.Body)
		allMatches := re.FindAllStringSubmatch(string(body), -1)
		if len(allMatches) > 0 {
			actressID = allMatches[0][1]
		}
	}
	if actressID == "" {
		return "", nil, ""
	}

	actorURL := fmt.Sprintf(minnanoActorURL, actressID)
	req, _ = http.NewRequest("GET", actorURL, nil)
	req.Header.Set("User-Agent", "Mozilla/5.0")
	resp2, err := httpClient.Do(req)
	if err != nil {
		return "", nil, ""
	}
	defer resp2.Body.Close()
	body2, _ := io.ReadAll(resp2.Body)

	// Get main name from JSON-LD.
	re = regexp.MustCompile(`"name"\s*:\s*"([^"]+)"`)
	if match := re.FindSubmatch(body2); len(match) >= 2 {
		mainName = string(match[1])
	}

	// Get aliases.
	re = regexp.MustCompile(`別名</span><p>([^<]+)`)
	aliasMatches := re.FindAllSubmatch(body2, -1)
	for i, m := range aliasMatches {
		if i == 0 {
			continue // skip first (main name)
		}
		aliasText := string(m[1])
		parts := strings.Split(aliasText, "（")[0]
		parts = strings.Split(parts, "(")[0]
		parts = strings.TrimSpace(parts)
		if parts != "" && parts != mainName {
			aliases = append(aliases, parts)
		}
	}

	return mainName, aliases, actressID
}