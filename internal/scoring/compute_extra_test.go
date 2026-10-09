package scoring

import (
	"encoding/json"
	"math"
	"testing"
	"time"

	"github.com/spkd/medcredix/internal/model"
)

// ============================================================
// compute_extra_test.go — pelengkap compute_test.go.
// Memakai helper yang sudah ada di compute_test.go:
// now, sp, f, bl, in, day, near, ind, validReqs, goldenData.
// Jalankan: go test ./internal/scoring/ -run Extra -v
// ============================================================

func domOf(out Intelligence, k DomainKey) DomainResult {
	for _, d := range out.Domains {
		if d.Key == k {
			return d
		}
	}
	return DomainResult{}
}

// ---------- Gerbang P1-P3 ----------

func TestExtraGateStatusAndCode(t *testing.T) {
	for _, st := range []string{"belum dilengkapi", "menunggu_verifikasi", "Menunggu Verifikasi", "kedaluwarsa", ""} {
		reqs := validReqs()
		reqs[0].Status = st
		if _, blocked := evaluateGates(Data{Now: now, Requirements: reqs}); !blocked {
			t.Errorf("status P1=%q harus memblokir", st)
		}
	}
	// kode huruf kecil + spasi harus tetap dikenali
	reqs := validReqs()
	reqs[0].Code, reqs[1].Code, reqs[2].Code = " p1 ", "p2", "P3"
	if _, blocked := evaluateGates(Data{Now: now, Requirements: reqs}); blocked {
		t.Error("kode ' p1 '/'p2' harus dinormalisasi (trim + upper)")
	}
	// status 'VALID' / 'Valid' (format referensi)
	reqs = validReqs()
	reqs[1].Status, reqs[2].Status = "Valid", "VALID"
	if _, blocked := evaluateGates(Data{Now: now, Requirements: reqs}); blocked {
		t.Error("status 'Valid'/'VALID' harus dianggap valid")
	}
}

func TestExtraMissingGateDoesNotBlock(t *testing.T) {
	gates, blocked := evaluateGates(Data{Now: now})
	if blocked {
		t.Error("tanpa prasyarat terdaftar, gerbang tidak memblokir (sama dengan referensi)")
	}
	if len(gates) != 6 || gates[0].Status != "belum_ada" || gates[0].Valid || gates[0].Note == "" {
		t.Errorf("baris gerbang kosong: %+v", gates)
	}
}

func TestExtraNonGateRequirementsDoNotBlock(t *testing.T) {
	reqs := append(validReqs(),
		model.Requirement{Code: "P4", Status: "menunggu verifikasi"},
		model.Requirement{Code: "P5", Status: "belum dilengkapi"},
		model.Requirement{Code: "P6", Status: "belum dilengkapi"},
	)
	gates, blocked := evaluateGates(Data{Now: now, Requirements: reqs})
	if blocked {
		t.Error("P4-P6 tidak boleh memblokir Stage")
	}
	if len(gates) != 6 {
		t.Errorf("harus ada 6 baris gerbang (P1-P6), dapat %d", len(gates))
	}
	for _, g := range gates {
		if g.IsGate != (g.Code == "P1" || g.Code == "P2" || g.Code == "P3") {
			t.Errorf("flag IsGate salah untuk %s", g.Code)
		}
	}
}

func TestExtraEachGateAloneBlocks(t *testing.T) {
	for i := 0; i < 3; i++ {
		d := goldenData()
		d.Requirements[i].Status = "belum dilengkapi"
		out := Compute(d)
		if !out.GateBlocked || out.StageEffective != 0 || out.Pass {
			t.Errorf("P%d tidak valid harus Stage 0 / tidak lulus: %+v", i+1, out.StageEffective)
		}
		if out.StageRaw < PassStage {
			t.Errorf("StageRaw tetap harus dihitung (>=5), dapat %d", out.StageRaw)
		}
	}
}

// ---------- Trust ----------

