package controller

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

// transition = satu langkah alur kredensial. Status asal DIVALIDASI:
// kredensial tidak bisa lompat dari "diajukan" langsung ke "aktif".
type transition struct {
	from []string
	to   string
	perm auth.Permission
}

var credTransitions = map[string]transition{
	"submit":     {[]string{"segera_berakhir", "kedaluwarsa"}, "diajukan", auth.PermEdit},
	"verify":     {[]string{"diajukan"}, "verifikasi", auth.PermVerify},
	"review":     {[]string{"verifikasi"}, "tinjauan_komite", auth.PermVerify},
	"approve":    {[]string{"tinjauan_komite"}, "disetujui", auth.PermApprove},
	"activate":   {[]string{"disetujui"}, "aktif", auth.PermApprove},
	"suspend":    {[]string{"aktif", "segera_berakhir"}, "ditangguhkan", auth.PermApprove},
	"reactivate": {[]string{"ditangguhkan"}, "aktif", auth.PermApprove},
	// Komite menolak pengajuan: kredensial ditahan (ditangguhkan) sampai diajukan ulang.
	"reject": {[]string{"tinjauan_komite"}, "ditangguhkan", auth.PermApprove},
}

// GetCredentials menampilkan daftar kredensial tenaga kesehatan (GET /api/credentials).
func GetCredentials(db *gorm.DB) gin.HandlerFunc {
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

// PatchCredentials menjalankan satu langkah alur kredensial dari credTransitions (PATCH /api/credentials).
// Status asal dicek dulu, dan yang menyetujui tidak boleh orang yang sama dengan yang memverifikasi.
func PatchCredentials(db *gorm.DB) gin.HandlerFunc {
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
