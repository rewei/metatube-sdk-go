package route

import (
	"net/http"
	"os"
	"path/filepath"
	"time"

	"github.com/gin-gonic/gin"
	cachecontrol "go.eigsys.de/gin-cachecontrol/v2"

	"github.com/metatube-community/metatube-sdk-go/engine"
	"github.com/metatube-community/metatube-sdk-go/provider/gfriends"
)

func cachePublicSMaxAge(duration time.Duration) gin.HandlerFunc {
	return cachecontrol.New(cachecontrol.Config{
		Public:  true,
		SMaxAge: cachecontrol.Duration(duration),
	})
}

func cacheNoStore() gin.HandlerFunc {
	return cachecontrol.New(cachecontrol.Config{
		NoStore: true,
	})
}

func getCacheClear(app *engine.Engine) gin.HandlerFunc {
	return func(c *gin.Context) {
		gfriends.ResetCache()
		if imageCacheDir := app.GetImageCacheDir(); imageCacheDir != "" {
			app.LockImageCache()
			entries, _ := os.ReadDir(imageCacheDir)
			for _, entry := range entries {
				os.RemoveAll(filepath.Join(imageCacheDir, entry.Name()))
			}
			app.UnlockImageCache()
		}
		c.JSON(http.StatusOK, &responseMessage{
			Data: "cache cleared",
		})
	}
}