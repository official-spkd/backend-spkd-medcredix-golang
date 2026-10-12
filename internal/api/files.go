package api

import (
	"fmt"
	"log"
	"mime/multipart"
	"net/http"
	"net/url"
	"path/filepath"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/spkd/medcredix/internal/audit"
	"github.com/spkd/medcredix/internal/auth"
	"github.com/spkd/medcredix/internal/model"
	"github.com/spkd/medcredix/internal/storage"
	"gorm.io/gorm"
)

// uploadLimit = batas body untuk endpoint unggah (berkas 10 MB + field form).
const uploadLimit = storage.MaxSize + 1<<20

// saveUpload menyimpan berkas dari field "file". required=false: boleh tanpa berkas.
// Mengembalikan kunci penyimpanan ("" bila tidak ada berkas).
func saveUpload(c *gin.Context, required bool) (key, mime, name string, size int, okay bool) {
	fh, err := c.FormFile("file")
	if err != nil {
		if err == http.ErrMissingFile && !required {
			return "", "", "", 0, true
		}
		if strings.Contains(err.Error(), "too large") {
			fail(c, http.StatusRequestEntityTooLarge, storage.ErrTooLarge.Error())
		} else if err == http.ErrMissingFile {
			fail(c, http.StatusBadRequest, "Pilih berkas dokumen (PDF, JPG, atau PNG).")
		} else {
			fail(c, http.StatusBadRequest, "Berkas tidak terbaca.")
		}
		return "", "", "", 0, false
	}
	if fh.Size > storage.MaxSize {
		fail(c, http.StatusRequestEntityTooLarge, storage.ErrTooLarge.Error())
		return "", "", "", 0, false
	}
	f, err := fh.Open()
	if err != nil {
		fail(c, http.StatusBadRequest, "Berkas tidak terbaca.")
		return "", "", "", 0, false
	}
	defer func(f multipart.File) { f.Close() }(f)
	key, mime, size, err = storage.Save(f)
	if err != nil {
		if storage.IsUserError(err) {
			fail(c, http.StatusBadRequest, err.Error())
		} else {
			log.Println("simpan berkas:", err)
			fail(c, http.StatusInternalServerError, "Gagal menyimpan berkas.")
		}
		return "", "", "", 0, false
	}
	return key, mime, cleanName(fh.Filename, mime), size, true
}

// cleanName: nama asli tanpa path dan karakter aneh, ekstensi disesuaikan isi berkas.
func cleanName(name, mime string) string {
	name = filepath.Base(strings.ReplaceAll(name, `\`, "/"))
	name = strings.Map(func(r rune) rune {
		if r < 32 || strings.ContainsRune(`"<>:|?*/`, r) {
			return -1
		}
		return r
	}, name)
	base := strings.TrimSuffix(name, filepath.Ext(name))
	if base == "" || base == "." {
		base = "dokumen"
	}
	if len(base) > 120 {
		base = base[:120]
	}
	return base + storage.Ext(mime)
}

// sendFile mendekripsi lalu mengirim berkas untuk dibuka di browser.
func sendFile(c *gin.Context, key, mime, name string) {
	data, err := storage.Open(key)
	if err != nil {
		if err == storage.ErrNotFound {
			fail(c, http.StatusNotFound, "Berkas belum diunggah atau sudah dihapus.")
		} else {
			log.Println("buka berkas:", err)
			fail(c, http.StatusInternalServerError, "Berkas tidak dapat dibuka.")
		}
		return
	}
	if mime == "" {
		mime = http.DetectContentType(data)
	}
	c.Header("Content-Disposition", "inline; filename*=UTF-8''"+url.PathEscape(name))
	c.Header("Cache-Control", "no-store")
	c.Data(http.StatusOK, mime, data)
}

// POST /api/requirements/upload  (multipart: id, docNumber, issuedAt, docExpiry, file)
// Simpan dokumen prasyarat dan ajukan verifikasi. Berkas wajib bila belum pernah diunggah.
func requirementsUploadHandler(db *gorm.DB) gin.HandlerFunc {
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

// GET /api/requirements/file?id=
func requirementsFileHandler(db *gorm.DB) gin.HandlerFunc {
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

// GET /api/documents — semua dokumen kredensial di RS aktif.
func documentsGetHandler(db *gorm.DB) gin.HandlerFunc {
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

// POST /api/documents/upload  (multipart: credentialId, file)
func documentsUploadHandler(db *gorm.DB) gin.HandlerFunc {
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

// GET /api/documents/file?id=
func documentsFileHandler(db *gorm.DB) gin.HandlerFunc {
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
