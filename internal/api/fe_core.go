package api

import (
	"log"
	"math"
	"net/http"
	"strings"

	"github.com/gin-gonic/gin"
	"gorm.io/gorm"

	"github.com/spkd/medcredix/internal/auth"
	"github.com/spkd/medcredix/internal/model"
	"github.com/spkd/medcredix/internal/scoring"
)

var openActionStatuses = []string{"baru", "ditugaskan", "dalam_pengerjaan", "menunggu_verifikasi", "dibuka_kembali"}

// Status risiko yang diizinkan (sama dengan CHECK di schema.sql). riskClosed = risiko sudah tertangani.
var riskStatuses = map[string]bool{
	"baru": true, "terbuka": true, "ditinjau": true, "dalam_pengerjaan": true,
	"terkelola": true, "teratasi": true, "selesai": true, "ditutup": true,
}
var riskClosed = map[string]bool{"teratasi": true, "selesai": true, "ditutup": true}
var riskClosedList = []string{"teratasi", "selesai", "ditutup"}

func badgesOf(db *gorm.DB, hid string) J {
	var risks, warnings, actions int64
	db.Model(&model.Risk{}).Where("hospital_id = ? AND status NOT IN ?", hid, riskClosedList).Count(&risks)
	db.Model(&model.EarlyWarning{}).Where("hospital_id = ? AND status = ?", hid, "baru").Count(&warnings)
	db.Model(&model.Action{}).Where("hospital_id = ? AND status IN ?", hid, openActionStatuses).Count(&actions)
	return J{"risks": risks, "warnings": warnings, "actions": actions}
}

func intelFail(c *gin.Context, hid string, err error) {
	log.Printf("scoring: gagal menghitung %s: %v", hid, err)
	fail(c, http.StatusInternalServerError, "Gagal menghitung skor.")
}

// ---------- GET /api/bootstrap ----------

func bootstrapHandler(db *gorm.DB) gin.HandlerFunc {
	return func(c *gin.Context) {
		user, _ := auth.CurrentUser(c)
		hid, err := auth.ResolveHospitalID(user, c.Query("hospitalId"))
		if err != nil {
			if err == auth.ErrNoHospital && auth.CanSwitchTenant(user) {
				var hs []model.Hospital
				db.Order("name").Find(&hs)
				list := make([]J, 0, len(hs))
				for _, h := range hs {
					list = append(list, J{"id": h.ID, "name": h.Name, "code": h.Code})
				}
				ok(c, J{"user": sessionOf(user), "hospital": nil, "needsHospital": true, "hospitals": list,
					"badges": J{"risks": 0, "warnings": 0, "actions": 0}, "notifications": []J{}})
				return
			}
			fail(c, http.StatusForbidden, err.Error())
			return
		}
		hospital, err := loadHospital(db, hid)
		if notFound(c, err, "Rumah sakit tidak ditemukan.") {
			return
		}
		in, intel, err := computeFE(db, hid)
		if err != nil {
			intelFail(c, hid, err)
			return
		}
		_ = in

		var notifs []model.Notification
		db.Where("hospital_id = ? AND user_id = ?", hid, user.ID).Order("id DESC").Limit(8).Find(&notifs)
		if len(notifs) == 0 {
			db.Where("hospital_id = ?", hid).Order("id DESC").Limit(8).Find(&notifs)
		}
		notifications := make([]J, 0, len(notifs))
		for _, n := range notifs {
			notifications = append(notifications, feNotification(n))
		}

		var rules []model.AlertRule
		db.Where("hospital_id = ?", hid).Order("name").Find(&rules)
		alertRules := make([]J, 0, len(rules))
		for _, r := range rules {
			alertRules = append(alertRules, feAlertRule(r))
		}

		var hospitals interface{}
		if auth.CanSwitchTenant(user) {
			var hs []model.Hospital
			db.Order("name").Find(&hs)
			list := make([]J, 0, len(hs))
			for _, h := range hs {
				list = append(list, J{"id": h.ID, "name": h.Name, "city": sv(h.City)})
			}
			hospitals = list
		}

		domains := intel["domains"].([]J)
		brief := make([]J, 0, len(domains))
		for _, d := range domains {
			brief = append(brief, J{"key": d["key"], "label": d["label"], "score": d["score"], "weight": d["weight"], "group": d["group"]})
		}

		ok(c, J{
			"user": sessionOf(user), "hospital": hospital, "hospitals": hospitals,
			"intel": J{
				"healthScore": intel["healthScore"], "healthStatus": intel["healthStatus"], "bcds": intel["bcds"],
				"kelompok": intel["kelompok"], "trust": intel["trust"], "ccs": intel["ccs"].(J)["score"],
				"cpcs": intel["cpcs"].(J)["score"], "ur": intel["ur"], "domains": brief,
				"trend": intel["trend"], "prereq": intel["prereq"],
			},
			"badges": badgesOf(db, hid), "notifications": notifications, "alertRules": alertRules,
			"users": userBriefs(db, hid),
		})
	}
}

