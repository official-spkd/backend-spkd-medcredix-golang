package controller

import (
	"net/http"
	"strings"

	"github.com/gin-gonic/gin"
	"gorm.io/gorm"

	"github.com/spkd/medcredix/internal/audit"
	"github.com/spkd/medcredix/internal/auth"
	"github.com/spkd/medcredix/internal/model"
)

var anomalyStatus = map[string]string{"review": "ditinjau", "confirm": "terkonfirmasi_bermasalah", "clarify": "diklarifikasi"}

var claimStatuses = map[string]bool{"diverifikasi": true, "disetujui": true, "ditunda": true, "ditolak": true}

// GetClaims menampilkan klaim BPJS beserta anomalinya (GET /api/claims).
func GetClaims(db *gorm.DB) gin.HandlerFunc {
	return func(c *gin.Context) {
		_, hid, good := feCtx(c, db)
		if !good {
			return
		}
		if id := c.Query("id"); id != "" {
			var cl model.Claim
			if notFound(c, db.Where("id = ? AND hospital_id = ?", id, hid).First(&cl).Error, "Klaim tidak ditemukan.") {
				return
			}
			var an []model.ClaimAnomaly
			db.Where("claim_id = ? AND hospital_id = ?", id, hid).Find(&an)
			anomalies := make([]J, 0, len(an))
			for _, a := range an {
				anomalies = append(anomalies, feAnomaly(a, cl.ClaimNumber))
			}
			var ev []model.EvidenceItem
			db.Where("hospital_id = ? AND ref_id = ?", hid, cl.ClaimNumber).Find(&ev)
			evidence := make([]J, 0, len(ev))
			for _, e := range ev {
				evidence = append(evidence, feEvidence(e))
			}
			ok(c, J{"claim": feClaim(cl), "anomalies": anomalies, "evidence": evidence})
			return
		}

		q := db.Where("hospital_id = ?", hid)
		if v := c.Query("status"); v != "" {
			q = q.Where("status = ?", codeOf(v))
		}
		if v := c.Query("careType"); v != "" {
			q = q.Where("LOWER(REPLACE(care_type, '_', ' ')) = ?", strings.ToLower(v))
		}
		if s := c.Query("q"); s != "" {
			like := "%" + s + "%"
			q = q.Where("claim_number ILIKE ? OR sep_number ILIKE ? OR inacbg_code ILIKE ?", like, like, like)
		}
		var rows []model.Claim
		if dbFail(c, q.Order("claim_number DESC").Find(&rows).Error) {
			return
		}
		claims := make([]J, 0, len(rows))
		for _, cl := range rows {
			claims = append(claims, feClaim(cl))
		}
		var all []model.Claim
		db.Select("id, claim_number").Where("hospital_id = ?", hid).Find(&all)
		num := map[string]string{}
		for _, cl := range all {
			num[cl.ID] = cl.ClaimNumber
		}
		var an []model.ClaimAnomaly
		db.Where("hospital_id = ?", hid).Find(&an)
		anomalies := make([]J, 0, len(an))
		for _, a := range an {
			anomalies = append(anomalies, feAnomaly(a, strOr(ptrOrNil(num[a.ClaimID]), "—")))
		}
		// urut: status naik, severity turun
		sortJ(anomalies, func(a, b J) bool {
			if a["status"] != b["status"] {
				return numStr(a["status"]) < numStr(b["status"])
			}
			return rank(codeOf(numStr(a["severity"]))) > rank(codeOf(numStr(b["severity"])))
		})
		_, intel, err := computeFE(db, hid)
		if err != nil {
			intelFail(c, hid, err)
			return
		}
		ok(c, J{"claims": claims, "anomalies": anomalies, "ccs": intel["ccs"], "total": len(claims)})
	}
}

// PatchClaims mengubah status anomali klaim atau status klaim (PATCH /api/claims).
func PatchClaims(db *gorm.DB) gin.HandlerFunc {
	return func(c *gin.Context) {
		user, hid, good := feCtx(c, db)
		if !good {
			return
		}
		var body struct {
			AnomalyID string `json:"anomalyId"`
			ClaimID   string `json:"claimId"`
			Action    string `json:"action"`
			Status    string `json:"status"`
		}
		if !bindBody(c, &body) {
			return
		}
		switch {
		case body.AnomalyID != "":
			var a model.ClaimAnomaly
			if notFound(c, db.Where("id = ? AND hospital_id = ?", body.AnomalyID, hid).First(&a).Error, "Anomali tidak ditemukan.") {
				return
			}
			if !requirePerm(c, db, user, hid, auth.PermEdit, "claim_anomalies", a.ID) {
				return
			}
			st, known := anomalyStatus[body.Action]
			if !known {
				fail(c, http.StatusBadRequest, "Aksi tidak dikenal.")
				return
			}
			upd := map[string]interface{}{"status": st, "reviewed_by": user.Name}
			if dbFail(c, audit.UpdateWithAudit(db, user, hid, "claim_anomalies", a.ID, upd, metaOf(c))) {
				return
			}
			db.First(&a, "id = ?", a.ID)
			var cl model.Claim
			db.Select("claim_number").First(&cl, "id = ?", a.ClaimID)
			ok(c, feAnomaly(a, cl.ClaimNumber))
		case body.ClaimID != "":
			var cl model.Claim
			if notFound(c, db.Where("id = ? AND hospital_id = ?", body.ClaimID, hid).First(&cl).Error, "Klaim tidak ditemukan.") {
				return
			}
			if !requirePerm(c, db, user, hid, auth.PermEdit, "claims", cl.ID) {
				return
			}
			st := codeOf(body.Status)
			if !claimStatuses[st] {
				fail(c, http.StatusBadRequest, "Status klaim tidak dikenal.")
				return
			}
			if dbFail(c, audit.UpdateWithAudit(db, user, hid, "claims", cl.ID, map[string]interface{}{"status": st}, metaOf(c))) {
				return
			}
			db.First(&cl, "id = ?", cl.ID)
			ok(c, feClaim(cl))
		default:
			fail(c, http.StatusBadRequest, "Payload tidak valid.")
		}
	}
}
