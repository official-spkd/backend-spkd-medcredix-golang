package controller

import (
	"github.com/gin-gonic/gin"
	"gorm.io/gorm"

	"github.com/spkd/medcredix/internal/model"
)

// GetAudit menampilkan jejak audit RS dengan filter q, action, dan entityType (GET /api/audit).
func GetAudit(db *gorm.DB) gin.HandlerFunc {
	return func(c *gin.Context) {
		user, hid, good := feCtx(c, db)
		if !good {
			return
		}
		q := db.Model(&model.AuditLog{})
		if user.Role != "super_admin" {
			q = q.Where("hospital_id = ? OR (hospital_id IS NULL AND actor_role IN ?)", hid, []string{"super_admin", "system"})
		}
		if v := c.Query("action"); v != "" {
			q = q.Where("action = ?", v)
		}
		if v := c.Query("entityType"); v != "" {
			q = q.Where("entity_type = ?", v)
		}
		if s := c.Query("q"); s != "" {
			like := "%" + s + "%"
			q = q.Where("actor_name ILIKE ? OR description ILIKE ? OR entity_id ILIKE ?", like, like, like)
		}
		var rows []model.AuditLog
		if dbFail(c, q.Order("created_at DESC").Limit(200).Find(&rows).Error) {
			return
		}
		logs := make([]J, 0, len(rows))
		for _, l := range rows {
			logs = append(logs, feAudit(l))
		}
		ok(c, J{"logs": logs, "total": len(logs)})
	}
}
