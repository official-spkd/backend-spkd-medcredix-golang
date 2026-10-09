package scoring

// Semua bobot, ambang, dan pemetaan skor ada di file ini (satu sumber kebenaran).
// Mengubah angka di sini = mengubah formula; naikkan FormulaVersion bila rumus berubah.
//
// Formula disamakan dengan engine referensi (port lib/scoring.ts, medcredix-scorer/1.0):
// domain tanpa data dihitung 0 (tidak ada normalisasi bobot), semua skor dibulatkan 1 desimal.

const FormulaVersion = "medcredix-scorer/1.0"

type DomainKey string

const (
	DomWorkforce        DomainKey = "workforce"
	DomCredentialing    DomainKey = "credentialing"
	DomDigital          DomainKey = "digital"
	DomMedication       DomainKey = "medication"
	DomDiagnostic       DomainKey = "diagnostic"
	DomInteroperability DomainKey = "interoperability"
	DomClaim            DomainKey = "claim"
	DomPathway          DomainKey = "pathway"
	DomSafety           DomainKey = "safety"
)

type domainDef struct {
	Key    DomainKey
	Label  string
	Weight float64 // persen; total 9 domain = 100 (SRS 3.3.1)
}

var domainDefs = []domainDef{
	{DomWorkforce, "Clinical Workforce", 12},
	{DomCredentialing, "Credentialing Readiness", 14},
	{DomDigital, "Digital Maturity", 10},
	{DomMedication, "Medication Management", 8},
	{DomDiagnostic, "Diagnostic Readiness", 8},
	{DomInteroperability, "Interoperability", 8},
	{DomClaim, "Claim Integrity", 16},
	{DomPathway, "Clinical Pathway", 10},
	{DomSafety, "Patient Safety", 14},
}

// Komponen kelompok A-D. Key = nilai yang dihitung di Compute (lihat groupValues).
const (
	CompWorkforce = "workforce" // skor domain Workforce
	CompSarana    = "sarana"    // skor sarana & prasarana (facilities)
	CompEMR       = "emr"       // RME: Rawat Inap 55% + Rawat Jalan 45%
	CompInfra     = "infrastruktur"
	CompMed       = "medication"
	CompDiag      = "diagnostic"
	CompInterop   = "interoperability"
	CompCCS       = "ccs"
	CompCPCS      = "cpcs"
	CompUR        = "ur"
	CompSafety    = "safety"
)

type groupComponentDef struct {
	Key    string
	Label  string
	Weight float64 // pecahan; total per kelompok = 1
}

// BDCS mentah = A*25% + B*30% + C*35% + D*10% (SRS 3.3.1).
// Isi kelompok mengikuti engine referensi. Catatan: Credentialing TIDAK masuk kelompok mana pun
// (hanya ikut Health Score), sama seperti engine referensi.
type groupDef struct {
	Code       string
	Label      string
	Weight     float64 // persen
	Domains    []DomainKey
	Components []groupComponentDef
}

var groupDefs = []groupDef{
	{"A", "Kapabilitas Klinis & SDM", 25, []DomainKey{DomWorkforce}, []groupComponentDef{
		{CompWorkforce, "Clinical Workforce", 0.5},
		{CompSarana, "Sarana & Prasarana", 0.5},
	}},
	{"B", "Digitalisasi & Sistem Informasi", 30, []DomainKey{DomDigital, DomMedication, DomDiagnostic, DomInteroperability}, []groupComponentDef{
		{CompEMR, "EMR/RME", 0.25},
		{CompInfra, "Infrastruktur", 0.15},
		{CompMed, "Medikasi", 0.20},
		{CompDiag, "Diagnostik", 0.15},
		{CompInterop, "Interoperabilitas", 0.25},
	}},
	{"C", "Kinerja JKN", 35, []DomainKey{DomClaim, DomPathway}, []groupComponentDef{
		{CompCCS, "Claim Consistency Score (CCS)", 0.50},
		{CompCPCS, "Clinical Pathway Compliance (CPCS)", 0.35},
		{CompUR, "Utilization Review", 0.15},
	}},
	{"D", "Keselamatan Pasien", 10, []DomainKey{DomSafety}, []groupComponentDef{
		{CompSafety, "Patient Safety", 1},
	}},
}

