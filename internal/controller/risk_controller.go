package controller

import (
	"net/http"

	"github.com/gin-gonic/gin"
	"gorm.io/gorm"

	"github.com/spkd/medcredix/internal/audit"
	"github.com/spkd/medcredix/internal/auth"
	"github.com/spkd/medcredix/internal/model"
)

// Status risiko yang diizinkan (sama dengan CHECK di schema.sql). riskClosed = risiko sudah tertangani.
var riskStatuses = map[string]bool{
	"baru": true, "terbuka": true, "ditinjau": true, "dalam_pengerjaan": true,
	"terkelola": true, "teratasi": true, "selesai": true, "ditutup": true,
}

var riskClosed = map[string]bool{"teratasi": true, "selesai": true, "ditutup": true}

var riskClosedList = []string{"teratasi", "selesai", "ditutup"}

// GetRisks menampilkan daftar risiko RS dengan filter status, level, dan category (GET /api/risks).
func GetRisks(db *gorm.DB) gin.HandlerFunc {
	return func(c *gin.Context) {
		_, hid, good := feCtx(c, db)
		if !good {
			return
		}
		q := db.Where("hospital_id = ?", hid)
		if v := c.Query("level"); v != "" {
			q = q.Where("level = ?", codeOf(v))
		}
		if v := c.Query("category"); v != "" {
			q = q.Where("category = ?", codeOf(v))
		}
		if v := c.Query("status"); v != "" {
			q = q.Where("status = ?", codeOf(v))
		}
		var rows []model.Risk
		if dbFail(c, q.Order("code").Find(&rows).Error) {
			return
		}
		risks := make([]J, 0, len(rows))
		for _, r := range rows {
			risks = append(risks, feRisk(r))
		}
		var ev []model.EvidenceItem
		db.Where("hospital_id = ? AND ref_type IN ?", hid, []string{"risiko", "Risiko"}).Find(&ev)
		evidence := make([]J, 0, len(ev))
		for _, e := range ev {
			evidence = append(evidence, feEvidence(e))
		}
		ok(c, J{"risks": risks, "evidence": evidence, "total": len(risks)})
	}
}

// PatchRisks mengubah risiko: ack, assign, atau ganti status (PATCH /api/risks).
func PatchRisks(db *gorm.DB) gin.HandlerFunc {
	return func(c *gin.Context) {
		user, hid, good := feCtx(c, db)
		if !good {
			return
		}
		var body struct {
			ID       string `json:"id"`
			Action   string `json:"action"`
			Status   string `json:"status"`
			Owner    string `json:"owner"`
			Deadline string `json:"deadline"`
		}
		if !bindBody(c, &body) {
			return
		}
		var r model.Risk
		if notFound(c, db.Where("id = ? AND hospital_id = ?", body.ID, hid).First(&r).Error, "Risiko tidak ditemukan.") {
			return
		}
		perm := auth.PermEdit
		if body.Action == "ack" || body.Action == "assign" {
			perm = auth.PermAssign
		}
		if !requirePerm(c, db, user, hid, perm, "risks", r.ID) {
			return
		}
		upd := map[string]interface{}{}
		if body.Status != "" {
			st := codeOf(body.Status)
			if !riskStatuses[st] {
				fail(c, http.StatusBadRequest, "Status risiko tidak dikenal.")
				return
			}
			if riskClosed[st] && !requirePerm(c, db, user, hid, auth.PermResolve, "risks", r.ID) {
				return
			}
			upd["status"] = st
		} else if body.Action == "ack" {
			upd["status"] = "ditinjau"
		}
		if body.Owner != "" {
			upd["suggested_owner"] = body.Owner
		}
		if d := parseDate(body.Deadline); d != nil {
			upd["deadline"] = *d
		}
		if len(upd) > 0 {
			if dbFail(c, audit.UpdateWithAudit(db, user, hid, "risks", r.ID, upd, metaOf(c))) {
				return
			}
			db.First(&r, "id = ?", r.ID)
		}
		ok(c, feRisk(r))
	}
}
