package controller

import (
	"github.com/gin-gonic/gin"
	"gorm.io/gorm"

	"github.com/spkd/medcredix/internal/model"
)

// GetEvidence menampilkan daftar evidence pendukung skor dan risiko (GET /api/evidence).
func GetEvidence(db *gorm.DB) gin.HandlerFunc {
	return func(c *gin.Context) {
		_, hid, good := feCtx(c, db)
		if !good {
			return
		}
		q := db.Where("hospital_id = ?", hid)
		if v := c.Query("refType"); v != "" {
			q = q.Where("ref_type = ?", codeOf(v))
		}
		if v := c.Query("refId"); v != "" {
			q = q.Where("ref_id = ?", v)
		}
		if s := c.Query("q"); s != "" {
			like := "%" + s + "%"
			q = q.Where("metric ILIKE ? OR source_system ILIKE ? OR ref_id ILIKE ?", like, like, like)
		}
		var rows []model.EvidenceItem
		if dbFail(c, q.Order("ref_type, id").Find(&rows).Error) {
			return
		}
		items := make([]J, 0, len(rows))
		byType := map[string]int{}
		for _, e := range rows {
			j := feEvidence(e)
			items = append(items, j)
			byType[j["refType"].(string)]++
		}
		ok(c, J{"items": items, "byType": byType, "total": len(items)})
	}
}
