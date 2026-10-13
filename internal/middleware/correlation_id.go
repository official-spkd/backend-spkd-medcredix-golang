package middleware

import (
	"regexp"

	"github.com/gin-gonic/gin"

	"github.com/spkd/medcredix/internal/model"
)

// Correlation ID dari klien: maksimal 64 karakter, tanpa spasi/baris baru.
var correlationIDPattern = regexp.MustCompile(`^[A-Za-z0-9._:-]{1,64}$`)

// CorrelationID memberi setiap request ID unik (header X-Correlation-Id) untuk melacak log.
func CorrelationID() gin.HandlerFunc {
	return func(c *gin.Context) {
		id := c.GetHeader("X-Correlation-Id")
		if !correlationIDPattern.MatchString(id) {
			id = model.NewID("cor")
		}
		c.Set("correlation_id", id)
		c.Header("X-Correlation-Id", id)
		c.Next()
	}
}
