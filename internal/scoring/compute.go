package scoring

import (
	"encoding/json"
	"fmt"
	"math"
	"sort"
	"strings"
	"time"

	"github.com/spkd/medcredix/internal/model"
)

// Data = semua masukan engine untuk SATU rumah sakit. Compute() murni (tanpa DB),
// sehingga bisa diuji unit tanpa database. Pemuatan dari DB ada di loader.go.
type Data struct {
	HospitalID    string
	Now           time.Time
	Practitioners []model.Practitioner
	Credentials   []model.PractitionerCredential
	Requirements  []model.Requirement
	Claims        []model.Claim
	Pathways      []model.ClinicalPathway
	Episodes      []model.PathwayEpisode
	Indicators    []model.DigitalIndicator
	Sources       []model.DataSource
	TrustFactors  []model.TrustFactor
	DQIssues      []model.DataQualityIssue
	Specialty     []model.SpecialtyStandard
	Utilization   []model.UtilizationStat
	Facilities    []model.Facility
}

func r1(v float64) float64 { return math.Round(v*10) / 10 }
func r2(v float64) float64 { return math.Round(v*100) / 100 }
func clamp(v float64) float64 {
	if math.IsNaN(v) {
		return 0
	}
	return math.Max(0, math.Min(100, v))
}
func fp(v float64) *float64 { return &v }

type weighted struct{ value, weight float64 }

// wavg = rata-rata berbobot; 0 bila total bobot 0 (tidak pernah NaN).
func wavg(rows []weighted) float64 {
	var sum, wsum float64
	for _, r := range rows {
		sum += r.value * r.weight
		wsum += r.weight
	}
	if wsum == 0 {
		return 0
	}
	return sum / wsum
}

// pct = part/total x 100 dengan penyebut minimal 1 (sama dengan referensi: max(1, n)).
func pct(part, total int) float64 {
	if total < 1 {
		total = 1
	}
	return clamp(float64(part) / float64(total) * 100)
}

func val(p *float64) float64 {
	if p == nil {
		return 0
	}
	return *p
}

// normStatus menyeragamkan penulisan status dari DB/UI: "Segera Berakhir", "segera_berakhir",
// " SEGERA BERAKHIR " -> "segera_berakhir".
func normStatus(s string) string {
	return strings.ReplaceAll(strings.ToLower(strings.TrimSpace(s)), " ", "_")
}

// daysLeft = sisa hari sampai tanggal t (dibulatkan ke atas); negatif = sudah lewat.
// Sama dengan referensi: ceil((t - now) / 24 jam).
func daysLeft(t *time.Time, now time.Time) int {
	if t == nil {
		return 9999
	}
	return int(math.Ceil(float64(t.Sub(now)) / float64(24*time.Hour)))
}

