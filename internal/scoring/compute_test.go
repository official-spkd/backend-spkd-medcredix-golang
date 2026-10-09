package scoring

import (
	"encoding/json"
	"math"
	"testing"
	"time"

	"github.com/spkd/medcredix/internal/model"
)

var now = time.Date(2026, 10, 8, 9, 0, 0, 0, time.UTC)

func sp(s string) *string         { return &s }
func f(v float64) *float64        { return &v }
func bl(v bool) *bool             { return &v }
func in(v int) *int               { return &v }
func day(n int) *time.Time        { t := now.AddDate(0, 0, n); return &t }
func near(a, b, tol float64) bool { return math.Abs(a-b) <= tol }

func ind(dim, name string, v, thr, w float64, dir string) model.DigitalIndicator {
	return model.DigitalIndicator{ID: name, Dimension: sp(dim), Indicator: name,
		MetricValue: f(v), Threshold: f(thr), Weight: f(w), Direction: sp(dir)}
}

func validReqs() []model.Requirement {
	return []model.Requirement{
		{Code: "P1", Name: "Izin", Status: "valid", DocExpiry: day(300)},
		{Code: "P2", Name: "PKS", Status: "valid", DocExpiry: day(35)},
		{Code: "P3", Name: "KARS", Status: "valid", DocExpiry: day(115)},
	}
}

// goldenData = semua domain terisi dan bisa dihitung tangan:
//
//	Workforce 75 (cakupan 50, SIP 100, aktif 100) | Credentialing 100
//	Digital 80.3 (EMR 81, Infra 79.5) | Medication 70 | Diagnostic 90 | Interop 100
//	Claim 87.5 | Pathway 58.6 | Safety 81 | Sarana 87.5 | UR 98
//	Health = (75*12+100*14+80.3*10+70*8+90*8+100*8+87.5*16+58.6*10+81*14)/100 = 83.0
//	A = 75*.5+87.5*.5 = 81.3 ; B = 81*.25+79.5*.15+70*.2+90*.15+100*.25 = 84.7
//	C = 87.5*.5+58.6*.35+98*.15 = 79.0 ; D = 81
//	BDCS mentah = 81.3*.25+84.7*.3+79*.35+81*.1 = 81.5 ; Trust default x1.00 ; Stage 6 ; LULUS
func goldenData() Data {
	allOK := `{"flags":{"diagnosis":"ok","medication":"ok","procedure":"ok","diagnostic":"ok","stock":"ok","timeline":"ok"}}`
	diagBad := `{"Diagnosis":"bad","Medication":"ok","Procedure":"ok","Diagnostic":"ok","Stock":"ok","Timeline":"ok"}`
	return Data{Now: now,
		Specialty: []model.SpecialtyStandard{{Specialty: sp("Saraf"), Required: in(1)}, {Specialty: sp("Mata"), Required: in(1)}},
		Practitioners: []model.Practitioner{
			{ID: "1", Profession: sp("Dokter Spesialis"), Specialty: sp("Saraf"), SIPExpiry: day(300)},
		},
		Credentials: []model.PractitionerCredential{
			{PractitionerID: "1", Type: "STR", Status: "aktif", ExpiryDate: day(300)},
			{PractitionerID: "1", Type: "SIP", Status: "aktif", ExpiryDate: day(300)},
		},
		Requirements: validReqs(),
		Indicators: []model.DigitalIndicator{
			ind("EMR", "RME Rawat Inap", 90, 85, 1, "up"),
			ind("EMR", "RME Rawat Jalan", 70, 85, 1, "up"),
			ind("Infrastruktur", "Uptime API", 99, 99, 1, "up"),
			ind("Infrastruktur", "Latensi API", 400, 300, 1, "down"),
			ind("Medikasi", "Barcode", 70, 85, 1, "up"),
			ind("Diagnostik", "Lab", 90, 85, 1, "up"),
			ind("Interoperabilitas", "FHIR", 100, 95, 1, "up"),
			ind("Keselamatan", "Alergi Obat", 90, 95, 1, "up"),
			ind("Keselamatan", "High-Alert Closed-Loop", 70, 80, 1, "up"),
			ind("Keselamatan", "Dokumentasi Pemulangan", 80, 80, 1, "up"),
		},
		Claims:      []model.Claim{{FlagsJSON: []byte(allOK)}, {FlagsJSON: []byte(diagBad)}},
		Pathways:    []model.ClinicalPathway{{ID: "p1", Diagnosis: "a"}, {ID: "p2", Diagnosis: "b"}},
		Episodes:    []model.PathwayEpisode{{PathwayID: "p1", Adherence: f(60)}},
		Utilization: []model.UtilizationStat{{Month: "2026-09", ALOS: f(5.3), Readmission: f(4.9), BOR: f(81)}},
		Facilities: []model.Facility{
			{Name: "CT Scan", Availability: bl(true), Functional: bl(true), Calibration: sp("Valid")},
			{Name: "Ventilator", Availability: bl(true), Functional: bl(false), Calibration: sp("Valid")},
		},
	}
}

