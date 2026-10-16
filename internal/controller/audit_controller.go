package controller

import (
	"bytes"
	"encoding/json"
	"fmt"
	"sort"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	"gorm.io/gorm"

	"github.com/spkd/medcredix/internal/model"
)

// GetAudit menampilkan jejak audit RS dengan filter q, action, dan entityType (GET /api/audit).
// Selain data mentah (before/after), setiap log diberi ringkasan yang mudah dibaca:
// entityLabel, subject (mis. "SIP dr. Aditya Nugraha"), dan changes (hanya field yang berubah).
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

		// Uraikan snapshot before/after sekali, lalu kumpulkan id praktisi & pengguna
		// agar namanya bisa diambil dengan 2 query saja (bukan per baris).
		befores := make([]map[string]interface{}, len(rows))
		afters := make([]map[string]interface{}, len(rows))
		practIDs, userIDs := map[string]bool{}, map[string]bool{}
		for i, l := range rows {
			befores[i], afters[i] = parseSnapshot(l.BeforeValue), parseSnapshot(l.AfterValue)
			for _, m := range []map[string]interface{}{befores[i], afters[i]} {
				if id := snapStr(m, "practitioner_id"); id != "" {
					practIDs[id] = true
				}
				for _, f := range []string{"approved_by_id", "verified_by_id"} {
					if id := snapStr(m, f); id != "" {
						userIDs[id] = true
					}
				}
			}
		}
		names := auditNames{practitioners: map[string]string{}, users: map[string]string{}}
		if len(practIDs) > 0 {
			var ps []model.Practitioner
			db.Select("id", "full_name").Where("id IN ?", setKeys(practIDs)).Find(&ps)
			for _, p := range ps {
				names.practitioners[p.ID] = p.FullName
			}
		}
		if len(userIDs) > 0 {
			var us []model.User
			db.Select("id", "name").Where("id IN ?", setKeys(userIDs)).Find(&us)
			for _, u := range us {
				names.users[u.ID] = u.Name
			}
		}

		logs := make([]J, 0, len(rows))
		for i, l := range rows {
			j := feAudit(l)
			et := str(l.EntityType)
			j["entityLabel"] = auditEntityLabel(et)
			snap := afters[i]
			if snap == nil {
				snap = befores[i]
			}
			subject := auditSubject(et, snap, names)
			j["subject"] = subject
			changes := auditChanges(befores[i], afters[i], names)
			j["changes"] = changes
			// Deskripsi lama berbentuk "Ubah practitioner_credentials: status" (nama tabel & kolom).
			// Ganti dengan kalimat yang dibaca manusia, mis. "Ubah kredensial SIP dr. Aditya Nugraha".
			if l.Action == "update" && subject != "" && strings.HasPrefix(str(l.Description), "Ubah "+et) {
				lbl := lowerFirst(auditEntityLabel(et))
				if et == "hospitals" {
					lbl = "profil" // nama RS sudah diawali "RS", hindari "profil RS RS Uji"
				}
				j["description"] = "Ubah " + lbl + " " + subject
			}
			logs = append(logs, j)
		}
		ok(c, J{"logs": logs, "total": len(logs)})
	}
}

type auditNames struct {
	practitioners map[string]string
	users         map[string]string
}

// Nama yang mudah dibaca untuk tiap tabel yang diaudit.
var auditEntityLabels = map[string]string{
	"practitioner_credentials": "Kredensial",
	"practitioners":            "Praktisi",
	"credential_documents":     "Dokumen kredensial",
	"requirements":             "Prasyarat RS",
	"risks":                    "Risiko",
	"early_warnings":           "Peringatan",
	"alert_rules":              "Aturan peringatan",
	"actions":                  "Aksi",
	"claims":                   "Klaim",
	"claim_anomalies":          "Anomali klaim",
	"data_sources":             "Sumber data",
	"data_quality_issues":      "Isu kualitas data",
	"hospitals":                "Profil RS",
	"users":                    "Pengguna",
	"report_snapshots":         "Laporan",
	"score_snapshots":          "Snapshot skor",
	"settings":                 "Pengaturan",
}

func auditEntityLabel(et string) string {
	if v, found := auditEntityLabels[et]; found {
		return v
	}
	if et == "" || et == "—" {
		return "—"
	}
	return label(et)
}

// Label kolom yang ditampilkan di kolom "Sebelum → Sesudah".
var auditFieldLabels = map[string]string{
	"status": "Status", "type": "Jenis", "number": "Nomor", "issue_date": "Terbit", "expiry_date": "Berlaku s.d.",
	"approved_at": "Disetujui", "verified_at": "Diverifikasi", "approved_by_id": "Disetujui oleh", "verified_by_id": "Diverifikasi oleh",
	"owner": "Penanggung jawab", "suggested_owner": "Penanggung jawab", "assigned_to": "Ditugaskan ke",
	"due_date": "Tenggat", "deadline": "Tenggat", "priority": "Prioritas", "level": "Tingkat", "severity": "Keparahan",
	"title": "Judul", "description": "Deskripsi", "resolution_note": "Catatan penyelesaian", "resolved_at": "Diselesaikan",
	"reviewed_by": "Ditinjau oleh", "active": "Aktif", "name": "Nama", "city": "Kota", "province": "Provinsi",
	"hospital_class": "Kelas RS", "bed_licensed": "TT berizin", "bpjs_facility_code": "Kode faskes BPJS",
	"satusehat_org_id": "Org ID SATUSEHAT", "doc_number": "No. dokumen", "doc_expiry": "Berlaku s.d.", "issued_at": "Terbit",
	"file_name": "File", "note": "Catatan", "category": "Kategori", "care_type": "Jenis rawat",
}