// ---------- GET /api/dashboard ----------

func dashboardHandler(db *gorm.DB) gin.HandlerFunc {
	return func(c *gin.Context) {
		_, hid, good := feCtx(c, db)
		if !good {
			return
		}
		_, intel, err := computeFE(db, hid)
		if err != nil {
			intelFail(c, hid, err)
			return
		}

		var riskRows []model.Risk
		db.Where("hospital_id = ? AND status NOT IN ?", hid, riskClosedList).Order("code").Find(&riskRows)
		risks := make([]J, 0, len(riskRows))
		summary := J{"tinggi": 0, "sedang": 0, "rendah": 0}
		for _, r := range riskRows {
			risks = append(risks, feRisk(r))
			if n, ok := summary[r.Level].(int); ok {
				summary[r.Level] = n + 1
			}
		}
		sortJ(risks, func(a, b J) bool { return rank(codeOf(a["level"].(string))) > rank(codeOf(b["level"].(string))) })

		var warnRows []model.EarlyWarning
		db.Where("hospital_id = ? AND status IN ?", hid, []string{"baru", "diketahui", "ditugaskan"}).Order("code").Find(&warnRows)
		warnings := make([]J, 0, len(warnRows))
		for _, w := range warnRows {
			warnings = append(warnings, feWarning(w))
		}
		sortJ(warnings, func(a, b J) bool {
			return rank(codeOf(a["severity"].(string))) > rank(codeOf(b["severity"].(string)))
		})

		var actRows []model.Action
		db.Where("hospital_id = ? AND status IN ?", hid, openActionStatuses).Order("due_date NULLS LAST").Find(&actRows)
		actions := make([]J, 0, len(actRows))
		for _, a := range actRows {
			actions = append(actions, feAction(a))
		}
		sortJ(actions, func(a, b J) bool {
			return rank(codeOf(a["priority"].(string))) > rank(codeOf(b["priority"].(string)))
		})
		top := actions
		if len(top) > 5 {
			top = top[:5]
		}

		var evRows []model.EvidenceItem
		db.Where("hospital_id = ?", hid).Order("id DESC").Limit(6).Find(&evRows)
		evidence := make([]J, 0, len(evRows))
		for _, e := range evRows {
			evidence = append(evidence, feEvidence(e))
		}

		var pws []model.ClinicalPathway
		db.Where("hospital_id = ?", hid).Order("avg_adherence ASC NULLS LAST").Limit(3).Find(&pws)
		worst := make([]J, 0, len(pws))
		for _, p := range pws {
			worst = append(worst, fePathway(p, nil))
		}

		timeline := []J{}
		for _, t := range intel["trend"].([]J) {
			timeline = append(timeline, J{
				"period": t["period"], "type": "Skor", "delta": nil,
				"label": "Health " + numStr(t["health"]) + " · BDCS " + numStr(t["bcds"]) + " · Trust " + numStr(t["trust"]),
			})
		}
		var resolved []model.Action
		db.Where("hospital_id = ? AND status IN ? AND resolved_at IS NOT NULL", hid, []string{"selesai", "ditutup"}).
			Order("resolved_at DESC").Limit(6).Find(&resolved)
		for _, a := range resolved {
			timeline = append(timeline, J{"period": a.ResolvedAt.Format("2006-01"), "type": "Perbaikan", "label": a.Code + " — " + a.Title, "delta": nil})
		}
		sortJ(timeline, func(a, b J) bool { return a["period"].(string) < b["period"].(string) })

		ok(c, J{
			"intel": intel, "risks": risks, "warnings": warnings, "actions": actions, "topActions": top,
			"evidence": evidence, "pathwayWorst": worst, "timeline": timeline, "riskSummary": summary,
		})
	}
}

