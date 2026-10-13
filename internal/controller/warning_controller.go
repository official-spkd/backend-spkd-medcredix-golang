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

// GetWarnings menampilkan daftar early warning (GET /api/warnings).
func GetWarnings(db *gorm.DB) gin.HandlerFunc {
	return func(c *gin.Context) {
		_, hid, good := feCtx(c, db)
		if !good {
			return
		}
		q := db.Where("hospital_id = ?", hid)
		if v := c.Query("severity"); v != "" {
			q = q.Where("severity = ?", codeOf(v))
		}
		if v := c.Query("status"); v != "" {
			q = q.Where("status = ?", codeOf(v))
		}
		var rows []model.EarlyWarning
		if dbFail(c, q.Order("code").Find(&rows).Error) {
			return
		}
		statusOrder := map[string]int{"baru": 0, "diketahui": 1, "ditugaskan": 2, "teratasi": 3}
		warnings := make([]J, 0, len(rows))
		for _, w := range rows {
			warnings = append(warnings, feWarning(w))
		}
		sortJ(warnings, func(a, b J) bool {
			return statusOrder[codeOf(a["status"].(string))] < statusOrder[codeOf(b["status"].(string))]
		})
		var ruleRows []model.AlertRule
		db.Where("hospital_id = ?", hid).Order("name").Find(&ruleRows)
		rules := make([]J, 0, len(ruleRows))
		for _, r := range ruleRows {
			rules = append(rules, feAlertRule(r))
		}
		ok(c, J{"warnings": warnings, "rules": rules, "total": len(warnings)})
	}
}

// PatchWarnings mengubah early warning: ack, assign, atau resolve (PATCH /api/warnings).
func PatchWarnings(db *gorm.DB) gin.HandlerFunc {
	return func(c *gin.Context) {
		user, hid, good := feCtx(c, db)
		if !good {
			return
		}
		var body struct {
			ID       string `json:"id"`
			Action   string `json:"action"`
			Assignee string `json:"assignee"`
			DueDate  string `json:"dueDate"`
		}
		if !bindBody(c, &body) {
			return
		}
		var w model.EarlyWarning
		if notFound(c, db.Where("id = ? AND hospital_id = ?", body.ID, hid).First(&w).Error, "Peringatan tidak ditemukan.") {
			return
		}
		status, known := map[string]string{"ack": "diketahui", "assign": "ditugaskan", "resolve": "teratasi"}[body.Action]
		if !known {
			fail(c, http.StatusBadRequest, "Aksi tidak dikenal.")
			return
		}
		perm := auth.PermAssign
		if body.Action == "resolve" {
			perm = auth.PermResolve
		}
		if !requirePerm(c, db, user, hid, perm, "early_warnings", w.ID) {
			return
		}
		upd := map[string]interface{}{"status": status}
		if body.Action == "assign" {
			assignee := strings.TrimSpace(body.Assignee)
			if assignee == "" {
				assignee = user.Name
			}
			upd["assigned_to"] = assignee
		}
		if d := parseDate(body.DueDate); d != nil {
			upd["due_date"] = *d
		}
		if dbFail(c, audit.UpdateWithAudit(db, user, hid, "early_warnings", w.ID, upd, metaOf(c))) {
			return
		}
		db.First(&w, "id = ?", w.ID)
		out := feWarning(w)
		if body.Action != "resolve" {
			out["acknowledgedBy"] = user.Name
		}
		ok(c, out)
	}
}
