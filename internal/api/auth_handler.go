package api

import (
	"errors"
	"fmt"
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/gin-gonic/gin"
	"golang.org/x/crypto/bcrypt"
	"gorm.io/gorm"

	"github.com/spkd/medcredix/internal/audit"
	"github.com/spkd/medcredix/internal/auth"
	"github.com/spkd/medcredix/internal/model"
)

// ---------- pembatas percobaan login (fix F-23: brute force) ----------

const (
	maxLoginFails   = 5  // per IP+email
	maxIPLoginFails = 30 // per IP
	loginLockFor    = 15 * time.Minute
	loginWindow     = 15 * time.Minute
	maxLimiterKeys  = 50000
	maxEmailLength  = 254
	maxPassLength   = 72
)

type failState struct {
	count int
	first time.Time
	until time.Time
}

type loginLimiter struct {
	mu sync.Mutex
	m  map[string]*failState
}

func newLoginLimiter() *loginLimiter { return &loginLimiter{m: map[string]*failState{}} }

func (l *loginLimiter) blocked(key string) (bool, time.Duration) {
	l.mu.Lock()
	defer l.mu.Unlock()
	st := l.m[key]
	if st == nil || st.until.IsZero() {
		return false, 0
	}
	if time.Now().After(st.until) {
		delete(l.m, key)
		return false, 0
	}
	return true, time.Until(st.until)
}

func (l *loginLimiter) fail(key string, limit int) {
	l.mu.Lock()
	defer l.mu.Unlock()
	now := time.Now()
	if len(l.m) >= maxLimiterKeys {
		l.pruneLocked(now)
	}
	st := l.m[key]
	if st == nil || now.Sub(st.first) > loginWindow && st.until.IsZero() {
		st = &failState{first: now}
		l.m[key] = st
	}
	st.count++
	if st.count >= limit {
		st.until = now.Add(loginLockFor)
	}
}

// pruneLocked membuang entri yang sudah kedaluwarsa.
func (l *loginLimiter) pruneLocked(now time.Time) {
	for k, st := range l.m {
		if (!st.until.IsZero() && now.After(st.until)) || (st.until.IsZero() && now.Sub(st.first) > loginWindow) {
			delete(l.m, k)
		}
	}
}

func (l *loginLimiter) reset(key string) {
	l.mu.Lock()
	delete(l.m, key)
	l.mu.Unlock()
}

// dummyHash: dipakai saat email tidak ada, supaya waktu respons tetap sama.
var dummyHash, _ = bcrypt.GenerateFromPassword([]byte("medcredix-dummy-password-for-timing"), bcrypt.DefaultCost)

// ---------- POST /api/auth ----------

func loginHandler(db *gorm.DB) gin.HandlerFunc {
	lim := newLoginLimiter()
	return func(c *gin.Context) {
		var body struct {
			Email    string `json:"email"`
			Password string `json:"password"`
		}
		if err := c.ShouldBindJSON(&body); err != nil ||
			strings.TrimSpace(body.Email) == "" || body.Password == "" {
			fail(c, http.StatusBadRequest, "Email dan kata sandi wajib diisi.")
			return
		}
		email := strings.ToLower(strings.TrimSpace(body.Email))
		if len(email) > maxEmailLength || len(body.Password) > maxPassLength {
			fail(c, http.StatusUnauthorized, "Email atau kata sandi salah.")
			return
		}
		ip := c.ClientIP()
		key := ip + "|" + email
		ipKey := "ip|" + ip
		meta := metaOf(c)

		for _, k := range []string{key, ipKey} {
			if blocked, wait := lim.blocked(k); blocked {
				fail(c, http.StatusTooManyRequests,
					fmt.Sprintf("Terlalu banyak percobaan gagal. Coba lagi dalam %d menit.", int(wait.Minutes())+1))
				return
			}
		}

		var user model.User
		err := db.Where("email = ?", email).First(&user).Error
		if err != nil && !errors.Is(err, gorm.ErrRecordNotFound) {
			fail(c, http.StatusInternalServerError, "Terjadi kesalahan pada server.")
			return
		}
		// Pesan dan waktu sama untuk email/sandi salah.
		hash := []byte(user.PasswordHash)
		if err != nil {
			hash = dummyHash
		}
		if bcrypt.CompareHashAndPassword(hash, []byte(body.Password)) != nil || err != nil {
			lim.fail(key, maxLoginFails)
			lim.fail(ipKey, maxIPLoginFails)
			_ = audit.LogAnonymous(db, email, "login_gagal", "Percobaan login gagal", meta)
			fail(c, http.StatusUnauthorized, "Email atau kata sandi salah.")
			return
		}
		if !user.Active {
			_ = audit.LogAnonymous(db, email, "login_ditolak", "Akun nonaktif mencoba login", meta)
			fail(c, http.StatusForbidden, "Akun Anda telah dinonaktifkan. Hubungi Admin RS.")
			return
		}

		lim.reset(key)
		now := time.Now()
		db.Model(&model.User{}).Where("id = ?", user.ID).Update("last_login", now)

		hid := ""
		if user.HospitalID != nil {
			hid = *user.HospitalID
		}
		token, exp, err := auth.GenerateToken(user.ID, hid, user.Role)
		if err != nil {
			fail(c, http.StatusInternalServerError, "Gagal membuat sesi.")
			return
		}
		// Aktor tercatat lengkap (fix F-18: sebelumnya nama aktor kosong).
		_ = audit.Log(db, user, hid, "login", "users", user.ID, "Login berhasil", meta)

		ok(c, gin.H{"token": token, "expiresAt": exp, "user": sessionOf(user)})
	}
}
