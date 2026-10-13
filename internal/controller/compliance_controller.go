package controller

import (
	"github.com/gin-gonic/gin"
	"gorm.io/gorm"

	"github.com/spkd/medcredix/internal/model"
)

func regStatus(s string) string {
	if s == "active" {
		return "Berlaku"
	}
	return label(s)
}

// GetCompliance menampilkan status kepatuhan regulasi RS (GET /api/compliance).
func GetCompliance(db *gorm.DB) gin.HandlerFunc {
	return func(c *gin.Context) {
		_, hid, good := feCtx(c, db)
		if !good {
			return
		}
		var regs []model.Regulation
		db.Order("year DESC NULLS LAST, code").Find(&regs)
		var vers []model.RegulationVersion
		db.Order("issued_at").Find(&vers)
		byReg := map[string][]J{}
		for _, v := range vers {
			byReg[v.RegulationID] = append(byReg[v.RegulationID], J{
				"id": v.ID, "regulationId": v.RegulationID, "version": v.Version, "issuedAt": tv(v.IssuedAt),
				"status": nil, "changeSummary": nil, "impactLevel": labelP(v.ImpactLevel), "supersededBy": nil,
			})
		}
		regMap := map[string]model.Regulation{}
		regulations := make([]J, 0, len(regs))
		for _, r := range regs {
			regMap[r.ID] = r
			vs := byReg[r.ID]
			if vs == nil {
				vs = []J{}
			}
			regulations = append(regulations, J{
				"id": r.ID, "code": r.Code, "title": r.Title, "number": str(r.Number), "authority": sv(r.Authority),
				"year": iv(r.Year), "category": nil, "status": regStatus(r.Status), "summary": nil,
				"source": nil, "effectiveAt": nil, "versions": vs,
			})
		}

		var ms []model.ComplianceMapping
		db.Where("hospital_id = ?", hid).Order("status, article").Find(&ms)
		count := map[string]int{}
		mappings := make([]J, 0, len(ms))
		for _, m := range ms {
			st := str(m.Status)
			count[st]++
			var reg interface{}
			if r, has := regMap[m.RegulationID]; has {
				reg = J{"code": r.Code, "title": r.Title, "number": str(r.Number), "category": nil}
			}
			mappings = append(mappings, J{
				"id": m.ID, "regulationId": m.RegulationID, "hospitalId": m.HospitalID, "article": str(m.Article),
				"requirement": strOr(m.Feature, "—"), "module": str(m.Module), "feature": sv(m.Feature),
				"dataField": nil, "validation": nil, "evidenceRef": nil, "auditRequired": true,
				"owner": str(m.Owner), "status": label(st), "lastReviewedAt": tv(m.ReviewedAt), "reviewer": nil,
				"regulation": reg,
			})
		}
		ok(c, J{
			"regulations": regulations, "mappings": mappings,
			"summary": J{"patuh": count["patuh"], "sebagian": count["sebagian"], "belum": count["belum"],
				"ditinjau": count["sedang_ditinjau"]},
			"total": len(mappings),
		})
	}
}
