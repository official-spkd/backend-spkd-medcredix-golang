package controller

import (
	"fmt"
	"net/http"
	"time"

	"github.com/gin-gonic/gin"
	"gorm.io/gorm"

	"github.com/spkd/medcredix/internal/audit"
	"github.com/spkd/medcredix/internal/auth"
	"github.com/spkd/medcredix/internal/model"
	"github.com/spkd/medcredix/internal/storage"
)

// GetDocuments menampilkan semua dokumen kredensial di RS aktif (GET /api/documents).
func GetDocuments(db *gorm.DB) gin.HandlerFunc {
	return func(c *gin.Context) {
		_, hid, good := feCtx(c, db)
		if !good {
			return
		}
		var docs []model.CredentialDocument
		err := db.Joins("JOIN practitioner_credentials pc ON pc.id = credential_documents.credential_id").
			Where("pc.hospital_id = ?", hid).Order("credential_documents.uploaded_at DESC NULLS LAST, credential_documents.file_name").
			Find(&docs).Error
		if dbFail(c, err) {
			return
		}
		out := make([]J, 0, len(docs))
		for _, d := range docs {
			out = append(out, feDocument(d, hid))
		}
		ok(c, J{"documents": out})
	}
}

// UploadDocument mengunggah dokumen untuk satu kredensial (POST /api/documents/upload).
// Form multipart: credentialId, file.
func UploadDocument(db *gorm.DB) gin.HandlerFunc {
	return func(c *gin.Context) {
		user, hid, good := feCtx(c, db)
		if !good {
			return
		}
		var cr model.PractitionerCredential
		if notFound(c, db.Where("id = ? AND hospital_id = ?", c.PostForm("credentialId"), hid).First(&cr).Error, "Kredensial tidak ditemukan.") {
			return
		}
		if !requirePerm(c, db, user, hid, auth.PermEdit, "credential_documents", cr.ID) {
			return
		}
		key, mime, name, size, okay := saveUpload(c, true)
		if !okay {
			return
		}
		now := time.Now()
		kb := (size + 1023) / 1024
		status := "menunggu_verifikasi"
		doc := model.CredentialDocument{
			ID: model.NewID("doc"), CredentialID: cr.ID, FileName: name, FileType: &mime,
			SizeKb: &kb, UploadedByID: &user.ID, Status: &status, StorageKey: &key, UploadedAt: &now,
		}
		err := db.Transaction(func(tx *gorm.DB) error {
			if err := tx.Create(&doc).Error; err != nil {
				return err
			}
			return audit.Log(tx, user, hid, "upload", "credential_documents", doc.ID,
				fmt.Sprintf("Mengunggah %s untuk kredensial %s", name, cr.Type), metaOf(c))
		})
		if err != nil {
			storage.Remove(key)
			dbFail(c, err)
			return
		}
		ok(c, feDocument(doc, hid))
	}
}

// GetDocumentFile mengirim isi berkas dokumen (GET /api/documents/file?id=...).
func GetDocumentFile(db *gorm.DB) gin.HandlerFunc {
	return func(c *gin.Context) {
		user, hid, good := feCtx(c, db)
		if !good {
			return
		}
		var d model.CredentialDocument
		err := db.Joins("JOIN practitioner_credentials pc ON pc.id = credential_documents.credential_id").
			Where("credential_documents.id = ? AND pc.hospital_id = ?", c.Query("id"), hid).First(&d).Error
		if notFound(c, err, "Dokumen tidak ditemukan.") {
			return
		}
		if d.StorageKey == nil || *d.StorageKey == "" {
			fail(c, http.StatusNotFound, "Berkas dokumen ini belum diunggah.")
			return
		}
		audit.Log(db, user, hid, "view_document", "credential_documents", d.ID, "Membuka dokumen "+d.FileName, metaOf(c))
		sendFile(c, *d.StorageKey, str(d.FileType), d.FileName)
	}
}

// PatchDocuments memverifikasi atau menolak satu dokumen kredensial (PATCH /api/documents).
// Body: { id, action: "verify" | "reject" }. Hanya dokumen "menunggu_verifikasi" yang bisa diputus,
// dan butuh izin VERIFY (Petugas Kredensial, Komite Medis, Admin RS).
func PatchDocuments(db *gorm.DB) gin.HandlerFunc {
	return func(c *gin.Context) {
		user, hid, good := feCtx(c, db)
		if !good {
			return
		}
		var body struct {
			ID     string `json:"id"`
			Action string `json:"action"`
		}
		if !bindBody(c, &body) {
			return
		}
		next, known := map[string]string{"verify": "diverifikasi", "reject": "ditolak"}[body.Action]
		if body.ID == "" || !known {
			fail(c, http.StatusBadRequest, "id dan action (verify/reject) wajib diisi.")
			return
		}
		var doc model.CredentialDocument
		err := db.Joins("JOIN practitioner_credentials pc ON pc.id = credential_documents.credential_id").
			Where("credential_documents.id = ? AND pc.hospital_id = ?", body.ID, hid).First(&doc).Error
		if notFound(c, err, "Dokumen tidak ditemukan.") {
			return
		}
		if !requirePerm(c, db, user, hid, auth.PermVerify, "credential_documents", doc.ID) {
			return
		}
		if doc.Status == nil || *doc.Status != "menunggu_verifikasi" {
			fail(c, http.StatusConflict, fmt.Sprintf("Dokumen sudah berstatus %q.", label(sOr(doc.Status))))
			return
		}
		err = db.Transaction(func(tx *gorm.DB) error {
			if err := tx.Model(&model.CredentialDocument{}).Where("id = ?", doc.ID).Update("status", next).Error; err != nil {
				return err
			}
			return audit.Log(tx, user, hid, body.Action, "credential_documents", doc.ID,
				fmt.Sprintf("Dokumen %s: %s", doc.FileName, label(next)), metaOf(c))
		})
		if dbFail(c, err) {
			return
		}
		doc.Status = &next
		ok(c, feDocument(doc, hid))
	}
}

func sOr(p *string) string {
	if p == nil {
		return ""
	}
	return *p
}
