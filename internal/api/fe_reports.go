package api

import (
	"encoding/csv"
	"encoding/json"
	"fmt"
	"net/http"
	"sort"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	"gorm.io/gorm"

	"github.com/spkd/medcredix/internal/audit"
	"github.com/spkd/medcredix/internal/auth"
	"github.com/spkd/medcredix/internal/model"
)

type reportType struct{ key, title, desc string }

var reportTypes = []reportType{
	{"kredensial", "Laporan Kredensial RS", "Skor BDCS, kelompok A–D, prasyarat, kredensial & tenaga medis"},
	{"kesehatan-rs", "Hospital Health Report", "9 domain, tren 6 bulan, rekomendasi perbaikan"},
	{"risiko", "Risk Report", "Risiko aktif, level, owner, dan tenggat"},
	{"aksi", "Action & Improvement Report", "Aksi perbaikan, status, resolusi, dampak skor"},
	{"kepatuhan", "Regulatory Compliance Report", "Matriks kepatuhan, regulasi, versi, temuan"},
	{"klaim", "BPJS Claim Integrity Report", "CCS, sub-skor, anomali aktif, prioritas tinjauan"},
}

func reportTypeOf(key string) (reportType, bool) {
	for _, t := range reportTypes {
		if t.key == key {
			return t, true
		}
	}
	return reportType{}, false
}

const reportMethodology = "Scoring engine %s; sumber: SIMRS/EMR/BPJS/SATUSEHAT via integration layer; verifikasi manual wajib untuk keputusan final."

func feReport(r model.ReportSnapshot, formula string) J {
	return J{
		"id": r.ID, "hospitalId": r.HospitalID, "type": r.Type, "title": r.Title, "period": strOr(r.Period, "—"),
		"generatedAt": r.CreatedAt, "generatedBy": strOr(r.CreatedBy, "—"),
		"methodology": fmt.Sprintf(reportMethodology, formula), "version": r.Version,
		"auditRef": sv(r.AuditRef), "contentJson": string(r.ContentJSON),
	}
}

func dash(t *time.Time) string {
	if t == nil {
		return "—"
	}
	return t.Format("2006-01-02")
}

// GET /reports            -> daftar + jenis laporan
// GET /reports?id=...     -> satu laporan + isinya
// GET /reports?export=... -> unduh CSV (izin EXPORT)
func reportsGetHandler(db *gorm.DB) gin.HandlerFunc {
	return func(c *gin.Context) {
		user, hid, good := feCtx(c, db)
		if !good {
			return
		}
		if id := c.Query("export"); id != "" {
			reportCSV(c, db, user, hid, id)
			return
		}
		if id := c.Query("id"); id != "" {
			var r model.ReportSnapshot
			if notFound(c, db.Where("id = ? AND hospital_id = ?", id, hid).First(&r).Error, "Laporan tidak ditemukan.") {
				return
			}
			var content interface{}
			_ = json.Unmarshal(r.ContentJSON, &content)
			ok(c, J{"report": feReport(r, "medcredix-scorer"), "content": content})
			return
		}
		var rows []model.ReportSnapshot
		if dbFail(c, db.Where("hospital_id = ?", hid).Order("created_at DESC").Find(&rows).Error) {
			return
		}
		reports := make([]J, 0, len(rows))
		for _, r := range rows {
			reports = append(reports, feReport(r, "medcredix-scorer"))
		}
		types := make([]J, 0, len(reportTypes))
		for _, t := range reportTypes {
			types = append(types, J{"type": t.key, "title": t.title, "desc": t.desc})
		}
		ok(c, J{"reports": reports, "types": types})
	}
}

