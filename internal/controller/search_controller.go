package controller

import (
	"strings"

	"github.com/gin-gonic/gin"
	"gorm.io/gorm"

	"github.com/spkd/medcredix/internal/model"
)

// Search mencari di praktisi, kredensial, risiko, klaim, aksi, regulasi, evidence, dan peringatan sekaligus (GET /api/search?q=...).
func Search(db *gorm.DB) gin.HandlerFunc {
	return func(c *gin.Context) {
		_, hid, good := feCtx(c, db)
		if !good {
			return
		}
		term := strings.TrimSpace(c.Query("q"))
		if len([]rune(term)) < 2 {
			ok(c, J{"results": []J{}, "term": term})
			return
		}
		like := "%" + term + "%"
		results := []J{}
		add := func(typ, icon, lbl, sub, view, id string) {
			results = append(results, J{"type": typ, "icon": icon, "label": lbl, "sub": sub, "view": view, "id": id})
		}

		var ps []model.Practitioner
		db.Where("hospital_id = ? AND (full_name ILIKE ? OR specialty ILIKE ? OR sip_number ILIKE ? OR str_number ILIKE ?)",
			hid, like, like, like, like).Limit(5).Find(&ps)
		for _, p := range ps {
			sub := strOr(p.Profession, "—")
			if p.Specialty != nil {
				sub += " — " + *p.Specialty
			}
			add("Praktisi", "Users", p.FullName, sub, "cred-practitioners", p.ID)
		}
		var cs []model.PractitionerCredential
		db.Where("hospital_id = ? AND (type ILIKE ? OR number ILIKE ?)", hid, like, like).Limit(5).Find(&cs)
		for _, cr := range cs {
			add("Kredensial", "IdCard", cr.Type+" "+str(cr.Number), cr.Type+" · "+label(cr.Status), "cred-credentials", cr.ID)
		}
		var rs []model.Risk
		db.Where("hospital_id = ? AND (title ILIKE ? OR code ILIKE ?)", hid, like, like).Limit(5).Find(&rs)
		for _, r := range rs {
			add("Risiko", "Radar", r.Title, r.Code+" · "+label(r.Category)+" · "+label(r.Level), "risk-radar", r.ID)
		}
		var cls []model.Claim
		db.Where("hospital_id = ? AND (claim_number ILIKE ? OR sep_number ILIKE ? OR inacbg_code ILIKE ?)",
			hid, like, like, like).Limit(5).Find(&cls)
		for _, cl := range cls {
			add("Klaim", "FileCheck2", cl.ClaimNumber, str(cl.INACBGCode)+" · "+label(str(cl.Status)), "claim-anomalies", cl.ID)
		}
		var as []model.Action
		db.Where("hospital_id = ? AND (title ILIKE ? OR code ILIKE ? OR owner ILIKE ?)", hid, like, like, like).Limit(5).Find(&as)
		for _, a := range as {
			add("Aksi", "CheckCircle2", a.Title, a.Code+" · "+label(a.Status)+" · "+strOr(a.Owner, "Belum ditugaskan"), "actions", a.ID)
		}
		var regs []model.Regulation
		db.Where("title ILIKE ? OR number ILIKE ? OR code ILIKE ?", like, like, like).Limit(4).Find(&regs)
		for _, r := range regs {
			add("Regulasi", "Scale", r.Title, str(r.Number), "compliance", r.ID)
		}
		var ev []model.EvidenceItem
		db.Where("hospital_id = ? AND (metric ILIKE ? OR ref_id ILIKE ?)", hid, like, like).Limit(4).Find(&ev)
		for _, e := range ev {
			add("Bukti", "Fingerprint", strOr(e.Metric, "Bukti data"), strOr(e.SourceSystem, "—"), "evidence", e.ID)
		}
		var ws []model.EarlyWarning
		db.Where("hospital_id = ? AND (title ILIKE ? OR code ILIKE ?)", hid, like, like).Limit(4).Find(&ws)
		for _, w := range ws {
			add("Peringatan", "AlertTriangle", w.Title, w.Code+" · "+label(w.Severity), "early-warnings", w.ID)
		}
		ok(c, J{"results": results, "term": term})
	}
}