// Dimensi indikator digital (kanonik).
const (
	dimEMR     = "emr"
	dimInfra   = "infrastruktur"
	dimMed     = "medikasi"
	dimDiag    = "diagnostik"
	dimInterop = "interoperabilitas"
	dimSafety  = "keselamatan"
	dimDigital = "digital" // format lama: dipecah ke EMR/Infrastruktur menurut nama indikator
)

// digital_indicators.dimension (huruf kecil, trim) -> dimensi kanonik.
// Menerima format seed lama ("Digital", "Keselamatan Pasien") maupun format referensi
// ("EMR", "Infrastruktur", "Keselamatan").
var dimensionAliases = map[string]string{
	"emr": dimEMR, "rme": dimEMR,
	"infrastruktur": dimInfra, "infrastructure": dimInfra, "infra": dimInfra,
	"digital": dimDigital, "digital maturity": dimDigital,
	"medikasi": dimMed, "medication": dimMed,
	"diagnostik": dimDiag, "diagnostic": dimDiag,
	"interoperabilitas": dimInterop, "interoperability": dimInterop, "interop": dimInterop,
	"keselamatan": dimSafety, "keselamatan pasien": dimSafety, "safety": dimSafety, "patient safety": dimSafety,
}

// Kata kunci nama indikator yang dianggap EMR bila dimensinya "Digital" (format lama).
var emrNameHints = []string{"rawat inap", "rawat jalan", "rme", "emr", "rekam medis"}

// Skor EMR (formula legacy TransDCA): Rawat Inap 55% + Rawat Jalan 45%.
const (
	emrInpatientW  = 0.55
	emrOutpatientW = 0.45
)

// Digital Maturity = EMR 50% + Infrastruktur 50%.
const (
	digitalEMRW   = 0.5
	digitalInfraW = 0.5
)

// Clinical Workforce = cakupan spesialis 50% + SIP aktif 30% + status tenaga aktif 20%.
const (
	wfCoverageW  = 0.5
	wfSIPActiveW = 0.3
	wfActiveW    = 0.2
)

// Credentialing = kelengkapan STR+SIP 40% + lisensi aktif 25% + pipeline 15% + expiry 20%.
// Skor expiry = 100 - (kedaluwarsa x10 + <=30 hari x4 + <=90 hari x2).
const (
	credCompleteW   = 0.40
	credLicenseW    = 0.25
	credPipelineW   = 0.15
	credExpiryW     = 0.20
	credExpiredPen  = 10
	credWithin30Pen = 4
	credWithin90Pen = 2
)

// Status kredensial (sudah dinormalisasi lewat normStatus) yang membuat STR/SIP bermasalah.
var credBadStatus = map[string]bool{"kedaluwarsa": true, "ditangguhkan": true, "dicabut": true}

// Status kredensial yang dianggap sudah final (pipeline sehat).
var credFinalStatus = map[string]bool{"aktif": true, "segera_berakhir": true, "disetujui": true, "kedaluwarsa": true, "dicabut": true}

// Status kredensial yang membuat praktisi dianggap tidak aktif (pengganti kolom status praktisi).
var practitionerInactiveCred = map[string]bool{"ditangguhkan": true, "dicabut": true}

// Patient Safety = Alergi Obat 40% + High-Alert & Closed-Loop 30% + Dokumentasi Pemulangan 30%.
// Nilai diambil dari indikator pertama yang namanya mengandung kata kunci berikut.
var safetyParts = []struct {
	Key, Contains string
	Weight        float64
}{
	{"allergy", "Alergi", 0.4},
	{"closedLoop", "High-Alert", 0.3},
	{"discharge", "Pemulangan", 0.3},
}

