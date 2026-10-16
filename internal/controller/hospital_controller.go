package controller

import (
	"encoding/json"
	"net/http"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	"golang.org/x/crypto/bcrypt"
	"gorm.io/gorm"

	"github.com/spkd/medcredix/internal/auth"
	"github.com/spkd/medcredix/internal/model"
	"github.com/spkd/medcredix/internal/wilayah"
)

// Tambah RS mitra baru (POST /api/hospitals), khusus SPKD Super Admin.
// Sekaligus membuat akun Admin RS pertama, supaya RS baru langsung bisa dipakai:
// Admin RS itulah yang kemudian membuat akun staf lainnya lewat Pengaturan > Pengguna.

var hospitalCodeRe = regexp.MustCompile(`^[A-Z0-9][A-Z0-9-]{1,19}$`)

// Kode faskes BPJS rumah sakit = kode PPK yang tercantum di PKS dengan BPJS Kesehatan, 8 karakter:
// 4 angka kode wilayah BPJS + huruf R (rumah sakit / FKRTL) + 3 angka nomor urut, mis. 0137R038.
// Huruf lain dipakai faskes non-RS (B klinik pratama, U klinik utama, G dokter gigi, A apotek), jadi ditolak di sini.
var bpjsFacilityCodeRe = regexp.MustCompile(`^[0-9]{4}R[0-9]{3}$`)

// parseBPJSFacilityCode: opsional (kosong = NULL). Bila diisi, spasi dibuang dan huruf dibesarkan lalu dicek polanya.
func parseBPJSFacilityCode(v *string) (*string, string) {
	if v == nil {
		return nil, ""
	}
	t := strings.ToUpper(strings.Join(strings.Fields(*v), ""))
	if t == "" {
		return nil, ""
	}
	if !bpjsFacilityCodeRe.MatchString(t) {
		return nil, "Kode faskes BPJS harus 8 karakter: 4 angka + R + 3 angka (mis. 0137R038)."
	}
	return &t, ""
}

// defaultAlertRules: aturan notifikasi bawaan untuk RS baru (sama dengan aturan RS demo).
var defaultAlertRules = []struct{ name, domain, metric, cond, sev, ch string }{
	{"Kredensial <= 30 hari", "Kredensial", "expiry_days", "<= 30", "tinggi", "in-app,email"},
	{"Sinyal klaim naik", "Klaim", "anomaly_growth", "> 15%", "tinggi", "in-app,email"},
	{"Latensi FHIR tinggi", "Interoperabilitas", "fhir_latency_ms", "> 300", "sedang", "in-app"},
	{"Adherence pathway rendah", "Clinical Pathway", "avg_adherence", "< 80", "sedang", "in-app"},
	{"Prasyarat mendekati kedaluwarsa", "Prasyarat", "doc_expiry_days", "<= 45", "tinggi", "in-app,whatsapp"},
	{"BOR melewati batas", "Utilisasi", "bor", "> 85%", "tinggi", "in-app,email"},
	{"Lonjakan volume data (anti-manipulasi)", "Kualitas Data", "data_volume_spike", "> 30%", "tinggi", "in-app,email"},
}

// optText: teks opsional, kosong disimpan sebagai NULL.
func optText(v *string, max int, label string) (*string, string) {
	if v == nil {
		return nil, ""
	}
	t := strings.TrimSpace(*v)
	if t == "" {
		return nil, ""
	}
	if len(t) > max {
		return nil, label + " maksimal " + strconv.Itoa(max) + " karakter."
	}
	return &t, ""
}