// Compute menjalankan seluruh formula medcredix-scorer/1.0.
func Compute(d Data) Intelligence {
	if d.Now.IsZero() {
		d.Now = time.Now()
	}
	out := Intelligence{
		HospitalID: d.HospitalID, FormulaVersion: FormulaVersion, ComputedAt: d.Now,
		Warnings: []string{},
	}

	wfDom, wfRes := workforceDomain(d)
	credDom, credRes := credentialingDomain(d)
	digDom, emrScore, infraScore := digitalDomain(d)
	medDom := indicatorDomain(d, dimMed)
	diagDom := indicatorDomain(d, dimDiag)
	interopDom := interoperabilityDomain(d)
	claimDom, claimRes := claimDomain(d)
	pathDom, pathRes := pathwayDomain(d)
	safetyDom, safetyRes := safetyDomain(d)
	sarana := saranaScore(d)
	ur := urScore(d)

	raw := map[DomainKey]DomainResult{
		DomWorkforce:        wfDom,
		DomCredentialing:    credDom,
		DomDigital:          digDom,
		DomMedication:       medDom,
		DomDiagnostic:       diagDom,
		DomInteroperability: interopDom,
		DomClaim:            claimDom,
		DomPathway:          pathDom,
		DomSafety:           safetyDom,
	}

	// ---- 1. Hospital Health Score = rata-rata berbobot 9 domain.
	// Domain tanpa data dihitung 0 (bobot TIDAK dinormalisasi).
	var hRows []weighted
	for _, def := range domainDefs {
		r := raw[def.Key]
		r.Key, r.Label, r.Weight, r.EffectiveWeight = def.Key, def.Label, def.Weight, def.Weight
		if r.Score == nil {
			r.Score = fp(0)
		}
		if r.Indicators == nil {
			r.Indicators = []Indicator{}
		}
		if r.Notes == nil {
			r.Notes = []string{}
		}
		r.Contribution = r2(*r.Score * def.Weight / 100)
		if !r.HasData {
			out.Warnings = append(out.Warnings,
				fmt.Sprintf("Domain %s belum punya data dan dihitung 0.", def.Label))
		}
		hRows = append(hRows, weighted{*r.Score, def.Weight})
		out.Domains = append(out.Domains, r)
	}
	out.HealthScore = r1(wavg(hRows))

	// ---- 2. Kelompok A-D -> BDCS mentah
	values := map[string]float64{
		CompWorkforce: *wfDom.Score,
		CompSarana:    sarana.Score,
		CompEMR:       emrScore,
		CompInfra:     infraScore,
		CompMed:       *medDom.Score,
		CompDiag:      *diagDom.Score,
		CompInterop:   *interopDom.Score,
		CompCCS:       *claimRes.CCS,
		CompCPCS:      *pathRes.CPCS,
		CompUR:        ur.Score,
		CompSafety:    safetyRes.Score,
	}
	var bdcsRaw float64
	for _, g := range groupDefs {
		gr := GroupResult{Code: g.Code, Label: g.Label, Weight: g.Weight, Domains: g.Domains, Components: []GroupComponent{}}
		var rows []weighted
		for _, c := range g.Components {
			v := values[c.Key]
			rows = append(rows, weighted{v, c.Weight})
			gr.Components = append(gr.Components, GroupComponent{Key: c.Key, Label: c.Label, Weight: c.Weight, Score: r1(v)})
		}
		gs := r1(wavg(rows))
		gr.Score = fp(gs)
		bdcsRaw += gs * g.Weight / 100
		out.Groups = append(out.Groups, gr)
	}
	if len(d.Facilities) == 0 {
		out.Warnings = append(out.Warnings, "Data sarana & prasarana belum ada; komponen Sarana kelompok A dihitung 0.")
	}
	if ur.Month == "" {
		out.Warnings = append(out.Warnings, "Data utilisasi belum ada; ALOS/Readmission/BOR dihitung 0.")
	}

	// ---- 3. Trust -> pengali -> BDCS akhir -> Stage
	out.Trust = computeTrust(d)
	out.BDCSRaw = r1(bdcsRaw)
	out.BDCS = r1(out.BDCSRaw * out.Trust.Multiplier)
	out.StageRaw = stageOf(out.BDCS)
	out.NextStage = nextStage(out.StageRaw, out.BDCS)

	// ---- 4. Gerbang prasyarat P1-P3
	out.Gates, out.GateBlocked = evaluateGates(d)
	out.StageEffective = out.StageRaw
	if out.GateBlocked {
		out.StageEffective = 0
	}
	out.Pass = out.StageEffective >= PassStage

	// ---- 5. Anomaly flag -> Under Review
	for _, q := range d.DQIssues {
		if q.Anomaly && normStatus(q.Status) != "diperbaiki" {
			out.AnomalyFlags++
		}
	}
	out.UnderReview = out.AnomalyFlags >= AnomalyReviewThreshold

	// ---- 6. Sub-skor
	out.Claim = claimRes
	out.Pathway = pathRes
	out.Utilization = ur
	out.SafetyScore = fp(safetyRes.Score)
	out.Safety = safetyRes
	out.Workforce = wfRes
	out.Credentialing = credRes
	out.Sarana = sarana
	return out
}

func stageOf(bdcs float64) int {
	for _, s := range stageBands {
		if bdcs >= s.Min {
			return s.Stage
		}
	}
	return 0
}

func nextStage(current int, bdcs float64) *NextStage {
	for _, s := range stageBands {
		if s.Stage == current+1 {
			return &NextStage{Stage: s.Stage, MinBDCS: s.Min, Gap: r1(s.Min - bdcs)}
		}
	}
	return nil // sudah Stage tertinggi
}

// ---------------------------------------------------------------- Trust

// Trust = rata-rata berbobot faktor; tanpa faktor -> 100. Total bobot 0 -> 0 (sama dengan referensi).
func computeTrust(d Data) TrustResult {
	tr := TrustResult{Factors: []TrustFactorResult{}}
	if len(d.TrustFactors) == 0 {
		tr.Score, tr.Defaulted = 100, true // tidak ada faktor -> dianggap 100 (SRS 3.3.2)
	} else {
		var den float64
		rows := make([]weighted, 0, len(d.TrustFactors))
		for _, f := range d.TrustFactors {
			rows = append(rows, weighted{f.Score, f.Weight})
			den += f.Weight
		}
		tr.Score = r1(wavg(rows))
		for _, f := range d.TrustFactors {
			c := 0.0
			if den != 0 {
				c = r2(f.Score * f.Weight / den)
			}
			tr.Factors = append(tr.Factors, TrustFactorResult{
				Factor: f.Factor, Score: f.Score, Weight: f.Weight, Contribution: c,
			})
		}
	}
	tr.Multiplier = trustFloorMultiplier
	for _, b := range trustBands {
		if tr.Score >= b.Min {
			tr.Multiplier = b.Mult
			break
		}
	}
	return tr
}

