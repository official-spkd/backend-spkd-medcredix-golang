package seed

import (
	"fmt"
	"time"

	"gorm.io/gorm"
	"gorm.io/gorm/clause"

	"github.com/spkd/medcredix/internal/model"
)

// Katalog indikator digital & kanal integrasi RS Nusantara Sehat, sesuai test case QA:
//   - Digital Maturity: Adopsi RME Rawat Inap & Rawat Jalan (dipakai formula EMR), plus
//     Latensi API Rata-rata di Infrastruktur.
//   - Medikasi: CPOE, Dispensing Digital, Administrasi Obat Digital, Closed-Loop Medication,
//     Kelengkapan Data Alergi.
//   - Diagnostik: Digitalisasi Laboratorium & Radiologi, Kelengkapan DiagnosticReport,
//     Lab Turnaround Time, Critical Value Flagging.
//   - Kanal integrasi eksternal: SIMRS Inti Sehat, BPJS VClaim, BPJS P-Care / JKN Mobile, SATUSEHAT.

type indDef struct {
	dim, name, unit, fhir, src, dir string
	val, thr, w                     float64
}

var nsIndicatorDefs = []indDef{
	// EMR/RME (Digital Maturity)
	{"EMR", "Adopsi RME Rawat Inap", "%", "Encounter", "EMR/RME", "higher", 88, 85, 40},
	{"EMR", "Adopsi RME Rawat Jalan", "%", "Encounter", "EMR/RME", "higher", 79, 85, 30},
	{"EMR", "Kelengkapan Coding ICD-10", "%", "Condition", "SIMRS", "higher", 91, 90, 10},
	{"EMR", "Validitas NIK Pasien", "%", "Patient", "SIMRS", "higher", 97, 98, 10},
	{"EMR", "Kelengkapan Resume Medis", "%", "Composition", "EMR/RME", "higher", 86, 90, 10},
	// Infrastruktur (Digital Maturity)
	{"Infrastruktur", "Uptime API RS (30 hari)", "%", "", "SIMRS", "higher", 99.2, 99, 30},
	{"Infrastruktur", "Success Rate Submission", "%", "", "SATUSEHAT", "higher", 96.4, 98, 30},
	{"Infrastruktur", "Latensi API Rata-rata", "ms", "", "SIMRS", "lower", 340, 300, 20},
	{"Infrastruktur", "Cakupan Multi-Departemen", "%", "Encounter", "SIMRS", "higher", 84, 90, 20},
	// Medication Management
	{"Medikasi", "CPOE (Computerized Provider Order Entry)", "%", "MedicationRequest", "Farmasi", "higher", 82, 90, 25},
	{"Medikasi", "Dispensing Digital", "%", "MedicationDispense", "Farmasi", "higher", 76, 85, 20},
	{"Medikasi", "Administrasi Obat Digital", "%", "MedicationAdministration", "Farmasi", "higher", 68, 80, 25},
	{"Medikasi", "Closed-Loop Medication", "%", "MedicationAdministration", "Farmasi", "higher", 74, 85, 20},
	{"Medikasi", "Kelengkapan Data Alergi", "%", "AllergyIntolerance", "EMR/RME", "higher", 93, 95, 10},
	// Diagnostic Readiness
	{"Diagnostik", "Digitalisasi Laboratorium", "%", "DiagnosticReport", "LIS", "higher", 92, 90, 25},
	{"Diagnostik", "Digitalisasi Radiologi", "%", "ImagingStudy", "RIS/PACS", "higher", 89, 90, 25},
	{"Diagnostik", "Kelengkapan DiagnosticReport", "%", "DiagnosticReport", "LIS", "higher", 88, 90, 20},
	{"Diagnostik", "Lab Turnaround Time", "jam", "DiagnosticReport", "LIS", "lower", 3.2, 4, 15},
	{"Diagnostik", "Critical Value Flagging", "%", "Observation", "LIS", "higher", 85, 95, 15},
	// Interoperabilitas
	{"Interoperabilitas", "Submission SATUSEHAT", "%", "All (TDS v2)", "SATUSEHAT", "higher", 88, 95, 35},
	{"Interoperabilitas", "Validasi FHIR", "%", "All (TDS v2)", "SATUSEHAT", "higher", 84, 95, 35},
	{"Interoperabilitas", "Sukses Integrasi BPJS", "%", "Claim", "BPJS VClaim", "higher", 89, 95, 30},
	// Keselamatan Pasien (dipakai formula Patient Safety lewat kata kunci Alergi/High-Alert/Pemulangan)
	{"Keselamatan", "Keamanan Alergi Obat", "%", "AllergyIntolerance", "EMR/RME", "higher", 96, 95, 40},
	{"Keselamatan", "High-Alert & Closed-Loop", "%", "MedicationAdministration", "Farmasi", "higher", 78, 80, 30},
	{"Keselamatan", "Dokumentasi Pemulangan Pasien", "%", "Composition", "EMR/RME", "higher", 85, 80, 30},
}

