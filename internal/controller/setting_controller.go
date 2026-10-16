package controller

import (
	"encoding/json"
	"net/http"
	"strconv"
	"strings"

	"github.com/gin-gonic/gin"
	"gorm.io/gorm"

	"github.com/spkd/medcredix/internal/audit"
	"github.com/spkd/medcredix/internal/auth"
	"github.com/spkd/medcredix/internal/model"
	"github.com/spkd/medcredix/internal/wilayah"
)

var roleOrder = []string{"super_admin", "admin_rs", "hospital_mgmt", "credential_officer", "medical_committee",
	"hr_sdm", "auditor", "claims_team", "it_simrs", "viewer"}

// GetSettings menampilkan data RS, user, peran, izin, dan aturan notifikasi (GET /api/settings).
func GetSettings(db *gorm.DB) gin.HandlerFunc {
	return func(c *gin.Context) {
		_, hid, good := feCtx(c, db)
		if !good {
			return
		}
		hospital, err := loadHospital(db, hid)
		if notFound(c, err, "Data tidak ditemukan.") {
			return
		}
		var us []model.User
		db.Where("hospital_id = ? OR (hospital_id IS NULL AND role IN ?)", hid, []string{"super_admin", "auditor"}).
			Order("name").Find(&us)
		users := make([]J, 0, len(us))
		for _, u := range us {
			users = append(users, J{
				"id": u.ID, "name": u.Name, "email": u.Email, "role": u.Role, "roleLabel": strOr(ptrOrNil(auth.RoleLabels[u.Role]), u.Role),
				"title": sv(u.Title), "active": u.Active, "lastLogin": tv(u.LastLogin), "hospitalId": sv(u.HospitalID),
			})
		}
		roles := make([]J, 0, len(roleOrder))
		for _, k := range roleOrder {
			roles = append(roles, J{"key": k, "label": auth.RoleLabels[k], "permissions": auth.RolePermissions[k]})
		}
		var rs []model.AlertRule
		db.Where("hospital_id = ?", hid).Order("name").Find(&rs)
		rules := make([]J, 0, len(rs))
		for _, r := range rs {
			rules = append(rules, feAlertRule(r))
		}
		perms := []auth.Permission{auth.PermView, auth.PermCreate, auth.PermEdit, auth.PermDelete, auth.PermVerify,
			auth.PermApprove, auth.PermSign, auth.PermExport, auth.PermPrint, auth.PermAssign, auth.PermResolve,
			auth.PermReopen, auth.PermConfigure}
		ok(c, J{"hospital": hospital, "users": users, "roles": roles, "permissions": perms, "rules": rules})
	}
}

