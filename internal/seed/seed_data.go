package seed

import (
	"fmt"
	"log"
	"time"

	"gorm.io/gorm"
	"gorm.io/gorm/clause"

	"github.com/spkd/medcredix/internal/model"
)

// Tanggal acuan data uji: "Data seed relatif terhadap 03-10-2026".
var refDate = time.Date(2026, 10, 3, 0, 0, 0, 0, time.UTC)

func day(n int) *time.Time   { t := refDate.AddDate(0, 0, n); return &t }
func at(n int) time.Time     { return refDate.AddDate(0, 0, n).Add(8*time.Hour + 30*time.Minute) }
func f64(v float64) *float64 { return &v }
func num(v int) *int         { return &v }
func bl(v bool) *bool        { return &v }
func sp(v string) *string {
	if v == "" {
		return nil
	}
	return &v
}

// ins menyisipkan baris; yang sudah ada dilewati (ON CONFLICT DO NOTHING).
func ins[T any](tx *gorm.DB, rows []T) error {
	if len(rows) == 0 {
		return nil
	}
	return tx.Clauses(clause.OnConflict{DoNothing: true}).CreateInBatches(rows, 100).Error
}

const (
	hNS = "h_nusantara"
	hMS = "h_medika"
)

// RunDemoData mengisi seluruh data uji selain hospitals & users.
// Urutan mengikuti foreign key. Semua dalam satu transaksi.
func RunDemoData(d *gorm.DB) error {
	return d.Transaction(func(tx *gorm.DB) error {
		steps := []struct {
			name string
			fn   func(*gorm.DB) error
		}{
			{"regulasi", seedRegulations},
			{"praktisi & kredensial", seedWorkforce},
			{"prasyarat", seedRequirements},
			{"risiko, peringatan, aksi", seedRiskWarningAction},
			{"klaim & anomali", seedClaims},
			{"clinical pathway", seedPathways},
			{"data intelligence", seedIntelligence},
			{"platform (sumber data, notifikasi)", seedPlatform},
		}
		for _, st := range steps {
			if err := st.fn(tx); err != nil {
				return fmt.Errorf("seed %s: %w", st.name, err)
			}
		}
		log.Println("seed data uji selesai")
		return nil
	})
}

// regulasi

func seedRegulations(tx *gorm.DB) error {
	// CATATAN: nomor/judul regulasi adalah isian awal - cek ulang sebelum UAT.
	type reg struct {
		code, title, number, authority string
		year                           int
	}
	list := []reg{
		{"UU-17-2023", "Undang-Undang Kesehatan", "17/2023", "Pemerintah RI / DPR", 2023},
		{"PP-28-2024", "Peraturan Pelaksanaan UU Kesehatan", "28/2024", "Pemerintah RI", 2024},
		{"UU-27-2022", "Pelindungan Data Pribadi", "27/2022", "Pemerintah RI / DPR", 2022},
		{"PMK-24-2022", "Rekam Medis", "24/2022", "Kementerian Kesehatan", 2022},
		{"PMK-3-2020", "Klasifikasi dan Perizinan Rumah Sakit", "3/2020", "Kementerian Kesehatan", 2020},
		{"PMK-12-2020", "Akreditasi Rumah Sakit", "12/2020", "Kementerian Kesehatan", 2020},
		{"PMK-11-2017", "Keselamatan Pasien", "11/2017", "Kementerian Kesehatan", 2017},
		{"PMK-18-2022", "Penyelenggaraan Satu Data Kesehatan (SATUSEHAT)", "18/2022", "Kementerian Kesehatan", 2022},
		{"PMK-26-2021", "Pedoman INA-CBG dalam Pelaksanaan JKN", "26/2021", "Kementerian Kesehatan", 2021},
		{"PMK-3-2023", "Standar Tarif Pelayanan Kesehatan dalam JKN", "3/2023", "Kementerian Kesehatan", 2023},
		{"PERPRES-82-2018", "Jaminan Kesehatan", "82/2018", "Presiden RI", 2018},
	}
	var regs []model.Regulation
	var vers []model.RegulationVersion
	var maps []model.ComplianceMapping
	impacts := []string{"tinggi", "sedang", "rendah"}
	modules := []string{"Kredensial", "Prasyarat Wajib", "Claim Intelligence", "Data Intelligence", "Audit Trail", "Clinical Intelligence"}

	for i, r := range list {
		id := fmt.Sprintf("reg_%02d", i+1)
		regs = append(regs, model.Regulation{ID: id, Code: r.code, Title: r.title,
			Number: sp(r.number), Authority: sp(r.authority), Year: num(r.year), Status: "active"})
		if i < 7 { // 7 versi total (baseline: Regulasi 11, versi 7)
			issued := time.Date(r.year, 6, 1, 0, 0, 0, 0, time.UTC)
			vers = append(vers, model.RegulationVersion{ID: fmt.Sprintf("regv_%02d", i+1), RegulationID: id,
				Version: "v1", IssuedAt: &issued, ImpactLevel: sp(impacts[i%3])})
		}
		for j := 0; j < 2; j++ { // 22 pemetaan kepatuhan
			n := i*2 + j
			status := "sebagian"
			switch {
			case n < 7:
				status = "patuh"
			case n >= 19:
				status = "sedang_ditinjau"
			}
			maps = append(maps, model.ComplianceMapping{
				ID: fmt.Sprintf("cm_ns_%02d", n+1), RegulationID: id, HospitalID: hNS,
				Article: sp(fmt.Sprintf("Pasal %d", 3+n)), Module: sp(modules[n%len(modules)]),
				Feature: sp("Fitur terkait " + r.code), Status: sp(status),
				Owner: sp("Admin RS"), ReviewedAt: func() *time.Time { t := at(-n); return &t }(),
			})
		}
	}
	if err := ins(tx, regs); err != nil {
		return err
	}
	if err := ins(tx, vers); err != nil {
		return err
	}
	return ins(tx, maps)
}

