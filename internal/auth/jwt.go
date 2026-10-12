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
			ID:        model.NewID("jti"), // dipakai untuk mencabut token saat logout
		},
	}
	signed, err := jwt.NewWithClaims(jwt.SigningMethodHS256, claims).SignedString(jwtSecret())
	return signed, exp, err
}

// ---------- daftar token yang sudah logout ----------
// Disimpan di memori sampai token kedaluwarsa. Catatan: hilang saat server restart dan
// tidak dibagi antar-instance; untuk banyak server pindahkan ke Redis / tabel database.

var (
	revokedMu sync.Mutex
	revoked   = map[string]time.Time{} // jti -> waktu kedaluwarsa token
)

// Revoke mencabut token (dipanggil saat logout).
func Revoke(jti string, exp time.Time) {
	if jti == "" {
		return
	}
	revokedMu.Lock()
	defer revokedMu.Unlock()
	now := time.Now()
	for k, t := range revoked { // buang entri yang tokennya sudah kedaluwarsa
		if now.After(t) {
			delete(revoked, k)
		}
	}
	revoked[jti] = exp
}

func isRevoked(jti string) bool {
	revokedMu.Lock()
	defer revokedMu.Unlock()
	_, ok := revoked[jti]
	return ok
}

// CurrentClaims mengambil klaim token yang sudah divalidasi oleh Wrap().
func CurrentClaims(c *gin.Context) (*Claims, bool) {
	v, ok := c.Get("claims")
	if !ok {
		return nil, false
	}
	cl, ok := v.(*Claims)
	return cl, ok
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
		if isRevoked(claims.ID) {
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

		// Peran & RS selalu diambil dari database, bukan dari klaim token.
		c.Set("user", user)
		c.Set("claims", claims)
		c.Next()
	}
}