// Trust Multiplier: >=90 -> 1,00; >=70 -> 0,95; >=50 -> 0,85; <50 -> 0,70.
var trustBands = []struct{ Min, Mult float64 }{{90, 1.00}, {70, 0.95}, {50, 0.85}}

const trustFloorMultiplier = 0.70

// Stage dari BDCS akhir: >=90=7, >=80=6, >=66=5, >=60=4, >=50=3, >=35=2, >=20=1, selain itu 0.
var stageBands = []struct {
	Min   float64
	Stage int
}{{90, 7}, {80, 6}, {66, 5}, {60, 4}, {50, 3}, {35, 2}, {20, 1}}

const PassStage = 5 // LULUS bila Stage efektif >= 5

// P1-P3 adalah gerbang Stage; P4-P6 hanya ditampilkan.
// Gerbang hanya memblokir bila prasyaratnya ADA dan statusnya bukan valid (sama dengan referensi).
var gateCodes = map[string]bool{"P1": true, "P2": true, "P3": true}
var allRequirementCodes = []string{"P1", "P2", "P3", "P4", "P5", "P6"}

// RS ditandai Under Review bila anomaly flag aktif >= 3.
const AnomalyReviewThreshold = 3

// CCS: 6 kategori (SRS 3.11.1). Key = kunci flag klaim (huruf kecil; "Diagnosis" juga diterima).
// Sub-skor = (ok + warn x 0,5) / total klaim x 100.
var claimCategories = []struct {
	Key    string
	Label  string
	Weight float64
}{
	{"diagnosis", "Diagnosis Consistency", 25},
	{"medication", "Medication Consistency", 20},
	{"procedure", "Procedure Consistency", 15},
	{"diagnostic", "Diagnostic Consistency", 15},
	{"stock", "Konsistensi Stok & Klaim (Triangle)", 15},
	{"timeline", "Completeness & Timeline", 10},
}

const claimWarnCredit = 0.5

// CPCS = cakupan 25% + kepatuhan 50% + dokumentasi deviasi 25% (SRS REQ-ENG-04).
const (
	cpcsCoverageW      = 0.25
	cpcsAdherenceW     = 0.50
	cpcsDocumentationW = 0.25
)

// Cakupan CPCS memakai angka tetap engine referensi:
// >= 8 pathway -> 16/20 diagnosis utama; selain itu min(jumlah pathway, 11)/14.
const (
	cpcsBigMinPathways = 8
	cpcsBigCovered     = 16
	cpcsBigTotal       = 20
	cpcsSmallTotal     = 14
	cpcsSmallMaxCover  = 11
)

// Utilization Review (linear): ALOS <= 5 hari, Readmission <= 5%, BOR 60-85%.
//
//	ALOS        : 100 - max(0, (ALOS-5) x 20)
//	Readmission : 100 - max(0, (nilai-5) x 15)
//	BOR         : 100 di 60-85, selain itu 100 - selisih x 2
const (
	urALOSMax        = 5.0
	urALOSPenalty    = 20.0
	urReadmitMax     = 5.0
	urReadmitPenalty = 15.0
	urBORMin         = 60.0
	urBORMax         = 85.0
	urBORPenalty     = 2.0
)

// Status indikator: naik (up) ok bila >= ambang, warn bila >= 85% ambang;
// turun (down) ok bila <= ambang, warn bila <= 125% ambang.
const (
	indWarnUpRatio   = 0.85
	indWarnDownRatio = 1.25
	indDownPenalty   = 1.2 // skor turun: 100 - ((nilai-ambang)/ambang) x 100 x 1,2
)

func claimGrade(ccs float64) string {
	switch {
	case ccs >= 90:
		return "EXCELLENT"
	case ccs >= 75:
		return "GOOD"
	case ccs >= 60:
		return "FAIR"
	default:
		return "POOR"
	}
}