// Kolom teknis yang tidak perlu ditampilkan walau nilainya ikut berubah.
var auditSkipFields = map[string]bool{
	"id": true, "hospital_id": true, "version": true, "created_at": true, "updated_at": true,
	"file_key": true, "file_size": true, "file_type": true, "flags_json": true,
}

// Kolom berisi kode (snake_case) yang ditampilkan sebagai label, mis. "dalam_pengerjaan" -> "Dalam Pengerjaan".
var auditCodeFields = map[string]bool{
	"status": true, "priority": true, "level": true, "severity": true, "category": true, "care_type": true,
}

// auditSubject: nama data yang diubah, mis. "SIP dr. Aditya Nugraha" atau "RSK-001 13 kredensial…".
func auditSubject(et string, m map[string]interface{}, names auditNames) string {
	if m == nil {
		return ""
	}
	switch et {
	case "practitioner_credentials":
		who := names.practitioners[snapStr(m, "practitioner_id")]
		if who == "" {
			who = snapStr(m, "number")
		}
		return strings.TrimSpace(snapStr(m, "type") + " " + who)
	case "practitioners":
		return snapStr(m, "full_name")
	case "claims":
		return snapStr(m, "claim_number")
	case "risks", "actions", "early_warnings", "claim_anomalies":
		return strings.TrimSpace(snapStr(m, "code") + " " + snapStr(m, "title"))
	default:
		if v := snapStr(m, "name"); v != "" {
			return v
		}
		return snapStr(m, "title")
	}
}

// auditChanges: daftar field yang nilainya beda antara before dan after, sudah diformat.
func auditChanges(before, after map[string]interface{}, names auditNames) []J {
	out := []J{}
	if before == nil || after == nil {
		return out
	}
	keys := make([]string, 0, len(after))
	for k := range after {
		if !auditSkipFields[k] {
			keys = append(keys, k)
		}
	}
	// Status selalu di depan, sisanya urut abjad agar tampilan stabil.
	sort.Slice(keys, func(a, b int) bool {
		if (keys[a] == "status") != (keys[b] == "status") {
			return keys[a] == "status"
		}
		return keys[a] < keys[b]
	})
	for _, k := range keys {
		from, to := auditValue(k, before[k], names), auditValue(k, after[k], names)
		if from == to {
			continue
		}
		lbl, found := auditFieldLabels[k]
		if !found {
			lbl = label(k)
		}
		out = append(out, J{"field": k, "label": lbl, "from": from, "to": to})
	}
	return out
}

var bulanID = []string{"Jan", "Feb", "Mar", "Apr", "Mei", "Jun", "Jul", "Agu", "Sep", "Okt", "Nov", "Des"}
var zonaWIB = time.FixedZone("WIB", 7*3600)

// auditValue memformat satu nilai snapshot menjadi teks pendek.
func auditValue(field string, v interface{}, names auditNames) string {
	switch x := v.(type) {
	case nil:
		return "—"
	case bool:
		if x {
			return "Ya"
		}
		return "Tidak"
	case json.Number:
		return x.String()
	case string:
		if x == "" {
			return "—"
		}
		if strings.HasSuffix(field, "_by_id") {
			if n := names.users[x]; n != "" {
				return n
			}
			return x
		}
		if auditCodeFields[field] {
			return label(x)
		}
		if t, err := time.Parse(time.RFC3339Nano, x); err == nil {
			if t.Hour() == 0 && t.Minute() == 0 && t.Second() == 0 {
				return fmt.Sprintf("%d %s %d", t.Day(), bulanID[t.Month()-1], t.Year())
			}
			t = t.In(zonaWIB)
			return fmt.Sprintf("%d %s %d %02d:%02d", t.Day(), bulanID[t.Month()-1], t.Year(), t.Hour(), t.Minute())
		}
		return x
	default:
		b, _ := json.Marshal(x)
		return string(b)
	}
}

func parseSnapshot(raw json.RawMessage) map[string]interface{} {
	if len(raw) == 0 || string(raw) == "null" {
		return nil
	}
	dec := json.NewDecoder(bytes.NewReader(raw))
	dec.UseNumber()
	var m map[string]interface{}
	if dec.Decode(&m) != nil {
		return nil
	}
	return m
}

func snapStr(m map[string]interface{}, key string) string {
	if m == nil {
		return ""
	}
	if s, isStr := m[key].(string); isStr {
		return s
	}
	return ""
}

// lowerFirst: "Profil RS" -> "profil RS" (singkatan tetap huruf besar).
func lowerFirst(s string) string {
	if s == "" {
		return s
	}
	return strings.ToLower(s[:1]) + s[1:]
}

func setKeys(s map[string]bool) []string {
	out := make([]string, 0, len(s))
	for k := range s {
		out = append(out, k)
	}
	return out
}
