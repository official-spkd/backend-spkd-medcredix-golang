package scoring

import (
	"time"

	"gorm.io/gorm"
)

// ComputeIntel = titik masuk untuk handler/laporan/panel admin: muat data RS lalu hitung.
// Perhitungan dilakukan on-demand (SRS 3.3.1, versi 2.0).
func ComputeIntel(db *gorm.DB, hospitalID string) (Intelligence, error) {
	d, err := Load(db, hospitalID, time.Now())
	if err != nil {
		return Intelligence{}, err
	}
	return Compute(d), nil
}