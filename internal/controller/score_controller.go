package controller

import (
	"errors"
	"fmt"
	"log"
	"math"
	"net/http"
	"regexp"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	"gorm.io/gorm"

	"github.com/spkd/medcredix/internal/audit"
	"github.com/spkd/medcredix/internal/model"
	"github.com/spkd/medcredix/internal/scoring"
)

// GetScores menampilkan rincian skor per domain (GET /api/scores?domain=...).
func GetScores(db *gorm.DB) gin.HandlerFunc {
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
				"kelompokDetail": intel["kelompokDetail"], "stageTable": stageTable, "risks": openRisks(6), "evidence": evidence, "actions": actions,
				"trend": intel["trend"], "recommendations": healthRecs(domains),
			})
			return
		case "bcds":
			b := intel["bcds"].(J)
			ok(c, J{
				"kind": "bcds",
				"formula": "BPJS Credential Readiness (BDCS) = (Kelompok A × 25%) + (B × 30%) + (C × 35%) + (D × 10%), " +
					"lalu dikalikan Trust Multiplier — ditampilkan transparan.",
				"kelompok": intel["kelompok"], "kelompokDetail": intel["kelompokDetail"],
				"bcds": b, "trust": intel["trust"], "trustBands": trustBands,
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

// periodRe: format periode snapshot YYYY-MM.
var periodRe = regexp.MustCompile(`^\d{4}-(0[1-9]|1[0-2])$`)

// PostScoreSnapshot menyimpan hasil hitung ke score_snapshots (POST /api/scores/snapshot).
// Satu baris per RS per periode, aman diulang. Nilainya selalu dihitung engine, tidak diinput manual.
// Izin CONFIGURE dicek di router.
func PostScoreSnapshot(db *gorm.DB) gin.HandlerFunc {
	return func(c *gin.Context) {
		user, hid, good := tenantID(c, db)
		if !good {
			return
		}
		var body struct {
			Period string `json:"period"`
		}
		_ = c.ShouldBindJSON(&body) // body opsional
		period := body.Period
		if period == "" {
			period = time.Now().Format("2006-01")
		}
		if !periodRe.MatchString(period) {
			fail(c, http.StatusBadRequest, "Periode harus berformat YYYY-MM.")
			return
		}

		intel, err := scoring.ComputeIntel(db, hid)
		if err != nil {
			log.Printf("scoring: gagal menghitung %s: %v", hid, err)
			fail(c, http.StatusInternalServerError, "Terjadi kesalahan internal.")
			return
		}

		var ur *float64
		if intel.Utilization != nil {
			v := intel.Utilization.Score
			ur = &v
		}
		health, bdcs, trust := intel.HealthScore, intel.BDCS, intel.Trust.Score

		var existing model.ScoreSnapshot
		err = db.Where("hospital_id = ? AND period = ?", hid, period).First(&existing).Error
		var snapID string
		switch {
		case err == nil:
			snapID = existing.ID
			err = db.Model(&model.ScoreSnapshot{}).Where("id = ?", existing.ID).Updates(map[string]interface{}{
				"health_score": health, "bdcs": bdcs, "trust": trust,
				"ccs": intel.Claim.CCS, "cpcs": intel.Pathway.CPCS, "ur_score": ur,
				"safety_score": intel.SafetyScore, "formula_version": scoring.FormulaVersion,
			}).Error
		case errors.Is(err, gorm.ErrRecordNotFound):
			snapID = model.NewID("ss")
			err = db.Create(&model.ScoreSnapshot{
				ID: snapID, HospitalID: hid, Period: period,
				HealthScore: &health, BDCS: &bdcs, Trust: &trust,
				CCS: intel.Claim.CCS, CPCS: intel.Pathway.CPCS, URScore: ur,
				SafetyScore: intel.SafetyScore, FormulaVersion: scoring.FormulaVersion,
			}).Error
		}
		if err != nil {
			log.Printf("scoring: gagal menyimpan snapshot %s/%s: %v", hid, period, err)
			fail(c, http.StatusInternalServerError, "Gagal menyimpan snapshot skor.")
			return
		}

		_ = audit.Log(db, user, hid, "config", "score_snapshots", snapID,
			fmt.Sprintf("Snapshot skor periode %s (%s)", period, scoring.FormulaVersion), metaOf(c))

		var saved model.ScoreSnapshot
		db.First(&saved, "id = ?", snapID)
		ok(c, saved)
	}
}
