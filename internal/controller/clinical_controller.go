package controller

import (
	"github.com/gin-gonic/gin"
	"gorm.io/gorm"

	"github.com/spkd/medcredix/internal/model"
)

// GetClinical menampilkan data klinis: pathway, utilisasi, keluhan, dan keselamatan pasien (GET /api/clinical).
func GetClinical(db *gorm.DB) gin.HandlerFunc {
	return func(c *gin.Context) {
		_, hid, good := feCtx(c, db)
		if !good {
			return
		}
		in, intel, err := computeFE(db, hid)
		if err != nil {
			intelFail(c, hid, err)
			return
		}
		var pws []model.ClinicalPathway
		db.Where("hospital_id = ?", hid).Order("avg_adherence ASC NULLS FIRST").Find(&pws)
		pwIDs := make([]string, 0, len(pws))
		stdLOS := map[string]*float64{}
		for _, p := range pws {
			pwIDs = append(pwIDs, p.ID)
			stdLOS[p.ID] = p.StandardLOS
		}
		var eps []model.PathwayEpisode
		if len(pwIDs) > 0 {
			db.Where("pathway_id IN ?", pwIDs).Order("adherence ASC NULLS FIRST").Find(&eps)
		}
		pathways := make([]J, 0, len(pws))
		for _, p := range pws {
			pathways = append(pathways, fePathway(p, eps))
		}

		typ := c.DefaultQuery("type", "all")
		if typ == "pathway" {
			episodes := make([]J, 0, len(eps))
			for _, e := range eps {
				episodes = append(episodes, feEpisode(e, hid, stdLOS[e.PathwayID]))
			}
			ok(c, J{"pathways": pathways, "episodes": episodes, "cpcs": intel["cpcs"],
				"coverage": J{"total": in.Pathway.CoverageTarget, "covered": in.Pathway.Covered}})
			return
		}

		var us []model.UtilizationStat
		db.Where("hospital_id = ?", hid).Order("month").Find(&us)
		utils := make([]J, 0, len(us))
		for _, u := range us {
			utils = append(utils, feUtil(u))
		}
		var cs []model.ComplaintStat
		db.Where("hospital_id = ?", hid).Order("month").Find(&cs)
		complaints := make([]J, 0, len(cs))
		for _, x := range cs {
			complaints = append(complaints, feComplaint(x))
		}

		switch typ {
		case "utilization":
			ok(c, J{"utilization": utils, "complaints": complaints, "ur": intel["ur"]})
		case "safety":
			var dom interface{}
			for _, d := range intel["domains"].([]J) {
				if d["key"] == "safety" {
					dom = d
				}
			}
			ok(c, J{"safety": intel["safety"], "domain": dom})
		default:
			ok(c, J{"pathways": pathways, "utilization": utils, "complaints": complaints,
				"cpcs": intel["cpcs"], "ur": intel["ur"], "safety": intel["safety"]})
		}
	}
}
