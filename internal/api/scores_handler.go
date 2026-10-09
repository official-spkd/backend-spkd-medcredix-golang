package api

import (
	"errors"
	"fmt"
	"log"
	"net/http"
	"regexp"
	"time"

	"github.com/gin-gonic/gin"
	"gorm.io/gorm"

	"github.com/spkd/medcredix/internal/audit"
	"github.com/spkd/medcredix/internal/model"
	"github.com/spkd/medcredix/internal/scoring"
)

// ---------- POST /api/scores/snapshot ----------
// Menyimpan hasil hitung ke score_snapshots (satu baris per RS per periode, idempoten).
// Nilai TIDAK diinput manual: selalu dihitung engine. Izin CONFIGURE dicek di router.

var periodRe = regexp.MustCompile(`^\d{4}-(0[1-9]|1[0-2])$`)

func snapshotHandler(db *gorm.DB) gin.HandlerFunc {
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