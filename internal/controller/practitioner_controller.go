package controller

import (
	"slices"

	"github.com/gin-gonic/gin"
	"gorm.io/gorm"

	"github.com/spkd/medcredix/internal/model"
)

// GetPractitioners menampilkan daftar tenaga kesehatan, atau detail satu orang bila ada ?id= (GET /api/practitioners).
func GetPractitioners(db *gorm.DB) gin.HandlerFunc {
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