// ---------------------------------------------------------------- Gerbang P1-P6

// Gerbang P1-P3 hanya memblokir bila prasyaratnya ADA dan statusnya bukan "valid".
// Prasyarat yang belum terdaftar dan dokumen lewat masa berlaku hanya diberi catatan.
func evaluateGates(d Data) ([]Gate, bool) {
	byCode := map[string]model.Requirement{}
	for _, r := range d.Requirements {
		byCode[strings.ToUpper(strings.TrimSpace(r.Code))] = r
	}
	gates := make([]Gate, 0, len(allRequirementCodes))
	blocked := false
	for _, code := range allRequirementCodes {
		g := Gate{Code: code, IsGate: gateCodes[code], Name: code, Status: "belum_ada"}
		r, found := byCode[code]
		if !found {
			g.Note = "prasyarat belum terdaftar untuk RS ini"
			gates = append(gates, g)
			continue
		}
		g.Name, g.Status = r.Name, r.Status
		g.Valid = normStatus(r.Status) == "valid"
		if r.DocExpiry != nil {
			dl := daysLeft(r.DocExpiry, d.Now)
			g.DaysLeft = &dl
			if dl < 0 {
				g.Note = "dokumen sudah melewati masa berlaku"
			}
		}
		if g.IsGate && !g.Valid {
			blocked = true
		}
		gates = append(gates, g)
	}
	return gates, blocked
}

// ---------------------------------------------------------------- Indikator

// normDirection: "down"/"lower" = makin kecil makin baik; selain itu "up".
func normDirection(dir *string) string {
	if dir != nil {
		switch strings.ToLower(strings.TrimSpace(*dir)) {
		case "down", "lower":
			return "down"
		}
	}
	return "up"
}

// indicatorScore: "up" -> nilainya sendiri (persen, dibatasi 0-100);
// "down" -> 100 bila <= ambang, selebihnya 100 - ((nilai-ambang)/ambang) x 100 x 1,2.
func indicatorScore(direction string, v, thr float64) float64 {
	if direction == "down" {
		if v <= thr {
			return 100
		}
		if thr <= 0 {
			return 0
		}
		return clamp(100 - (v-thr)/thr*100*indDownPenalty)
	}
	return clamp(v)
}

func indicatorStatus(direction string, v, thr float64) string {
	if direction == "down" {
		switch {
		case v <= thr:
			return "ok"
		case v <= thr*indWarnDownRatio:
			return "warn"
		}
		return "bad"
	}
	switch {
	case v >= thr:
		return "ok"
	case v >= thr*indWarnUpRatio:
		return "warn"
	}
	return "bad"
}

func containsAny(s string, subs []string) bool {
	s = strings.ToLower(s)
	for _, x := range subs {
		if strings.Contains(s, x) {
			return true
		}
	}
	return false
}

// dimensionOf mengembalikan dimensi kanonik indikator ("" bila tidak dikenal).
// Dimensi "Digital" (format lama) dipecah: indikator RME/EMR -> emr, lainnya -> infrastruktur.
func dimensionOf(x model.DigitalIndicator) string {
	if x.Dimension == nil {
		return ""
	}
	dim := dimensionAliases[strings.ToLower(strings.TrimSpace(*x.Dimension))]
	if dim == dimDigital {
		if containsAny(x.Indicator, emrNameHints) {
			return dimEMR
		}
		return dimInfra
	}
	return dim
}

// findIndicator = indikator pertama yang namanya mengandung kata kunci (cocok persis dulu,
// lalu tanpa membedakan huruf besar/kecil). dims kosong = semua dimensi.
func findIndicator(d Data, contains string, dims ...string) (model.DigitalIndicator, bool) {
	match := func(x model.DigitalIndicator) bool {
		if len(dims) == 0 {
			return true
		}
		dim := dimensionOf(x)
		for _, want := range dims {
			if dim == want {
				return true
			}
		}
		return false
	}
	for _, x := range d.Indicators {
		if match(x) && strings.Contains(x.Indicator, contains) {
			return x, true
		}
	}
	lc := strings.ToLower(contains)
	for _, x := range d.Indicators {
		if match(x) && strings.Contains(strings.ToLower(x.Indicator), lc) {
			return x, true
		}
	}
	return model.DigitalIndicator{}, false
}

