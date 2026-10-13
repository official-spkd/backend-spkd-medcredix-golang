package controller

import (
	"net/http"

	"github.com/gin-gonic/gin"
	"gorm.io/gorm"

	"github.com/spkd/medcredix/internal/audit"
	"github.com/spkd/medcredix/internal/auth"
	"github.com/spkd/medcredix/internal/model"
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
				"title": sv(u.Title), "active": u.Active, "lastLogin": tv(u.LastLogin),
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
			Target           string  `json:"target"`
			ID               string  `json:"id"`
			Active           *bool   `json:"active"`
			Name             *string `json:"name"`
			Class            *string `json:"class"`
			City             *string `json:"city"`
			Province         *string `json:"province"`
			BedLicensed      *int    `json:"bedLicensed"`
			BPJSFacilityCode *string `json:"bpjsFacilityCode"`
			SatusehatOrgID   *string `json:"satusehatOrgId"`
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
			set("name", body.Name)
			set("hospital_class", body.Class)
			set("city", body.City)
			set("province", body.Province)
			set("bpjs_facility_code", body.BPJSFacilityCode)
			set("satusehat_org_id", body.SatusehatOrgID)
			if body.BedLicensed != nil {
				if *body.BedLicensed < 0 {
					fail(c, http.StatusBadRequest, "Jumlah TT tidak valid.")
					return
				}
				upd["bed_licensed"] = *body.BedLicensed
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
