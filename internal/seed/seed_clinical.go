package seed

import (
	"encoding/json"
	"fmt"
	"math"
	"sort"

	"gorm.io/gorm"

	"github.com/spkd/medcredix/internal/model"
)

type diag struct {
	name, icd string
	los       float64
}

// 20 diagnosis: dipakai sebagai clinical pathway dan label diagnosis klaim.
var diagnoses = []diag{
	{"Dengue Haemorrhagic Fever", "A91", 4}, {"Appendicitis Akut", "K35.8", 3}, {"Stroke Iskemik", "I63.9", 7},
	{"PPOK Berat", "J44.1", 6}, {"DM dengan Ulkus Kaki", "E11.5", 8}, {"Pneumonia", "J18.9", 5},
	{"Gagal Jantung", "I50.0", 6}, {"Hipertensi Emergensi", "I16.1", 4}, {"Sectio Caesarea", "O82", 4},
	{"Fraktur Femur", "S72.0", 7}, {"Sepsis", "A41.9", 8}, {"Demam Tifoid", "A01.0", 4},
	{"Diare Akut", "A09", 3}, {"Infark Miokard Akut", "I21.9", 6}, {"Asma Eksaserbasi", "J45.1", 3},
	{"Infeksi Saluran Kemih", "N39.0", 4}, {"Katarak", "H25.9", 2}, {"Hernia Inguinalis", "K40.9", 3},
	{"Kolelitiasis", "K80.2", 4}, {"Anemia", "D64.9", 3},
}

var flagCats = []string{"diagnosis", "medication", "procedure", "diagnostic", "stock", "timeline"}

var anomalyTitles = map[string]string{
	"diagnosis":  "Kode diagnosis tidak sejalan dengan tindakan",
	"medication": "Obat tidak sejalan dengan diagnosis",
	"procedure":  "Prosedur tanpa indikasi tercatat",
	"diagnostic": "Pemeriksaan penunjang tidak lengkap",
	"stock":      "Pemakaian obat melebihi stok tercatat",
	"timeline":   "Urutan waktu layanan tidak konsisten",
}

var inacbgSamples = []string{"A-4-10-I", "K-1-10-I", "G-4-14-II", "J-4-10-II", "E-4-10-III", "J-1-20-I"}

func claimJSON(label string, bad map[string]bool) []byte {
	flags := map[string]string{}
	for _, c := range flagCats {
		flags[c] = "ok"
		if bad[c] {
			flags[c] = "bad"
		}
	}
	// Tabel claims tidak punya kolom diagnosis, jadi label diagnosis disimpan di flags_json.
	b, _ := json.Marshal(map[string]interface{}{"diagnosis": label, "flags": flags})
	return b
}

func seedClaims(tx *gorm.DB) error {
	// RS Nusantara: 42 klaim (CLM-2026-00101..00142)
	labelOverride := map[int]string{1: "Dengue Haemorrhagic Fever", 18: "Appendicitis Akut",
		28: "Stroke Iskemik", 29: "DM dengan Ulkus Kaki", 42: "PPOK Berat"}
	rejected := map[int]bool{18: true}
	pending := map[int]bool{15: true, 22: true, 29: true, 35: true}                                                                                          // 4 ditunda
	verified := map[int]bool{4: true, 7: true, 10: true, 13: true, 16: true, 20: true, 24: true, 26: true, 28: true, 31: true, 37: true, 40: true, 42: true} // 13
	unflagged := map[int]bool{1: true, 2: true, 3: true, 5: true, 7: true, 9: true, 11: true, 13: true, 14: true, 17: true,
		19: true, 21: true, 25: true, 30: true, 33: true, 38: true} // 16 tanpa sinyal -> 26 bersinyal

	var flagged []int
	label := map[int]string{}
	for i := 1; i <= 42; i++ {
		if l, found := labelOverride[i]; found {
			label[i] = l
		} else {
			label[i] = diagnoses[i%len(diagnoses)].name
		}
		if !unflagged[i] {
			flagged = append(flagged, i)
		}
	}

	// 83 anomali (Baru 58 / Ditinjau 25; Tinggi 23 / Sedang 60) disebar ke 26 klaim bersinyal
	const reviewer = "Silvia Rahmawati, AMd.Kep"
	var anoms []model.ClaimAnomaly
	badByClaim := map[int]map[string]bool{}
	owner := []int{}
	for k := 0; k < 83; k++ {
		idx := flagged[k%len(flagged)]
		cat := flagCats[k%len(flagCats)]
		if idx == 29 && k < len(flagged) {
			cat = "medication" // CLM-2026-00129: Medication bad (test case)
		}
		sev, status := "sedang", "baru"
		if (k*11)%83 < 23 {
			sev = "tinggi"
		}
		var rev *string
		if (k*7)%83 < 25 {
			status, rev = "ditinjau", sp(reviewer)
		}
		if badByClaim[idx] == nil {
			badByClaim[idx] = map[string]bool{}
		}
		badByClaim[idx][cat] = true
		anoms = append(anoms, model.ClaimAnomaly{
			ID: fmt.Sprintf("can_ns_%03d", k+1), ClaimID: fmt.Sprintf("clm_ns_%02d", idx), HospitalID: hNS,
			Category: sp(cat), Severity: sp(sev), Title: anomalyTitles[cat], Status: status, ReviewedBy: rev,
		})
		owner = append(owner, idx)
	}
	// Pastikan klaim 00129 punya minimal satu anomali "ditinjau" (E2E tinjauan klaim).
	has129 := false
	for k, a := range anoms {
		if owner[k] == 29 && a.Status == "ditinjau" {
			has129 = true
		}
	}
	if !has129 {
		first129, donor := -1, -1
		for k, a := range anoms {
			if owner[k] == 29 && first129 < 0 {
				first129 = k
			}
			if owner[k] != 29 && a.Status == "ditinjau" && donor < 0 {
				donor = k
			}
		}
		if first129 >= 0 && donor >= 0 {
			anoms[first129].Status, anoms[first129].ReviewedBy = "ditinjau", sp(reviewer)
			anoms[donor].Status, anoms[donor].ReviewedBy = "baru", nil
		}
	}

	var claims []model.Claim
	for i := 1; i <= 42; i++ {
		status := "disetujui"
		switch {
		case rejected[i]:
			status = "ditolak"
		case pending[i]:
			status = "ditunda"
		case verified[i]:
			status = "diverifikasi"
		}
		care := "rawat inap"
		if i%3 == 0 {
			care = "rawat jalan"
		}
		claims = append(claims, model.Claim{
			ID: fmt.Sprintf("clm_ns_%02d", i), HospitalID: hNS, ClaimNumber: fmt.Sprintf("CLM-2026-%05d", 100+i),
			SEPNumber: sp(fmt.Sprintf("SEP-%07d", 2026100+i)), INACBGCode: sp(inacbgSamples[i%len(inacbgSamples)]),
			CareType: sp(care), BilledAmount: f64(float64(2500000 + i*375000)), Status: sp(status),
			FlagsJSON: claimJSON(label[i], badByClaim[i]),
		})
	}

	// RS Medika: 6 klaim (CLM-2026-00201..00206), 4 anomali pada klaim 204 dan 206
	msStatus := []string{"disetujui", "disetujui", "diverifikasi", "disetujui", "ditunda", "disetujui"}
	msBad := map[int]map[string]bool{4: {"medication": true, "stock": true}, 6: {"timeline": true, "diagnostic": true}}
	for i := 1; i <= 6; i++ {
		claims = append(claims, model.Claim{
			ID: fmt.Sprintf("clm_ms_%02d", i), HospitalID: hMS, ClaimNumber: fmt.Sprintf("CLM-2026-%05d", 200+i),
			SEPNumber: sp(fmt.Sprintf("SEP-%07d", 2026200+i)), INACBGCode: sp(inacbgSamples[i%len(inacbgSamples)]),
			CareType: sp("rawat jalan"), BilledAmount: f64(float64(1800000 + i*210000)), Status: sp(msStatus[i-1]),
			FlagsJSON: claimJSON(diagnoses[(i+3)%len(diagnoses)].name, msBad[i]),
		})
	}
	n := 0
	for _, idx := range []int{4, 6} {
		for _, cat := range flagCats {
			if msBad[idx][cat] {
				n++
				anoms = append(anoms, model.ClaimAnomaly{
					ID: fmt.Sprintf("can_ms_%02d", n), ClaimID: fmt.Sprintf("clm_ms_%02d", idx), HospitalID: hMS,
					Category: sp(cat), Severity: sp("sedang"), Title: anomalyTitles[cat], Status: "baru",
				})
			}
		}
	}

	if err := ins(tx, claims); err != nil {
		return err
	}
	return ins(tx, anoms)
}

