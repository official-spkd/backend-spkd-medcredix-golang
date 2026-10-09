package auth

import (
	"errors"
	"log"
	"os"
	"strings"
	"sync"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/golang-jwt/jwt/v5"
	"github.com/spkd/medcredix/internal/appenv"
	"github.com/spkd/medcredix/internal/model"
	"gorm.io/gorm"
)

const (
	tokenIssuer     = "medcredix"
	tokenAudience   = "medcredix-api"
	tokenLifetime   = 12 * time.Hour // sesi kedaluwarsa (fix F-02)
	minSecretLength = 32
	maxTokenLength  = 4096
)

const devSecret = "dev-only-medcredix-secret-change-me"

var (
	secretOnce sync.Once
	secretKey  []byte
)

// CheckConfig: di produksi JWT_SECRET wajib ada dan minimal 32 karakter.
func CheckConfig() error {
	v := os.Getenv("JWT_SECRET")
	if appenv.IsProduction() {
		if v == "" {
			return errors.New("JWT_SECRET wajib diisi saat APP_ENV=production")
		}
		if len(v) < minSecretLength {
			return errors.New("JWT_SECRET terlalu pendek (minimal 32 karakter)")
		}
		if v == devSecret {
			return errors.New("JWT_SECRET masih memakai secret dev")
		}
	}
	return nil
}

// jwtSecret dibaca dari env JWT_SECRET. Secret dev hanya dipakai di luar produksi.
func jwtSecret() []byte {
	secretOnce.Do(func() {
		if v := os.Getenv("JWT_SECRET"); v != "" {
			secretKey = []byte(v)
			return
		}
		if appenv.IsProduction() {
			log.Fatal("JWT_SECRET kosong di produksi")
		}
		log.Println("PERINGATAN: JWT_SECRET belum diisi, memakai secret dev. WAJIB diisi di produksi.")
		secretKey = []byte(devSecret)
	})
	return secretKey
}

type Claims struct {
	UserID     string `json:"user_id"`
	HospitalID string `json:"hospital_id"` // "" untuk akun tanpa RS (Auditor BPJS)
	Role       string `json:"role"`
	jwt.RegisteredClaims
}

// GenerateToken dipanggil saat login berhasil. Mengembalikan token + waktu kedaluwarsa.
func GenerateToken(userID, hospitalID, role string) (string, time.Time, error) {
	now := time.Now()
	exp := now.Add(tokenLifetime)
	claims := &Claims{
		UserID:     userID,
		HospitalID: hospitalID,
		Role:       role,
		RegisteredClaims: jwt.RegisteredClaims{
			Issuer:    tokenIssuer,
			Subject:   userID,
			Audience:  jwt.ClaimStrings{tokenAudience},
			ExpiresAt: jwt.NewNumericDate(exp),
			IssuedAt:  jwt.NewNumericDate(now),
			NotBefore: jwt.NewNumericDate(now),
		},
	}
	signed, err := jwt.NewWithClaims(jwt.SigningMethodHS256, claims).SignedString(jwtSecret())
	return signed, exp, err
}

func deny(c *gin.Context, msg string) {
	c.AbortWithStatusJSON(401, gin.H{"ok": false, "error": msg})
}

// Wrap = middleware autentikasi: validasi JWT, lalu cek ulang user di database
// (akun yang dinonaktifkan langsung ditolak walau token belum kedaluwarsa).
func Wrap(db *gorm.DB) gin.HandlerFunc {
	return func(c *gin.Context) {
		h := c.GetHeader("Authorization")
		if len(h) < 8 || len(h) > maxTokenLength || !strings.EqualFold(h[:7], "Bearer ") {
			deny(c, "Sesi tidak valid atau kosong. Silakan masuk kembali.")
			return
		}

		claims := &Claims{}
		token, err := jwt.ParseWithClaims(strings.TrimSpace(h[7:]), claims,
			func(t *jwt.Token) (interface{}, error) { return jwtSecret(), nil },
			jwt.WithValidMethods([]string{"HS256"}), // tolak alg=none / alg confusion
			jwt.WithIssuer(tokenIssuer),
			jwt.WithAudience(tokenAudience),
			jwt.WithExpirationRequired(),
			jwt.WithLeeway(30*time.Second),
		)
		if err != nil || !token.Valid || claims.UserID == "" {
			deny(c, "Token kedaluwarsa atau dimanipulasi.")
			return
		}

		var user model.User
		if err := db.First(&user, "id = ? AND active = ?", claims.UserID, true).Error; err != nil {
			deny(c, "Akun tidak ditemukan atau telah dinonaktifkan.")
			return
		}

		// Peran & RS selalu diambil dari database, bukan dari klaim token.
		c.Set("user", user)
		c.Next()
	}
}
