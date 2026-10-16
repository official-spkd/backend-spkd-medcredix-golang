package controller

import (
	"encoding/json"
	"errors"
	"net/http"
	"regexp"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	"golang.org/x/crypto/bcrypt"
	"gorm.io/gorm"

	"github.com/spkd/medcredix/internal/audit"
	"github.com/spkd/medcredix/internal/auth"
	"github.com/spkd/medcredix/internal/model"
)

// Kelola pengguna (POST/PATCH /api/users) dan simpan Profil RS hanya untuk Admin RS & Super Admin.
// Izin CONFIGURE saja tidak cukup: IT / SIMRS punya CONFIGURE untuk integrasi, bukan untuk akun & profil RS.
func isHospitalAdmin(u model.User) bool {
	return u.Role == "super_admin" || u.Role == "admin_rs"
}

// requireHospitalAdmin menolak (403) dan mencatat ke audit bila pelaku bukan Admin RS / Super Admin.
func requireHospitalAdmin(c *gin.Context, db *gorm.DB, user model.User, hid, entity, entityID, what string) bool {
	if isHospitalAdmin(user) {
		return true
	}
	_ = audit.Log(db, user, hid, "akses_ditolak", entity, entityID, "Aksi ditolak: "+what+" hanya untuk Admin RS / Super Admin", metaOf(c))
	fail(c, http.StatusForbidden, "Hanya Admin RS dan Super Admin yang dapat "+what+".")
	return false
}

const minPassLength = 8

var emailRe = regexp.MustCompile(`^[^\s@]+@[^\s@]+\.[^\s@]+$`)

func feUser(u model.User) J {
	return J{
		"id": u.ID, "name": u.Name, "email": u.Email, "role": u.Role, "roleLabel": strOr(ptrOrNil(auth.RoleLabels[u.Role]), u.Role),
		"title": sv(u.Title), "active": u.Active, "lastLogin": tv(u.LastLogin), "hospitalId": sv(u.HospitalID),
	}
}

// userSnapshot: isi audit untuk tabel users, tanpa password_hash.
// Peran disimpan sebagai label (mis. "HR / SDM") agar mudah dibaca di Audit Trail.
func userSnapshot(u model.User, passwordNote string) json.RawMessage {
	b, _ := json.Marshal(map[string]interface{}{
		"id": u.ID, "hospital_id": u.HospitalID, "name": u.Name, "email": u.Email, "title": u.Title,
		"peran": strOr(ptrOrNil(auth.RoleLabels[u.Role]), u.Role), "active": u.Active, "kata_sandi": passwordNote,
	})
	return b
}

func auditUser(tx *gorm.DB, actor model.User, hid, action, desc string, before, after json.RawMessage, meta audit.Meta) error {
	et, id := "users", ""
	var m map[string]interface{}
	if json.Unmarshal(after, &m) == nil {
		id, _ = m["id"].(string)
	}
	entry := model.AuditLog{
		ID: model.NewID("aud"), HospitalID: ptrOrNil(hid), ActorName: actor.Name, ActorRole: actor.Role,
		Action: action, EntityType: &et, EntityID: &id, Description: &desc,
		BeforeValue: before, AfterValue: after, IP: ptrOrNil(meta.IP), CorrelationID: ptrOrNil(meta.CorrelationID),
		CreatedAt: time.Now(),
	}
	return tx.Create(&entry).Error
}

// validRole: peran harus dikenal; hanya Super Admin yang boleh memberi peran Super Admin.
func validRole(actor model.User, role string) string {
	if _, known := auth.RolePermissions[role]; !known {
		return "Peran tidak dikenal."
	}
	if role == "super_admin" && actor.Role != "super_admin" {
		return "Hanya Super Admin yang dapat memberi peran SPKD Super Admin."
	}
	return ""
}

func cleanEmail(v string) (string, string) {
	e := strings.ToLower(strings.TrimSpace(v))
	if e == "" {
		return "", "Email wajib diisi."
	}
	if len(e) > maxEmailLength || !emailRe.MatchString(e) {
		return "", "Format email tidak valid."
	}
	return e, ""
}

