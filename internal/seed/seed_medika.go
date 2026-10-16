package seed

import (
	"fmt"
	"time"

	"gorm.io/gorm"
	"gorm.io/gorm/clause"

	"github.com/spkd/medcredix/internal/model"
)

// Data kesiapan RS Medika Sejahtera (kelas C, 120 TT): sarana, tempat tidur, standar spesialis,
// profesi praktisi, indikator digital, dan sumber data. Sebelumnya RS Medika hanya punya praktisi,
// kredensial, klaim, dan 2 sumber data, sehingga halaman Kesiapan RS tampil kosong / 0.
//
// Aman dijalankan berulang dan juga mengisi database lama: baris baru ditambah dengan
// ON CONFLICT DO NOTHING, dan baris lama RS Medika hanya diperbarui sekali (lihat penanda di bawah).

// Indikator memakai nama yang sama dengan katalog RS Nusantara agar formula skor
// (EMR Rawat Inap/Jalan, Latensi API, Patient Safety) menemukan indikatornya.
var msIndicatorDefs = []indDef{
	// EMR/RME (Digital Maturity)
	{"EMR", "Adopsi RME Rawat Inap", "%", "Encounter", "EMR/RME", "higher", 72, 85, 40},
	{"EMR", "Adopsi RME Rawat Jalan", "%", "Encounter", "EMR/RME", "higher", 81, 85, 30},
	{"EMR", "Kelengkapan Coding ICD-10", "%", "Condition", "SIMRS", "higher", 86, 90, 10},
	{"EMR", "Validitas NIK Pasien", "%", "Patient", "SIMRS", "higher", 95, 98, 10},
	{"EMR", "Kelengkapan Resume Medis", "%", "Composition", "EMR/RME", "higher", 78, 90, 10},
	// Infrastruktur (Digital Maturity)
	{"Infrastruktur", "Uptime API RS (30 hari)", "%", "", "SIMRS", "higher", 98.1, 99, 30},
	{"Infrastruktur", "Success Rate Submission", "%", "", "SATUSEHAT", "higher", 92.5, 98, 30},
	{"Infrastruktur", "Latensi API Rata-rata", "ms", "", "SIMRS", "lower", 420, 300, 20},
	{"Infrastruktur", "Cakupan Multi-Departemen", "%", "Encounter", "SIMRS", "higher", 70, 90, 20},
	// Medication Management
	{"Medikasi", "CPOE (Computerized Provider Order Entry)", "%", "MedicationRequest", "Farmasi", "higher", 64, 90, 25},
	{"Medikasi", "Dispensing Digital", "%", "MedicationDispense", "Farmasi", "higher", 70, 85, 20},
	{"Medikasi", "Administrasi Obat Digital", "%", "MedicationAdministration", "Farmasi", "higher", 55, 80, 25},
	{"Medikasi", "Closed-Loop Medication", "%", "MedicationAdministration", "Farmasi", "higher", 58, 85, 20},
	{"Medikasi", "Kelengkapan Data Alergi", "%", "AllergyIntolerance", "EMR/RME", "higher", 88, 95, 10},
	// Diagnostic Readiness
	{"Diagnostik", "Digitalisasi Laboratorium", "%", "DiagnosticReport", "LIS", "higher", 85, 90, 25},
	{"Diagnostik", "Digitalisasi Radiologi", "%", "ImagingStudy", "SIMRS", "higher", 74, 90, 25},
	{"Diagnostik", "Kelengkapan DiagnosticReport", "%", "DiagnosticReport", "LIS", "higher", 80, 90, 20},
	{"Diagnostik", "Lab Turnaround Time", "jam", "DiagnosticReport", "LIS", "lower", 4.5, 4, 15},
	{"Diagnostik", "Critical Value Flagging", "%", "Observation", "LIS", "higher", 78, 95, 15},
	// Interoperabilitas
	{"Interoperabilitas", "Submission SATUSEHAT", "%", "All (TDS v2)", "SATUSEHAT", "higher", 82, 95, 35},
	{"Interoperabilitas", "Validasi FHIR", "%", "All (TDS v2)", "SATUSEHAT", "higher", 79, 95, 35},
	{"Interoperabilitas", "Sukses Integrasi BPJS", "%", "Claim", "BPJS VClaim", "higher", 90, 95, 30},
	// Keselamatan Pasien
	{"Keselamatan", "Keamanan Alergi Obat", "%", "AllergyIntolerance", "EMR/RME", "higher", 92, 95, 40},
	{"Keselamatan", "High-Alert & Closed-Loop", "%", "MedicationAdministration", "Farmasi", "higher", 70, 80, 30},
	{"Keselamatan", "Dokumentasi Pemulangan Pasien", "%", "Composition", "EMR/RME", "higher", 82, 80, 30},
}