func TestExtraTrustWeightedAverage9Factors(t *testing.T) {
	fs := []model.TrustFactor{
		{Factor: "Kelengkapan", Score: 94, Weight: 15}, {Factor: "Keandalan Sumber", Score: 91, Weight: 15},
		{Factor: "Konsistensi", Score: 88, Weight: 12}, {Factor: "Freshness", Score: 96, Weight: 12},
		{Factor: "Kesehatan Sinkronisasi", Score: 87, Weight: 12}, {Factor: "Duplikasi", Score: 93, Weight: 10},
		{Factor: "Anomali", Score: 90, Weight: 10}, {Factor: "Status Validasi", Score: 95, Weight: 8},
		{Factor: "Integritas Audit", Score: 98, Weight: 6},
	}
	tr := computeTrust(Data{TrustFactors: fs})
	var num, den float64
	for _, x := range fs {
		num += x.Score * x.Weight
		den += x.Weight
	}
	if !near(tr.Score, num/den, 0.051) || len(tr.Factors) != 9 || tr.Defaulted {
		t.Errorf("trust=%v want %v (faktor=%d)", tr.Score, num/den, len(tr.Factors))
	}
	var contrib float64
	for _, c := range tr.Factors {
		contrib += c.Contribution
	}
	if !near(contrib, tr.Score, 0.1) {
		t.Errorf("jumlah kontribusi %v != trust %v", contrib, tr.Score)
	}
}

func TestExtraTrustZeroWeightsIsZero(t *testing.T) {
	// sama dengan referensi: ada faktor tapi total bobot 0 -> trust 0 -> x0,70 (bukan default 100)
	tr := computeTrust(Data{TrustFactors: []model.TrustFactor{{Factor: "x", Score: 10, Weight: 0}}})
	if tr.Defaulted || tr.Score != 0 || tr.Multiplier != 0.70 || math.IsNaN(tr.Factors[0].Contribution) {
		t.Errorf("bobot total 0 harus 0/0.70 tanpa NaN, dapat %+v", tr)
	}
}

func TestExtraTrustDropsStage(t *testing.T) {
	// BDCS mentah 81.5 (Stage 6) + trust 40 -> x0,70 = 57.05 -> Stage 3 -> TIDAK LULUS
	d := goldenData()
	d.TrustFactors = []model.TrustFactor{{Factor: "x", Score: 40, Weight: 1}}
	out := Compute(d)
	if !near(out.BDCS, 57.05, 0.051) || out.StageEffective != 3 || out.Pass {
		t.Errorf("bdcs=%v stage=%d pass=%v", out.BDCS, out.StageEffective, out.Pass)
	}
}

// ---------- Indikator ----------

func TestExtraIndicatorStatusBoundaries(t *testing.T) {
	cases := []struct {
		dir    string
		v, thr float64
		want   string
	}{
		{"up", 80, 80, "ok"}, {"up", 68, 80, "warn"}, {"up", 67.99, 80, "bad"},
		{"down", 300, 300, "ok"}, {"down", 375, 300, "warn"}, {"down", 375.01, 300, "bad"},
	}
	for _, c := range cases {
		if got := indicatorStatus(c.dir, c.v, c.thr); got != c.want {
			t.Errorf("status(%s,%v,%v)=%s want %s", c.dir, c.v, c.thr, got, c.want)
		}
	}
}

func TestExtraIndicatorScoreEdges(t *testing.T) {
	if indicatorScore("up", 120, 80) != 100 || indicatorScore("up", -5, 80) != 0 {
		t.Error("up harus di-clamp 0..100")
	}
	if indicatorScore("down", 0, 300) != 100 {
		t.Error("down dengan nilai 0 harus 100")
	}
	if indicatorScore("down", 100, 0) != 0 {
		t.Error("down dengan ambang 0 dan nilai > 0 harus 0 (tanpa pembagian nol)")
	}
	if indicatorScore("down", 1000, 300) != 0 {
		t.Error("down jauh di atas ambang harus di-clamp ke 0")
	}
}