// dimScore = rata-rata berbobot skor indikator satu dimensi (1 desimal) + daftar indikatornya.
// Nilai kosong dihitung 0, bobot kosong dianggap 1.
func dimScore(d Data, dim string) (float64, []Indicator, int) {
	inds := []Indicator{}
	var rows []weighted
	for _, x := range d.Indicators {
		if dimensionOf(x) != dim {
			continue
		}
		dir := normDirection(x.Direction)
		w := 1.0
		if x.Weight != nil {
			w = math.Max(0, *x.Weight)
		}
		v, thr := val(x.MetricValue), val(x.Threshold)
		sc := indicatorScore(dir, v, thr)
		ind := Indicator{Name: x.Indicator, Value: x.MetricValue, Threshold: x.Threshold, Direction: dir, Weight: w,
			Score: r1(sc), Status: indicatorStatus(dir, v, thr)}
		if x.MetricValue == nil {
			ind.Status = "no_data"
		}
		if x.FHIRResource != nil {
			ind.Source = *x.FHIRResource
		}
		inds = append(inds, ind)
		rows = append(rows, weighted{sc, w})
	}
	return r1(wavg(rows)), inds, len(inds)
}

func indicatorDomain(d Data, dim string) DomainResult {
	sc, inds, n := dimScore(d, dim)
	res := DomainResult{Score: fp(sc), HasData: n > 0, Indicators: inds,
		Formula: "Rata-rata tertimbang skor indikator vs ambang (indikator turun: 100 - selisih% x 1,2)"}
	if n == 0 {
		res.Notes = []string{"Data indikator belum tersedia dari SIMRS (dihitung 0)."}
	}
	return res
}

// ---------------------------------------------------------------- Domain: Digital Maturity

// Digital = EMR 50% + Infrastruktur 50%; EMR = Rawat Inap 55% + Rawat Jalan 45% (formula legacy).
func digitalDomain(d Data) (DomainResult, float64, float64) {
	_, emrInds, nEMR := dimScore(d, dimEMR)
	infra, infraInds, nInfra := dimScore(d, dimInfra)
	var ri, rj float64
	if x, ok := findIndicator(d, "Rawat Inap", dimEMR); ok {
		ri = val(x.MetricValue)
	}
	if x, ok := findIndicator(d, "Rawat Jalan", dimEMR); ok {
		rj = val(x.MetricValue)
	}
	emr := r1(ri*emrInpatientW + rj*emrOutpatientW)
	res := DomainResult{
		Score:      fp(r1(emr*digitalEMRW + infra*digitalInfraW)),
		HasData:    nEMR+nInfra > 0,
		Formula:    "EMR (Rawat Inap x 55% + Rawat Jalan x 45%) x 50% + Infrastruktur x 50%",
		Indicators: append(append([]Indicator{}, emrInds...), infraInds...),
		Notes:      []string{fmt.Sprintf("Skor EMR %.1f (RI %.1f%%, RJ %.1f%%), Infrastruktur %.1f.", emr, ri, rj, infra)},
	}
	if !res.HasData {
		res.Notes = []string{"Data indikator EMR/infrastruktur belum tersedia (dihitung 0)."}
	}
	return res, emr, infra
}

// Interoperability = skor indikator dimensi interoperabilitas saja.
// Success rate sumber data hanya ditampilkan sebagai catatan, tidak ikut dihitung.
func interoperabilityDomain(d Data) DomainResult {
	res := indicatorDomain(d, dimInterop)
	var sum float64
	n := 0
	for _, s := range d.Sources {
		if s.SuccessRate != nil {
			sum += *s.SuccessRate
			n++
		}
	}
	if n > 0 {
		res.Notes = append(res.Notes, fmt.Sprintf("Info: rata-rata success rate %d sumber data %.1f%% (tidak ikut skor).", n, sum/float64(n)))
	}
	return res
}

// ---------------------------------------------------------------- Domain: Patient Safety

// Safety = Alergi Obat 40% + High-Alert & Closed-Loop 30% + Dokumentasi Pemulangan 30%.
func safetyDomain(d Data) (DomainResult, SafetyResult) {
	_, inds, n := dimScore(d, dimSafety)
	parts := map[string]float64{}
	found := 0
	var sum float64
	for _, p := range safetyParts {
		if x, ok := findIndicator(d, p.Contains); ok {
			parts[p.Key] = val(x.MetricValue)
			found++
		}
		sum += parts[p.Key] * p.Weight
	}
	sr := SafetyResult{Score: r1(sum), Allergy: parts["allergy"], ClosedLoop: parts["closedLoop"], Discharge: parts["discharge"]}
	res := DomainResult{
		Score: fp(sr.Score), HasData: n > 0 || found > 0, Indicators: inds,
		Formula: "Alergi Obat x 40% + High-Alert & Closed-Loop x 30% + Dokumentasi Pemulangan x 30%",
		Notes: []string{fmt.Sprintf("Alergi %.1f%%, closed-loop %.1f%%, pemulangan %.1f%%.",
			sr.Allergy, sr.ClosedLoop, sr.Discharge)},
	}
	if found < len(safetyParts) {
		res.Notes = append(res.Notes, "Indikator keselamatan yang tidak ditemukan dihitung 0.")
	}
	return res, sr
}