func checkPassword(p string) string {
	if len(p) < minPassLength {
		return "Kata sandi minimal 8 karakter."
	}
	if len(p) > maxPassLength {
		return "Kata sandi maksimal 72 karakter."
	}
	return ""
}

func emailTaken(db *gorm.DB, email, exceptID string) bool {
	var n int64
	q := db.Model(&model.User{}).Where("LOWER(email) = ?", email)
	if exceptID != "" {
		q = q.Where("id <> ?", exceptID)
	}
	q.Count(&n)
	return n > 0
}

// PostUsers membuat akun baru di RS aktif (POST /api/users).
// Body: {"name","email","role","title","password"}
func PostUsers(db *gorm.DB) gin.HandlerFunc {
	return func(c *gin.Context) {
		user, hid, good := feCtx(c, db)
		if !good {
			return
		}
		if !requireHospitalAdmin(c, db, user, hid, "users", "", "menambah pengguna") {
			return
		}
		var body struct {
			Name     string  `json:"name"`
			Email    string  `json:"email"`
			Role     string  `json:"role"`
			Title    *string `json:"title"`
			Password string  `json:"password"`
		}
		if !bindBody(c, &body) {
			return
		}
		name := strings.TrimSpace(body.Name)
		if name == "" || len(name) > 150 {
			fail(c, http.StatusBadRequest, "Nama wajib diisi (maksimal 150 karakter).")
			return
		}
		email, msg := cleanEmail(body.Email)
		if msg == "" {
			msg = validRole(user, body.Role)
		}
		if msg == "" {
			msg = checkPassword(body.Password)
		}
		if msg != "" {
			fail(c, http.StatusBadRequest, msg)
			return
		}
		if emailTaken(db, email, "") {
			fail(c, http.StatusConflict, "Email sudah dipakai akun lain.")
			return
		}
		hash, err := bcrypt.GenerateFromPassword([]byte(body.Password), bcrypt.DefaultCost)
		if dbFail(c, err) {
			return
		}
		var title *string
		if body.Title != nil && strings.TrimSpace(*body.Title) != "" {
			t := strings.TrimSpace(*body.Title)
			title = &t
		}
		nu := model.User{
			ID: model.NewID("u"), HospitalID: &hid, Email: email, PasswordHash: string(hash),
			Name: name, Title: title, Role: body.Role, Active: true,
		}
		err = db.Transaction(func(tx *gorm.DB) error {
			if err := tx.Create(&nu).Error; err != nil {
				return err
			}
			return auditUser(tx, user, hid, "create", "Tambah pengguna "+nu.Name+" ("+auth.RoleLabels[nu.Role]+")",
				nil, userSnapshot(nu, "Dibuat"), metaOf(c))
		})
		if dbFail(c, err) {
			return
		}
		ok(c, feUser(nu))
	}
}