func TestExtraIndicatorNoDataCountsZero(t *testing.T) {
	d := Data{Now: now, Indicators: []model.DigitalIndicator{
		{ID: "a", Dimension: sp("Medikasi"), Indicator: "kosong", MetricValue: nil, Threshold: f(80), Weight: f(5)},
		{ID: "b", Dimension: sp("Medikasi"), Indicator: "bobot nil", MetricValue: f(60), Threshold: f(80), Weight: nil},
		{ID: "c", Dimension: sp("Medikasi"), Indicator: "bobot 0", MetricValue: f(100), Threshold: f(80), Weight: f(0)},
	}}
	r := indicatorDomain(d, dimMed)
	// (0*5 + 60*1 + 100*0) / 6 = 10 ; nilai kosong = 0, bobot nil = 1, bobot 0 = tidak berpengaruh
	if r.Score == nil || !near(*r.Score, 10, 0.001) || !r.HasData {
		t.Fatalf("score=%v want 10", *r.Score)
	}
	if r.Indicators[0].Status != "no_data" {
		t.Errorf("indikator tanpa nilai harus no_data, dapat %s", r.Indicators[0].Status)
	}
	if e := indicatorDomain(Data{}, dimMed); *e.Score != 0 || e.HasData {
		t.Error("tanpa indikator harus 0 dan HasData=false")
	}
}

func TestExtraDimensionMapping(t *testing.T) {
	cases := []struct{ dim, name, want string }{
		{"EMR", "apa saja", dimEMR},
		{"Digital", "RME Rawat Inap", dimEMR}, // format lama: dipecah lewat nama
		{"Digital", "Uptime API", dimInfra},   // format lama: bukan RME -> infrastruktur
		{"Infrastruktur", "Uptime API", dimInfra},
		{"  MEDIKASI ", "x", dimMed},
		{"Diagnostik", "x", dimDiag},
		{"Interoperabilitas", "x", dimInterop},
		{"Keselamatan Pasien", "x", dimSafety}, // format lama
		{"Keselamatan", "x", dimSafety},        // format referensi
		{"Tidak Dikenal", "x", ""},
	}
	for _, c := range cases {
		if got := dimensionOf(model.DigitalIndicator{Dimension: sp(c.dim), Indicator: c.name}); got != c.want {
			t.Errorf("dimensi %q/%q -> %q want %q", c.dim, c.name, got, c.want)
		}
	}
	if dimensionOf(model.DigitalIndicator{Indicator: "x"}) != "" {
		t.Error("dimensi nil tidak boleh dikenali")
	}
}

func TestExtraDigitalDomain(t *testing.T) {
	d := Data{Indicators: []model.DigitalIndicator{
		ind("Digital", "RME Rawat Inap", 90, 85, 1, "higher"),
		ind("Digital", "RME Rawat Jalan", 70, 85, 1, "higher"),
		ind("Digital", "Uptime API", 99, 99, 1, "higher"),
		ind("Digital", "Latensi API", 400, 300, 1, "lower"),
	}}
	res, emr, infra := digitalDomain(d)
	// EMR = 90*.55 + 70*.45 = 81 ; Infra = (99 + 60)/2 = 79.5 ; Digital = 80.25 -> 80.3
	if !near(emr, 81, 0.001) || !near(infra, 79.5, 0.001) || !near(*res.Score, 80.3, 0.001) {
		t.Errorf("emr=%v infra=%v digital=%v", emr, infra, *res.Score)
	}
	if len(res.Indicators) != 4 || !res.HasData {
		t.Errorf("indikator digital=%d", len(res.Indicators))
	}
}

func TestExtraInteroperabilityIndicatorsOnly(t *testing.T) {
	d := Data{Now: now,
		Indicators: []model.DigitalIndicator{ind("Interoperabilitas", "FHIR", 80, 80, 1, "up")},
		Sources:    []model.DataSource{{SuccessRate: f(90)}, {SuccessRate: f(100)}, {SuccessRate: nil}}}
	if r := interoperabilityDomain(d); r.Score == nil || !near(*r.Score, 80, 0.001) {
		t.Errorf("interop=%v want 80 (sumber data tidak ikut dihitung)", *r.Score)
	}
	d.Indicators = nil
	if r := interoperabilityDomain(d); *r.Score != 0 || r.HasData {
		t.Errorf("interop tanpa indikator harus 0, dapat %v", *r.Score)
	}
}

