package auth

import (
	"errors"

	"github.com/gin-gonic/gin"

	"github.com/spkd/medcredix/internal/model"
)

// Permission = 13 izin MedCredix.
type Permission string

const (
	PermView      Permission = "VIEW"
	PermCreate    Permission = "CREATE"
	PermEdit      Permission = "EDIT"
	PermDelete    Permission = "DELETE"
	PermVerify    Permission = "VERIFY"
	PermApprove   Permission = "APPROVE"
	PermSign      Permission = "SIGN"
	PermExport    Permission = "EXPORT"
	PermPrint     Permission = "PRINT"
	PermAssign    Permission = "ASSIGN"
	PermResolve   Permission = "RESOLVE"
	PermReopen    Permission = "REOPEN"
	PermConfigure Permission = "CONFIGURE"
)

var allPermissions = []Permission{
	PermView, PermCreate, PermEdit, PermDelete, PermVerify, PermApprove, PermSign,
	PermExport, PermPrint, PermAssign, PermResolve, PermReopen, PermConfigure,
}

// RolePermissions: kunci = nilai kolom users.role (lihat CHECK di schema.sql).
var RolePermissions = map[string][]Permission{
	"super_admin":        allPermissions,
	"admin_rs":           allPermissions,
	"hospital_mgmt":      {PermView, PermExport, PermPrint, PermAssign},
	"credential_officer": {PermView, PermCreate, PermEdit, PermVerify, PermExport, PermPrint, PermAssign},
	"medical_committee":  {PermView, PermVerify, PermApprove, PermSign, PermExport, PermResolve, PermReopen},
	"hr_sdm":             {PermView, PermCreate, PermEdit, PermExport},
	"auditor":            {PermView, PermExport, PermPrint},
	"claims_team":        {PermView, PermCreate, PermEdit, PermExport, PermAssign, PermResolve},
	"it_simrs":           {PermView, PermCreate, PermEdit, PermExport, PermConfigure},
	"viewer":             {PermView},
}

var RoleLabels = map[string]string{
	"super_admin":        "SPKD Super Admin",
	"admin_rs":           "Admin RS",
	"hospital_mgmt":      "Manajemen RS",
	"credential_officer": "Petugas Kredensial",
	"medical_committee":  "Komite Medis",
	"hr_sdm":             "HR / SDM",
	"auditor":            "Auditor",
	"claims_team":        "Tim Klaim",
	"it_simrs":           "IT / SIMRS",
	"viewer":             "Viewer",
}

func HasPermission(role string, p Permission) bool {
	for _, x := range RolePermissions[role] {
		if x == p {
			return true
		}
	}
	return false
}

// CurrentUser mengambil user yang sudah divalidasi oleh middleware.Auth.
func CurrentUser(c *gin.Context) (model.User, bool) {
	v, ok := c.Get("user")
	if !ok {
		return model.User{}, false
	}
	u, ok := v.(model.User)
	return u, ok
}

var (
	ErrNoHospital  = errors.New("akun Anda tidak terikat ke rumah sakit, sertakan parameter hospitalId")
	ErrCrossTenant = errors.New("akses lintas rumah sakit tidak diizinkan")
)

// CanSwitchTenant: super_admin selalu boleh; auditor hanya bila tidak terikat RS
// (contoh: Auditor BPJS). Auditor yang terikat ke satu RS tidak boleh pindah.
func CanSwitchTenant(u model.User) bool {
	return u.Role == "super_admin" || (u.Role == "auditor" && u.HospitalID == nil)
}

// ResolveHospitalID = pusat isolasi multi-RS. SEMUA query data wajib memakai hasilnya.
func ResolveHospitalID(u model.User, requested string) (string, error) {
	own := ""
	if u.HospitalID != nil {
		own = *u.HospitalID
	}
	if requested == "" || requested == own {
		if own == "" {
			return "", ErrNoHospital
		}
		return own, nil
	}
	if CanSwitchTenant(u) {
		return requested, nil
	}
	return "", ErrCrossTenant
}