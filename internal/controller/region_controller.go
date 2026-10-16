package controller

import (
	"net/http"

	"github.com/gin-gonic/gin"

	"github.com/spkd/medcredix/internal/wilayah"
)

// GetRegions mengirim daftar wilayah Indonesia untuk dropdown bertingkat (GET /api/regions).
// Tanpa parent = 38 provinsi; parent=kode provinsi = kabupaten/kota; parent=kode kab/kota = kecamatan;
// parent=kode kecamatan = kelurahan/desa. Data dari paket wilayah (Kepmendagri 2025, ter-embed di binary).
func GetRegions(c *gin.Context) {
	list := wilayah.Children(c.Query("parent"))
	if list == nil {
		fail(c, http.StatusNotFound, "Kode wilayah tidak ditemukan.")
		return
	}
	c.Header("Cache-Control", "private, max-age=86400")
	ok(c, list)
}