func TestExtraSafetyDomain(t *testing.T) {
	d := Data{Indicators: []model.DigitalIndicator{
		ind("Keselamatan Pasien", "keamanan alergi obat", 90, 95, 1, "up"), // huruf kecil tetap ketemu
		ind("Keselamatan Pasien", "High-Alert & Closed-Loop", 70, 80, 1, "up"),
		ind("Keselamatan Pasien", "Dokumentasi Pemulangan", 80, 80, 1, "up"),
	}}
	res, sr := safetyDomain(d)
	// 90*.4 + 70*.3 + 80*.3 = 81
	if !near(sr.Score, 81, 0.001) || !near(*res.Score, 81, 0.001) || sr.Allergy != 90 || sr.ClosedLoop != 70 || sr.Discharge != 80 {
		t.Errorf("safety=%+v", sr)
	}
	d.Indicators = d.Indicators[:1] // hanya alergi -> 36
	if _, sr = safetyDomain(d); !near(sr.Score, 36, 0.001) {
		t.Errorf("safety (komponen hilang = 0)=%v want 36", sr.Score)
	}
}

// ---------- Credentialing ----------

func TestExtraCredentialingScore(t *testing.T) {
	d := Data{Now: now,
		Practitioners: []model.Practitioner{{ID: "1"}, {ID: "2"}, {ID: "3"}, {ID: "4"}},
		Credentials: []model.PractitionerCredential{
			{PractitionerID: "1", Type: "STR", Status: "aktif", ExpiryDate: day(200)},
			{PractitionerID: "1", Type: "SIP", Status: "aktif", ExpiryDate: day(200)},
			{PractitionerID: "2", Type: "STR", Status: "aktif", ExpiryDate: day(20)},           // <=30 hari
			{PractitionerID: "2", Type: "SIP", Status: "Segera Berakhir", ExpiryDate: day(60)}, // <=90 hari
			{PractitionerID: "3", Type: "STR", Status: "kedaluwarsa", ExpiryDate: day(-10)},    // kedaluwarsa
			{PractitionerID: "3", Type: "SIP", Status: "aktif", ExpiryDate: day(300)},
			{PractitionerID: "4", Type: "SIP", Status: "ditangguhkan", ExpiryDate: day(100)}, // bukan status final
		},
	}
	res, cr := credentialingDomain(d)
	// kelengkapan 2/4 = 50, lisensi 50, pipeline 6/7 = 85.71, expiry 100-(10+4+2) = 84
	// 50*.4 + 50*.25 + 85.71*.15 + 84*.2 = 62.16 -> 62.2
	if !near(cr.Completeness, 50, 0.001) || !near(cr.PipelineHealth, 85.7, 0.001) || !near(cr.ExpiryScore, 84, 0.001) {
		t.Errorf("komponen=%+v", cr)
	}
	if cr.Expired != 1 || cr.Within30 != 1 || cr.Within90 != 1 {
		t.Errorf("expired=%d within30=%d within90=%d", cr.Expired, cr.Within30, cr.Within90)
	}
	if !near(*res.Score, 62.2, 0.001) {
		t.Errorf("credentialing=%v want 62.2", *res.Score)
	}
}

func TestExtraCredentialingNoData(t *testing.T) {
	// sama dengan referensi: tanpa data, hanya skor expiry (100 x 20%) yang tersisa -> 20
	res, _ := credentialingDomain(Data{Now: now})
	if !near(*res.Score, 20, 0.001) || res.HasData {
		t.Errorf("credentialing tanpa data=%v want 20", *res.Score)
	}
}

func TestExtraCredentialStatusSpelling(t *testing.T) {
	for _, st := range []string{"segera_berakhir", "Segera Berakhir", "SEGERA_BERAKHIR"} {
		if !credFinalStatus[normStatus(st)] {
			t.Errorf("status %q harus dianggap final", st)
		}
	}
	for _, st := range []string{"Kedaluwarsa", "ditangguhkan", "Dicabut"} {
		if !credBadStatus[normStatus(st)] {
			t.Errorf("status %q harus dianggap bermasalah", st)
		}
	}
}

// ---------- Workforce ----------

