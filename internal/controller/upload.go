package controller

import (
	"log"
	"mime/multipart"
	"net/http"
	"net/url"
	"path/filepath"
	"strings"

	"github.com/gin-gonic/gin"

	"github.com/spkd/medcredix/internal/storage"
)

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
			fail(c, http.StatusBadRequest, "Pilih berkas dokumen (PDF atau Word DOC/DOCX, maks 5 MB).")
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
