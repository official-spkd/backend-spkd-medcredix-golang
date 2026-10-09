package scoring

import (
	"fmt"
	"time"

	"gorm.io/gorm"
)

// Load membaca semua data satu RS. Setiap query dibatasi hospital_id (isolasi tenant).
func Load(db *gorm.DB, hospitalID string, now time.Time) (Data, error) {
	d := Data{HospitalID: hospitalID, Now: now}

	steps := []struct {
		name string
		dest interface{}
	}{
		{"praktisi", &d.Practitioners},
		{"kredensial", &d.Credentials},
		{"prasyarat", &d.Requirements},
		{"klaim", &d.Claims},
		{"clinical pathway", &d.Pathways},
		{"indikator digital", &d.Indicators},
		{"sumber data", &d.Sources},
		{"faktor trust", &d.TrustFactors},
		{"isu kualitas data", &d.DQIssues},
		{"standar spesialis", &d.Specialty},
		{"utilisasi", &d.Utilization},
		{"sarana", &d.Facilities},
	}
	for _, s := range steps {
		if err := db.Where("hospital_id = ?", hospitalID).Find(s.dest).Error; err != nil {
			return d, fmt.Errorf("memuat %s: %w", s.name, err)
		}
	}

	// pathway_episodes tidak punya hospital_id; difilter lewat pathway milik RS ini.
	if len(d.Pathways) > 0 {
		ids := make([]string, 0, len(d.Pathways))
		for _, p := range d.Pathways {
			ids = append(ids, p.ID)
		}
		if err := db.Where("pathway_id IN ?", ids).Find(&d.Episodes).Error; err != nil {
			return d, fmt.Errorf("memuat episode pathway: %w", err)
		}
	}
	return d, nil
}