func TestStageBoundaries(t *testing.T) {
	cases := map[float64]int{90: 7, 89.99: 6, 80: 6, 79.99: 5, 66: 5, 65.99: 4, 60: 4, 50: 3, 35: 2, 20: 1, 19.99: 0, 0: 0}
	for bdcs, want := range cases {
		if got := stageOf(bdcs); got != want {
			t.Errorf("stageOf(%v)=%d want %d", bdcs, got, want)
		}
	}
}

func TestTrustMultiplierBands(t *testing.T) {
	cases := map[float64]float64{100: 1.0, 90: 1.0, 89.9: 0.95, 70: 0.95, 69.9: 0.85, 50: 0.85, 49.9: 0.70}
	for score, want := range cases {
		tr := computeTrust(Data{TrustFactors: []model.TrustFactor{{Factor: "x", Score: score, Weight: 1}}})
		if tr.Multiplier != want {
			t.Errorf("trust %v -> %v want %v", score, tr.Multiplier, want)
		}
	}
	if tr := computeTrust(Data{}); tr.Score != 100 || tr.Multiplier != 1.0 || !tr.Defaulted {
		t.Errorf("tanpa faktor harus 100/1.00, dapat %+v", tr)
	}
}

func TestGateBlocksStage(t *testing.T) {
	d := goldenData()
	d.Requirements = validReqs()[:2] // P3 belum terdaftar -> tidak memblokir (hanya cek yang ada)
	out := Compute(d)
	if out.StageRaw < PassStage {
		t.Fatalf("StageRaw harus >=5, dapat %d", out.StageRaw)
	}
	if out.GateBlocked || out.StageEffective != out.StageRaw || !out.Pass {
		t.Errorf("prasyarat yang belum terdaftar tidak boleh memblokir: blocked=%v eff=%d", out.GateBlocked, out.StageEffective)
	}
	d.Requirements = append(validReqs()[:2], model.Requirement{Code: "P3", Name: "KARS", Status: "menunggu_verifikasi"})
	out = Compute(d)
	if !out.GateBlocked || out.StageEffective != 0 || out.Pass {
		t.Errorf("P3 tidak valid harus memblokir: %+v", out.StageEffective)
	}
}

func TestExpiredGateDocumentOnlyNotes(t *testing.T) {
	d := goldenData()
	d.Requirements[1].DocExpiry = day(-3) // PKS lewat masa berlaku, status tetap valid
	out := Compute(d)
	if out.GateBlocked {
		t.Error("masa berlaku dokumen tidak ikut menentukan gerbang (hanya status)")
	}
	g := out.Gates[1]
	if g.Note == "" || g.DaysLeft == nil || *g.DaysLeft != -3 {
		t.Errorf("gerbang P2 harus diberi catatan & daysLeft -3: %+v", g)
	}
}

func TestBDCSTimesMultiplier(t *testing.T) {
	d := goldenData()
	d.TrustFactors = []model.TrustFactor{{Factor: "x", Score: 80, Weight: 1}}
	out := Compute(d)
	if !near(out.BDCSRaw, 81.5, 0.001) || out.Trust.Multiplier != 0.95 || !near(out.BDCS, 77.4, 0.001) {
		t.Errorf("raw=%v mult=%v bdcs=%v", out.BDCSRaw, out.Trust.Multiplier, out.BDCS)
	}
	if out.StageRaw != 5 {
		t.Errorf("77.4 -> stage 5, dapat %d", out.StageRaw)
	}
}