// ---------------------------------------------------------------- Domain: Workforce

// Workforce = cakupan spesialis 50% + SIP aktif 30% + status tenaga aktif 20%.
//
//	cakupan   = rata-rata min(tersedia/wajib, 100%) per spesialis (standar Required <= 0 diabaikan)
//	SIP aktif = % praktisi dengan SIP belum lewat
//	aktif     = % praktisi tanpa kredensial ditangguhkan/dicabut (pengganti kolom status praktisi)
func workforceDomain(d Data) (DomainResult, WorkforceResult) {
	inactive := map[string]bool{}
	for _, c := range d.Credentials {
		if practitionerInactiveCred[normStatus(c.Status)] {
			inactive[c.PractitionerID] = true
		}
	}
	avail := map[string]int{}
	activeN, sipN := 0, 0
	for _, p := range d.Practitioners {
		if p.SIPExpiry != nil && daysLeft(p.SIPExpiry, d.Now) >= 0 {
			sipN++
		}
		if inactive[p.ID] {
			continue
		}
		activeN++
		if p.Specialty == nil || strings.TrimSpace(*p.Specialty) == "" {
			continue
		}
		if p.Profession != nil && strings.TrimSpace(*p.Profession) != "" &&
			!strings.Contains(strings.ToLower(*p.Profession), "spesialis") {
			continue // hanya dokter spesialis/subspesialis
		}
		avail[strings.ToLower(strings.TrimSpace(*p.Specialty))]++
	}

	wr := WorkforceResult{Practitioners: len(d.Practitioners), Specialists: []SpecialistCoverage{}}
	var covRows []weighted
	met := 0
	for _, s := range d.Specialty {
		if s.Specialty == nil || s.Required == nil || *s.Required <= 0 {
			continue // hindari pembagian dengan 0
		}
		have := avail[strings.ToLower(strings.TrimSpace(*s.Specialty))]
		cov := clamp(float64(have) / float64(*s.Required) * 100)
		if cov >= 100 {
			met++
		}
		wr.Specialists = append(wr.Specialists, SpecialistCoverage{
			Specialty: *s.Specialty, Required: *s.Required, Available: have, Coverage: r1(cov),
		})
		covRows = append(covRows, weighted{cov, 1})
	}
	coverage := wavg(covRows)
	wr.Coverage = r1(coverage)
	wr.SIPActive = r1(pct(sipN, len(d.Practitioners)))
	wr.Active = r1(pct(activeN, len(d.Practitioners)))

	sipPct, actPct := pct(sipN, len(d.Practitioners)), pct(activeN, len(d.Practitioners))
	score := r1(wavg([]weighted{{coverage, wfCoverageW}, {sipPct, wfSIPActiveW}, {actPct, wfActiveW}}))
	res := DomainResult{
		Score:   fp(score),
		HasData: len(d.Practitioners) > 0 || len(wr.Specialists) > 0,
		Formula: "Cakupan spesialis (50%) + SIP aktif (30%) + status tenaga aktif (20%)",
		Indicators: []Indicator{
			{Name: "Cakupan spesialis wajib", Source: "Practitioner", Value: fp(r1(coverage)), Threshold: fp(100), Direction: "up",
				Weight: wfCoverageW, Score: r1(coverage), Status: indicatorStatus("up", coverage, 100),
				Note: fmt.Sprintf("%d/%d spesialis terpenuhi", met, len(wr.Specialists))},
			{Name: "Praktisi dengan SIP masih berlaku", Source: "Practitioner", Value: fp(r1(sipPct)), Threshold: fp(95), Direction: "up",
				Weight: wfSIPActiveW, Score: r1(sipPct), Status: indicatorStatus("up", sipPct, 95)},
			{Name: "Status tenaga aktif", Source: "Practitioner", Value: fp(r1(actPct)), Threshold: fp(90), Direction: "up",
				Weight: wfActiveW, Score: r1(actPct), Status: indicatorStatus("up", actPct, 90)},
		},
		Notes: []string{fmt.Sprintf("%d dari %d spesialis memenuhi standar.", met, len(wr.Specialists))},
	}
	if len(d.Specialty) == 0 {
		res.Notes = append(res.Notes, "Standar spesialis kelas RS belum diisi (cakupan dihitung 0).")
	}
	return res, wr
}

// ---------------------------------------------------------------- Domain: Credentialing