// praktisi & kredensial

type staff struct{ name, prof, spec, dept string }

// 28 praktisi RS Nusantara. Indeks 0,7,11,13,15,19,23,24 mengikuti nama di test case.
// Spesialis mencakup 12 dari 13 spesialis standar kelas B: Mata sengaja kosong (defisit).
var nsStaff = []staff{
	{"dr. Agus Prasetyo, Sp.PD", "Dokter Spesialis", "Penyakit Dalam", "Penyakit Dalam"},
	{"dr. Budi Santoso, Sp.B", "Dokter Spesialis", "Bedah", "Bedah"},
	{"dr. Citra Lestari, Sp.A", "Dokter Spesialis", "Anak", "Anak"},
	{"dr. Dewi Anggraeni, Sp.OG", "Dokter Spesialis", "Obstetri & Ginekologi", "Kebidanan"},
	{"dr. Eko Wahyudi, Sp.PD", "Dokter Spesialis", "Penyakit Dalam", "Penyakit Dalam"},
	{"dr. Farida Hanum, Sp.N", "Dokter Spesialis", "Saraf", "Saraf"},
	{"dr. Gunawan Setiadi, Sp.Rad", "Dokter Spesialis", "Radiologi", "Radiologi"},
	{"dr. Rudi Hartono, Sp.JP", "Dokter Spesialis", "Jantung", "Jantung"},
	{"dr. Hendra Kusuma, Sp.P", "Dokter Spesialis", "Paru", "Paru"},
	{"dr. Indra Wijaya, Sp.THT-KL", "Dokter Spesialis", "THT", "THT"},
	{"dr. Jeni Marlina, Sp.KK", "Dokter Spesialis", "Kulit & Kelamin", "Kulit & Kelamin"},
	{"dr. Fitri Handayani, Sp.An", "Dokter Spesialis", "Anestesiologi", "Anestesi"},
	{"dr. Kurniawan Adi, Sp.PD", "Dokter Spesialis", "Penyakit Dalam", "Penyakit Dalam"},
	{"dr. Luna Kartika, Sp.PK", "Dokter Spesialis", "Patologi Klinik", "Laboratorium"},
	{"dr. Marwan Hakim, Sp.B", "Dokter Spesialis", "Bedah", "Bedah"},
	{"dr. Tono Sugiarto", "Dokter Umum", "", "IGD"},
	{"dr. Nina Safitri", "Dokter Umum", "", "Poli Umum"},
	{"dr. Oka Pratama", "Dokter Umum", "", "IGD"},
	{"drg. Putri Ayu", "Dokter Gigi", "", "Poli Gigi"},
	{"dr. Galih Saputra", "Dokter Umum", "", "Poli Umum"},
	{"Ns. Rina Marlina, S.Kep", "Perawat", "", "Rawat Inap"},
	{"Ns. Santi Dewi, S.Kep", "Perawat", "", "Rawat Inap"},
	{"Ns. Taufik Hidayat, S.Kep", "Perawat", "", "ICU"},
	{"Ns. Sri Wahyuni, S.Kep", "Perawat", "", "IGD"},
	{"Ns. Indah Permatasari, S.Kep", "Perawat", "", "Rawat Inap"},
	{"Bd. Umi Kalsum, S.Tr.Keb", "Bidan", "", "Kebidanan"},
	{"apt. Vina Maulida, S.Farm", "Apoteker", "", "Farmasi"},
	{"Wawan Setiawan, Amd.AK", "Analis Kesehatan", "", "Laboratorium"},
}

