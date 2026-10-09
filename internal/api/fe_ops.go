package api

import (
	"fmt"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	"gorm.io/gorm"

	"github.com/spkd/medcredix/internal/audit"
	"github.com/spkd/medcredix/internal/auth"
	"github.com/spkd/medcredix/internal/model"
)

// ---------- Risk Radar ----------

func risksGetHandler(db *gorm.DB) gin.HandlerFunc {
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

func risksPatchHandler(db *gorm.DB) gin.HandlerFunc {
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
			upd["status"] = codeOf(body.Status)
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

// ---------- Early Warning ----------

func warningsGetHandler(db *gorm.DB) gin.HandlerFunc {
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

func warningsPatchHandler(db *gorm.DB) gin.HandlerFunc {
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

// ---------- Action Center ----------

// Status aksi yang diizinkan (CHECK di schema.sql).
var actionStatuses = map[string]bool{
	"baru": true, "ditugaskan": true, "dalam_pengerjaan": true, "menunggu_verifikasi": true,
	"selesai": true, "ditutup": true, "ditolak": true, "dibuka_kembali": true,
}

func addActionEvent(tx *gorm.DB, actionID, typ, from, to, by, note string) error {
	return tx.Create(&model.ActionEvent{
		ID: model.NewID("ae"), ActionID: actionID, Type: typ, FromValue: ptrOrNil(from), ToValue: ptrOrNil(to),
		ByName: ptrOrNil(by), Note: ptrOrNil(note), CreatedAt: time.Now(),
	}).Error
}

func ptrOrNil(s string) *string {
	if s == "" {
		return nil
	}
	return &s
}

func actionsGetHandler(db *gorm.DB) gin.HandlerFunc {
	return func(c *gin.Context) {
		_, hid, good := feCtx(c, db)
		if !good {
			return
		}
		if id := c.Query("id"); id != "" {
			var a model.Action
			if notFound(c, db.Where("id = ? AND hospital_id = ?", id, hid).First(&a).Error, "Aksi tidak ditemukan.") {
				return
			}
			var evs []model.ActionEvent
			db.Where("action_id = ?", id).Order("created_at").Find(&evs)
			events := make([]J, 0, len(evs))
			for _, e := range evs {
				events = append(events, feActionEvent(e))
			}
			ok(c, J{"action": feAction(a), "events": events, "users": userBriefs(db, hid)})
			return
		}
		q := db.Where("hospital_id = ?", hid)
		if v := c.Query("status"); v != "" {
			q = q.Where("status = ?", codeOf(v))
		}
		if v := c.Query("priority"); v != "" {
			q = q.Where("priority = ?", codeOf(v))
		}
		var rows []model.Action
		if dbFail(c, q.Order("created_at DESC").Find(&rows).Error) {
			return
		}
		term := c.Query("q")
		actions := make([]J, 0, len(rows))
		open, high, resolved := 0, 0, 0
		for _, a := range rows {
			if term != "" && !likeAny(term, a.Title, a.Code, str(a.Owner)) {
				continue
			}
			actions = append(actions, feAction(a))
			done := a.Status == "selesai" || a.Status == "ditutup"
			if done {
				resolved++
			}
			for _, s := range openActionStatuses {
				if a.Status == s {
					open++
				}
			}
			if a.Priority == "tinggi" && !done {
				high++
			}
		}
		ok(c, J{"actions": actions, "stats": J{"total": len(actions), "open": open, "high": high, "resolved": resolved}})
	}
}

func actionsPostHandler(db *gorm.DB) gin.HandlerFunc {
	return func(c *gin.Context) {
		user, hid, good := feCtx(c, db)
		if !good {
			return
		}
		var body struct {
			Title           string `json:"title"`
			Description     string `json:"description"`
			Priority        string `json:"priority"`
			SourceType      string `json:"sourceType"`
			SourceRef       string `json:"sourceRef"`
			Owner           string `json:"owner"`
			DueDate         string `json:"dueDate"`
			ExpectedOutcome string `json:"expectedOutcome"`
		}
		if !bindBody(c, &body) {
			return
		}
		if strings.TrimSpace(body.Title) == "" {
			fail(c, http.StatusBadRequest, "Judul aksi wajib diisi.")
			return
		}
		if !requirePerm(c, db, user, hid, auth.PermCreate, "actions", "") {
			return
		}
		priority := codeOf(body.Priority)
		if rank(priority) == 0 || priority == "kritis" {
			priority = "sedang"
		}
		srcType := codeOf(body.SourceType)
		if srcType == "" {
			srcType = "manual"
		}

		var created model.Action
		err := db.Transaction(func(tx *gorm.DB) error {
			var codes []string
			if err := tx.Model(&model.Action{}).Where("hospital_id = ?", hid).Pluck("code", &codes).Error; err != nil {
				return err
			}
			next := 1
			for _, cd := range codes {
				if n, err := strconv.Atoi(strings.TrimPrefix(cd, "ACT-")); err == nil && n >= next {
					next = n + 1
				}
			}
			status := "baru"
			if strings.TrimSpace(body.Owner) != "" {
				status = "ditugaskan"
			}
			created = model.Action{
				ID: model.NewID("act"), HospitalID: hid, Code: fmt.Sprintf("ACT-%03d", next),
				Title: strings.TrimSpace(body.Title), Description: ptrOrNil(body.Description), Priority: priority,
				Status: status, SourceType: &srcType, SourceRef: ptrOrNil(body.SourceRef),
				Owner: ptrOrNil(strings.TrimSpace(body.Owner)), DueDate: parseDate(body.DueDate),
				ExpectedResult: ptrOrNil(body.ExpectedOutcome), CreatedAt: time.Now(),
			}
			if err := tx.Create(&created).Error; err != nil {
				return err
			}
			note := "Aksi dibuat dari " + label(srcType)
			if body.SourceRef != "" {
				note += " (" + body.SourceRef + ")"
			}
			return addActionEvent(tx, created.ID, "dibuat", "", "baru", user.Name, note)
		})
		if dbFail(c, err) {
			return
		}
		_ = audit.Log(db, user, hid, "create", "actions", created.ID,
			fmt.Sprintf("Membuat aksi %s: %s", created.Code, created.Title), metaOf(c))
		ok(c, feAction(created))
	}
}

func actionsPatchHandler(db *gorm.DB) gin.HandlerFunc {
	return func(c *gin.Context) {
		user, hid, good := feCtx(c, db)
		if !good {
			return
		}
		var body struct {
			ID             string  `json:"id"`
			Action         string  `json:"action"`
			Owner          string  `json:"owner"`
			DueDate        string  `json:"dueDate"`
			Status         string  `json:"status"`
			Note           *string `json:"note"`
			ResolutionNote *string `json:"resolutionNote"`
		}
		if !bindBody(c, &body) {
			return
		}
		var a model.Action
		if notFound(c, db.Where("id = ? AND hospital_id = ?", body.ID, hid).First(&a).Error, "Aksi tidak ditemukan.") {
			return
		}
		note := ""
		if body.Note != nil {
			note = *body.Note
		}

		switch body.Action {
		case "assign":
			if !requirePerm(c, db, user, hid, auth.PermAssign, "actions", a.ID) {
				return
			}
			upd := map[string]interface{}{}
			owner := strings.TrimSpace(body.Owner)
			if owner != "" {
				upd["owner"] = owner
				if a.Status == "baru" {
					upd["status"] = "ditugaskan"
				}
			}
			if d := parseDate(body.DueDate); d != nil {
				upd["due_date"] = *d
			}
			if len(upd) == 0 {
				ok(c, feAction(a))
				return
			}
			if dbFail(c, audit.UpdateWithAudit(db, user, hid, "actions", a.ID, upd, metaOf(c))) {
				return
			}
			if owner != "" {
				_ = addActionEvent(db, a.ID, "ditugaskan", str(a.Owner), owner, user.Name, note)
			}
		case "status":
			if !requirePerm(c, db, user, hid, auth.PermEdit, "actions", a.ID) {
				return
			}
			to := codeOf(body.Status)
			if !actionStatuses[to] {
				fail(c, http.StatusBadRequest, "Status aksi tidak dikenal.")
				return
			}
			upd := map[string]interface{}{"status": to}
			if to == "selesai" && body.ResolutionNote != nil {
				upd["resolution_note"] = *body.ResolutionNote
			}
			if to == "selesai" || to == "ditutup" {
				upd["resolved_at"] = time.Now()
			}
			if to == "dibuka_kembali" {
				upd["resolved_at"] = nil
			}
			if dbFail(c, audit.UpdateWithAudit(db, user, hid, "actions", a.ID, upd, metaOf(c))) {
				return
			}
			_ = addActionEvent(db, a.ID, "status", a.Status, to, user.Name, note)
		case "comment":
			if strings.TrimSpace(note) == "" {
				fail(c, http.StatusBadRequest, "Komentar kosong.")
				return
			}
			ev := model.ActionEvent{ID: model.NewID("ae"), ActionID: a.ID, Type: "komentar", ByName: &user.Name, Note: &note, CreatedAt: time.Now()}
			if dbFail(c, db.Create(&ev).Error) {
				return
			}
			ok(c, feActionEvent(ev))
			return
		default:
			fail(c, http.StatusBadRequest, "Aksi tidak dikenal.")
			return
		}
		db.First(&a, "id = ?", a.ID)
		ok(c, feAction(a))
	}
}