func TestHealthScoreNoDataCountsZero(t *testing.T) {
	out := Compute(Data{Now: now, Indicators: []model.DigitalIndicator{ind("Medikasi", "Barcode", 50, 85, 1, "up")}})
	// hanya Medication (50 x 8%) + Credentialing tanpa data (expiry 100 x 20% = 20, x 14%) + CPCS tanpa data (25, x 10%)
	want := (50*8.0 + 20*14.0 + 25*10.0) / 100
	if !near(out.HealthScore, want, 0.051) {
		t.Errorf("health=%v want %v (domain tanpa data dihitung 0, tanpa normalisasi)", out.HealthScore, want)
	}
	var sum float64
	for _, dm := range out.Domains {
		sum += dm.Contribution
		if dm.Score == nil || dm.EffectiveWeight != dm.Weight {
			t.Errorf("domain %s: skor harus terisi & bobot efektif = bobot", dm.Key)
		}
	}
	if !near(sum, out.HealthScore, 0.06) {
		t.Errorf("jumlah kontribusi %v != health %v", sum, out.HealthScore)
	}
}

func TestDownIsBetterIndicator(t *testing.T) {
	if indicatorScore("down", 280, 300) != 100 {
		t.Error("latensi di bawah ambang harus 100")
	}
	// 100 - ((400-300)/300) x 100 x 1,2 = 60
	if !near(indicatorScore("down", 400, 300), 60, 0.001) {
		t.Errorf("latensi 400 vs ambang 300 harus 60, dapat %v", indicatorScore("down", 400, 300))
	}
	if normDirection(sp("lower")) != "down" || normDirection(sp("Down")) != "down" || normDirection(sp("higher")) != "up" || normDirection(nil) != "up" {
		t.Error("arah lower/down -> down, selain itu up")
	}
}

func TestCPCSMatchesBaseline(t *testing.T) {
	var paths []model.ClinicalPathway
	for i := 0; i < 20; i++ {
		paths = append(paths, model.ClinicalPathway{ID: string(rune('a' + i)), Diagnosis: "dx"})
	}
	var eps []model.PathwayEpisode
	for i := 0; i < 26; i++ {
		e := model.PathwayEpisode{PathwayID: paths[i%16].ID, Adherence: f(80.7)}
		if i < 11 {
			e.Documented = bl(i < 7) // 7 dari 11 deviasi terdokumentasi
		}
		eps = append(eps, e)
	}
	_, pr := pathwayDomain(Data{Pathways: paths, Episodes: eps})
	// >= 8 pathway -> cakupan tetap 16/20 = 80%
	if !near(pr.Coverage, 80, 0.001) || !near(pr.Adherence, 80.7, 0.001) || !near(pr.Documentation, 63.6, 0.001) {
		t.Fatalf("komponen salah: %+v", pr)
	}
	if pr.Covered != 16 || pr.CoverageTarget != 20 {
		t.Errorf("cakupan %d/%d want 16/20", pr.Covered, pr.CoverageTarget)
	}
	// 80*.25 + 80.7*.5 + 63.636*.25 = 76.26 -> 76.3
	if !near(*pr.CPCS, 76.3, 0.001) {
		t.Errorf("CPCS=%v want 76.3", *pr.CPCS)
	}
}

func TestCCSAndGrade(t *testing.T) {
	mk := func(diag, med string) model.Claim {
		b, _ := json.Marshal(map[string]interface{}{"diagnosis": "x", "flags": map[string]string{"diagnosis": diag, "medication": med}})
		return model.Claim{FlagsJSON: b}
	}
	d := Data{Claims: []model.Claim{mk("ok", "ok"), mk("bad", "ok"), mk("ok", "bad"), mk("ok", "ok")}}
	_, cr := claimDomain(d)
	// diagnosis 75% x 25 + medication 75% x 20; 4 kategori lain tanpa flag = 0 -> 33.75 -> 33.8
	if cr.CCS == nil || !near(*cr.CCS, 33.8, 0.001) || cr.Grade != "POOR" {
		t.Errorf("CCS=%v grade=%s", *cr.CCS, cr.Grade)
	}
	if cr.FlaggedClaims != 2 || cr.Evaluated != 4 || len(cr.Categories) != 6 {
		t.Errorf("flagged=%d evaluated=%d kategori=%d", cr.FlaggedClaims, cr.Evaluated, len(cr.Categories))
	}
	for sc, want := range map[float64]string{95: "EXCELLENT", 90: "EXCELLENT", 89: "GOOD", 75: "GOOD", 74: "FAIR", 60: "FAIR", 59: "POOR"} {
		if claimGrade(sc) != want {
			t.Errorf("grade(%v) != %s", sc, want)
		}
	}
}

