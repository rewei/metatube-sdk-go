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
			// Rename first, then remove. This prevents in-flight requests
			// from reading partially deleted files.
			tmpDir := imageCacheDir + ".tmp"
			os.MkdirAll(tmpDir, 0755)
			for _, entry := range entries {
				oldPath := filepath.Join(imageCacheDir, entry.Name())
				tmpPath := filepath.Join(tmpDir, entry.Name())
				os.Rename(oldPath, tmpPath)
			}
			app.UnlockImageCache()
			// Remove the renamed files outside the lock.
			os.RemoveAll(tmpDir)
		}
		c.JSON(http.StatusOK, &responseMessage{
			Data: "cache cleared",
		})
	}
}