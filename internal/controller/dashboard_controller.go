package controller

import (
	"log"
	"math"
	"net/http"

	"github.com/gin-gonic/gin"
	"gorm.io/gorm"

	"github.com/spkd/medcredix/internal/auth"
	"github.com/spkd/medcredix/internal/model"
)

func badgesOf(db *gorm.DB, hid string) J {
	var risks, warnings, actions int64
	db.Model(&model.Risk{}).Where("hospital_id = ? AND status NOT IN ?", hid, riskClosedList).Count(&risks)
	db.Model(&model.EarlyWarning{}).Where("hospital_id = ? AND status = ?", hid, "baru").Count(&warnings)
	db.Model(&model.Action{}).Where("hospital_id = ? AND status IN ?", hid, openActionStatuses).Count(&actions)
	return J{"risks": risks, "warnings": warnings, "actions": actions}
}

// GetBootstrap mengirim data awal setelah login: user, RS, skor, dan badge menu (GET /api/bootstrap).
func GetBootstrap(db *gorm.DB) gin.HandlerFunc {
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

// GetDashboard mengirim ringkasan untuk halaman dashboard (GET /api/dashboard).
func GetDashboard(db *gorm.DB) gin.HandlerFunc {
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

// GetAdmin mengirim ringkasan semua RS untuk halaman admin: skor, stage, dan tren (GET /api/admin).
func GetAdmin(db *gorm.DB) gin.HandlerFunc {
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