func TestExtraWorkforceProfessionAndSIP(t *testing.T) {
	d := Data{Now: now,
		Specialty: []model.SpecialtyStandard{
			{Specialty: sp("Saraf"), Required: in(2)},
			{Specialty: sp("Bedah"), Required: in(0)}, // diabaikan (tanpa pembagian nol)
			{Specialty: sp("Mata"), Required: nil},
		},
		Practitioners: []model.Practitioner{
			{ID: "1", Profession: sp("Dokter Spesialis"), Specialty: sp("  saraf "), SIPExpiry: day(100)},
			{ID: "2", Profession: sp("Dokter Umum"), Specialty: sp("SARAF"), SIPExpiry: day(-5)}, // bukan spesialis
			{ID: "3", Specialty: sp("Saraf"), SIPExpiry: day(10)},                                // ditangguhkan -> tidak aktif
		},
		Credentials: []model.PractitionerCredential{{PractitionerID: "3", Type: "STR", Status: "ditangguhkan"}},
	}
	res, wr := workforceDomain(d)
	// cakupan Saraf 1/2 = 50 ; SIP 2/3 = 66.67 ; aktif 2/3 = 66.67 -> 25 + 20 + 13.33 = 58.3
	if len(wr.Specialists) != 1 || !near(wr.Coverage, 50, 0.001) || !near(wr.SIPActive, 66.7, 0.001) || !near(wr.Active, 66.7, 0.001) {
		t.Errorf("komponen=%+v", wr)
	}
	if !near(*res.Score, 58.3, 0.001) || len(res.Indicators) != 3 {
		t.Errorf("workforce=%v indikator=%d", *res.Score, len(res.Indicators))
	}
	if w, _ := workforceDomain(Data{Now: now}); *w.Score != 0 || w.HasData {
		t.Error("tanpa data workforce harus 0")
	}
}

// ---------- Claim (CCS) ----------

func TestExtraClaimWarnCountsHalf(t *testing.T) {
	mk := func(v string) model.Claim {
		return model.Claim{FlagsJSON: []byte(`{"flags":{"diagnosis":"` + v + `"}}`)}
	}
	_, cr := claimDomain(Data{Claims: []model.Claim{mk("ok"), mk("warn")}})
	// diagnosis (1 + 0,5)/2 = 75 -> CCS = 75 x 25% = 18.75 -> 18.8
	if !near(cr.Categories[0].Score, 75, 0.001) || !near(*cr.CCS, 18.8, 0.001) {
		t.Errorf("diagnosis=%v CCS=%v", cr.Categories[0].Score, *cr.CCS)
	}
}

func TestExtraClaimBothFormats(t *testing.T) {
	enc, _ := json.Marshal(`{"Diagnosis":"ok"}`) // flags_json berisi string JSON
	cl := []model.Claim{
		{FlagsJSON: []byte(`{"flags":{"diagnosis":"ok","medication":"OK"}}`)}, // format lama
		{FlagsJSON: []byte(`{"Diagnosis":"warn","Medication":"bad"}`)},        // format referensi
		{FlagsJSON: nil},
		{FlagsJSON: []byte(`bukan json`)},
		{FlagsJSON: enc},
	}
	dom, cr := claimDomain(Data{Claims: cl})
	// diagnosis: ok 2, warn 1 -> 2.5/5 = 50 ; medication: ok 1, bad 1 -> 1/5 = 20
	// CCS = 50 x .25 + 20 x .20 = 16.5
	if cr.TotalClaims != 5 || cr.Evaluated != 3 || cr.FlaggedClaims != 1 {
		t.Errorf("total=%d evaluated=%d flagged=%d", cr.TotalClaims, cr.Evaluated, cr.FlaggedClaims)
	}
	if !near(cr.Categories[0].Score, 50, 0.001) || !near(cr.Categories[1].Score, 20, 0.001) || !near(*dom.Score, 16.5, 0.001) {
		t.Errorf("diag=%v med=%v CCS=%v", cr.Categories[0].Score, cr.Categories[1].Score, *dom.Score)
	}
	if d2, c2 := claimDomain(Data{}); *d2.Score != 0 || *c2.CCS != 0 || c2.Grade != "POOR" || d2.HasData {
		t.Error("tanpa klaim harus 0 / POOR")
	}
}