func numStr(v interface{}) string {
	if f, ok := v.(float64); ok {
		return fmtNum(f)
	}
	return "—"
}

// ---------- GET /api/scores?domain=health|bcds|<domain> ----------

func scoresHandler(db *gorm.DB) gin.HandlerFunc {
	return func(c *gin.Context) {
		_, hid, good := feCtx(c, db)
		if !good {
			return
		}
		_, intel, err := computeFE(db, hid)
		if err != nil {
			intelFail(c, hid, err)
			return
		}
		key := c.DefaultQuery("domain", "health")
		domains := intel["domains"].([]J)

		openRisks := func(limit int) []J {
			var rows []model.Risk
			q := db.Where("hospital_id = ? AND status NOT IN ?", hid, riskClosedList).Order("code")
			if limit > 0 {
				q = q.Limit(limit)
			}
			q.Find(&rows)
			out := make([]J, 0, len(rows))
			for _, r := range rows {
				out = append(out, feRisk(r))
			}
			return out
		}

		switch key {
		case "health":
			list := make([]J, 0, len(domains))
			for _, d := range domains {
				list = append(list, J{
					"key": d["key"], "label": d["label"], "score": d["score"], "weight": d["weight"],
					"contribution": math.Round(d["score"].(float64)*d["weight"].(float64)*10) / 10,
				})
			}
			var evRows []model.EvidenceItem
			db.Where("hospital_id = ? AND ref_type IN ?", hid, []string{"skor", "Skor"}).Limit(10).Find(&evRows)
			evidence := make([]J, 0, len(evRows))
			for _, e := range evRows {
				evidence = append(evidence, feEvidence(e))
			}
			var actRows []model.Action
			db.Where("hospital_id = ? AND status IN ?", hid, []string{"baru", "ditugaskan", "dalam_pengerjaan"}).Limit(6).Find(&actRows)
			actions := make([]J, 0, len(actRows))
			for _, a := range actRows {
				actions = append(actions, feAction(a))
			}
			ok(c, J{
				"kind": "health",
				"formula": "Hospital Health Score = Σ(skor domain × bobot domain). Bobot: Claim Integrity 16%, Credentialing 14%, " +
					"Patient Safety 14%, Clinical Workforce 12%, Clinical Pathway 10%, Digital Maturity 10%, Medication 8%, Diagnostik 8%, Interop 8%.",
				"formulaVersion": scoring.FormulaVersion, "domains": list,
				"healthScore": intel["healthScore"], "bcds": intel["bcds"], "trust": intel["trust"], "kelompok": intel["kelompok"],
				"stageTable": stageTable, "risks": openRisks(6), "evidence": evidence, "actions": actions,
				"trend": intel["trend"], "recommendations": healthRecs(domains),
			})
			return
		case "bcds":
			b := intel["bcds"].(J)
			ok(c, J{
				"kind": "bcds",
				"formula": "BPJS Credential Readiness (BDCS) = (Kelompok A × 25%) + (B × 30%) + (C × 35%) + (D × 10%), " +
					"lalu dikalikan Trust Multiplier — ditampilkan transparan.",
				"kelompok": intel["kelompok"], "bcds": b, "trust": intel["trust"], "trustBands": trustBands,
				"stage": b["stage"], "lulus": b["lulus"], "prereqOk": b["prereqOk"], "trend": intel["trend"],
				"formulaVersion": scoring.FormulaVersion, "stageTable": stageTable,
			})
			return
		}

		var domain J
		for _, d := range domains {
			if d["key"] == key || d["key"] == feDomainKey(scoring.DomainKey(engineDomainKey(key))) {
				domain = d
			}
		}
		if domain == nil {
			fail(c, http.StatusNotFound, "Domain tidak dikenal.")
			return
		}
		dl, _ := domain["label"].(string)
		related := []J{}
		codes := map[string]bool{}
		for _, r := range openRisks(0) {
			rd, _ := r["domain"].(string)
			if rd == dl || strings.Contains(strings.ToLower(rd), strings.ToLower(key)) {
				related = append(related, r)
				codes[r["code"].(string)] = true
			}
		}
		var actRows []model.Action
		db.Where("hospital_id = ? AND source_ref IS NOT NULL", hid).Find(&actRows)
		actions := []J{}
		for _, a := range actRows {
			ref := str(a.SourceRef)
			if ref == "DOMAIN:"+key || ref == key || codes[ref] {
				actions = append(actions, feAction(a))
			}
		}
		var evRows []model.EvidenceItem
		db.Where("hospital_id = ? AND ref_id IN ?", hid, []string{"DOMAIN:" + key, key}).Find(&evRows)
		evidence := make([]J, 0, len(evRows))
		for _, e := range evRows {
			evidence = append(evidence, feEvidence(e))
		}
		var risks, acts interface{}
		if len(related) > 0 {
			risks = related
		}
		if len(actions) > 0 {
			acts = actions
		}
		ok(c, J{
			"kind": "domain", "domain": domain, "formulaVersion": scoring.FormulaVersion, "evidence": evidence,
			"risks": risks, "actions": acts, "recommendations": domainRecs(domain),
			"trend": intel["trend"], "healthScore": intel["healthScore"],
		})
	}
}

