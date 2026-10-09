package api

import (
	"fmt"
	"net/http"
	"slices"
	"time"

	"github.com/gin-gonic/gin"
	"gorm.io/gorm"

	"github.com/spkd/medcredix/internal/audit"
	"github.com/spkd/medcredix/internal/auth"
	"github.com/spkd/medcredix/internal/model"
)

// ---------- Tenaga medis ----------

func practitionersGetHandler(db *gorm.DB) gin.HandlerFunc {
	return func(c *gin.Context) {
		_, hid, good := feCtx(c, db)
		if !good {
			return
		}
		if id := c.Query("id"); id != "" {
			var p model.Practitioner
			if notFound(c, db.Where("id = ? AND hospital_id = ?", id, hid).First(&p).Error, "Praktisi tidak ditemukan.") {
				return
			}
			var creds []model.PractitionerCredential
			db.Where("practitioner_id = ? AND hospital_id = ?", id, hid).Order("type").Find(&creds)
			ids := make([]string, 0, len(creds))
			credentials := make([]J, 0, len(creds))
			for _, cr := range creds {
				ids = append(ids, cr.ID)
				credentials = append(credentials, feCredential(cr, p.FullName))
			}
			documents := []J{}
			if len(ids) > 0 {
				var docs []model.CredentialDocument
				db.Where("credential_id IN ?", ids).Order("file_name").Find(&docs)
				for _, d := range docs {
					documents = append(documents, feDocument(d, hid))
				}
			}
			var standard interface{}
			if p.Specialty != nil {
				var s model.SpecialtyStandard
				if db.Where("hospital_id = ? AND specialty = ?", hid, *p.Specialty).First(&s).Error == nil {
					standard = J{"id": s.ID, "hospitalId": s.HospitalID, "rsClass": sv(s.RsClass),
						"specialty": sv(s.Specialty), "required": iv(s.Required), "note": nil}
				}
			}
			ok(c, J{"practitioner": fePractitioner(p, practitionerStatus(creds)),
				"credentials": credentials, "documents": documents, "standard": standard})
			return
		}

		q := db.Where("hospital_id = ?", hid)
		if s := c.Query("q"); s != "" {
			like := "%" + s + "%"
			q = q.Where("full_name ILIKE ? OR specialty ILIKE ? OR sip_number ILIKE ? OR str_number ILIKE ?", like, like, like, like)
		}
		if v := c.Query("prof"); v != "" {
			q = q.Where("profession = ?", v)
		}
		if v := c.Query("dept"); v != "" {
			q = q.Where("department ILIKE ?", "%"+v+"%")
		}
		var rows []model.Practitioner
		if dbFail(c, q.Order("full_name").Find(&rows).Error) {
			return
		}
		var creds []model.PractitionerCredential
		db.Where("hospital_id = ?", hid).Find(&creds)
		byP := map[string][]model.PractitionerCredential{}
		for _, cr := range creds {
			byP[cr.PractitionerID] = append(byP[cr.PractitionerID], cr)
		}
		status := c.Query("status")
		out := []J{}
		depts := []string{}
		for _, p := range rows {
			st := practitionerStatus(byP[p.ID])
			if status != "" && st != status {
				continue
			}
			out = append(out, fePractitioner(p, st))
			d := strOr(p.Department, "—")
			if !slices.Contains(depts, d) {
				depts = append(depts, d)
			}
		}
		ok(c, J{"practitioners": out, "depts": depts, "total": len(out)})
	}
}

// ---------- Kredensial ----------

func credentialsGetHandler(db *gorm.DB) gin.HandlerFunc {
	return func(c *gin.Context) {
		_, hid, good := feCtx(c, db)
		if !good {
			return
		}
		q := db.Where("hospital_id = ?", hid)
		if v := c.Query("status"); v != "" {
			q = q.Where("status = ?", codeOf(v))
		}
		if v := c.Query("type"); v != "" {
			q = q.Where("type = ?", v)
		}
		var rows []model.PractitionerCredential
		if dbFail(c, q.Order("expiry_date ASC NULLS FIRST").Find(&rows).Error) {
			return
		}
		var ps []model.Practitioner
		db.Where("hospital_id = ?", hid).Find(&ps)
		pmap := map[string]model.Practitioner{}
		for _, p := range ps {
			pmap[p.ID] = p
		}
		term := c.Query("q")
		out := []J{}
		for _, cr := range rows {
			p, has := pmap[cr.PractitionerID]
			j := feCredential(cr, p.FullName)
			if term != "" && !likeAny(term, j["name"].(string), str(cr.Number)) {
				continue
			}
			j["practitionerName"], j["profession"], j["specialty"] = "—", "—", nil
			if has {
				j["practitionerName"], j["profession"], j["specialty"] = p.FullName, strOr(p.Profession, "—"), sv(p.Specialty)
			}
			out = append(out, j)
		}
		ok(c, J{"credentials": out, "total": len(out)})
	}
}

// credentialsPatchHandler: alur kredensial dari credTransitions (status asal divalidasi + SoD).
func credentialsPatchHandler(db *gorm.DB) gin.HandlerFunc {
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
		if body.ID == "" {
			fail(c, http.StatusBadRequest, "id dan action wajib diisi.")
			return
		}
		t, known := credTransitions[body.Action]
		if !known {
			fail(c, http.StatusBadRequest, "Aksi tidak dikenal.")
			return
		}
		var cred model.PractitionerCredential
		if notFound(c, db.Where("id = ? AND hospital_id = ?", body.ID, hid).First(&cred).Error, "Kredensial tidak ditemukan.") {
			return
		}
		if !requirePerm(c, db, user, hid, t.perm, "practitioner_credentials", cred.ID) {
			return
		}
		if !slices.Contains(t.from, cred.Status) {
			fail(c, http.StatusConflict, fmt.Sprintf("Aksi %s tidak dapat dilakukan dari status %q.", body.Action, label(cred.Status)))
			return
		}
		if body.Action == "approve" && cred.VerifiedByID != nil && *cred.VerifiedByID == user.ID {
			fail(c, http.StatusForbidden, "Pemverifikasi tidak boleh menyetujui kredensial yang sama.")
			return
		}
		now := time.Now()
		updates := map[string]interface{}{"status": t.to}
		switch body.Action {
		case "verify", "review":
			updates["verified_by_id"], updates["verified_at"] = user.ID, now
		case "approve":
			updates["approved_by_id"], updates["approved_at"] = user.ID, now
		case "activate":
			updates["version"] = gorm.Expr("version + 1")
		}
		if dbFail(c, audit.UpdateWithAudit(db, user, hid, "practitioner_credentials", cred.ID, updates, metaOf(c))) {
			return
		}
		db.First(&cred, "id = ?", cred.ID)
		var p model.Practitioner
		db.First(&p, "id = ?", cred.PractitionerID)
		ok(c, feCredential(cred, p.FullName))
	}
}

// ---------- Prasyarat P1–P6 ----------

func requirementsGetHandler(db *gorm.DB) gin.HandlerFunc {
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

func requirementsPatchHandler(db *gorm.DB) gin.HandlerFunc {
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
		if body.Action == "verify" {
			perm = auth.PermVerify
		}
		if !requirePerm(c, db, user, hid, perm, "requirements", r.ID) {
			return
		}
		updates := map[string]interface{}{}
		switch body.Action {
		case "submit":
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
