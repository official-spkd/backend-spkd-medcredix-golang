package scoring

import "time"

type Indicator struct {
	Name      string   `json:"name"`
	Source    string   `json:"source,omitempty"` // resource FHIR
	System    string   `json:"system,omitempty"` // sistem asal data (EMR/RME, LIS, ...)
	Unit      string   `json:"unit,omitempty"`   // %, ms, jam (kosong = %)
	Value     *float64 `json:"value"`
	Threshold *float64 `json:"threshold"`
	Direction string   `json:"direction"` // up | down
	Weight    float64  `json:"weight"`
	Score     float64  `json:"score"`  // 0-100
	Status    string   `json:"status"` // ok | warn | bad | no_data
	Note      string   `json:"note,omitempty"`
}

type DomainResult struct {
	Key             DomainKey   `json:"key"`
	Label           string      `json:"label"`
	Weight          float64     `json:"weight"`          // bobot resmi (persen)
	EffectiveWeight float64     `json:"effectiveWeight"` // = Weight (domain tanpa data dihitung 0, tanpa normalisasi)
	Score           *float64    `json:"score"`           // selalu terisi; 0 bila data belum ada (lihat HasData)
	HasData         bool        `json:"hasData"`         // false = data belum tersedia, skor dihitung 0
	Contribution    float64     `json:"contribution"`    // kontribusi ke Health Score
	Formula         string      `json:"formula"`
	Indicators      []Indicator `json:"indicators"`
	Notes           []string    `json:"notes"`
}

type GroupComponent struct {
	Key    string  `json:"key"`
	Label  string  `json:"label"`
	Weight float64 `json:"weight"` // pecahan dalam kelompok
	Score  float64 `json:"score"`
}

type GroupResult struct {
	Code       string           `json:"code"`
	Label      string           `json:"label"`
	Weight     float64          `json:"weight"`
	Score      *float64         `json:"score"`
	Domains    []DomainKey      `json:"domains"`
	Components []GroupComponent `json:"components"`
}

type TrustFactorResult struct {
	Factor       string  `json:"factor"`
	Score        float64 `json:"score"`
	Weight       float64 `json:"weight"`
	Contribution float64 `json:"contribution"`
}

type TrustResult struct {
	Score      float64             `json:"score"`
	Multiplier float64             `json:"multiplier"`
	Defaulted  bool                `json:"defaulted"` // true bila tidak ada faktor -> dianggap 100
	Factors    []TrustFactorResult `json:"factors"`
}

type Gate struct {
	Code     string `json:"code"`
	Name     string `json:"name"`
	Status   string `json:"status"`
	IsGate   bool   `json:"isGate"` // P1-P3
	Valid    bool   `json:"valid"`
	DaysLeft *int   `json:"daysLeft,omitempty"`
	Note     string `json:"note,omitempty"`
}

type NextStage struct {
	Stage   int     `json:"stage"`
	MinBDCS float64 `json:"minBdcs"`
	Gap     float64 `json:"gap"`
}

type ClaimCategory struct {
	Key       string  `json:"key"`
	Label     string  `json:"label"`
	Weight    float64 `json:"weight"`
	Score     float64 `json:"score"`
	Ok        int     `json:"ok"`
	Warn      int     `json:"warn"`
	Bad       int     `json:"bad"`
	Evaluated int     `json:"evaluated"` // ok + warn + bad
}

type ClaimResult struct {
	CCS           *float64        `json:"ccs"`
	Grade         string          `json:"grade"`
	TotalClaims   int             `json:"totalClaims"`
	Evaluated     int             `json:"evaluated"`     // klaim yang punya flag
	FlaggedClaims int             `json:"flaggedClaims"` // klaim dengan minimal satu flag bukan "ok"
	Categories    []ClaimCategory `json:"categories"`
}

type WeakPathway struct {
	Diagnosis string  `json:"diagnosis"`
	Adherence float64 `json:"adherence"`
}

