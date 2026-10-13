package controller

import (
	"fmt"
	"net/http"
	"strings"

	"github.com/gin-gonic/gin"
	"gorm.io/gorm"

	"github.com/spkd/medcredix/internal/audit"
	"github.com/spkd/medcredix/internal/auth"
	"github.com/spkd/medcredix/internal/model"
	"github.com/spkd/medcredix/internal/storage"
)

// GetRequirements menampilkan daftar prasyarat wajib RS (GET /api/requirements).
func GetRequirements(db *gorm.DB) gin.HandlerFunc {
	return func(c *gin.Context) {
		_, hid, good := feCtx(c, db)
		if !good {
			return
		}
		var rows []model.Requirement
		if dbFail(c, db.Where("hospital_id = ?", hid).Order("code").Find(&rows).Error) {
			return
		}
		out := make([]J, 0, len(rows))
		for _, r := range rows {
			out = append(out, feRequirement(r))
		}
		ok(c, J{"requirements": out})
	}
}

// PatchRequirements mengubah prasyarat: update, submit, verify, atau invalidate (PATCH /api/requirements).
func PatchRequirements(db *gorm.DB) gin.HandlerFunc {
	return func(c *gin.Context) {
		user, hid, good := feCtx(c, db)
		if !good {
			return
		}
		var body struct {
			ID        string  `json:"id"`
			Action    string  `json:"action"`
			DocNumber *string `json:"docNumber"`
			IssuedAt  string  `json:"issuedAt"`
			DocExpiry string  `json:"docExpiry"`
			FileName  *string `json:"fileName"`
			Notes     *string `json:"notes"`
		}
		if !bindBody(c, &body) {
			return
		}
		var r model.Requirement
		if notFound(c, db.Where("id = ? AND hospital_id = ?", body.ID, hid).First(&r).Error, "Prasyarat tidak ditemukan.") {
			return
		}
		perm := auth.PermEdit
		if body.Action == "verify" || body.Action == "invalidate" {
			perm = auth.PermVerify
		}
		if !requirePerm(c, db, user, hid, perm, "requirements", r.ID) {
			return
		}
		updates := map[string]interface{}{}
		switch body.Action {
		case "submit":
			// Pengajuan wajib punya berkas (unggah lewat POST /requirements/upload).
			if r.FileKey == nil || *r.FileKey == "" {
				fail(c, http.StatusBadRequest, "Unggah berkas dokumen dulu sebelum diajukan.")
				return
			}
			updates["status"] = "menunggu_verifikasi"
		case "verify":
			if r.Status != "menunggu_verifikasi" {
				fail(c, http.StatusConflict, "Hanya prasyarat yang menunggu verifikasi yang bisa diverifikasi.")
				return
			}
			updates["status"] = "valid"
		case "invalidate":
			updates["status"] = "belum_dilengkapi"
		case "", "update":
		default:
			fail(c, http.StatusBadRequest, "Aksi tidak dikenal.")
			return
		}
		if body.DocNumber != nil {
			updates["doc_number"] = *body.DocNumber
		}
		if d := parseDate(body.IssuedAt); d != nil {
			updates["issued_at"] = *d
		}
		if d := parseDate(body.DocExpiry); d != nil {
			updates["doc_expiry"] = *d
		}
		if body.FileName != nil {
			updates["file_name"] = *body.FileName
		}
		if body.Notes != nil {
			updates["note"] = *body.Notes
		}
		if len(updates) == 0 {
			ok(c, feRequirement(r))
			return
		}
		if dbFail(c, audit.UpdateWithAudit(db, user, hid, "requirements", r.ID, updates, metaOf(c))) {
			return
		}
		db.First(&r, "id = ?", r.ID)
		ok(c, feRequirement(r))
	}
}

// UploadRequirementFile menyimpan dokumen prasyarat (POST /api/requirements/upload).
// Form multipart: id, docNumber, issuedAt, docExpiry, file.
// Simpan dokumen prasyarat dan ajukan verifikasi. Berkas wajib bila belum pernah diunggah.
func UploadRequirementFile(db *gorm.DB) gin.HandlerFunc {
	return func(c *gin.Context) {
		user, hid, good := feCtx(c, db)
		if !good {
			return
		}
		var r model.Requirement
		if notFound(c, db.Where("id = ? AND hospital_id = ?", c.PostForm("id"), hid).First(&r).Error, "Prasyarat tidak ditemukan.") {
			return
		}
		if !requirePerm(c, db, user, hid, auth.PermEdit, "requirements", r.ID) {
			return
		}
		docNumber := strings.TrimSpace(c.PostForm("docNumber"))
		if docNumber == "" {
			fail(c, http.StatusBadRequest, "Nomor dokumen wajib diisi.")
			return
		}
		if len(docNumber) > 80 {
			fail(c, http.StatusBadRequest, "Nomor dokumen maksimal 80 karakter.")
			return
		}
		hadFile := r.FileKey != nil && *r.FileKey != ""
		key, mime, name, size, okay := saveUpload(c, !hadFile)
		if !okay {
			return
		}

		updates := map[string]interface{}{"status": "menunggu_verifikasi", "doc_number": docNumber}
		if d := parseDate(c.PostForm("issuedAt")); d != nil {
			updates["issued_at"] = *d
		}
		if v := c.PostForm("docExpiry"); v != "" {
			d := parseDate(v)
			if d == nil {
				storage.Remove(key)
				fail(c, http.StatusBadRequest, "Tanggal kedaluwarsa tidak valid.")
				return
			}
			updates["doc_expiry"] = *d
		}
		if key != "" {
			updates["file_key"] = key
			updates["file_name"] = name
			updates["file_type"] = mime
			updates["file_size"] = size
		}
		if err := audit.UpdateWithAudit(db, user, hid, "requirements", r.ID, updates, metaOf(c)); err != nil {
			storage.Remove(key)
			dbFail(c, err)
			return
		}
		if key != "" && hadFile {
			storage.Remove(*r.FileKey) // berkas lama diganti
		}
		db.First(&r, "id = ?", r.ID)
		ok(c, feRequirement(r))
	}
}

// GetRequirementFile mengirim isi berkas prasyarat (GET /api/requirements/file?id=...).
func GetRequirementFile(db *gorm.DB) gin.HandlerFunc {
	return func(c *gin.Context) {
		user, hid, good := feCtx(c, db)
		if !good {
			return
		}
		var r model.Requirement
		if notFound(c, db.Where("id = ? AND hospital_id = ?", c.Query("id"), hid).First(&r).Error, "Prasyarat tidak ditemukan.") {
			return
		}
		if r.FileKey == nil || *r.FileKey == "" {
			fail(c, http.StatusNotFound, "Dokumen belum diunggah.")
			return
		}
		audit.Log(db, user, hid, "view_document", "requirements", r.ID,
			fmt.Sprintf("Membuka dokumen %s (%s)", r.Code, str(r.FileName)), metaOf(c))
		sendFile(c, *r.FileKey, str(r.FileType), str(r.FileName))
	}
}
