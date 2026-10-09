package api

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

// ---------- Pengaturan ----------

func settingsGetHandler(db *gorm.DB) gin.HandlerFunc {
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

// PATCH /settings  body: {"target":"hospital", ...} | {"target":"alert-rule","id":"...","active":true}
func settingsPatchHandler(db *gorm.DB) gin.HandlerFunc {
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

// ---------- Data intelligence: kualitas, trust, sumber data ----------

func dataGetHandler(db *gorm.DB) gin.HandlerFunc {
	return func(c *gin.Context) {
		_, hid, good := feCtx(c, db)
		if !good {
			return
		}
		typ := c.DefaultQuery("type", "all")

		var iss []model.DataQualityIssue
		db.Where("hospital_id = ?", hid).Order("status, id").Find(&iss)
		issues := make([]J, 0, len(iss))
		terbuka, anomaly := 0, 0
		for _, i := range iss {
			issues = append(issues, feDQIssue(i))
			if i.Status == "terbuka" {
				terbuka++
			}
			if i.Anomaly {
				anomaly++
			}
		}
		sortJ(issues, func(a, b J) bool {
			if a["status"] != b["status"] {
				return numStr(a["status"]) > numStr(b["status"]) // Terbuka dulu
			}
			return rank(codeOf(numStr(a["severity"]))) > rank(codeOf(numStr(b["severity"])))
		})
		if typ == "quality" {
			ok(c, J{"issues": issues, "summary": J{"terbuka": terbuka, "total": len(issues), "anomaly": anomaly}})
			return
		}

		_, intel, err := computeFE(db, hid)
		if err != nil {
			intelFail(c, hid, err)
			return
		}
		if typ == "trust" {
			ok(c, J{"trust": intel["trust"], "bands": trustBands, "bcds": intel["bcds"]})
			return
		}

		var ss []model.DataSource
		db.Where("hospital_id = ?", hid).Order("name").Find(&ss)
		sources := make([]J, 0, len(ss))
		srcIDs := make([]string, 0, len(ss))
		for _, s := range ss {
			sources = append(sources, feDataSource(s))
			srcIDs = append(srcIDs, s.ID)
		}
		jobs := []J{}
		if len(srcIDs) > 0 {
			var js []model.IntegrationJob
			db.Where("data_source_id IN ?", srcIDs).Order("id DESC").Limit(24).Find(&js)
			for _, j := range js {
				jobs = append(jobs, feJob(j, hid))
			}
		}
		var fs []model.Facility
		db.Where("hospital_id = ?", hid).Order("category, name").Find(&fs)
		facilities := make([]J, 0, len(fs))
		for _, f := range fs {
			facilities = append(facilities, feFacility(f))
		}
		var bs []model.BedStat
		db.Where("hospital_id = ?", hid).Order("bed_class, ward").Find(&bs)
		beds := make([]J, 0, len(bs))
		for _, b := range bs {
			beds = append(beds, feBed(b))
		}
		hospital, _ := loadHospital(db, hid)

		if typ == "sources" {
			ok(c, J{"sources": sources, "jobs": jobs, "facilities": facilities, "beds": beds, "hospital": hospital})
			return
		}
		ok(c, J{"issues": issues, "sources": sources, "jobs": jobs, "facilities": facilities, "beds": beds,
			"hospital": hospital, "trust": intel["trust"]})
	}
}