func TestExtraCCSFullWeights(t *testing.T) {
	var w float64
	for _, c := range claimCategories {
		w += c.Weight
	}
	if w != 100 {
		t.Fatalf("total bobot CCS=%v harus 100 (SRS 3.11.1)", w)
	}
	ok := `{"flags":{"diagnosis":"ok","medication":"ok","procedure":"ok","diagnostic":"ok","stock":"ok","timeline":"ok"}}`
	bad := `{"Diagnosis":"bad","Medication":"bad","Procedure":"bad","Diagnostic":"bad","Stock":"bad","Timeline":"bad"}`
	_, cr := claimDomain(Data{Claims: []model.Claim{{FlagsJSON: []byte(ok)}, {FlagsJSON: []byte(bad)}}})
	if !near(*cr.CCS, 50, 0.001) || len(cr.Categories) != 6 || cr.FlaggedClaims != 1 {
		t.Errorf("CCS=%v kategori=%d flagged=%d", *cr.CCS, len(cr.Categories), cr.FlaggedClaims)
	}
}

// ---------- Pathway (CPCS) ----------

func TestExtraPathwayEdgeCases(t *testing.T) {
	// tanpa pathway & episode: cakupan 0, kepatuhan 0, dokumentasi 100 -> 25
	if r, pr := pathwayDomain(Data{}); !near(*pr.CPCS, 25, 0.001) || r.HasData {
		t.Errorf("CPCS kosong=%v want 25", *pr.CPCS)
	}
	// 1 pathway: cakupan 1/14 = 7.14 ; episode adherence 80 & kosong -> 40 ; dokumentasi 100
	// 7.14*.25 + 40*.5 + 100*.25 = 46.79 -> 46.8
	d := Data{Pathways: []model.ClinicalPathway{{ID: "a", Diagnosis: "x"}},
		Episodes: []model.PathwayEpisode{{PathwayID: "a", Adherence: f(80)}, {PathwayID: "a"}}}
	if _, pr := pathwayDomain(d); !near(*pr.CPCS, 46.8, 0.001) || pr.Covered != 1 || pr.CoverageTarget != 14 {
		t.Errorf("CPCS=%v cakupan=%d/%d", *pr.CPCS, pr.Covered, pr.CoverageTarget)
	}
	// cakupan tetap: 7 pathway -> 7/14 ; 8 pathway -> 16/20
	mkN := func(n int) Data {
		var ps []model.ClinicalPathway
		for i := 0; i < n; i++ {
			ps = append(ps, model.ClinicalPathway{ID: string(rune('a' + i))})
		}
		return Data{Pathways: ps}
	}
	if _, pr := pathwayDomain(mkN(7)); !near(pr.Coverage, 50, 0.001) {
		t.Errorf("7 pathway cakupan=%v want 50", pr.Coverage)
	}
	if _, pr := pathwayDomain(mkN(8)); !near(pr.Coverage, 80, 0.001) {
		t.Errorf("8 pathway cakupan=%v want 80", pr.Coverage)
	}
	// pathway terlemah diurutkan naik & dibatasi 3
	var ps []model.ClinicalPathway
	for i, a := range []float64{90, 40, 70, 20, 60} {
		ps = append(ps, model.ClinicalPathway{ID: string(rune('a' + i)), Diagnosis: "dx", AvgAdherence: f(a)})
	}
	_, pr := pathwayDomain(Data{Pathways: ps})
	if len(pr.Weakest) != 3 || pr.Weakest[0].Adherence != 20 || pr.Weakest[2].Adherence != 60 {
		t.Errorf("weakest=%+v", pr.Weakest)
	}
}

// ---------- Utilization ----------

func TestExtraURBoundaries(t *testing.T) {
	one := func(alos, re, bor float64) *URResult {
		return urScore(Data{Utilization: []model.UtilizationStat{{Month: "2026-09", ALOS: f(alos), Readmission: f(re), BOR: f(bor)}}})
	}
	if u := one(5, 5, 60); u.Score != 100 {
		t.Errorf("tepat di ambang (5,5,60) harus 100, dapat %v", u.Score)
	}
	if u := one(5, 5, 85); u.Score != 100 {
		t.Errorf("BOR 85 harus 100, dapat %v", u.Score)
	}
	if u := one(5, 5, 59); !near(u.Score, 99.3, 0.001) { // BOR 100-1x2 = 98
		t.Errorf("BOR 59 -> %v", u.Score)
	}
	if u := one(10, 5, 50); !near(u.Score, 60, 0.001) { // ALOS 100-5x20 = 0, BOR 100-10x2 = 80
		t.Errorf("(10,5,50) -> %v", u.Score)
	}
	if u := one(6, 7, 90); !near(u.Score, 80, 0.001) { // 80, 70, 90
		t.Errorf("(6,7,90) -> %v", u.Score)
	}
	// tanpa data: nilai 0 -> ALOS 100, Readmission 100, BOR 0 -> 66.7 (sama dengan referensi)
	for _, d := range []Data{{}, {Utilization: []model.UtilizationStat{{Month: "2026-09"}}}} {
		if u := urScore(d); u == nil || !near(u.Score, 66.7, 0.001) || len(u.Components) != 3 {
			t.Errorf("UR tanpa nilai=%+v want 66.7", u)
		}
	}
}

