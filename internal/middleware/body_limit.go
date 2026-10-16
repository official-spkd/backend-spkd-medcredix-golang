package middleware

import (
	"net/http"
	"strings"

	"github.com/gin-gonic/gin"

	"github.com/spkd/medcredix/internal/storage"
)

// uploadLimit = batas body untuk endpoint unggah (berkas 5 MB + field form).
const uploadLimit = storage.MaxSize + 1<<20

// BodyLimit membatasi ukuran body request. Endpoint unggah (/upload) boleh sampai uploadLimit.
func BodyLimit(n int64) gin.HandlerFunc {
	return func(c *gin.Context) {
		if c.Request.Body != nil {
			limit := n
			if strings.HasSuffix(c.Request.URL.Path, "/upload") {
				limit = uploadLimit
			}
			c.Request.Body = http.MaxBytesReader(c.Writer, c.Request.Body, limit)
		}
		c.Next()
	}
}
