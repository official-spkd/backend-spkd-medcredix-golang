package middleware

import (
	"errors"
	"fmt"
	"net/http"
	"strings"

	"github.com/gin-gonic/gin"
	"gorm.io/gorm"

	"github.com/spkd/medcredix/internal/auth"
	"github.com/spkd/medcredix/internal/model"
)

// deny membalas 401 dengan pesan untuk pengguna.
func deny(c *gin.Context, msg string) {
	c.AbortWithStatusJSON(401, gin.H{"ok": false, "error": msg})
}

// Auth memeriksa token login di header Authorization, lalu mengecek ulang akunnya di database.
// Akun yang sudah dinonaktifkan langsung ditolak walau tokennya belum kedaluwarsa.
func Auth(db *gorm.DB) gin.HandlerFunc {
	return func(c *gin.Context) {
		h := c.GetHeader("Authorization")
		if len(h) < 8 || len(h) > auth.MaxTokenLength || !strings.EqualFold(h[:7], "Bearer ") {
			deny(c, "Sesi tidak valid atau kosong. Silakan masuk kembali.")
			return
		}

		claims, valid := auth.ParseToken(strings.TrimSpace(h[7:]))
		if !valid {
			deny(c, "Token kedaluwarsa atau dimanipulasi.")
			return
		}
		if auth.IsRevoked(claims.ID) {
			deny(c, "Sesi sudah keluar. Silakan masuk kembali.")
			return
		}

		var user model.User
		if err := db.First(&user, "id = ? AND active = ?", claims.UserID, true).Error; err != nil {
			if !errors.Is(err, gorm.ErrRecordNotFound) {
				// Database bermasalah: jangan balas 401, nanti frontend mengira sesi habis dan logout.
				c.AbortWithStatusJSON(503, gin.H{"ok": false, "error": "Database sedang tidak dapat diakses. Coba lagi sebentar."})
				return
			}
			deny(c, "Akun tidak ditemukan atau telah dinonaktifkan.")
			return
		}

		// Peran dan RS selalu diambil dari database, bukan dari isi token.
		c.Set("user", user)
		c.Set("claims", claims)
		c.Next()
	}
}

// RequirePermission menolak request bila peran pengguna tidak punya izin p.
// Untuk endpoint yang izinnya berbeda per aksi, cek dengan auth.HasPermission di controller.
func RequirePermission(p auth.Permission) gin.HandlerFunc {
	return func(c *gin.Context) {
		u, ok := auth.CurrentUser(c)
		if !ok {
			c.AbortWithStatusJSON(http.StatusUnauthorized, gin.H{"ok": false, "error": "Sesi tidak valid."})
			return
		}
		if !auth.HasPermission(u.Role, p) {
			c.AbortWithStatusJSON(http.StatusForbidden, gin.H{"ok": false,
				"error": fmt.Sprintf("Peran %s tidak memiliki izin %s.", auth.RoleLabels[u.Role], p)})
			return
		}
		c.Next()
	}
}