func nsIndicators() []model.DigitalIndicator {
	out := make([]model.DigitalIndicator, 0, len(nsIndicatorDefs))
	for i, x := range nsIndicatorDefs {
		out = append(out, model.DigitalIndicator{ID: fmt.Sprintf("dig_ns_%02d", i+1), HospitalID: hNS,
			Dimension: sp(x.dim), Indicator: x.name, MetricValue: f64(x.val), Threshold: f64(x.thr),
			Weight: f64(x.w), Direction: sp(x.dir), FHIRResource: sp(x.fhir), Unit: sp(x.unit), SourceSystem: sp(x.src)})
	}
	return out
}

type srcDef struct {
	name, typ, status, auth, config, notes string
	success, coverage, uptime              float64
	latency, fhir                          int
	syncHoursAgo                           int
}

// Urutan dipertahankan (id ds_ns_01..10) agar job integrasi yang sudah ada tetap menunjuk sumber yang sama.
var nsSourceDefs = []srcDef{
	{"SIMRS Inti Sehat (Core HIS)", "SIMRS", "terhubung", "OAuth 2.0 Client Credentials",
		"Base URL FHIR internal; 24 resource TDS v2.0.0", "", 98.6, 96, 99.2, 210, 24, 1},
	{"EMR/RME Terpadu", "EMR/RME", "terhubung", "OAuth 2.0 Client Credentials",
		"CPPT, resume medis, asuhan keperawatan", "", 97.2, 92, 98.9, 260, 19, 1},
	{"HRIS Nusantara", "HRIS", "terhubung", "API Key",
		"Praktisi, lisensi, sertifikasi (endpoint REST)", "", 99.1, 98, 99.8, 140, 3, 3},
	{"BPJS VClaim (e-Klaim)", "BPJS VClaim", "terhubung_sebagian", "OAuth 2.0 + User Key",
		"Kode faskes 3301R011; INACBGs bundling",
		"Intermittent timeout pada jam sibuk 10.00-12.00; eskalasi ke vendor middleware.", 89.3, 84, 97.1, 480, 2, 18},
	{"BPJS P-Care / JKN Mobile", "BPJS P-Care", "terhubung", "User Key + Secret",
		"Data keluhan & kepuasan JKN", "", 94.7, 90, 98.2, 390, 1, 12},
	{"SATUSEHAT Platform", "SATUSEHAT", "terhubung_sebagian", "OAuth 2.0 (Client ID/Secret Kemenkes)",
		"Organization ID org-034-9f2a-nusantara; cross-reference Encounter/Patient",
		"2 resource ditolak karena field wajib kosong (Composition, MedicationDispense).", 88, 88, 99.4, 620, 24, 30},
	{"LIS (Laboratory Information System)", "LIS", "terhubung", "API Key",
		"Hasil lab, critical values, TAT", "", 96.4, 94, 99, 180, 4, 6},
	{"RIS/PACS (Radiologi)", "RIS/PACS", "terhubung", "DICOM + API Key",
		"Hasil radiologi & imaging study", "", 95.8, 92, 98.7, 320, 3, 6},
	{"Sistem Farmasi", "Farmasi", "terhubung_sebagian", "API Key",
		"e-Resep, dispensing, administrasi, stok", "42 rekaman dispensasi gagal sinkron (retry queue aktif).", 91.5, 87, 98.4, 410, 4, 6},
	{"Sistem Keuangan & Kasir", "Keuangan", "terhubung", "API Key",
		"Piutang BPJS, rekonsiliasi pembayaran", "", 97.9, 95, 99.5, 150, 2, 12},
}