// ---------- GET /api/admin ----------

func adminHandler(db *gorm.DB) gin.HandlerFunc {
	return func(c *gin.Context) {
		user, _ := auth.CurrentUser(c)
		if !auth.CanSwitchTenant(user) {
			fail(c, http.StatusForbidden, "Panel Admin hanya untuk Super Admin & Auditor BPJS.")
			return
		}
		var hospitals []model.Hospital
		if dbFail(c, db.Order("name").Find(&hospitals).Error) {
			return
		}
		rows := make([]J, 0, len(hospitals))
		stageCount := make([]int, 8)
		var sumB, sumH float64
		lulus, onboarding, underReview := 0, 0, 0
		for _, h := range hospitals {
			in, intel, err := computeFE(db, h.ID)
			if err != nil {
				log.Printf("scoring: gagal menghitung %s: %v", h.ID, err)
				continue
			}
			trend := []J{}
			for _, t := range intel["trend"].([]J) {
				trend = append(trend, J{"period": t["period"], "bcds": t["bcds"], "health": t["health"]})
			}
			fh := feHospital(h, 0)
			row := J{
				"id": h.ID, "name": h.Name, "code": h.Code, "class": sv(h.HospitalClass), "city": sv(h.City),
				"province": sv(h.Province), "tier": sv(h.Tier), "onboarding": fh["onboarding"],
				"healthScore": in.HealthScore, "bcds": in.BDCS, "stage": in.StageEffective, "lulus": in.Pass,
				"trust": in.Trust.Score, "ccs": intel["ccs"].(J)["score"], "cpcs": intel["cpcs"].(J)["score"],
				"anomalyFlags": in.AnomalyFlags, "underReview": in.UnderReview, "trend": trend,
			}
			rows = append(rows, row)
			if in.StageEffective >= 0 && in.StageEffective < 8 {
				stageCount[in.StageEffective]++
			}
			sumB += in.BDCS
			sumH += in.HealthScore
			if in.Pass {
				lulus++
			}
			if fh["onboarding"] == true {
				onboarding++
			}
			if in.UnderReview {
				underReview++
			}
		}
		dist := make([]J, 8)
		for s := range dist {
			dist[s] = J{"stage": s, "count": stageCount[s]}
		}
		n := float64(len(rows))
		summary := J{"totalRS": 0, "rataBdc": 0, "rataHealth": 0, "lulus": 0, "onboarding": 0, "underReview": 0}
		if n > 0 {
			summary = J{
				"totalRS": len(rows), "rataBdc": math.Floor(sumB/n*10+0.5) / 10,
				"rataHealth": math.Floor(sumH/n*10+0.5) / 10, "lulus": lulus, "onboarding": onboarding,
				"underReview": underReview,
			}
		}
		ok(c, J{"hospitals": rows, "summary": summary, "stageDist": dist})
	}
}
