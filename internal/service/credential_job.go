package service

import (
	"context"
	"fmt"
	"log"
	"time"

	"gorm.io/gorm"

	"github.com/spkd/medcredix/internal/audit"
	"github.com/spkd/medcredix/internal/model"
)

// Kredensial dianggap "segera berakhir" bila sisa masa berlakunya <= 90 hari
// (sama dengan batas skor expiry di scoring/config.go).
const credExpiringDays = 90

var systemActor = model.User{Name: "Sistem MedCredix", Role: "system"}

// SyncCredentialStatus memperbarui status kredensial berdasarkan expiry_date:
//   - aktif            -> segera_berakhir  (sisa <= 90 hari)
//   - aktif / segera_berakhir -> kedaluwarsa (tanggal sudah lewat)
//
// Status alur pengajuan (diajukan, verifikasi, dst.) dan ditangguhkan tidak disentuh.
func SyncCredentialStatus(db *gorm.DB) error {
	today := time.Now().Truncate(24 * time.Hour)
	steps := []struct {
		to, desc string
		from     []string
		where    string
		arg      time.Time
	}{
		{"kedaluwarsa", "kedaluwarsa", []string{"aktif", "segera_berakhir"}, "expiry_date < ?", today},
		{"segera_berakhir", "segera berakhir", []string{"aktif"}, "expiry_date <= ?", today.AddDate(0, 0, credExpiringDays)},
	}
	changed := false
	for _, st := range steps {
		var rows []struct{ HospitalID string }
		res := db.Raw(`UPDATE practitioner_credentials SET status = ?
			WHERE status IN ? AND expiry_date IS NOT NULL AND `+st.where+` RETURNING hospital_id`,
			st.to, st.from, st.arg).Scan(&rows)
		if res.Error != nil {
			return res.Error
		}
		perRS := map[string]int{}
		for _, r := range rows {
			perRS[r.HospitalID]++
		}
		for hid, n := range perRS {
			changed = true
			_ = audit.Log(db, systemActor, hid, "update", "practitioner_credentials", "",
				fmt.Sprintf("Otomatis: %d kredensial ditandai %s", n, st.desc), audit.Meta{})
		}
	}
	if changed {
		InvalidateIntel()
	}
	return nil
}

// StartCredentialJob menjalankan SyncCredentialStatus saat server mulai, lalu tiap jam.
func StartCredentialJob(ctx context.Context, db *gorm.DB) {
	run := func() {
		if err := SyncCredentialStatus(db); err != nil {
			log.Printf("job kredensial: %v", err)
		}
	}
	run()
	go func() {
		t := time.NewTicker(time.Hour)
		defer t.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case <-t.C:
				run()
			}
		}
	}()
}
