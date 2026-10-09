package db

import (
	_ "embed"
	"fmt"
	"os"
	"strings"

	"gorm.io/driver/postgres"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"

	"github.com/spkd/medcredix/internal/appenv"
)

// schema.sql = ERD resmi MedCredix (SRS Appendix B, 33 tabel) lengkap dengan
// foreign key, CHECK, UNIQUE, dan indeks. Satu-satunya sumber kebenaran skema.
//
//go:embed schema.sql
var schemaSQL string

func getenv(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}

func Open() (*gorm.DB, error) {
	prod := appenv.IsProduction()
	host := getenv("DB_HOST", "localhost")
	password := getenv("DB_PASSWORD", "")
	defaultSSL := "disable"
	if prod {
		defaultSSL = "require"
	}
	sslmode := getenv("DB_SSLMODE", defaultSSL)

	if prod {
		if len(password) < 16 {
			return nil, fmt.Errorf("DB_PASSWORD wajib diisi minimal 16 karakter saat APP_ENV=production")
		}
		local := host == "localhost" || host == "127.0.0.1" || host == "::1" || strings.HasPrefix(host, "/")
		if sslmode == "disable" && !local {
			return nil, fmt.Errorf("DB_SSLMODE=disable ke database %q tidak aman di produksi (pakai require / verify-full)", host)
		}
	}

	dsn := fmt.Sprintf(
		"host=%s port=%s user=%s password=%s dbname=%s sslmode=%s TimeZone=Asia/Jakarta",
		dsnQuote(host),
		dsnQuote(getenv("DB_PORT", "5432")),
		dsnQuote(getenv("DB_USER", "medcredix")),
		dsnQuote(password),
		dsnQuote(getenv("DB_NAME", "medcredix")),
		dsnQuote(sslmode),
	)

	cfg := &gorm.Config{}
	if prod {
		// Query tidak ditulis ke log di produksi.
		cfg.Logger = logger.Default.LogMode(logger.Error)
	}
	d, err := gorm.Open(postgres.Open(dsn), cfg)
	if err != nil {
		return nil, err
	}
	if err := Migrate(d); err != nil {
		return nil, err
	}
	return d, nil
}

// dsnQuote mengutip nilai DSN supaya karakter spesial aman.
func dsnQuote(v string) string {
	v = strings.ReplaceAll(v, `\`, `\\`)
	v = strings.ReplaceAll(v, `'`, `\'`)
	return "'" + v + "'"
}

// Migrate membuat seluruh tabel dari schema.sql bila belum ada.
// Aman dijalankan berulang: kalau skema baru sudah terpasang, tidak melakukan apa-apa.
func Migrate(d *gorm.DB) error {
	var hospitalsExist bool
	if err := d.Raw("SELECT to_regclass('public.hospitals') IS NOT NULL").Scan(&hospitalsExist).Error; err != nil {
		return err
	}

	if hospitalsExist {
		var newSchema bool
		err := d.Raw(`SELECT EXISTS (
			SELECT 1 FROM information_schema.columns
			WHERE table_schema='public' AND table_name='hospitals' AND column_name='hospital_class')`).
			Scan(&newSchema).Error
		if err != nil {
			return err
		}
		if !newSchema {
			return fmt.Errorf("tabel lama terdeteksi: jalankan dulu `DROP TABLE IF EXISTS users, hospitals CASCADE;` di DBeaver, lalu jalankan ulang server")
		}
		return upgrade(d) // skema baru sudah terpasang
	}

	return d.Transaction(func(tx *gorm.DB) error {
		for _, stmt := range splitSQL(schemaSQL) {
			if err := tx.Exec(stmt).Error; err != nil {
				return fmt.Errorf("gagal menjalankan schema.sql: %w\n--- statement ---\n%s", err, stmt)
			}
		}
		return nil
	})
}

// upgrade: perubahan skema untuk database yang sudah terpasang (aman dijalankan berulang).
func upgrade(d *gorm.DB) error {
	return d.Exec(`ALTER TABLE actions DROP CONSTRAINT IF EXISTS actions_status_check;
		ALTER TABLE actions ADD CONSTRAINT actions_status_check CHECK (status IN ('baru','ditugaskan',
		'dalam_pengerjaan','menunggu_verifikasi','selesai','ditutup','ditolak','dibuka_kembali'))`).Error
}

// splitSQL membuang komentar "-- ..." lalu memecah per statement (;).
func splitSQL(src string) []string {
	var b strings.Builder
	for _, line := range strings.Split(src, "\n") {
		if i := strings.Index(line, "--"); i >= 0 {
			line = line[:i]
		}
		b.WriteString(line)
		b.WriteString("\n")
	}
	var out []string
	for _, s := range strings.Split(b.String(), ";") {
		if s = strings.TrimSpace(s); s != "" {
			out = append(out, s)
		}
	}
	return out
}
