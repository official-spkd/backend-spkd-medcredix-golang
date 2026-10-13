package seed

import (
	"fmt"
	"time"

	"gorm.io/gorm"

	"github.com/spkd/medcredix/internal/model"
)

// Data Intelligence + kesiapan RS. Angka dipilih agar mendekati baseline sheet "Data Uji & Akun";
// nilai skor final tetap harus dihitung oleh scoring engine (internal/scoring), bukan dari seed ini.
func seedIntelligence(tx *gorm.DB) error {
	// isu kualitas data: 10 (5 terbuka), anomaly flag aktif = 1 (+1 "Gap Encounter" sudah diperbaiki)
	type dq struct {
		dim, title, sev, status string
		anomaly                 bool
	}
	issues := []dq{
		{"Kelengkapan", "Field alergi kosong pada 4% rekam medis", "sedang", "terbuka", false},
		{"Akurasi", "Selisih tanggal lahir antar sistem", "sedang", "terbuka", false},
		{"Konsistensi", "Kode diagnosis berbeda antara SIMRS dan klaim", "tinggi", "terbuka", true},
		{"Ketepatan Waktu", "Sinkronisasi laboratorium tertunda", "sedang", "terbuka", false},
		{"Keunikan", "Rekaman pasien ganda", "rendah", "terbuka", false},
		{"Konsistensi", "Gap Encounter antar sistem", "tinggi", "diperbaiki", true},
		{"Validitas", "Format NIK tidak valid pada 12 rekaman", "sedang", "diperbaiki", false},
		{"Kelengkapan", "Resume pemulangan belum terisi", "sedang", "diperbaiki", false},
		{"Ketepatan Waktu", "Keterlambatan unggah dokumen kredensial", "rendah", "diperbaiki", false},
		{"Akurasi", "Satuan dosis obat tidak baku", "rendah", "diperbaiki", false},
	}
	var dqRows []model.DataQualityIssue
	for i, x := range issues {
		dqRows = append(dqRows, model.DataQualityIssue{ID: fmt.Sprintf("dq_ns_%02d", i+1), HospitalID: hNS,
			Dimension: sp(x.dim), Title: x.title, Severity: sp(x.sev), Anomaly: x.anomaly, Status: x.status})
	}
	if err := ins(tx, dqRows); err != nil {
		return err
	}

	// 9 faktor Trust Score (bobot total 100, rata-rata berbobot ~92,3)
	factors := []struct {
		name          string
		score, weight float64
	}{
		{"Kelengkapan Data", 95, 15}, {"Akurasi", 94, 15}, {"Konsistensi Lintas Sistem", 90, 12},
		{"Ketepatan Waktu Sinkronisasi", 92, 12}, {"Keunikan Rekaman", 93, 10}, {"Validitas Format", 91, 10},
		{"Tingkat Anomali", 92, 10}, {"Ketersediaan Sumber Data", 90, 8}, {"Keberhasilan Integrasi", 91.4, 8},
	}
	var tf []model.TrustFactor
	for i, f := range factors {
		tf = append(tf, model.TrustFactor{ID: fmt.Sprintf("tf_ns_%02d", i+1), HospitalID: hNS,
			Factor: f.name, Score: f.score, Weight: f.weight})
	}
	if err := ins(tx, tf); err != nil {
		return err
	}

	// indikator digital. Urutan sengaja: "Kelengkapan Data Alergi" (93) muncul sebelum
	// "Skrining Alergi Sebelum Peresepan" (96) - kasus uji urutan indikator di scoring engine.
	type di struct {
		dim, name, fhir, dir string
		val, thr, w          float64
	}
	inds := []di{
		{"Keselamatan Pasien", "Kelengkapan Data Alergi", "AllergyIntolerance", "higher", 93, 90, 10},
		{"Keselamatan Pasien", "Skrining Alergi Sebelum Peresepan", "AllergyIntolerance", "higher", 96, 90, 30},
		{"Keselamatan Pasien", "Closed-loop Medikasi", "MedicationDispense", "higher", 78, 85, 30},
		{"Keselamatan Pasien", "Kelengkapan Resume Pemulangan", "Composition", "higher", 85, 90, 30},
		{"Digital", "Digitalisasi Laboratorium", "DiagnosticReport", "higher", 92, 80, 10},
		{"Digital", "Digitalisasi Radiologi", "ImagingStudy", "higher", 88, 80, 10},
		{"Digital", "Rekam Medis Elektronik Terisi", "Encounter", "higher", 90, 85, 15},
		{"Interoperabilitas", "Latensi API FHIR (ms)", "Patient", "lower", 280, 300, 15},
		{"Interoperabilitas", "Keberhasilan Sinkronisasi (%)", "Encounter", "higher", 94, 90, 15},
		{"Interoperabilitas", "Cakupan Resource FHIR (%)", "Patient", "higher", 88, 85, 15},
		{"Medikasi", "Verifikasi Barcode Obat (%)", "MedicationRequest", "higher", 76, 85, 20},
		{"Diagnostik", "Hasil Lab Terintegrasi (%)", "Observation", "higher", 91, 85, 20},
	}
	var dis []model.DigitalIndicator
	for i, x := range inds {
		dis = append(dis, model.DigitalIndicator{ID: fmt.Sprintf("dig_ns_%02d", i+1), HospitalID: hNS,
			Dimension: sp(x.dim), Indicator: x.name, MetricValue: f64(x.val), Threshold: f64(x.thr),
			Weight: f64(x.w), Direction: sp(x.dir), FHIRResource: sp(x.fhir)})
	}
	if err := ins(tx, dis); err != nil {
		return err
	}

	// keluhan & utilisasi 6 bulan (Sep 2026: keluhan 15, kepuasan 87%, ALOS 5,3, readmisi 4,9%, BOR 81%)
	months := []string{"2026-04", "2026-05", "2026-06", "2026-07", "2026-08", "2026-09"}
	complaints := []int{22, 19, 18, 20, 17, 15}
	satisf := []float64{82, 83, 85, 84, 86, 87}
	alos := []float64{5.9, 5.7, 5.6, 5.5, 5.4, 5.3}
	readm := []float64{6.1, 5.8, 5.5, 5.2, 5.0, 4.9}
	bor := []float64{74, 76, 77, 79, 80, 81}
	var cs []model.ComplaintStat
	var us []model.UtilizationStat
	for i, m := range months {
		cs = append(cs, model.ComplaintStat{ID: "cs_ns_" + m, HospitalID: hNS, Month: m,
			Total: num(complaints[i]), Satisfaction: f64(satisf[i]), PeerAvg: f64(84)})
		us = append(us, model.UtilizationStat{ID: "us_ns_" + m, HospitalID: hNS, Month: m,
			ALOS: f64(alos[i]), Readmission: f64(readm[i]), BOR: f64(bor[i])})
	}
	if err := ins(tx, cs); err != nil {
		return err
	}
	if err := ins(tx, us); err != nil {
		return err
	}

	// tempat tidur: total 240, aktif 196
	beds := []struct {
		ward, class   string
		total, active int
	}{
		{"ICU", "ICU", 12, 10}, {"HCU", "HCU", 8, 6}, {"Penyakit Dalam", "Kelas 1", 40, 34},
		{"Bedah", "Kelas 1", 40, 32}, {"Anak", "Kelas 2", 36, 30}, {"Kebidanan", "Kelas 2", 30, 24},
		{"Perinatologi", "Kelas 2", 14, 10}, {"VIP", "VIP", 20, 16}, {"Umum", "Kelas 3", 40, 34},
	}
	var bs []model.BedStat
	for i, b := range beds {
		bs = append(bs, model.BedStat{ID: fmt.Sprintf("bed_ns_%02d", i+1), HospitalID: hNS,
			Ward: sp(b.ward), BedClass: sp(b.class), Total: num(b.total), Active: num(b.active)})
	}
	if err := ins(tx, bs); err != nil {
		return err
	}

	// standar spesialis kelas B: 13 spesialis (Mata tanpa dokter -> defisit)
	specs := []string{"Penyakit Dalam", "Bedah", "Anak", "Obstetri & Ginekologi", "Anestesiologi", "Radiologi",
		"Patologi Klinik", "Jantung", "Saraf", "Mata", "THT", "Kulit & Kelamin", "Paru"}
	var ss []model.SpecialtyStandard
	for i, name := range specs {
		req := 1
		if name == "Penyakit Dalam" {
			req = 2 // tersedia 3 -> kasus uji cakupan > 100%
		}
		ss = append(ss, model.SpecialtyStandard{ID: fmt.Sprintf("spc_ns_%02d", i+1), HospitalID: hNS,
			RsClass: sp("B"), Specialty: sp(name), Required: num(req)})
	}
	if err := ins(tx, ss); err != nil {
		return err
	}

	// sarana: 14 fasilitas (13 wajib + MRI opsional).
	// Schema belum punya kolom wajib/opsional, jadi dikodekan di kolom category.
	facs := []struct{ name, cat string }{
		{"Ruang Operasi", "wajib"}, {"ICU", "wajib"}, {"IGD", "wajib"}, {"Laboratorium", "wajib"},
		{"Radiologi (X-Ray)", "wajib"}, {"CT Scan", "wajib"}, {"Farmasi", "wajib"}, {"Hemodialisa", "wajib"},
		{"Ambulans", "wajib"}, {"Ruang Isolasi", "wajib"}, {"Bank Darah", "wajib"}, {"Sterilisasi (CSSD)", "wajib"},
		{"Genset", "wajib"}, {"MRI 1.5 Tesla", "opsional"},
	}
	var fs []model.Facility
	for i, f := range facs {
		cal := "valid"
		if i == 4 || i == 5 {
			cal = "segera_kalibrasi"
		}
		fs = append(fs, model.Facility{ID: fmt.Sprintf("fac_ns_%02d", i+1), HospitalID: hNS, Name: f.name,
			Category: sp(f.cat), Availability: bl(true), Functional: bl(true), Calibration: sp(cal)})
	}
	if err := ins(tx, fs); err != nil {
		return err
	}

	// 30 evidence: skor 8, risiko 6, indikator 6, klaim 3, peringatan 3, regulasi 4
	var ev []model.EvidenceItem
	n := 0
	add := func(refType, refID, metric, val, src, fhir string, cnt int, vs string) {
		n++
		ev = append(ev, model.EvidenceItem{ID: fmt.Sprintf("evd_ns_%02d", n), HospitalID: hNS, RefType: refType,
			RefID: sp(refID), Metric: sp(metric), MetricValue: sp(val), SourceSystem: sp(src),
			FHIRResource: sp(fhir), RecordCount: num(cnt), ValidationStatus: sp(vs)})
	}
	for i, m := range []string{"Hospital Health Score", "BDCS", "Trust Score", "CCS", "CPCS", "Utilization Review", "Patient Safety", "Workforce"} {
		add("skor", fmt.Sprintf("skor_%d", i+1), m, "lihat scoring engine", "SIMRS Utama", "Encounter", 100+i*20, "valid")
	}
	for _, r := range []string{"rsk_ns_001", "rsk_ns_001", "rsk_ns_002", "rsk_ns_004", "rsk_ns_007", "rsk_ns_008"} { // RSK-003 sengaja tanpa bukti
		add("risiko", r, "Rekaman terdampak", "terverifikasi", "SIMRS Utama", "Practitioner", 14, "valid")
	}
	for i := 0; i < 6; i++ {
		add("indikator", fmt.Sprintf("dig_ns_%02d", i+1), inds[i].name, fmt.Sprintf("%.0f", inds[i].val), "SIMRS Utama", inds[i].fhir, 250, "valid")
	}
	for _, c := range []string{"clm_ns_28", "clm_ns_28", "clm_ns_42"} { // CLM-00128 (2 bukti), CLM-00142 (1 bukti)
		add("klaim", c, "Dokumen pendukung klaim", "lengkap", "BPJS VClaim", "Claim", 1, "valid")
	}
	for _, w := range []string{"ew_ns_001", "ew_ns_002", "ew_ns_004"} {
		add("peringatan", w, "Pemicu peringatan", "tercatat", "SIMRS Utama", "Observation", 5, "valid")
	}
	for i := 1; i <= 4; i++ {
		add("regulasi", fmt.Sprintf("reg_%02d", i), "Pemetaan pasal", "terpetakan", "Registri Regulasi", "", 2, "valid")
	}
	if err := ins(tx, ev); err != nil {
		return err
	}

	// snapshot skor 3 bulan terakhir (placeholder sampai scoring engine menulis sendiri)
	snap := func(period string, health, bdcs float64) model.ScoreSnapshot {
		return model.ScoreSnapshot{ID: "ss_ns_" + period, HospitalID: hNS, Period: period,
			HealthScore: f64(health), BDCS: f64(bdcs), Trust: f64(92.3), CCS: f64(78.9), CPCS: f64(76.3),
			URScore: f64(98.0), SafetyScore: f64(86.1)}
	}
	return ins(tx, []model.ScoreSnapshot{snap("2026-07", 80.9, 82.4), snap("2026-08", 82.6, 84.7), snap("2026-09", 84.4, 86.6)})
}