// PatchUsers mengubah akun (PATCH /api/users).
// Body: {"id", "name"?, "email"?, "title"?, "role"?, "active"?, "password"?}
// Field yang tidak dikirim tidak diubah. Akun nonaktif langsung tidak bisa memakai sesinya lagi.
func PatchUsers(db *gorm.DB) gin.HandlerFunc {
	return func(c *gin.Context) {
		user, hid, good := feCtx(c, db)
		if !good {
			return
		}
		var body struct {
			ID       string  `json:"id"`
			Name     *string `json:"name"`
			Email    *string `json:"email"`
			Title    *string `json:"title"`
			Role     *string `json:"role"`
			Active   *bool   `json:"active"`
			Password *string `json:"password"`
		}
		if !bindBody(c, &body) {
			return
		}
		if !requireHospitalAdmin(c, db, user, hid, "users", body.ID, "mengubah pengguna") {
			return
		}
		var target model.User
		if err := db.First(&target, "id = ?", body.ID).Error; err != nil {
			if errors.Is(err, gorm.ErrRecordNotFound) {
				fail(c, http.StatusNotFound, "Pengguna tidak ditemukan.")
				return
			}
			dbFail(c, err)
			return
		}
		// Admin RS hanya mengelola akun RS-nya sendiri. Super Admin juga boleh akun lintas RS (tanpa RS).
		inTenant := target.HospitalID != nil && *target.HospitalID == hid
		global := target.HospitalID == nil && user.Role == "super_admin"
		if !inTenant && !global {
			fail(c, http.StatusNotFound, "Pengguna tidak ditemukan.")
			return
		}
		if target.Role == "super_admin" && user.Role != "super_admin" {
			fail(c, http.StatusForbidden, "Akun SPKD Super Admin hanya dapat diubah oleh Super Admin.")
			return
		}
		self := target.ID == user.ID

		upd := map[string]interface{}{}
		after := target
		if body.Name != nil {
			n := strings.TrimSpace(*body.Name)
			if n == "" || len(n) > 150 {
				fail(c, http.StatusBadRequest, "Nama wajib diisi (maksimal 150 karakter).")
				return
			}
			upd["name"], after.Name = n, n
		}
		if body.Email != nil {
			e, msg := cleanEmail(*body.Email)
			if msg != "" {
				fail(c, http.StatusBadRequest, msg)
				return
			}
			if emailTaken(db, e, target.ID) {
				fail(c, http.StatusConflict, "Email sudah dipakai akun lain.")
				return
			}
			upd["email"], after.Email = e, e
		}
		if body.Title != nil {
			t := strings.TrimSpace(*body.Title)
			if t == "" {
				upd["title"], after.Title = nil, nil
			} else {
				upd["title"], after.Title = t, &t
			}
		}
		if body.Role != nil && *body.Role != target.Role {
			if self {
				fail(c, http.StatusBadRequest, "Anda tidak dapat mengubah peran akun sendiri.")
				return
			}
			if msg := validRole(user, *body.Role); msg != "" {
				fail(c, http.StatusBadRequest, msg)
				return
			}
			upd["role"], after.Role = *body.Role, *body.Role
		}
		if body.Active != nil && *body.Active != target.Active {
			if self {
				fail(c, http.StatusBadRequest, "Anda tidak dapat menonaktifkan akun sendiri.")
				return
			}
			upd["active"], after.Active = *body.Active, *body.Active
		}
		pwNote := "—"
		if body.Password != nil && *body.Password != "" {
			if msg := checkPassword(*body.Password); msg != "" {
				fail(c, http.StatusBadRequest, msg)
				return
			}
			hash, err := bcrypt.GenerateFromPassword([]byte(*body.Password), bcrypt.DefaultCost)
			if dbFail(c, err) {
				return
			}
			upd["password_hash"] = string(hash)
			pwNote = "Diubah"
		}
		if len(upd) == 0 {
			ok(c, feUser(target))
			return
		}

		var what []string
		for _, k := range []struct{ col, label string }{
			{"name", "nama"}, {"email", "email"}, {"title", "jabatan"}, {"role", "peran"}, {"active", "status"}, {"password_hash", "kata sandi"},
		} {
			if _, changed := upd[k.col]; changed {
				what = append(what, k.label)
			}
		}
		desc := "Ubah pengguna " + target.Name + ": " + strings.Join(what, ", ")
		if a, changed := upd["active"]; changed && len(upd) == 1 {
			desc = "Aktifkan pengguna " + target.Name
			if a == false {
				desc = "Nonaktifkan pengguna " + target.Name
			}
		}
		err := db.Transaction(func(tx *gorm.DB) error {
			if err := tx.Model(&model.User{}).Where("id = ?", target.ID).Updates(upd).Error; err != nil {
				return err
			}
			return auditUser(tx, user, hid, "update", desc, userSnapshot(target, "—"), userSnapshot(after, pwNote), metaOf(c))
		})
		if err != nil {
			if strings.Contains(err.Error(), "duplicate") || strings.Contains(err.Error(), "unique") {
				fail(c, http.StatusConflict, "Email sudah dipakai akun lain.")
				return
			}
			dbFail(c, err)
			return
		}
		ok(c, feUser(after))
	}
}