// Credentialing = kelengkapan STR+SIP 40% + lisensi aktif 25% + pipeline 15% + skor expiry 20%.
//
//	kelengkapan = % praktisi yang punya STR & SIP (kredensial pertama per jenis) yang tidak bermasalah
//	pipeline    = % kredensial pada status final
//	expiry      = 100 - (kedaluwarsa x10 + <=30 hari x4 + <=90 hari x2)
func credentialingDomain(d Data) (DomainResult, CredentialingResult) {
	byPract := map[string][]model.PractitionerCredential{}
	for _, c := range d.Credentials {
		byPract[c.PractitionerID] = append(byPract[c.PractitionerID], c)
	}
	firstOf := func(cs []model.PractitionerCredential, typ string) *model.PractitionerCredential {
		for i := range cs {
			if strings.EqualFold(strings.TrimSpace(cs[i].Type), typ) {
				return &cs[i]
			}
		}
		return nil
	}
	complete := 0
	for _, p := range d.Practitioners {
		cs := byPract[p.ID]
		str, sip := firstOf(cs, "STR"), firstOf(cs, "SIP")
		if str != nil && sip != nil && !credBadStatus[normStatus(str.Status)] && !credBadStatus[normStatus(sip.Status)] {
			complete++
		}
	}
	final := 0
	var cr CredentialingResult
	for _, c := range d.Credentials {
		if credFinalStatus[normStatus(c.Status)] {
			final++
		}
		if c.ExpiryDate != nil {
			switch dl := daysLeft(c.ExpiryDate, d.Now); {
			case dl < 0:
				cr.Expired++
			case dl <= 30:
				cr.Within30++
			case dl <= 90:
				cr.Within90++
			}
		}
	}
	completeness := pct(complete, len(d.Practitioners))
	license := completeness // STR & SIP tidak bermasalah = definisi yang sama dengan referensi
	pipeline := pct(final, len(d.Credentials))
	expiry := clamp(100 - float64(cr.Expired*credExpiredPen+cr.Within30*credWithin30Pen+cr.Within90*credWithin90Pen))
	cr.Completeness, cr.LicenseActive, cr.PipelineHealth, cr.ExpiryScore = r1(completeness), r1(license), r1(pipeline), r1(expiry)

	score := r1(wavg([]weighted{{completeness, credCompleteW}, {license, credLicenseW}, {pipeline, credPipelineW}, {expiry, credExpiryW}}))
	res := DomainResult{
		Score:   fp(score),
		HasData: len(d.Practitioners) > 0 || len(d.Credentials) > 0,
		Formula: "Kelengkapan STR+SIP (40%) + lisensi aktif (25%) + pipeline (15%) + expiry (20%); expiry = 100 - (kedaluwarsa x10 + <=30 hari x4 + <=90 hari x2)",
		Indicators: []Indicator{
			{Name: "Kelengkapan kredensial inti (STR+SIP)", Source: "Practitioner", Value: fp(cr.Completeness), Threshold: fp(95),
				Direction: "up", Weight: credCompleteW, Score: cr.Completeness, Status: indicatorStatus("up", completeness, 95),
				Note: fmt.Sprintf("%d/%d praktisi lengkap", complete, len(d.Practitioners))},
			{Name: "Lisensi aktif (STR & SIP tidak bermasalah)", Source: "Practitioner", Value: fp(cr.LicenseActive), Threshold: fp(95),
				Direction: "up", Weight: credLicenseW, Score: cr.LicenseActive, Status: indicatorStatus("up", license, 95)},
			{Name: "Kredensial pada status final", Source: "PractitionerCredential", Value: fp(cr.PipelineHealth), Threshold: fp(90),
				Direction: "up", Weight: credPipelineW, Score: cr.PipelineHealth, Status: indicatorStatus("up", pipeline, 90)},
			{Name: "Skor kesehatan expiry", Source: "PractitionerCredential", Value: fp(cr.ExpiryScore), Threshold: fp(80),
				Direction: "up", Weight: credExpiryW, Score: cr.ExpiryScore, Status: indicatorStatus("up", expiry, 80),
				Note: fmt.Sprintf("%d kedaluwarsa; %d <=30 hari; %d <=90 hari", cr.Expired, cr.Within30, cr.Within90)},
		},
	}
	byStatus := map[string]int{}
	for _, c := range d.Credentials {
		byStatus[c.Status]++
	}
	keys := make([]string, 0, len(byStatus))
	for k := range byStatus {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	parts := make([]string, 0, len(keys))
	for _, k := range keys {
		parts = append(parts, fmt.Sprintf("%s=%d", k, byStatus[k]))
	}
	res.Notes = []string{fmt.Sprintf("%d dari %d praktisi memiliki STR & SIP lengkap.", complete, len(d.Practitioners))}
	if len(parts) > 0 {
		res.Notes = append(res.Notes, "Rincian status: "+strings.Join(parts, ", "))
	}
	return res, cr
}

// ---------------------------------------------------------------- Domain: Claim (CCS)

// parseClaimFlags membaca flags_json dalam dua format:
//
//	{"flags": {"diagnosis": "ok", ...}}   (format lama)
//	{"Diagnosis": "ok", ...}              (format referensi)
//
// Kunci & nilai dijadikan huruf kecil. Nilai bukan string diabaikan.
func parseClaimFlags(raw json.RawMessage) map[string]string {
	out := map[string]string{}
	if len(raw) == 0 {
		return out
	}
	var top map[string]json.RawMessage
	if json.Unmarshal(raw, &top) != nil {
		// flags_json bisa tersimpan sebagai string JSON di dalam JSONB
		var s string
		if json.Unmarshal(raw, &s) != nil || json.Unmarshal([]byte(s), &top) != nil {
			return out
		}
	}
	src := top
	if inner, ok := top["flags"]; ok {
		var m map[string]json.RawMessage
		if json.Unmarshal(inner, &m) == nil {
			src = m
		}
	}
	for k, v := range src {
		var s string
		if json.Unmarshal(v, &s) != nil {
			continue
		}
		out[strings.ToLower(strings.TrimSpace(k))] = strings.ToLower(strings.TrimSpace(s))
	}
	return out
}

// CCS = sum(bobot kategori x sub-skor); sub-skor = (ok + warn x 0,5) / total klaim x 100.
func claimDomain(d Data) (DomainResult, ClaimResult) {
	cr := ClaimResult{TotalClaims: len(d.Claims), Categories: []ClaimCategory{}}
	all := make([]map[string]string, 0, len(d.Claims))
	for _, c := range d.Claims {
		fl := parseClaimFlags(c.FlagsJSON)
		all = append(all, fl)
		if len(fl) > 0 {
			cr.Evaluated++
		}
		for _, v := range fl {
			if v != "ok" {
				cr.FlaggedClaims++
				break
			}
		}
	}
	total := len(d.Claims)
	if total < 1 {
		total = 1
	}
	var rows []weighted
	res := DomainResult{Formula: "CCS = Diagnosis 25% + Medication 20% + Procedure 15% + Diagnostic 15% + Stock 15% + Timeline 10%; sub-skor = (ok + warn x 0,5) / total klaim"}
	for _, cat := range claimCategories {
		cc := ClaimCategory{Key: cat.Key, Label: cat.Label, Weight: cat.Weight}
		for _, fl := range all {
			switch fl[cat.Key] {
			case "ok":
				cc.Ok++
			case "warn":
				cc.Warn++
			case "bad":
				cc.Bad++
			}
		}
		cc.Evaluated = cc.Ok + cc.Warn + cc.Bad
		cc.Score = r1((float64(cc.Ok) + float64(cc.Warn)*claimWarnCredit) / float64(total) * 100)
		cr.Categories = append(cr.Categories, cc)
		rows = append(rows, weighted{cc.Score, cat.Weight})
		res.Indicators = append(res.Indicators, Indicator{
			Name: cat.Label, Source: "Claim", Value: fp(cc.Score), Threshold: fp(85), Direction: "up",
			Weight: cat.Weight / 100, Score: cc.Score, Status: indicatorStatus("up", cc.Score, 85),
			Note: fmt.Sprintf("%d ok, %d perlu tinjauan, %d tinggi", cc.Ok, cc.Warn, cc.Bad),
		})
	}
	ccs := r1(wavg(rows))
	cr.CCS = fp(ccs)
	cr.Grade = claimGrade(ccs)
	res.Score = fp(ccs)
	res.HasData = len(d.Claims) > 0
	if res.HasData {
		res.Notes = []string{fmt.Sprintf("%d klaim bersih, %d klaim dengan minimal satu sinyal inkonsistensi.",
			len(d.Claims)-cr.FlaggedClaims, cr.FlaggedClaims)}
	} else {
		res.Notes = []string{"Belum ada klaim yang dievaluasi (dihitung 0)."}
	}
	return res, cr
}

// ---------------------------------------------------------------- Domain: Pathway (CPCS)

// CPCS = cakupan 25% + kepatuhan 50% + dokumentasi deviasi 25%.
//
//	cakupan      = angka tetap referensi: >= 8 pathway -> 16/20, selain itu min(pathway, 11)/14
//	kepatuhan    = rata-rata adherence semua episode (kosong dihitung 0)
//	dokumentasi  = % episode berdeviasi (documented != null) yang terdokumentasi; tanpa deviasi = 100
func pathwayDomain(d Data) (DomainResult, PathwayResult) {
	pr := PathwayResult{Pathways: len(d.Pathways), Episodes: len(d.Episodes), Weakest: []WeakPathway{}}

	if len(d.Pathways) >= cpcsBigMinPathways {
		pr.Covered, pr.CoverageTarget = cpcsBigCovered, cpcsBigTotal
	} else {
		pr.Covered, pr.CoverageTarget = len(d.Pathways), cpcsSmallTotal
		if pr.Covered > cpcsSmallMaxCover {
			pr.Covered = cpcsSmallMaxCover
		}
	}
	coverage := float64(pr.Covered) / float64(pr.CoverageTarget) * 100

	var adhSum float64
	docT, docN := 0, 0
	for _, e := range d.Episodes {
		adhSum += val(e.Adherence)
		if e.Documented != nil {
			docN++
			if *e.Documented {
				docT++
			}
		}
	}
	adherence := 0.0
	if len(d.Episodes) > 0 {
		adherence = adhSum / float64(len(d.Episodes))
	}
	documentation := 100.0 // tidak ada deviasi yang wajib didokumentasikan
	if docN > 0 {
		documentation = float64(docT) / float64(docN) * 100
	}
	pr.Coverage, pr.Adherence, pr.Documentation = r1(coverage), r1(adherence), r1(documentation)
	pr.DeviationRequired, pr.DeviationDocumented = docN, docT

	cpcs := r1(coverage*cpcsCoverageW + adherence*cpcsAdherenceW + documentation*cpcsDocumentationW)
	pr.CPCS = fp(cpcs)
	res := DomainResult{
		Score: fp(cpcs), HasData: len(d.Pathways) > 0 || len(d.Episodes) > 0,
		Formula: "CPCS = Coverage 25% + Adherence 50% + Dokumentasi Deviasi 25%",
		Notes: []string{fmt.Sprintf("Cakupan %.1f%% (%d/%d diagnosis utama), kepatuhan %.1f%% (%d episode), dokumentasi deviasi %.1f%% (%d/%d).",
			pr.Coverage, pr.Covered, pr.CoverageTarget, pr.Adherence, pr.Episodes, pr.Documentation, docT, docN)},
	}
	if len(d.Pathways) == 0 {
		res.Notes = append(res.Notes, "Belum ada clinical pathway.")
	}

	var weak []WeakPathway
	for _, p := range d.Pathways {
		if p.AvgAdherence != nil {
			weak = append(weak, WeakPathway{Diagnosis: p.Diagnosis, Adherence: *p.AvgAdherence})
		}
	}
	sort.SliceStable(weak, func(i, j int) bool { return weak[i].Adherence < weak[j].Adherence })
	if len(weak) > 3 {
		weak = weak[:3]
	}
	if weak != nil {
		pr.Weakest = weak
	}
	return res, pr
}

// ---------------------------------------------------------------- Sarana & Prasarana

// Sarana (kelompok A) = rata-rata skor fasilitas wajib (kategori tidak mengandung "opsional"):
// ada + berfungsi + kalibrasi valid -> 100; ada tapi tidak berfungsi / perlu perbaikan -> 75; selain itu 40.
func saranaScore(d Data) SaranaResult {
	sr := SaranaResult{Total: len(d.Facilities)}
	var rows []weighted
	for _, f := range d.Facilities {
		if f.Category != nil && strings.Contains(strings.ToLower(*f.Category), "opsional") {
			continue
		}
		sr.Wajib++
		available := f.Availability != nil && *f.Availability
		functional := f.Functional != nil && *f.Functional
		cal := ""
		if f.Calibration != nil {
			cal = normStatus(*f.Calibration)
		}
		v := 40.0
		switch {
		case available && functional && cal == "valid":
			v = 100
		case cal == "perlu_perbaikan" || (available && f.Functional != nil && !functional):
			v = 75
		}
		rows = append(rows, weighted{v, 1})
	}
	sr.Score = r1(wavg(rows))
	return sr
}

// ---------------------------------------------------------------- Utilization Review

// Skor UR = rata-rata 3 komponen pada bulan terbaru (nilai kosong dihitung 0):
//
//	ALOS        : 100 - max(0, (ALOS-5) x 20)
//	Readmission : 100 - max(0, (nilai-5) x 15)
//	BOR         : 60-85% -> 100; di luar itu 100 - selisih x 2
func urScore(d Data) *URResult {
	var latest model.UtilizationStat
	for i, u := range d.Utilization {
		if i == 0 || u.Month > latest.Month {
			latest = u
		}
	}
	alos, readm, bor := val(latest.ALOS), val(latest.Readmission), val(latest.BOR)
	alosS := clamp(100 - math.Max(0, (alos-urALOSMax)*urALOSPenalty))
	readmS := clamp(100 - math.Max(0, (readm-urReadmitMax)*urReadmitPenalty))
	borS := 100.0
	switch {
	case bor < urBORMin:
		borS = clamp(100 - (urBORMin-bor)*urBORPenalty)
	case bor > urBORMax:
		borS = clamp(100 - (bor-urBORMax)*urBORPenalty)
	}
	return &URResult{
		Month: latest.Month,
		Score: r1((alosS + readmS + borS) / 3),
		Components: []URComponent{
			{"ALOS (hari)", alos, "<= 5", r1(alosS)},
			{"Readmission (%)", readm, "<= 5", r1(readmS)},
			{"BOR (%)", bor, "60-85", r1(borS)},
		},
	}
}
