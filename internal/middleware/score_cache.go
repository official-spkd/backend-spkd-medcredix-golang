package middleware

import (
	"net/http"

	"github.com/gin-gonic/gin"

	"github.com/spkd/medcredix/internal/service"
)

// RefreshScoreCache membuang cache skor setelah request yang berhasil mengubah data,
// supaya dashboard langsung menampilkan angka terbaru.
func RefreshScoreCache() gin.HandlerFunc {
	return func(c *gin.Context) {
		c.Next()
		if c.Request.Method != http.MethodGet && c.Writer.Status() < 400 {
			service.InvalidateIntel()
		}
	}
}