func nsSources() []model.DataSource {
	out := make([]model.DataSource, 0, len(nsSourceDefs))
	for i, x := range nsSourceDefs {
		sync := refDate.Add(8 * time.Hour).Add(-time.Duration(x.syncHoursAgo) * time.Hour)
		out = append(out, model.DataSource{ID: fmt.Sprintf("ds_ns_%02d", i+1), HospitalID: hNS, Name: x.name,
			Type: sp(x.typ), Status: sp(x.status), AuthMethod: sp(x.auth), SuccessRate: f64(x.success),
			CoveragePct: f64(x.coverage), LastSyncAt: &sync, ConfigSummary: sp(x.config), AvgLatencyMs: num(x.latency),
			Uptime30d: f64(x.uptime), FHIRResources: num(x.fhir), Notes: sp(x.notes)})
	}
	return out
}

// syncDemoCatalog memperbarui data uji lama (dari seed versi sebelumnya) ke katalog di atas.
// Hanya jalan sekali: penanda katalog lama = indikator "Rekam Medis Elektronik Terisi" /
// sumber data "SIMRS Utama" milik RS Nusantara. Database yang sudah memakai katalog baru tidak disentuh.
func syncDemoCatalog(tx *gorm.DB) error {
	var n int64
	if err := tx.Model(&model.DigitalIndicator{}).
		Where("hospital_id = ? AND indicator = ?", hNS, "Rekam Medis Elektronik Terisi").Count(&n).Error; err != nil {
		return err
	}
	if n > 0 {
		if err := tx.Where("hospital_id = ? AND id LIKE ?", hNS, "dig_ns_%").Delete(&model.DigitalIndicator{}).Error; err != nil {
			return err
		}
		if err := tx.Create(nsIndicators()).Error; err != nil {
			return err
		}
		// bukti data indikator (seed lama menunjuk indikator dig_ns_01..06 dengan nama lama)
		for i := 0; i < 6; i++ {
			x := nsIndicatorDefs[i]
			if err := tx.Model(&model.EvidenceItem{}).
				Where("hospital_id = ? AND ref_type = ? AND ref_id = ?", hNS, "indikator", fmt.Sprintf("dig_ns_%02d", i+1)).
				Updates(map[string]interface{}{"metric": x.name, "metric_value": fmt.Sprintf("%g", x.val),
					"source_system": x.src, "fhir_resource": sp(x.fhir)}).Error; err != nil {
				return err
			}
		}
	}
	if err := tx.Model(&model.DataSource{}).
		Where("id = ? AND name = ?", "ds_ns_01", "SIMRS Utama").Count(&n).Error; err != nil {
		return err
	}
	if n > 0 {
		if err := tx.Clauses(clause.OnConflict{UpdateAll: true}).Create(nsSources()).Error; err != nil {
			return err
		}
		// RS Medika: tipe "BPJS" lama -> "BPJS VClaim" supaya tampil di kanal integrasi.
		if err := tx.Model(&model.DataSource{}).Where("id = ? AND type = ?", "ds_ms_02", "BPJS").
			Update("type", "BPJS VClaim").Error; err != nil {
			return err
		}
	}
	return nil
}