// Sumber data, job integrasi, aturan peringatan, notifikasi.
func seedPlatform(tx *gorm.DB) error {
	// 10 sumber data RS Nusantara (7 SIMRS, 1 SATUSEHAT, 2 BPJS) + 18 job
	type dsrc struct{ name, typ, status, auth string }
	list := []dsrc{
		{"SIMRS Utama", "SIMRS", "terhubung", "OAuth2"}, {"SIMRS Farmasi", "SIMRS", "terhubung", "OAuth2"},
		{"SIMRS Laboratorium", "SIMRS", "terhubung", "API Key"}, {"SIMRS Radiologi (PACS)", "SIMRS", "terhubung", "API Key"},
		{"SIMRS Rekam Medis", "SIMRS", "terhubung", "OAuth2"}, {"SIMRS Billing", "SIMRS", "perlu_perbaikan", "API Key"},
		{"SIMRS Kepegawaian", "SIMRS", "terhubung", "API Key"}, {"SATUSEHAT Platform", "SATUSEHAT", "terhubung", "OAuth2"},
		{"BPJS VClaim", "BPJS", "terhubung", "API Key"}, {"BPJS P-Care", "BPJS", "gagal", "API Key"},
	}
	var srcs []model.DataSource
	var jobs []model.IntegrationJob
	jn := 0
	for i, x := range list {
		id := fmt.Sprintf("ds_ns_%02d", i+1)
		rate := 98.0 - float64(i)*0.7
		if x.status == "gagal" {
			rate = 61.5
		}
		srcs = append(srcs, model.DataSource{ID: id, HospitalID: hNS, Name: x.name, Type: sp(x.typ), Status: sp(x.status),
			AuthMethod: sp(x.auth), SuccessRate: f64(rate), CoveragePct: f64(95 - float64(i)*1.5), LastSyncAt: func() *time.Time { t := at(-i % 3); return &t }()})
		count := 2
		if i >= 8 {
			count = 1
		}
		for j := 0; j < count; j++ {
			jn++
			st, jt := "sukses", "sync_inkremental"
			if j == 1 {
				jt = "sync_penuh"
			}
			if x.status == "gagal" {
				st = "gagal"
			}
			jobs = append(jobs, model.IntegrationJob{ID: fmt.Sprintf("job_ns_%02d", jn), DataSourceID: id,
				JobType: sp(jt), Status: sp(st), RecordsPulled: num(400 + jn*37), DurationMs: num(900 + jn*61)})
		}
	}
	// RS Medika: 2 sumber data
	srcs = append(srcs,
		model.DataSource{ID: "ds_ms_01", HospitalID: hMS, Name: "SIMRS Utama", Type: sp("SIMRS"), Status: sp("terhubung"),
			AuthMethod: sp("API Key"), SuccessRate: f64(90), CoveragePct: f64(70)},
		model.DataSource{ID: "ds_ms_02", HospitalID: hMS, Name: "BPJS VClaim", Type: sp("BPJS"), Status: sp("terhubung"),
			AuthMethod: sp("API Key"), SuccessRate: f64(88), CoveragePct: f64(65)})
	if err := ins(tx, srcs); err != nil {
		return err
	}
	if err := ins(tx, jobs); err != nil {
		return err
	}

	// aturan notifikasi
	rules := []struct{ name, domain, metric, cond, sev, ch string }{
		{"Kredensial <= 30 hari", "Kredensial", "expiry_days", "<= 30", "tinggi", "in-app,email"},
		{"Sinyal klaim naik", "Klaim", "anomaly_growth", "> 15%", "tinggi", "in-app,email"},
		{"Latensi FHIR tinggi", "Interoperabilitas", "fhir_latency_ms", "> 300", "sedang", "in-app"},
		{"Adherence pathway rendah", "Clinical Pathway", "avg_adherence", "< 80", "sedang", "in-app"},
		{"Prasyarat mendekati kedaluwarsa", "Prasyarat", "doc_expiry_days", "<= 45", "tinggi", "in-app,whatsapp"},
	}
	var ar []model.AlertRule
	for i, r := range rules {
		ar = append(ar, model.AlertRule{ID: fmt.Sprintf("ar_ns_%02d", i+1), HospitalID: hNS, Name: r.name,
			Domain: sp(r.domain), Metric: sp(r.metric), RuleCondition: sp(r.cond), Severity: sp(r.sev),
			Channels: sp(r.ch), Active: true})
	}
	if err := ins(tx, ar); err != nil {
		return err
	}

	// 6 notifikasi
	notes := []struct{ user, title, sev string }{
		{"u_admin_ns", "13 kredensial mendekati kedaluwarsa", "tinggi"},
		{"u_admin_ns", "Sinyal klaim naik 18%", "tinggi"},
		{"u_admin_ns", "Akreditasi KARS berakhir dalam 115 hari", "tinggi"},
		{"u_cred_ns", "3 kredensial menunggu verifikasi", "sedang"},
		{"u_claim_ns", "58 anomali klaim baru perlu ditinjau", "sedang"},
		{"u_it_ns", "Sinkronisasi BPJS P-Care gagal", "sedang"},
	}
	var nt []model.Notification
	for i, x := range notes {
		nt = append(nt, model.Notification{ID: fmt.Sprintf("ntf_ns_%02d", i+1), HospitalID: sp(hNS),
			UserID: x.user, Title: x.title, Severity: sp(x.sev), Channel: sp("in-app"), IsRead: false})
	}
	return ins(tx, nt)
}