func seedWorkforce(tx *gorm.DB) error {
	// RS Nusantara: 28 praktisi, 71 kredensial (56 STR/SIP + 15 sertifikat)
	override := map[string]string{
		"c_pns07_sip": "verifikasi", "c_pns23_sip": "verifikasi", "c_pns15_str": "verifikasi",
		"c_pns13_str": "tinjauan_komite", "c_pns19_sip": "tinjauan_komite",
		"c_pns11_sip": "ditangguhkan", "c_pns00_str": "aktif",
		"c_pns24_sip": "kedaluwarsa", "c_pns05_sip": "kedaluwarsa",
	}
	overrideExp := map[string]int{"c_pns24_sip": -10, "c_pns05_sip": -35}
	segeraOffsets := []int{5, 12, 18, 25, 29, 40, 55, 63, 70, 80, 85, 90} // 5 <=30 hari, 7 <=90 hari
	seg := 0

	var creds []model.PractitionerCredential
	strExp := map[string]*time.Time{}
	sipExp := map[string]*time.Time{}

	add := func(pid, id, typ, number string, k int) {
		status := "aktif"
		exp := 400 + (k*37)%700
		if st, found := override[id]; found {
			status = st
			if o, has := overrideExp[id]; has {
				exp = o
			}
		} else if seg < len(segeraOffsets) && k%5 == 2 {
			status, exp = "segera_berakhir", segeraOffsets[seg]
			seg++
		}
		c := model.PractitionerCredential{
			ID: id, PractitionerID: pid, HospitalID: hNS, Type: typ, Number: sp(number),
			IssueDate: day(exp - 1095), ExpiryDate: day(exp), Status: status, Version: 1,
		}
		switch status {
		case "aktif", "segera_berakhir", "kedaluwarsa", "ditangguhkan":
			c.Version = 2
			c.VerifiedByID, c.VerifiedAt = sp("u_cred_ns"), func() *time.Time { t := at(-200); return &t }()
			c.ApprovedByID, c.ApprovedAt = sp("u_komed_ns"), func() *time.Time { t := at(-199); return &t }()
		case "tinjauan_komite":
			c.VerifiedByID, c.VerifiedAt = sp("u_cred_ns"), func() *time.Time { t := at(-3); return &t }()
		}
		creds = append(creds, c)
		switch typ {
		case "STR":
			strExp[pid] = c.ExpiryDate
		case "SIP":
			sipExp[pid] = c.ExpiryDate
		}
	}

	for i := range nsStaff {
		pid := fmt.Sprintf("pns_%02d", i)
		add(pid, fmt.Sprintf("c_pns%02d_str", i), "STR", fmt.Sprintf("STR-D-%d-%05d", 2019+i%5, 70000+i), 2*i)
		add(pid, fmt.Sprintf("c_pns%02d_sip", i), "SIP", fmt.Sprintf("SIP-%03d/DPMPTSP/2025", 100+i), 2*i+1)
	}
	for i := 0; i < 15; i++ {
		add(fmt.Sprintf("pns_%02d", i), fmt.Sprintf("c_pns%02d_cert", i), "Sertifikat Kompetensi",
			fmt.Sprintf("SK-%04d", 5000+i), 56+i)
	}

	var pracs []model.Practitioner
	for i, st := range nsStaff {
		pid := fmt.Sprintf("pns_%02d", i)
		pracs = append(pracs, model.Practitioner{
			ID: pid, HospitalID: hNS, FullName: st.name, Profession: sp(st.prof), Specialty: sp(st.spec),
			Department: sp(st.dept),
			STRNumber:  sp(fmt.Sprintf("STR-D-%d-%05d", 2019+i%5, 70000+i)), STRExpiry: strExp[pid],
			SIPNumber: sp(fmt.Sprintf("SIP-%03d/DPMPTSP/2025", 100+i)), SIPExpiry: sipExp[pid],
			FHIRID: sp(fmt.Sprintf("Practitioner/ns-%02d", i)),
		})
	}

	// RS Medika: 10 praktisi, 20 kredensial (STR + SIP, semua aktif)
	msNames := []string{"dr. Arif Budiman", "dr. Bella Kusuma", "dr. Candra Wijaya, Sp.PD", "dr. Dian Pertiwi, Sp.A",
		"dr. Edo Saputra", "Ns. Fani Rahayu, S.Kep", "Ns. Gilang Ramadhan, S.Kep", "Bd. Hesti Lestari, S.Tr.Keb",
		"apt. Irfan Hakim, S.Farm", "dr. Jihan Aulia"}
	for i, nm := range msNames {
		pid := fmt.Sprintf("pms_%02d", i)
		exp := 500 + i*45
		pracs = append(pracs, model.Practitioner{
			ID: pid, HospitalID: hMS, FullName: nm, Profession: sp("Tenaga Kesehatan"), Department: sp("Umum"),
			STRNumber: sp(fmt.Sprintf("STR-M-%05d", 80000+i)), STRExpiry: day(exp),
			SIPNumber: sp(fmt.Sprintf("SIP-M-%03d", 200+i)), SIPExpiry: day(exp),
		})
		for _, typ := range []string{"STR", "SIP"} {
			creds = append(creds, model.PractitionerCredential{
				ID:             fmt.Sprintf("c_pms%02d_%s", i, map[string]string{"STR": "str", "SIP": "sip"}[typ]),
				PractitionerID: pid, HospitalID: hMS, Type: typ, Number: sp(fmt.Sprintf("%s-M-%05d", typ, 80000+i)),
				IssueDate: day(exp - 1095), ExpiryDate: day(exp), Status: "aktif", Version: 2,
				VerifiedByID: sp("u_admin_ms"), ApprovedByID: sp("u_admin_ms"),
			})
		}
	}

	if err := ins(tx, pracs); err != nil {
		return err
	}
	return ins(tx, creds)
}