// ---------- Sarana ----------

func TestExtraSarana(t *testing.T) {
	d := Data{Facilities: []model.Facility{
		{Name: "a", Availability: bl(true), Functional: bl(true), Calibration: sp("Valid")},  // 100
		{Name: "b", Availability: bl(true), Functional: bl(false), Calibration: sp("Valid")}, // 75
		{Name: "c"}, // 40
		{Name: "d", Availability: bl(false), Calibration: sp("Perlu Perbaikan")},                        // 75
		{Name: "e", Category: sp("Penunjang (opsional)"), Availability: bl(true), Functional: bl(true)}, // bukan wajib
	}}
	sr := saranaScore(d)
	if !near(sr.Score, 72.5, 0.001) || sr.Total != 5 || sr.Wajib != 4 {
		t.Errorf("sarana=%+v want 72.5 total 5 wajib 4", sr)
	}
	if s := saranaScore(Data{}); s.Score != 0 {
		t.Error("tanpa sarana harus 0")
	}
}

// ---------- Integrasi formula (golden) ----------

func TestExtraGoldenFullPipeline(t *testing.T) {
	out := Compute(goldenData())
	want := map[DomainKey]float64{DomWorkforce: 75, DomCredentialing: 100, DomDigital: 80.3, DomMedication: 70,
		DomDiagnostic: 90, DomInteroperability: 100, DomClaim: 87.5, DomPathway: 58.6, DomSafety: 81}
	for k, w := range want {
		if s := domOf(out, k).Score; s == nil || !near(*s, w, 0.001) {
			t.Errorf("domain %s = %v want %v", k, *s, w)
		}
	}
	if !near(out.HealthScore, 83.0, 0.001) {
		t.Errorf("health=%v want 83.0", out.HealthScore)
	}
	grp := map[string]float64{"A": 81.3, "B": 84.7, "C": 79.0, "D": 81}
	for _, g := range out.Groups {
		if g.Score == nil || !near(*g.Score, grp[g.Code], 0.001) {
			t.Errorf("kelompok %s = %v want %v", g.Code, *g.Score, grp[g.Code])
		}
	}
	if !near(out.BDCSRaw, 81.5, 0.001) || !near(out.BDCS, 81.5, 0.001) {
		t.Errorf("bdcsRaw=%v bdcs=%v want 81.5", out.BDCSRaw, out.BDCS)
	}
	if out.StageRaw != 6 || out.StageEffective != 6 || !out.Pass || out.GateBlocked {
		t.Errorf("stage raw=%d eff=%d pass=%v blocked=%v", out.StageRaw, out.StageEffective, out.Pass, out.GateBlocked)
	}
	if len(out.Warnings) != 0 {
		t.Errorf("semua data terisi, warnings harus kosong: %v", out.Warnings)
	}
	var wsum, csum float64
	for _, d := range out.Domains {
		wsum += d.EffectiveWeight
		csum += d.Contribution
	}
	if !near(wsum, 100, 0.001) || !near(csum, out.HealthScore, 0.06) {
		t.Errorf("sum bobot efektif=%v sum kontribusi=%v health=%v", wsum, csum, out.HealthScore)
	}
	if out.SafetyScore == nil || !near(*out.SafetyScore, 81, 0.001) || out.Claim.Grade != "GOOD" {
		t.Errorf("SafetyScore=%v grade=%s", out.SafetyScore, out.Claim.Grade)
	}
	if !near(out.Sarana.Score, 87.5, 0.001) || !near(out.Utilization.Score, 98, 0.001) {
		t.Errorf("sarana=%v ur=%v", out.Sarana.Score, out.Utilization.Score)
	}
}