func msIndicators() []model.DigitalIndicator {
	out := make([]model.DigitalIndicator, 0, len(msIndicatorDefs))
	for i, x := range msIndicatorDefs {
		out = append(out, model.DigitalIndicator{ID: fmt.Sprintf("dig_ms_%02d", i+1), HospitalID: hMS,
			Dimension: sp(x.dim), Indicator: x.name, MetricValue: f64(x.val), Threshold: f64(x.thr),
			Weight: f64(x.w), Direction: sp(x.dir), FHIRResource: sp(x.fhir), Unit: sp(x.unit), SourceSystem: sp(x.src)})
	}
	return out
}

// ds_ms_01 & ds_ms_02 sudah ada sejak seed awal (nama & tipe dipertahankan), 03-06 baru.
var msSourceDefs = []srcDef{
	{"SIMRS Utama", "SIMRS", "terhubung", "API Key",
		"Base URL FHIR internal; 16 resource TDS v2.0.0", "", 90, 70, 98.1, 420, 16, 2},
	{"BPJS VClaim", "BPJS VClaim", "terhubung", "API Key",
		"Kode faskes 3374R007; INACBGs bundling", "", 88, 65, 96.8, 520, 2, 20},
	{"BPJS P-Care / JKN Mobile", "BPJS P-Care", "terhubung_sebagian", "User Key + Secret",
		"Data keluhan & kepuasan JKN", "Sinkronisasi data kepuasan tertunda 2 hari (antrean vendor).", 86, 60, 96.5, 610, 1, 48},
	{"SATUSEHAT Platform", "SATUSEHAT", "terhubung_sebagian", "OAuth 2.0 (Client ID/Secret Kemenkes)",
		"Organization ID org-033-medika; cross-reference Encounter/Patient",
		"Resource Composition & MedicationDispense belum dikirim (menunggu modul EMR rawat inap).", 82, 72, 98.9, 680, 14, 30},
	{"LIS (Laboratory Information System)", "LIS", "terhubung", "API Key",
		"Hasil lab, critical values, TAT", "", 93, 85, 98.6, 240, 3, 6},
	{"Sistem Farmasi", "Farmasi", "terhubung", "API Key",
		"e-Resep, dispensing, stok", "", 89, 78, 98, 380, 3, 6},
}

func msSources() []model.DataSource {
	out := make([]model.DataSource, 0, len(msSourceDefs))
	for i, x := range msSourceDefs {
		sync := refDate.Add(8 * time.Hour).Add(-time.Duration(x.syncHoursAgo) * time.Hour)
		out = append(out, model.DataSource{ID: fmt.Sprintf("ds_ms_%02d", i+1), HospitalID: hMS, Name: x.name,
			Type: sp(x.typ), Status: sp(x.status), AuthMethod: sp(x.auth), SuccessRate: f64(x.success),
			CoveragePct: f64(x.coverage), LastSyncAt: &sync, ConfigSummary: sp(x.config), AvgLatencyMs: num(x.latency),
			Uptime30d: f64(x.uptime), FHIRResources: num(x.fhir), Notes: sp(x.notes)})
	}
	return out
}

// profesi praktisi RS Medika (pms_00..09); seed awal menulis semuanya "Tenaga Kesehatan" / "Umum",
// sehingga cakupan spesialis tidak bisa dihitung.
var msStaffProfile = []struct{ prof, spec, dept string }{
	{"Dokter Umum", "", "IGD"},                               // dr. Arif Budiman
	{"Dokter Umum", "", "Poli Umum"},                         // dr. Bella Kusuma
	{"Dokter Spesialis", "Penyakit Dalam", "Penyakit Dalam"}, // dr. Candra Wijaya, Sp.PD
	{"Dokter Spesialis", "Anak", "Anak"},                     // dr. Dian Pertiwi, Sp.A
	{"Dokter Umum", "", "Rawat Inap"},                        // dr. Edo Saputra
	{"Perawat", "", "Rawat Inap"},                            // Ns. Fani Rahayu
	{"Perawat", "", "IGD"},                                   // Ns. Gilang Ramadhan
	{"Bidan", "", "Kebidanan"},                               // Bd. Hesti Lestari
	{"Apoteker", "", "Farmasi"},                              // apt. Irfan Hakim
	{"Dokter Umum", "", "Poli Umum"},                         // dr. Jihan Aulia
}