func reportCSV(c *gin.Context, db *gorm.DB, user model.User, hid, id string) {
	var r model.ReportSnapshot
	if notFound(c, db.Where("id = ? AND hospital_id = ?", id, hid).First(&r).Error, "Laporan tidak ditemukan.") {
		return
	}
	if !requirePerm(c, db, user, hid, auth.PermExport, "report_snapshots", r.ID) {
		return
	}
	var content map[string]interface{}
	_ = json.Unmarshal(r.ContentJSON, &content)
	rows := [][]string{{"Field", "Nilai"}}
	var flat func(obj map[string]interface{}, prefix string)
	flat = func(obj map[string]interface{}, prefix string) {
		keys := make([]string, 0, len(obj))
		for k := range obj {
			keys = append(keys, k)
		}
		sort.Strings(keys)
		for _, k := range keys {
			switch v := obj[k].(type) {
			case []interface{}:
				b, _ := json.Marshal(v)
				rows = append(rows, []string{prefix + k, string(b)})
			case map[string]interface{}:
				flat(v, prefix+k+".")
			case nil:
				rows = append(rows, []string{prefix + k, "—"})
			default:
				rows = append(rows, []string{prefix + k, fmt.Sprint(v)})
			}
		}
	}
	flat(content, "")
	var sb strings.Builder
	sb.WriteString("\uFEFF")
	w := csv.NewWriter(&sb)
	_ = w.WriteAll(rows)

	_ = audit.Log(db, user, hid, "export", "report_snapshots", r.ID, "Ekspor CSV: "+r.Title, metaOf(c))
	name := fmt.Sprintf("MedCredix-%s-%s.csv", r.Type, strings.ReplaceAll(strOr(r.Period, "laporan"), " ", ""))
	c.Header("Content-Disposition", fmt.Sprintf(`attachment; filename="%s"`, name))
	c.Data(http.StatusOK, "text/csv; charset=utf-8", []byte(sb.String()))
}

