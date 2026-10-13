package controller

import (
	"errors"
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	"golang.org/x/crypto/bcrypt"
	"gorm.io/gorm"

	"github.com/spkd/medcredix/internal/audit"
	"github.com/spkd/medcredix/internal/auth"
	"github.com/spkd/medcredix/internal/model"
)

// dummyHash: dipakai saat email tidak ada, supaya waktu respons tetap sama.
var dummyHash, _ = bcrypt.GenerateFromPassword([]byte("medcredix-dummy-password-for-timing"), bcrypt.DefaultCost)

// Login memeriksa email dan kata sandi lalu mengembalikan token JWT (POST /api/auth).
func Login(db *gorm.DB) gin.HandlerFunc {
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
		// Aktor tercatat lengkap di audit.
		_ = audit.Log(db, user, hid, "login", "users", user.ID, "Login berhasil", meta)

		ok(c, gin.H{"token": token, "expiresAt": exp, "user": sessionOf(user)})
	}
}

// Logout mencabut token yang sedang dipakai (POST /api/auth/logout).
// Token itu tidak bisa dipakai lagi walau belum 12 jam.
func Logout(db *gorm.DB) gin.HandlerFunc {
	return func(c *gin.Context) {
		user, _ := auth.CurrentUser(c)
		if cl, found := auth.CurrentClaims(c); found && cl.ExpiresAt != nil {
			auth.Revoke(cl.ID, cl.ExpiresAt.Time)
		}
		hid := ""
		if user.HospitalID != nil {
			hid = *user.HospitalID
		}
		_ = audit.Log(db, user, hid, "logout", "users", user.ID, "Logout", metaOf(c))
		ok(c, gin.H{"loggedOut": true})
	}
}