func TestExtraNextStage(t *testing.T) {
	if ns := nextStage(6, 85.5); ns == nil || ns.Stage != 7 || ns.MinBDCS != 90 || !near(ns.Gap, 4.5, 0.001) {
		t.Errorf("next(6,85.5)=%+v", ns)
	}
	if ns := nextStage(0, 10); ns == nil || ns.Stage != 1 || ns.MinBDCS != 20 || !near(ns.Gap, 10, 0.001) {
		t.Errorf("next(0,10)=%+v", ns)
	}
	if nextStage(7, 95) != nil {
		t.Error("Stage 7 tidak punya stage berikutnya")
	}
}

func TestExtraStageMonotonic(t *testing.T) {
	prev := -1
	for v := 0.0; v <= 100; v += 0.5 {
		s := stageOf(v)
		if s < prev {
			t.Fatalf("stage turun saat BDCS naik di %v", v)
		}
		prev = s
	}
}

func TestExtraConfigIntegrity(t *testing.T) {
	var dw float64
	for _, d := range domainDefs {
		dw += d.Weight
	}
	var gw float64
	seen := map[DomainKey]int{}
	for _, g := range groupDefs {
		gw += g.Weight
		var cw float64
		for _, c := range g.Components {
			cw += c.Weight
		}
		if !near(cw, 1, 1e-9) {
			t.Errorf("bobot komponen kelompok %s = %v harus 1", g.Code, cw)
		}
		for _, k := range g.Domains {
			seen[k]++
		}
	}
	if dw != 100 || gw != 100 {
		t.Errorf("bobot domain=%v bobot kelompok=%v harus 100", dw, gw)
	}
	for _, d := range domainDefs {
		if seen[d.Key] > 1 {
			t.Errorf("domain %s muncul %d kali di kelompok A-D", d.Key, seen[d.Key])
		}
	}
	if seen[DomCredentialing] != 0 {
		t.Error("Credentialing tidak masuk kelompok A-D (sama dengan referensi)")
	}
	if !near(cpcsCoverageW+cpcsAdherenceW+cpcsDocumentationW, 1, 1e-9) {
		t.Error("bobot CPCS harus 1.0")
	}
}

// ---------- Perilaku ketika data tipis ----------

// Domain tanpa data dihitung 0, sehingga satu domain bagus + gerbang valid TIDAK bisa lulus.
func TestExtraSparseDataCannotPass(t *testing.T) {
	out := Compute(Data{Now: now, Indicators: []model.DigitalIndicator{ind("Interoperabilitas", "FHIR", 95, 90, 1, "up")},
		Requirements: validReqs()})
	if out.Pass || out.StageEffective >= PassStage {
		t.Errorf("1 domain terisi tidak boleh lulus: stage=%d bdcs=%v", out.StageEffective, out.BDCS)
	}
}

// ---------- Under Review ----------

func TestExtraUnderReviewStatusCase(t *testing.T) {
	q := func(st string) model.DataQualityIssue { return model.DataQualityIssue{Anomaly: true, Status: st} }
	d := Data{Now: now, DQIssues: []model.DataQualityIssue{q("Diperbaiki"), q("DIPERBAIKI"), q("terbuka"), q("Terbuka"), q("")}}
	if out := Compute(d); out.AnomalyFlags != 3 || !out.UnderReview {
		t.Errorf("flags=%d review=%v want 3 dan true", out.AnomalyFlags, out.UnderReview)
	}
}

// ---------- Ketahanan ----------

func TestExtraDoesNotMutateInputAndIsDeterministic(t *testing.T) {
	d := goldenData()
	a, b := Compute(d), Compute(d)
	if a.HealthScore != b.HealthScore || a.BDCS != b.BDCS || a.StageEffective != b.StageEffective {
		t.Error("Compute harus deterministik untuk input yang sama")
	}
}

func TestExtraZeroNowUsesClock(t *testing.T) {
	before := time.Now()
	out := Compute(Data{})
	if out.ComputedAt.Before(before) {
		t.Error("Now kosong harus diisi time.Now()")
	}
	if out.FormulaVersion != "medcredix-scorer/1.0" {
		t.Errorf("versi formula=%s", out.FormulaVersion)
	}
}