// Baseline: 20 pathway, coverage 16/20, 26 episode (adherence rata-rata 80,7%),
// dokumentasi deviasi 7 dari 11.
func seedPathways(tx *gorm.DB) error {
	type ep struct {
		pathway   int
		adherence float64
		los       float64
		doc       *bool
	}
	eps := make([]ep, 26)
	sum := 0.0
	for e := 0; e < 25; e++ {
		eps[e].adherence = float64(66 + (e*17)%30)
		sum += eps[e].adherence
	}
	eps[25].adherence = math.Round((80.7*26-sum)*10) / 10 // paksa rata-rata = 80,7
	for e := range eps {
		eps[e].pathway = e % 16 // hanya 16 pathway yang punya episode -> coverage 16/20
		eps[e].los = diagnoses[eps[e].pathway].los + float64((e*3)%5-1)*0.5
	}

	// 11 episode dengan adherence terendah wajib dokumentasi deviasi: 7 terdokumentasi, 4 belum.
	order := make([]int, len(eps))
	for i := range order {
		order[i] = i
	}
	sort.Slice(order, func(a, b int) bool { return eps[order[a]].adherence < eps[order[b]].adherence })
	notDoc := map[int]bool{3: true, 7: true, 9: true, 10: true}
	for pos := 0; pos < 11; pos++ {
		eps[order[pos]].doc = bl(!notDoc[pos])
	}

	var paths []model.ClinicalPathway
	var rows []model.PathwayEpisode
	for p, d := range diagnoses {
		pid := fmt.Sprintf("cp_ns_%02d", p+1)
		var total, count, docTrue, docAll float64
		for e, x := range eps {
			if x.pathway != p {
				continue
			}
			total += x.adherence
			count++
			if x.doc != nil {
				docAll++
				if *x.doc {
					docTrue++
				}
			}
			rows = append(rows, model.PathwayEpisode{
				ID: fmt.Sprintf("pe_ns_%02d", e+1), PathwayID: pid, EpisodeNumber: fmt.Sprintf("EP-2026-%03d", e+1),
				Adherence: f64(x.adherence), LOSActual: f64(x.los), Documented: x.doc,
			})
		}
		row := model.ClinicalPathway{ID: pid, HospitalID: hNS, Diagnosis: d.name, ICD10: sp(d.icd), StandardLOS: f64(d.los)}
		if count > 0 {
			row.AvgAdherence = f64(math.Round(total/count*100) / 100)
		}
		if docAll > 0 {
			row.DocumentedDeviation = f64(math.Round(docTrue/docAll*10000) / 100)
		}
		paths = append(paths, row)
	}
	if err := ins(tx, paths); err != nil {
		return err
	}
	return ins(tx, rows)
}
