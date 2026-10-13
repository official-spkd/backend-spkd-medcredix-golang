package controller

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

var openActionStatuses = []string{"baru", "ditugaskan", "dalam_pengerjaan", "menunggu_verifikasi", "dibuka_kembali"}

// actionFlow: status asal yang boleh menuju status tujuan + izin yang dibutuhkan.
// Aksi tidak bisa lompat (mis. "baru" langsung "ditutup").
var actionFlow = map[string]transition{
	"ditugaskan":          {[]string{"baru", "dibuka_kembali"}, "ditugaskan", auth.PermAssign},
	"dalam_pengerjaan":    {[]string{"baru", "ditugaskan", "menunggu_verifikasi", "dibuka_kembali"}, "dalam_pengerjaan", auth.PermEdit},
	"menunggu_verifikasi": {[]string{"dalam_pengerjaan"}, "menunggu_verifikasi", auth.PermEdit},
	"selesai":             {[]string{"menunggu_verifikasi"}, "selesai", auth.PermResolve},
	"ditutup":             {[]string{"selesai"}, "ditutup", auth.PermResolve},
	"ditolak":             {[]string{"baru", "ditugaskan", "dalam_pengerjaan", "menunggu_verifikasi"}, "ditolak", auth.PermResolve},
	"dibuka_kembali":      {[]string{"selesai", "ditutup", "ditolak"}, "dibuka_kembali", auth.PermReopen},
}

func addActionEvent(tx *gorm.DB, actionID, typ, from, to, by, note string) error {
	return tx.Create(&model.ActionEvent{
		ID: model.NewID("ae"), ActionID: actionID, Type: typ, FromValue: ptrOrNil(from), ToValue: ptrOrNil(to),
		ByName: ptrOrNil(by), Note: ptrOrNil(note), CreatedAt: time.Now(),
	}).Error
}

// GetActions menampilkan daftar rencana aksi, atau detail satu aksi bila ada ?id= (GET /api/actions).
func GetActions(db *gorm.DB) gin.HandlerFunc {
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

// PostActions membuat rencana aksi baru (POST /api/actions).
func PostActions(db *gorm.DB) gin.HandlerFunc {
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

// PatchActions mengubah rencana aksi: assign, status, atau comment (PATCH /api/actions).
func PatchActions(db *gorm.DB) gin.HandlerFunc {
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
			to := codeOf(body.Status)
			t, known := actionFlow[to]
			if !known {
				fail(c, http.StatusBadRequest, fmt.Sprintf("Status aksi tidak bisa diubah ke %q.", label(to)))
				return
			}
			if !requirePerm(c, db, user, hid, t.perm, "actions", a.ID) {
				return
			}
			if !contains(t.from, a.Status) {
				fail(c, http.StatusConflict, fmt.Sprintf("Status %q tidak bisa langsung diubah ke %q.", label(a.Status), label(to)))
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