// prasyarat wajib

func seedRequirements(tx *gorm.DB) error {
	type rq struct {
		code, name, cat, src, status, docNo string
		exp                                 int
	}
	// P2/P3/P4 sesuai test case. Nama P1, P5, P6 adalah isian sementara - ganti sesuai dokumen resmi.
	list := []rq{
		{"P1", "Izin Operasional Rumah Sakit", "Perizinan", "PMK 3/2020", "valid", "IZIN-OPS-2024-0017", 900},
		{"P2", "Perjanjian Kerja Sama (PKS) BPJS Kesehatan", "Kerja Sama", "Perpres 82/2018", "valid", "PKS-BPJS-2025-118", 35},
		{"P3", "Sertifikat Akreditasi KARS", "Akreditasi", "PMK 12/2020", "valid", "KARS-2023-0451", 115},
		{"P4", "Izin Radiologi (PKR)", "Perizinan", "Peraturan BAPETEN", "menunggu_verifikasi", "PKR-2026-0033", 600},
		{"P5", "Izin Pengelolaan Limbah B3", "Lingkungan", "Peraturan Lingkungan Hidup", "valid", "LB3-2025-0210", 420},
		{"P6", "Sertifikat Kalibrasi Alat Kesehatan", "Sarana", "PMK 54/2015", "belum_dilengkapi", "", 0},
	}
	var rows []model.Requirement
	for _, r := range list {
		row := model.Requirement{
			ID: "req_ns_" + r.code, HospitalID: hNS, Code: r.code, Name: r.name, Category: sp(r.cat),
			RegulatorySource: sp(r.src), Status: r.status,
			Note: sp("Data demo; nama prasyarat P1/P5/P6 isian sementara"),
		}
		if r.docNo != "" {
			row.DocNumber, row.IssuedAt, row.DocExpiry = sp(r.docNo), day(r.exp-1095), day(r.exp)
			row.FileName = sp(r.code + "_" + r.docNo + ".pdf")
		}
		rows = append(rows, row)
	}
	return ins(tx, rows)
}