func seedMedikaReadiness(tx *gorm.DB) error {
	// profesi & spesialisasi praktisi (hanya baris yang masih berisi isian awal)
	for i, p := range msStaffProfile {
		if err := tx.Model(&model.Practitioner{}).
			Where("id = ? AND hospital_id = ? AND profession = ?", fmt.Sprintf("pms_%02d", i), hMS, "Tenaga Kesehatan").
			Updates(map[string]interface{}{"profession": p.prof, "specialty": sp(p.spec), "department": p.dept}).Error; err != nil {
			return err
		}
	}

	// standar spesialis kelas C: 4 spesialis dasar + 3 penunjang medik (Sp.PD & Sp.A tersedia)
	specs := []string{"Penyakit Dalam", "Anak", "Bedah", "Obstetri & Ginekologi", "Anestesiologi", "Radiologi", "Patologi Klinik"}
	var ss []model.SpecialtyStandard
	for i, name := range specs {
		ss = append(ss, model.SpecialtyStandard{ID: fmt.Sprintf("spc_ms_%02d", i+1), HospitalID: hMS,
			RsClass: sp("C"), Specialty: sp(name), Required: num(1)})
	}
	if err := ins(tx, ss); err != nil {
		return err
	}

	// sarana: 12 fasilitas (11 wajib kelas C + CT Scan opsional yang belum ada)
	facs := []struct {
		name, cat, cal string
		avail, fn      bool
	}{
		{"Ruang Operasi", "wajib", "valid", true, true}, {"ICU", "wajib", "valid", true, true},
		{"IGD", "wajib", "valid", true, true}, {"Laboratorium", "wajib", "valid", true, true},
		{"Radiologi (X-Ray)", "wajib", "segera_kalibrasi", true, true}, {"Farmasi", "wajib", "valid", true, true},
		{"Ambulans", "wajib", "valid", true, true}, {"Ruang Isolasi", "wajib", "valid", true, true},
		{"Bank Darah", "wajib", "valid", true, true}, {"Sterilisasi (CSSD)", "wajib", "valid", true, false},
		{"Genset", "wajib", "valid", true, true}, {"CT Scan", "opsional", "belum_kalibrasi", false, false},
	}
	var fs []model.Facility
	for i, f := range facs {
		fs = append(fs, model.Facility{ID: fmt.Sprintf("fac_ms_%02d", i+1), HospitalID: hMS, Name: f.name,
			Category: sp(f.cat), Availability: bl(f.avail), Functional: bl(f.fn), Calibration: sp(f.cal)})
	}
	if err := ins(tx, fs); err != nil {
		return err
	}

	// tempat tidur: total 120 (= TT terlisensi), aktif 99
	beds := []struct {
		ward, class   string
		total, active int
	}{
		{"ICU", "ICU", 6, 5}, {"Penyakit Dalam", "Kelas 2", 24, 20}, {"Bedah", "Kelas 2", 20, 16},
		{"Anak", "Kelas 2", 18, 15}, {"Kebidanan", "Kelas 2", 16, 13}, {"VIP", "VIP", 8, 6}, {"Umum", "Kelas 3", 28, 24},
	}
	var bs []model.BedStat
	for i, b := range beds {
		bs = append(bs, model.BedStat{ID: fmt.Sprintf("bed_ms_%02d", i+1), HospitalID: hMS,
			Ward: sp(b.ward), BedClass: sp(b.class), Total: num(b.total), Active: num(b.active)})
	}
	if err := ins(tx, bs); err != nil {
		return err
	}

	// indikator digital, medikasi, diagnostik, interoperabilitas, keselamatan
	if err := ins(tx, msIndicators()); err != nil {
		return err
	}

	// sumber data: ds_ms_01/02 lama belum punya latensi/uptime -> diperbarui sekali, sisanya ditambah
	var n int64
	if err := tx.Model(&model.DataSource{}).
		Where("id = ? AND avg_latency_ms IS NULL", "ds_ms_01").Count(&n).Error; err != nil {
		return err
	}
	if n > 0 {
		return tx.Clauses(clause.OnConflict{UpdateAll: true}).Create(msSources()).Error
	}
	return ins(tx, msSources())
}