type PathwayResult struct {
	CPCS                *float64      `json:"cpcs"`
	Coverage            float64       `json:"coverage"`
	Adherence           float64       `json:"adherence"`
	Documentation       float64       `json:"documentation"`
	Pathways            int           `json:"pathways"`
	Covered             int           `json:"covered"`        // diagnosis utama yang tercakup
	CoverageTarget      int           `json:"coverageTarget"` // total diagnosis utama
	Episodes            int           `json:"episodes"`
	DeviationRequired   int           `json:"deviationRequired"`
	DeviationDocumented int           `json:"deviationDocumented"`
	Weakest             []WeakPathway `json:"weakest"`
}

type URComponent struct {
	Name   string  `json:"name"`
	Value  float64 `json:"value"`
	Target string  `json:"target"`
	Score  float64 `json:"score"`
}

type URResult struct {
	Month      string        `json:"month"` // kosong bila belum ada data utilisasi
	Score      float64       `json:"score"`
	Components []URComponent `json:"components"`
}

type SpecialistCoverage struct {
	Specialty string  `json:"specialty"`
	Required  int     `json:"required"`
	Available int     `json:"available"`
	Coverage  float64 `json:"coverage"`
}

type WorkforceResult struct {
	Coverage      float64              `json:"coverage"`
	SIPActive     float64              `json:"sipActive"`
	Active        float64              `json:"active"`
	Practitioners int                  `json:"practitioners"`
	Specialists   []SpecialistCoverage `json:"specialists"`
}

type CredentialingResult struct {
	Completeness   float64 `json:"completeness"`
	LicenseActive  float64 `json:"licenseActive"`
	PipelineHealth float64 `json:"pipelineHealth"`
	ExpiryScore    float64 `json:"expiryScore"`
	Expired        int     `json:"expired"`
	Within30       int     `json:"within30"`
	Within90       int     `json:"within90"`
}

type SafetyResult struct {
	Score      float64 `json:"score"`
	Allergy    float64 `json:"allergy"`
	ClosedLoop float64 `json:"closedLoop"`
	Discharge  float64 `json:"discharge"`
}

type SaranaResult struct {
	Score float64 `json:"score"`
	Total int     `json:"total"`
	Wajib int     `json:"wajib"`
}

type Intelligence struct {
	HospitalID     string    `json:"hospitalId"`
	FormulaVersion string    `json:"formulaVersion"`
	ComputedAt     time.Time `json:"computedAt"`

	HealthScore float64        `json:"healthScore"`
	Domains     []DomainResult `json:"domains"`
	Groups      []GroupResult  `json:"groups"`

	BDCSRaw float64     `json:"bdcsRaw"`
	Trust   TrustResult `json:"trust"`
	BDCS    float64     `json:"bdcs"` // BDCS akhir = mentah x Trust Multiplier

	StageRaw       int        `json:"stageRaw"`
	StageEffective int        `json:"stageEffective"`
	Pass           bool       `json:"pass"` // LULUS bila StageEffective >= 5
	GateBlocked    bool       `json:"gateBlocked"`
	Gates          []Gate     `json:"gates"`
	NextStage      *NextStage `json:"nextStage"`

	AnomalyFlags int  `json:"anomalyFlags"`
	UnderReview  bool `json:"underReview"`

	Claim         ClaimResult         `json:"claim"`
	Pathway       PathwayResult       `json:"pathway"`
	Utilization   *URResult           `json:"utilization"` // selalu terisi (nilai kosong dihitung 0)
	SafetyScore   *float64            `json:"safetyScore"`
	Safety        SafetyResult        `json:"safety"`
	Workforce     WorkforceResult     `json:"workforce"`
	Credentialing CredentialingResult `json:"credentialing"`
	Sarana        SaranaResult        `json:"sarana"`

	Warnings []string `json:"warnings"`
}