// PostHospitals membuat RS baru beserta akun Admin RS pertamanya (POST /api/hospitals).
// Body: {"name","code","class","province","city","district","village","bedLicensed","bpjsFacilityCode",
// Wilayah (provinsi > kab/kota > kecamatan > kelurahan/desa) harus cocok dengan data GET /api/regions.
//
//	"admin":{"name","email","title","password"}}
func PostHospitals(db *gorm.DB) gin.HandlerFunc {
	return func(c *gin.Context) {
		user, _ := auth.CurrentUser(c)
		if user.Role != "super_admin" {
			fail(c, http.StatusForbidden, "Hanya SPKD Super Admin yang dapat menambah rumah sakit.")
			return
		}
		var body struct {
			Name             string          `json:"name"`
			Code             string          `json:"code"`
			Class            string          `json:"class"`
			Province         string          `json:"province"`
			City             string          `json:"city"`
			District         string          `json:"district"`
			Village          string          `json:"village"`
			BedLicensed      json.RawMessage `json:"bedLicensed"`
			BPJSFacilityCode *string         `json:"bpjsFacilityCode"`
			Admin            struct {
				Name     string  `json:"name"`
				Email    string  `json:"email"`
				Title    *string `json:"title"`
				Password string  `json:"password"`
			} `json:"admin"`
		}
		if !bindBody(c, &body) {
			return
		}
		bad := func(msg string) { fail(c, http.StatusBadRequest, msg) }

		// ---- Data RS ----
		name := strings.TrimSpace(body.Name)
		if name == "" || len(name) > 200 {
			bad("Nama RS wajib diisi (maksimal 200 karakter).")
			return
		}
		code := strings.ToUpper(strings.TrimSpace(body.Code))
		if !hospitalCodeRe.MatchString(code) {
			bad("Kode RS 2-20 karakter, hanya huruf, angka, dan tanda hubung (mis. RSHB).")
			return
		}
		cls, valid := parseHospitalClass(body.Class)
		if !valid {
			bad("Kelas RS harus A, B, C, atau D.")
			return
		}
		if len(body.BedLicensed) == 0 {
			bad("Jumlah TT terlisensi wajib diisi.")
			return
		}
		beds, msg := parseBedLicensed(body.BedLicensed)
		if msg != "" {
			bad(msg)
			return
		}
		if strings.TrimSpace(body.Province) == "" || strings.TrimSpace(body.City) == "" {
			bad("Provinsi dan kabupaten/kota wajib dipilih.")
			return
		}
		region, msg := wilayah.Resolve(body.Province, body.City, body.District, body.Village)
		prov, city, district, village := ptrOrNil(region[0]), ptrOrNil(region[1]), ptrOrNil(region[2]), ptrOrNil(region[3])
		var bpjs *string
		if msg == "" {
			bpjs, msg = parseBPJSFacilityCode(body.BPJSFacilityCode)
		}
		if msg != "" {
			bad(msg)
			return
		}

		// ---- Akun Admin RS pertama ----
		adminName := strings.TrimSpace(body.Admin.Name)
		if adminName == "" || len(adminName) > 150 {
			bad("Nama Admin RS wajib diisi (maksimal 150 karakter).")
			return
		}
		email, msg := cleanEmail(body.Admin.Email)
		if msg == "" {
			msg = checkPassword(body.Admin.Password)
		}
		var title *string
		if msg == "" {
			title, msg = optText(body.Admin.Title, 150, "Jabatan")
		}
		if msg != "" {
			bad(msg)
			return
		}

		// ---- Cek bentrok ----
		hid := "h_" + strings.ToLower(strings.ReplaceAll(code, "-", "_"))
		var n int64
		db.Model(&model.Hospital{}).Where("UPPER(code) = ? OR id = ?", code, hid).Count(&n)
		if n > 0 {
			fail(c, http.StatusConflict, "Kode RS "+code+" sudah dipakai rumah sakit lain.")
			return
		}
		if emailTaken(db, email, "") {
			fail(c, http.StatusConflict, "Email Admin RS sudah dipakai akun lain.")
			return
		}
		hash, err := bcrypt.GenerateFromPassword([]byte(body.Admin.Password), bcrypt.DefaultCost)
		if dbFail(c, err) {
			return
		}

		onboarding := "berjalan"
		h := model.Hospital{
			ID: hid, Name: name, Code: code, HospitalClass: &cls, Province: prov, City: city, District: district, Village: village,
			BedLicensed: beds, BPJSFacilityCode: bpjs, Onboarding: &onboarding, CreatedAt: time.Now(),
		}
		admin := model.User{
			ID: model.NewID("u"), HospitalID: &hid, Email: email, PasswordHash: string(hash),
			Name: adminName, Title: title, Role: "admin_rs", Active: true,
		}
		err = db.Transaction(func(tx *gorm.DB) error {
			if err := tx.Create(&h).Error; err != nil {
				return err
			}
			rules := make([]model.AlertRule, 0, len(defaultAlertRules))
			for _, r := range defaultAlertRules {
				rules = append(rules, model.AlertRule{ID: model.NewID("ar"), HospitalID: hid, Name: r.name,
					Domain: ptrOrNil(r.domain), Metric: ptrOrNil(r.metric), RuleCondition: ptrOrNil(r.cond),
					Severity: ptrOrNil(r.sev), Channels: ptrOrNil(r.ch), Active: true})
			}
			if err := tx.Create(&rules).Error; err != nil {
				return err
			}
			if err := tx.Create(&admin).Error; err != nil {
				return err
			}
			after, _ := json.Marshal(map[string]interface{}{
				"id": h.ID, "name": h.Name, "code": h.Code, "hospital_class": cls, "province": prov, "city": city, "district": district, "village": village,
				"bed_licensed": beds, "bpjs_facility_code": bpjs, "onboarding": onboarding,
			})
			et, desc, meta := "hospitals", "Tambah rumah sakit "+h.Name+" ("+code+")", metaOf(c)
			entry := model.AuditLog{
				ID: model.NewID("aud"), HospitalID: &hid, ActorName: user.Name, ActorRole: user.Role,
				Action: "create", EntityType: &et, EntityID: &hid, Description: &desc, AfterValue: after,
				IP: ptrOrNil(meta.IP), CorrelationID: ptrOrNil(meta.CorrelationID), CreatedAt: time.Now(),
			}
			if err := tx.Create(&entry).Error; err != nil {
				return err
			}
			return auditUser(tx, user, hid, "create", "Tambah pengguna "+admin.Name+" ("+auth.RoleLabels[admin.Role]+")",
				nil, userSnapshot(admin, "Dibuat"), metaOf(c))
		})
		if err != nil {
			if strings.Contains(err.Error(), "duplicate") || strings.Contains(err.Error(), "unique") {
				fail(c, http.StatusConflict, "Kode RS atau email Admin RS sudah dipakai.")
				return
			}
			dbFail(c, err)
			return
		}
		ok(c, J{"hospital": feHospital(h, 0), "admin": feUser(admin)})
	}
}
