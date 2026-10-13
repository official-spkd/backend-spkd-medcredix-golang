package controller

import (
	"errors"
	"fmt"
	"net/http"

	"github.com/gin-gonic/gin"
	"gorm.io/gorm"

	"github.com/spkd/medcredix/internal/audit"
	"github.com/spkd/medcredix/internal/auth"
	"github.com/spkd/medcredix/internal/model"
)

func metaOf(c *gin.Context) audit.Meta {
	cid, _ := c.Get("correlation_id")
	s, _ := cid.(string)
	return audit.Meta{IP: c.ClientIP(), CorrelationID: s}
}

// tenantID = ResolveHospitalID + pastikan RS yang diminta benar-benar ada.
func tenantID(c *gin.Context, db *gorm.DB) (model.User, string, bool) {
	user, _ := auth.CurrentUser(c)
	hid, err := auth.ResolveHospitalID(user, c.Query("hospitalId"))
	if err != nil {
		code := http.StatusForbidden
		if err == auth.ErrNoHospital {
			code = http.StatusBadRequest
		}
		fail(c, code, err.Error())
		return user, "", false
	}
	var n int64
	if err := db.Model(&model.Hospital{}).Where("id = ?", hid).Count(&n).Error; err != nil || n == 0 {
		fail(c, http.StatusNotFound, "Rumah sakit tidak ditemukan.")
		return user, "", false
	}
	return user, hid, true
}

// feCtx: user + RS hasil resolve (isolasi tenant). Mengirim respons error sendiri bila gagal.
func feCtx(c *gin.Context, db *gorm.DB) (model.User, string, bool) {
	return tenantID(c, db)
}

func requirePerm(c *gin.Context, db *gorm.DB, user model.User, hid string, p auth.Permission, entity, entityID string) bool {
	if auth.HasPermission(user.Role, p) {
		return true
	}
	_ = audit.Log(db, user, hid, "akses_ditolak", entity, entityID,
		fmt.Sprintf("Aksi ditolak: butuh izin %s", p), metaOf(c))
	fail(c, http.StatusForbidden, fmt.Sprintf("Peran %s tidak memiliki izin %s.", auth.RoleLabels[user.Role], p))
	return false
}

func bindBody(c *gin.Context, dst interface{}) bool {
	if err := c.ShouldBindJSON(dst); err != nil {
		fail(c, http.StatusBadRequest, "Payload tidak valid.")
		return false
	}
	return true
}

func notFound(c *gin.Context, err error, msg string) bool {
	if errors.Is(err, gorm.ErrRecordNotFound) {
		fail(c, http.StatusNotFound, msg)
		return true
	}
	if err != nil {
		fail(c, http.StatusInternalServerError, "Gagal membaca data.")
		return true
	}
	return false
}

func dbFail(c *gin.Context, err error) bool {
	if err != nil {
		fail(c, http.StatusInternalServerError, "Gagal memproses data.")
		return true
	}
	return false
}