// POST /reports  body: {"type":"kesehatan-rs"}
func reportsPostHandler(db *gorm.DB) gin.HandlerFunc {
	return func(c *gin.Context) {
		user, hid, good := feCtx(c, db)
		if !good {
			return
		}
		if !requirePerm(c, db, user, hid, auth.PermCreate, "report_snapshots", "") {
			return
		}
		var body struct {
			Type string `json:"type"`
		}
		_ = c.ShouldBindJSON(&body)
		if body.Type == "" {
			body.Type = "kesehatan-rs"
		}
		meta, known := reportTypeOf(body.Type)
		if !known {
			fail(c, http.StatusBadRequest, "Jenis laporan tidak dikenal.")
			return
		}
		in, intel, err := computeFE(db, hid)
		if err != nil {
			intelFail(c, hid, err)
			return
		}
		h, _ := loadHospital(db, hid)
		trend := intel["trend"].([]J)
		period := "—"
		if len(trend) > 0 {
			period = fmt.Sprintf("%v–%v", trend[0]["period"], trend[len(trend)-1]["period"])
		}
		bcds := intel["bcds"].(J)
		content := J{"ringkasan": J{
			"namaRS": h["name"], "kelas": h["class"], "kota": h["city"], "periode": period,
			"healthScore": in.HealthScore, "bcds": bcds["final"], "stage": bcds["stage"],
			"trust": in.Trust.Score, "ccs": intel["ccs"].(J)["score"], "cpcs": intel["cpcs"].(J)["score"],
		}}

		switch body.Type {
		case "kredensial":
			var creds []model.PractitionerCredential
			db.Where("hospital_id = ?", hid).Find(&creds)
			statusCount := map[string]int{}
			for _, cr := range creds {
				statusCount[label(cr.Status)]++
			}
			var nP int64
			db.Model(&model.Practitioner{}).Where("hospital_id = ?", hid).Count(&nP)
			var reqs []model.Requirement
			db.Where("hospital_id = ?", hid).Order("code").Find(&reqs)
			prasyarat := make([]J, 0, len(reqs))
			for _, r := range reqs {
				prasyarat = append(prasyarat, J{"kode": r.Code, "nama": r.Name, "status": label(r.Status), "kedaluwarsa": dash(r.DocExpiry)})
			}
			content["tenagaMedis"] = J{"total": nP}
			content["kredensial"] = statusCount
			content["prasyarat"] = prasyarat
			content["kelompok"] = intel["kelompok"]
			content["bdcS"] = bcds
			content["workforce"] = intel["workforce"]
		case "risiko":
			var rs []model.Risk
			db.Where("hospital_id = ?", hid).Order("code").Find(&rs)
			list := make([]J, 0, len(rs))
			for _, r := range rs {
				list = append(list, J{"kode": r.Code, "kategori": label(r.Category), "level": label(r.Level), "judul": r.Title,
					"owner": strOr(r.SuggestedOwner, "—"), "tenggat": dash(r.Deadline), "status": label(r.Status)})
			}
			content["risiko"] = list
		case "aksi":
			var as []model.Action
			db.Where("hospital_id = ?", hid).Order("code").Find(&as)
			list := make([]J, 0, len(as))
			for _, a := range as {
				list = append(list, J{"kode": a.Code, "judul": a.Title, "prioritas": label(a.Priority), "status": label(a.Status),
					"owner": strOr(a.Owner, "Belum ditugaskan"), "tenggat": dash(a.DueDate), "resolusi": strOr(a.ResolutionNote, "—")})
			}
			content["aksi"] = list
			content["catatan"] = "Closed-loop: resolusi aksi memicu usulan recalculation skor."
		case "kepatuhan":
			var ms []model.ComplianceMapping
			db.Where("hospital_id = ?", hid).Order("article").Find(&ms)
			var regs []model.Regulation
			db.Find(&regs)
			num := map[string]string{}
			for _, r := range regs {
				num[r.ID] = strOr(r.Number, r.Code)
			}
			list := make([]J, 0, len(ms))
			for _, m := range ms {
				list = append(list, J{"regulasi": strOr(ptrOrNil(num[m.RegulationID]), "—"), "pasal": str(m.Article),
					"requirement": strOr(m.Feature, "—"), "modul": str(m.Module), "status": label(str(m.Status)),
					"owner": str(m.Owner), "review": dash(m.ReviewedAt)})
			}
			content["matriks"] = list
			content["disclaimer"] = "Designed to support compliance, subject to regulatory and hospital verification."
		case "klaim":
			var total, ditunda, ditolak, aTotal, aTinggi, aReview int64
			db.Model(&model.Claim{}).Where("hospital_id = ?", hid).Count(&total)
			db.Model(&model.Claim{}).Where("hospital_id = ? AND status = ?", hid, "ditunda").Count(&ditunda)
			db.Model(&model.Claim{}).Where("hospital_id = ? AND status = ?", hid, "ditolak").Count(&ditolak)
			db.Model(&model.ClaimAnomaly{}).Where("hospital_id = ?", hid).Count(&aTotal)
			db.Model(&model.ClaimAnomaly{}).Where("hospital_id = ? AND severity = ?", hid, "tinggi").Count(&aTinggi)
			db.Model(&model.ClaimAnomaly{}).Where("hospital_id = ? AND status = ?", hid, "ditinjau").Count(&aReview)
			content["ccs"] = intel["ccs"]
			content["klaim"] = J{"total": total, "ditunda": ditunda, "ditolak": ditolak}
			content["anomali"] = J{"total": aTotal, "tinggi": aTinggi, "perluTinjauan": aReview}
			content["disclaimer"] = "Sinyal bersifat netral dan memerlukan verifikasi manual — bukan kesimpulan penyalahgunaan."
		default:
			domains := intel["domains"].([]J)
			list := make([]J, 0, len(domains))
			for _, d := range domains {
				list = append(list, J{"domain": d["label"], "skor": d["score"], "bobot": d["weight"]})
			}
			sorted := append([]J(nil), domains...)
			sortJ(sorted, func(a, b J) bool { return a["score"].(float64) < b["score"].(float64) })
			saran := []J{}
			for i, d := range sorted {
				if i == 3 {
					break
				}
				catatan := "Pertahankan kinerja"
				if neg, _ := d["negatives"].([]string); len(neg) > 0 {
					catatan = neg[0]
				}
				saran = append(saran, J{"domain": d["label"], "skor": d["score"], "catatan": catatan})
			}
			content["domain"] = list
			content["tren"] = trend
			content["saran"] = saran
		}

		raw, err := json.Marshal(content)
		if dbFail(c, err) {
			return
		}
		var n int64
		db.Model(&model.ReportSnapshot{}).Where("hospital_id = ?", hid).Count(&n)
		ref := fmt.Sprintf("AUD-%X", time.Now().UnixMilli())
		snap := model.ReportSnapshot{
			ID: model.NewID("rpt"), HospitalID: hid, Type: body.Type, Title: meta.title, Period: &period,
			Version: fmt.Sprintf("v1.%d", n+1), AuditRef: &ref, ContentJSON: raw, CreatedBy: &user.Name,
			CreatedAt: time.Now(),
		}
		if dbFail(c, db.Create(&snap).Error) {
			return
		}
		_ = audit.Log(db, user, hid, "create", "report_snapshots", snap.ID, "Generate laporan: "+snap.Title, metaOf(c))
		ok(c, feReport(snap, in.FormulaVersion))
	}
}
