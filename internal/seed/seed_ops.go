package seed

import (
	"fmt"
	"strings"
	"time"

	"gorm.io/gorm"

	"github.com/spkd/medcredix/internal/model"
)

// Baseline test case: Risiko 3 tinggi / 6 sedang / 3 rendah (badge Risk Radar = 6 berstatus "terbuka"),
// Peringatan 10 (badge = 2 "baru", 7 aktif), Aksi 16.
func seedRiskWarningAction(tx *gorm.DB) error {
	// risiko (12)
	type rk struct {
		code, cat, level, title, status, owner, rec string
		due                                         int
	}
	risks := []rk{
		{"RSK-001", "kredensial", "tinggi", "13 kredensial mendekati kedaluwarsa", "terbuka", "Petugas Kredensial", "Proses perpanjangan STR/SIP sebelum jatuh tempo", 20},
		{"RSK-002", "klaim", "sedang", "Sinyal anomali klaim meningkat 18%", "terbuka", "Tim Klaim", "Tinjau klaim bersinyal dan lengkapi dokumen pendukung", 30},
		{"RSK-003", "clinical_pathway", "sedang", "Kepatuhan clinical pathway menurun", "ditinjau", "Komite Medis", "Evaluasi deviasi pathway terburuk", 45},
		{"RSK-004", "kualitas_data", "sedang", "Gap data Encounter antar sistem", "terbuka", "IT / SIMRS", "Rekonsiliasi data Encounter SIMRS", 30},
		{"RSK-005", "medikasi", "sedang", "Closed-loop medikasi belum merata", "terbuka", "Farmasi", "Perluas verifikasi barcode ke seluruh bangsal", 60},
		{"RSK-006", "interoperabilitas", "sedang", "Latensi API FHIR mendekati ambang", "terbuka", "IT / SIMRS", "Optimalkan query dan cache endpoint FHIR", 45},
		{"RSK-007", "kredensial", "tinggi", "1 SIP kedaluwarsa dan 3 SIP berakhir <=30 hari", "dalam_pengerjaan", "Petugas Kredensial", "Ajukan perpanjangan SIP segera", 10},
		{"RSK-008", "regulasi", "tinggi", "Akreditasi KARS berakhir dalam 115 hari", "ditinjau", "Admin RS", "Bentuk tim dan mulai persiapan re-akreditasi", 90},
		{"RSK-009", "workforce", "sedang", "Defisit dokter spesialis Mata", "terbuka", "HR / SDM", "Mulai rekrutmen dokter spesialis Mata", 60},
		{"RSK-010", "sarana", "rendah", "Kalibrasi alat mendekati jatuh tempo", "selesai", "IT / SIMRS", "Jadwalkan kalibrasi berkala", 75},
		{"RSK-011", "regulasi", "rendah", "Pembaruan regulasi perlu ditinjau", "selesai", "Admin RS", "Petakan dampak pembaruan regulasi", 90},
		{"RSK-012", "kualitas_data", "rendah", "Duplikasi rekaman pasien", "selesai", "IT / SIMRS", "Jalankan proses merger rekaman duplikat", 50},
	}
	var riskRows []model.Risk
	for i, r := range risks {
		impact := map[string]string{
			"tinggi": "Dapat menurunkan skor BDCS dan menghambat kelulusan kredensial BPJS",
			"sedang": "Berpotensi menurunkan skor domain terkait",
			"rendah": "Dampak terbatas bila ditangani sesuai jadwal",
		}[r.level]
		riskRows = append(riskRows, model.Risk{
			ID: fmt.Sprintf("rsk_ns_%03d", i+1), HospitalID: hNS, Code: r.code, Category: r.cat, Level: r.level,
			Title: r.title, Description: sp(r.title + " (data demo)"), BusinessImpact: sp(impact),
			Recommendation: sp(r.rec), SuggestedOwner: sp(r.owner), Deadline: day(r.due), Status: r.status,
		})
	}
	if err := ins(tx, riskRows); err != nil {
		return err
	}

	// early warning (10)
	type ew struct {
		title, sev, rule, trig, status, assigned string
		due                                      int
	}
	warns := []ew{
		{"Kredensial berakhir <=30 hari", "tinggi", "expiry <= 30 hari", "5 kredensial", "baru", "", 0},
		{"Sinyal klaim naik +18%", "tinggi", "kenaikan sinyal > 15%", "+18%", "baru", "", 0},
		{"Latensi API FHIR meningkat", "sedang", "latensi > 300 ms", "280 ms", "diketahui", "", 0},
		{"Sinkronisasi SIMRS terlambat", "sedang", "sinkron > 24 jam", "30 jam", "ditugaskan", "Andika Pratama, S.Kom", 7},
		{"Kepatuhan pathway di bawah ambang", "sedang", "adherence < 80%", "76,3%", "diketahui", "", 0},
		{"Isu kualitas data terbuka", "sedang", "isu terbuka > 3", "5 isu", "ditugaskan", "Andika Pratama, S.Kom", 14},
		{"Prasyarat P4 menunggu verifikasi", "sedang", "status menunggu > 7 hari", "9 hari", "diketahui", "", 0},
		{"Stok obat tercatat tidak sinkron", "rendah", "selisih stok > 5%", "6%", "teratasi", "", 0},
		{"Kalibrasi alat jatuh tempo", "rendah", "kalibrasi <= 60 hari", "55 hari", "teratasi", "", 0},
		{"Dokumen kredensial belum lengkap", "rendah", "kelengkapan < 95%", "92,9%", "teratasi", "", 0},
	}
	var warnRows []model.EarlyWarning
	for i, w := range warns {
		// Waktu deteksi bertahap: peringatan pertama paling baru, tiap peringatan berikutnya 7 jam lebih awal.
		detected := at(0).Add(-time.Duration(i*7) * time.Hour)
		row := model.EarlyWarning{
			ID: fmt.Sprintf("ew_ns_%03d", i+1), HospitalID: hNS, Code: fmt.Sprintf("EW-%03d", i+1),
			Title: w.title, Severity: w.sev, ThresholdRule: sp(w.rule), TriggerValue: sp(w.trig), Status: w.status,
			AssignedTo: sp(w.assigned),
			DetectedAt: &detected,
		}
		if w.due != 0 {
			row.DueDate = day(w.due)
		}
		warnRows = append(warnRows, row)
	}
	if err := ins(tx, warnRows); err != nil {
		return err
	}

	// action center (16)
	// CATATAN: test case menyebut ACT-003 & ACT-016 berstatus "Ditinjau", tetapi CHECK di schema.sql
	// tidak mengizinkan status itu. Di sini keduanya dipetakan ke
	// 'menunggu_verifikasi', sehingga badge Action Center = 13 (bukan 11) sampai dokumen uji direvisi.
	type ac struct {
		title, prio, status, srcType, srcRef, owner, note string
		due                                               int
	}
	acts := []ac{
		{"Perpanjang 13 kredensial mendekati kedaluwarsa", "tinggi", "dalam_pengerjaan", "risiko", "RSK-001", "Ratna Puspitasari, S.KM", "", 20},
		{"Tinjau lonjakan sinyal klaim", "sedang", "ditugaskan", "risiko", "RSK-002", "Silvia Rahmawati, AMd.Kep", "", 30},
		{"Tinjau kepatuhan clinical pathway", "sedang", "menunggu_verifikasi", "risiko", "RSK-003", "dr. Bambang Sutrisno, Sp.PD-KIC", "", 45},
		{"Perpanjang SIP kedaluwarsa dan <=30 hari", "tinggi", "dalam_pengerjaan", "risiko", "RSK-007", "Ratna Puspitasari, S.KM", "", 10},
		{"Bentuk tim re-akreditasi KARS", "tinggi", "baru", "risiko", "RSK-008", "", "", 40},
		{"Perbaiki gap data Encounter", "sedang", "ditugaskan", "risiko", "RSK-004", "Andika Pratama, S.Kom", "", 30},
		{"Optimalkan latensi API FHIR", "sedang", "dalam_pengerjaan", "peringatan", "EW-003", "Andika Pratama, S.Kom", "", 25},
		{"Merger rekaman pasien duplikat", "sedang", "baru", "risiko", "RSK-012", "", "", 50},
		{"Ajukan perpanjangan PKS BPJS", "tinggi", "menunggu_verifikasi", "skor", "P2", "dr. Aditya Nugraha, MARS", "", 21},
		{"Jadwalkan kalibrasi alat", "rendah", "ditugaskan", "risiko", "RSK-010", "Andika Pratama, S.Kom", "", 60},
		{"Rekrut dokter spesialis Mata", "tinggi", "dalam_pengerjaan", "risiko", "RSK-009", "Tri Handayani, S.Psi", "", 60},
		{"Kaji pembaruan regulasi", "rendah", "baru", "risiko", "RSK-011", "", "", 90},
		{"Lengkapi data alergi pasien", "sedang", "selesai", "skor", "safety", "dr. Kartika Dewi", "Data alergi dilengkapi pada seluruh rekam medis rawat inap", -14},
		{"Verifikasi ulang izin radiologi", "sedang", "ditutup", "skor", "P4", "Ratna Puspitasari, S.KM", "Dokumen diverifikasi dan dinyatakan sah", -20},
		{"Sinkronkan jadwal praktik dokter", "rendah", "selesai", "manual", "", "Tri Handayani, S.Psi", "Jadwal praktik disinkronkan dengan SIMRS", -10},
		{"Evaluasi adopsi closed-loop medikasi", "sedang", "menunggu_verifikasi", "skor", "medication", "dr. Kartika Dewi", "", 35},
	}
	var actRows []model.Action
	var evRows []model.ActionEvent
	for i, a := range acts {
		id := fmt.Sprintf("act_ns_%03d", i+1)
		row := model.Action{
			ID: id, HospitalID: hNS, Code: fmt.Sprintf("ACT-%03d", i+1), Title: a.title,
			Description: sp(a.title + " (data demo)"), Priority: a.prio, Status: a.status,
			SourceType: sp(a.srcType), SourceRef: sp(a.srcRef), Owner: sp(a.owner), DueDate: day(a.due),
			ExpectedResult: sp("Skor domain terkait membaik"), CreatedAt: at(-30 + i),
		}
		if a.status == "selesai" || a.status == "ditutup" {
			row.ResolutionNote = sp(a.note)
			t := at(-5 - i%5)
			row.ResolvedAt = &t
		}
		actRows = append(actRows, row)

		n := 1
		event := func(typ, from, to, by, note string) {
			evRows = append(evRows, model.ActionEvent{
				ID: fmt.Sprintf("ae_%s_%d", strings.TrimPrefix(id, "act_"), n), ActionID: id, Type: typ,
				FromValue: sp(from), ToValue: sp(to), ByName: sp(by), Note: sp(note), CreatedAt: at(-30 + i + n),
			})
			n++
		}
		event("dibuat", "", "baru", "dr. Aditya Nugraha, MARS", "")
		if a.owner != "" {
			event("ditugaskan", "", a.owner, "dr. Aditya Nugraha, MARS", "")
		}
		if a.status != "baru" && a.status != "ditugaskan" {
			event("status", "ditugaskan", a.status, a.owner, a.note)
		}
	}
	if err := ins(tx, actRows); err != nil {
		return err
	}
	return ins(tx, evRows)
}
