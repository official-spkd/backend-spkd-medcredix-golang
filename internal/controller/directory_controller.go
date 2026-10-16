package controller

import (
	"net/http"

	"github.com/gin-gonic/gin"
	"gorm.io/gorm"

	"github.com/spkd/medcredix/internal/auth"
	"github.com/spkd/medcredix/internal/direktori"
	"github.com/spkd/medcredix/internal/model"
	"github.com/spkd/medcredix/internal/wilayah"
)

// GetHospitalDirectory mengirim daftar RS Kemenkes di satu kabupaten/kota untuk form Tambah RS
// (GET /api/hospital-directory?province=...&city=...). Khusus Super Admin.
// RS yang kodenya sudah terdaftar di MedCredix ditandai registered=true.
func GetHospitalDirectory(db *gorm.DB) gin.HandlerFunc {
	return func(c *gin.Context) {
		user, _ := auth.CurrentUser(c)
		if user.Role != "super_admin" {
			fail(c, http.StatusForbidden, "Daftar RS hanya untuk SPKD Super Admin.")
			return
		}
		code := wilayah.Code(c.Query("province"), c.Query("city"))
		if code == "" {
			fail(c, http.StatusBadRequest, "Pilih provinsi dan kabupaten/kota dari daftar.")
			return
		}
		list := direktori.ByCity(code)
		codes := make([]string, 0, len(list))
		for _, h := range list {
			codes = append(codes, h.Code)
		}
		var taken []string
		if len(codes) > 0 {
			db.Model(&model.Hospital{}).Where("code IN ?", codes).Pluck("code", &taken)
		}
		reg := map[string]bool{}
		for _, t := range taken {
			reg[t] = true
		}
		rows := make([]J, 0, len(list))
		for _, h := range list {
			rows = append(rows, J{
				"code": h.Code, "name": h.Name, "class": h.Class, "type": h.Type, "ownership": h.Ownership,
				"beds": h.Beds, "address": h.Address, "registered": reg[h.Code],
			})
		}
		ok(c, J{"hospitals": rows, "source": "SIRS Kemenkes 2024", "total": direktori.Total()})
	}
}