// PatchSettings mengubah data RS atau menyalakan/mematikan aturan notifikasi (PATCH /api/settings).
// Body: {"target":"hospital", ...} atau {"target":"alert-rule","id":"...","active":true}
func PatchSettings(db *gorm.DB) gin.HandlerFunc {
	return func(c *gin.Context) {
		user, hid, good := feCtx(c, db)
		if !good {
			return
		}
		if !requirePerm(c, db, user, hid, auth.PermConfigure, "settings", "") {
			return
		}
		var body struct {
			Target           string          `json:"target"`
			ID               string          `json:"id"`
			Active           *bool           `json:"active"`
			Name             *string         `json:"name"`
			Class            *string         `json:"class"`
			City             *string         `json:"city"`
			Province         *string         `json:"province"`
			District         *string         `json:"district"`
			Village          *string         `json:"village"`
			BedLicensed      json.RawMessage `json:"bedLicensed"` // dibaca mentah agar desimal/teks/kosong bisa ditolak dengan pesan jelas
			BPJSFacilityCode *string         `json:"bpjsFacilityCode"`
			SatusehatOrgID   *string         `json:"satusehatOrgId"`
		}
		if !bindBody(c, &body) {
			return
		}
		switch body.Target {
		case "hospital":
			upd := map[string]interface{}{}
			set := func(col string, v *string) {
				if v != nil {
					upd[col] = *v
				}
			}
			if body.Name != nil && *body.Name == "" {
				fail(c, http.StatusBadRequest, "Nama RS wajib diisi.")
				return
			}
			if body.Class != nil {
				cls, valid := parseHospitalClass(*body.Class)
				if !valid {
					fail(c, http.StatusBadRequest, "Kelas RS harus A, B, C, atau D.")
					return
				}
				upd["hospital_class"] = cls
			}
			// Wilayah dicek berurutan (provinsi > kab/kota > kecamatan > kelurahan/desa) bila ada yang diubah.
			// Nilai lama yang tidak diubah tetap diterima, supaya profil lama tetap bisa disimpan.
			var cur model.Hospital
			if notFound(c, db.First(&cur, "id = ?", hid).Error, "RS tidak ditemukan.") {
				return
			}
			pick := func(v, old *string) (string, bool) {
				if v == nil {
					return sv0(old), false
				}
				return strings.TrimSpace(*v), strings.TrimSpace(*v) != sv0(old)
			}
			prov, c1 := pick(body.Province, cur.Province)
			city, c2 := pick(body.City, cur.City)
			dist, c3 := pick(body.District, cur.District)
			vill, c4 := pick(body.Village, cur.Village)
			set("name", body.Name)
			if c1 || c2 || c3 || c4 {
				region, msg := wilayah.Resolve(prov, city, dist, vill)
				if msg != "" {
					fail(c, http.StatusBadRequest, msg)
					return
				}
				for i, col := range []string{"province", "city", "district", "village"} {
					upd[col] = ptrOrNil(region[i]) // simpan nama baku; tingkat yang kosong jadi NULL
				}
			}
			// Kode faskes BPJS hanya dicek bila diubah, supaya nilai lama tetap bisa disimpan.
			if body.BPJSFacilityCode != nil && strings.TrimSpace(*body.BPJSFacilityCode) != sv0(cur.BPJSFacilityCode) {
				code, msg := parseBPJSFacilityCode(body.BPJSFacilityCode)
				if msg != "" {
					fail(c, http.StatusBadRequest, msg)
					return
				}
				upd["bpjs_facility_code"] = code
			}
			set("satusehat_org_id", body.SatusehatOrgID)
			if len(body.BedLicensed) > 0 {
				beds, msg := parseBedLicensed(body.BedLicensed)
				if msg != "" {
					fail(c, http.StatusBadRequest, msg)
					return
				}
				upd["bed_licensed"] = beds
			}
			if len(upd) > 0 {
				if dbFail(c, audit.UpdateWithAudit(db, user, hid, "hospitals", hid, upd, metaOf(c))) {
					return
				}
			}
			h, err := loadHospital(db, hid)
			if notFound(c, err, "RS tidak ditemukan.") {
				return
			}
			ok(c, h)
		case "alert-rule":
			var r model.AlertRule
			if notFound(c, db.Where("id = ? AND hospital_id = ?", body.ID, hid).First(&r).Error, "Aturan tidak ditemukan.") {
				return
			}
			active := !r.Active
			if body.Active != nil {
				active = *body.Active
			}
			if dbFail(c, audit.UpdateWithAudit(db, user, hid, "alert_rules", r.ID, map[string]interface{}{"active": active}, metaOf(c))) {
				return
			}
			r.Active = active
			ok(c, feAlertRule(r))
		default:
			fail(c, http.StatusBadRequest, "Target tidak dikenal.")
		}
	}
}

// sv0: isi pointer string (kosong bila nil), sudah di-trim.
func sv0(p *string) string {
	if p == nil {
		return ""
	}
	return strings.TrimSpace(*p)
}

// parseHospitalClass: kelas RS hanya A, B, C, atau D (huruf kecil diterima lalu disimpan huruf besar).
func parseHospitalClass(v string) (string, bool) {
	cls := strings.ToUpper(strings.TrimSpace(v))
	switch cls {
	case "A", "B", "C", "D":
		return cls, true
	}
	return "", false
}

// parseBedLicensed: jumlah TT terlisensi wajib bilangan bulat > 0.
// Menolak kosong, null, 0, negatif, desimal (240.5 / "240,5"), dan teks.
func parseBedLicensed(raw json.RawMessage) (int, string) {
	v := strings.TrimSpace(string(raw))
	if v == "null" {
		return 0, "Jumlah TT terlisensi wajib diisi."
	}
	if strings.HasPrefix(v, `"`) {
		var s string
		if json.Unmarshal(raw, &s) != nil {
			return 0, "Jumlah TT terlisensi tidak valid."
		}
		v = strings.TrimSpace(s)
	}
	if v == "" {
		return 0, "Jumlah TT terlisensi wajib diisi."
	}
	n, err := strconv.Atoi(v)
	if err != nil {
		return 0, "Jumlah TT terlisensi harus bilangan bulat (tanpa koma atau desimal)."
	}
	if n <= 0 {
		return 0, "Jumlah TT terlisensi harus lebih dari 0."
	}
	return n, ""
}
