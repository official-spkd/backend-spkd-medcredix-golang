package api

import "github.com/spkd/medcredix/internal/auth"

// transition = satu langkah alur kredensial. Status asal DIVALIDASI (fix F-08):
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
}
