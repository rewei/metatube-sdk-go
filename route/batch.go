package route

import (
	"net/http"
	"sync"

	"github.com/gin-gonic/gin"

	"github.com/metatube-community/metatube-sdk-go/engine"
)

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

				var result *actorBatchResult

				// Try KUTIKOMIYA first (fast, instant).
				if k, err := app.SearchActor(actor, "KUTIKOMIYA", false); err == nil && len(k) > 0 {
					result = &actorBatchResult{
						Provider: "KUTIKOMIYA",
						ID:       k[0].ID,
					}
				} else {
					// Fallback to MINNANO (slow).
					if m, err := app.SearchActor(actor, "MINNANO", false); err == nil && len(m) > 0 {
						result = &actorBatchResult{
							Provider: "MINNANO",
							ID:       m[0].ID,
						}
					}
				}

				if result != nil {
					mu.Lock()
					results[actor] = result
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