func TestUnderReview(t *testing.T) {
	q := func(anom bool, st string) model.DataQualityIssue {
		return model.DataQualityIssue{Anomaly: anom, Status: st}
	}
	d := Data{Now: now, DQIssues: []model.DataQualityIssue{q(true, "terbuka"), q(true, "terbuka"), q(true, "diperbaiki"), q(false, "terbuka")}}
	if out := Compute(d); out.AnomalyFlags != 2 || out.UnderReview {
		t.Errorf("flags=%d review=%v", out.AnomalyFlags, out.UnderReview)
	}
	d.DQIssues = append(d.DQIssues, q(true, "terbuka"))
	if out := Compute(d); out.AnomalyFlags != 3 || !out.UnderReview {
		t.Errorf("flags=%d review=%v", out.AnomalyFlags, out.UnderReview)
	}
}

func TestWorkforceIgnoresSuspendedAndCapsSurplus(t *testing.T) {
	d := Data{Now: now,
		Specialty: []model.SpecialtyStandard{
			{Specialty: sp("Penyakit Dalam"), Required: in(2)},
			{Specialty: sp("Saraf"), Required: in(1)},
			{Specialty: sp("Mata"), Required: in(1)},
		},
		Practitioners: []model.Practitioner{
			{ID: "1", Specialty: sp("Penyakit Dalam")}, {ID: "2", Specialty: sp("Penyakit Dalam")},
			{ID: "3", Specialty: sp("Penyakit Dalam")}, // kelebihan, tidak boleh menutup Mata
			{ID: "4", Specialty: sp("Saraf")},
		},
		Credentials: []model.PractitionerCredential{
			{PractitionerID: "4", Type: "SIP", Status: "ditangguhkan", ExpiryDate: day(100)},
		},
	}
	res, wr := workforceDomain(d)
	// cakupan: PD 100, Saraf 0 (ditangguhkan), Mata 0 -> 33.33; SIP aktif 0/4; aktif 3/4 = 75
	// 33.33*.5 + 0*.3 + 75*.2 = 31.67 -> 31.7
	if !near(wr.Coverage, 33.3, 0.001) || !near(wr.Active, 75, 0.001) || wr.SIPActive != 0 {
		t.Errorf("komponen workforce: %+v", wr)
	}
	if res.Score == nil || !near(*res.Score, 31.7, 0.001) {
		t.Errorf("workforce=%v", *res.Score)
	}
}

func TestUtilizationReview(t *testing.T) {
	d := Data{Utilization: []model.UtilizationStat{
		{Month: "2026-09", ALOS: f(5.3), Readmission: f(4.9), BOR: f(81)},
		{Month: "2026-08", ALOS: f(9), Readmission: f(9), BOR: f(40)},
	}}
	ur := urScore(d)
	// bulan terbaru: ALOS 100-(0.3x20)=94, Readmission 100, BOR 100 -> 98
	if ur == nil || ur.Month != "2026-09" || !near(ur.Score, 98, 0.001) {
		t.Errorf("UR=%+v", ur)
	}
}

func TestNoDataNeverPanics(t *testing.T) {
	out := Compute(Data{})
	if out.StageEffective != 0 || out.Pass || len(out.Domains) != 9 || len(out.Groups) != 4 {
		t.Errorf("data kosong: %+v", out)
	}
	// Credentialing 20 x 14% + CPCS 25 x 10% = 5.3
	if !near(out.HealthScore, 5.3, 0.001) {
		t.Errorf("health data kosong=%v want 5.3", out.HealthScore)
	}
	for _, dm := range out.Domains {
		if dm.Score == nil || math.IsNaN(*dm.Score) || dm.HasData {
			t.Errorf("domain %s data kosong: score=%v hasData=%v", dm.Key, dm.Score, dm.HasData)
		}
	}
	if math.IsNaN(out.BDCS) || math.IsNaN(out.BDCSRaw) || out.Utilization == nil {
		t.Errorf("tidak boleh NaN / UR nil: %+v", out)
	}
	if _, err := json.Marshal(out); err != nil {
		t.Error(err)
	}
}
