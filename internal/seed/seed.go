package seed

import (
	"errors"
	"log"

	"golang.org/x/crypto/bcrypt"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"

	"github.com/spkd/medcredix/internal/appenv"
	"github.com/spkd/medcredix/internal/model"
)

// Kata sandi semua akun demo (sesuai sheet "Data Uji & Akun").
// Hanya untuk lokal/UAT.
const DemoPassword = "medcredix2026"

func s(v string) *string { return &v }

// Run mengisi 2 rumah sakit, 12 akun demo, lalu seluruh data uji (RunDemoData).
// Aman dijalankan berulang: data yang sudah ada dilewati (ON CONFLICT DO NOTHING).
func Run(d *gorm.DB) error {
	if appenv.IsProduction() {
		return errors.New("seed demo ditolak: APP_ENV=production")
	}
	hash, err := bcrypt.GenerateFromPassword([]byte(DemoPassword), bcrypt.DefaultCost)
	if err != nil {
		return err
	}
	pw := string(hash)

	// Kode, provinsi, kota, dan jumlah TT RS Medika adalah isian sementara: ganti sesuai data aslimu.
	hospitals := []model.Hospital{
		{ID: "h_nusantara", Name: "RS Nusantara Sehat", Code: "RSNS", HospitalClass: s("B"), BedLicensed: 240},
		{ID: "h_medika", Name: "RS Medika Sejahtera", Code: "RSMS", HospitalClass: s("C"), BedLicensed: 120},
	}

	ns, ms := s("h_nusantara"), s("h_medika")
	users := []model.User{
		{ID: "u_super", HospitalID: ns, Email: "superadmin@spkd.co.id", Name: "Surya Atmaja, M.Kom", Role: "super_admin"},
		{ID: "u_admin_ns", HospitalID: ns, Email: "admin@nusantarasehat.co.id", Name: "dr. Aditya Nugraha, MARS", Role: "admin_rs"},
		{ID: "u_mgmt_ns", HospitalID: ns, Email: "manajemen@nusantarasehat.co.id", Name: "dr. Kartika Dewi", Role: "hospital_mgmt"},
		{ID: "u_cred_ns", HospitalID: ns, Email: "kredensial@nusantarasehat.co.id", Name: "Ratna Puspitasari, S.KM", Role: "credential_officer"},
		{ID: "u_komed_ns", HospitalID: ns, Email: "komite@nusantarasehat.co.id", Name: "dr. Bambang Sutrisno, Sp.PD-KIC", Role: "medical_committee"},
		{ID: "u_sdm_ns", HospitalID: ns, Email: "sdm@nusantarasehat.co.id", Name: "Tri Handayani, S.Psi", Role: "hr_sdm"},
		{ID: "u_audit", HospitalID: ns, Email: "auditor@spkd.co.id", Name: "Ir. Gunawan Wibisono, MHSA", Role: "auditor"},
		{ID: "u_claim_ns", HospitalID: ns, Email: "klaim@nusantarasehat.co.id", Name: "Silvia Rahmawati, AMd.Kep", Role: "claims_team"},
		{ID: "u_it_ns", HospitalID: ns, Email: "it@nusantarasehat.co.id", Name: "Andika Pratama, S.Kom", Role: "it_simrs"},
		{ID: "u_view_ns", HospitalID: ns, Email: "viewer@nusantarasehat.co.id", Name: "Dewi Anggraini, S.E", Role: "viewer"},
		{ID: "u_admin_ms", HospitalID: ms, Email: "admin@medikasejahtera.co.id", Name: "dr. Rina Oktaviani", Role: "admin_rs"},
		{ID: "u_audit_bpjs", HospitalID: nil, Email: "auditorbpjs@spkd.co.id", Name: "Joko Susilo, S.Sos", Role: "auditor"},
	}
	for i := range users {
		users[i].PasswordHash = pw
		users[i].Active = true
	}

	err = d.Transaction(func(tx *gorm.DB) error {
		onConflict := clause.OnConflict{DoNothing: true}
		if err := tx.Clauses(onConflict).Create(&hospitals).Error; err != nil {
			return err
		}
		return tx.Clauses(onConflict).Create(&users).Error
	})
	if err != nil {
		return err
	}
	log.Printf("seed dasar selesai: %d rumah sakit, %d akun demo", len(hospitals), len(users))

	return RunDemoData(d)
}
