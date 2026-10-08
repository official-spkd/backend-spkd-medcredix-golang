package db

import (
	_ "embed"
	"fmt"
	"os"
	"strings"

	"gorm.io/driver/postgres"
	"gorm.io/gorm"
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
	dsn := fmt.Sprintf(
		"host=%s port=%s user=%s password=%s dbname=%s sslmode=%s TimeZone=Asia/Jakarta",
		getenv("DB_HOST", "localhost"),
		getenv("DB_PORT", "5432"),
		getenv("DB_USER", "medcredix"),
		getenv("DB_PASSWORD", ""),
		getenv("DB_NAME", "medcredix"),
		getenv("DB_SSLMODE", "disable"),
	)

	d, err := gorm.Open(postgres.Open(dsn), &gorm.Config{})
	if err != nil {
		return nil, err
	}
	if err := Migrate(d); err != nil {
		return nil, err
	}
	return d, nil
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
		return nil // skema baru sudah terpasang
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
