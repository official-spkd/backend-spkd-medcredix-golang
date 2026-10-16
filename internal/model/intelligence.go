package model

import (
	"time"
)

// DataQualityIssue memetakan tabel data_quality_issues.
type DataQualityIssue struct {
	ID         string  `gorm:"column:id;primaryKey" json:"id"`
	HospitalID string  `gorm:"column:hospital_id" json:"hospital_id"`
	Dimension  *string `gorm:"column:dimension" json:"dimension"`
	Title      string  `gorm:"column:title" json:"title"`
	Severity   *string `gorm:"column:severity" json:"severity"`
	Anomaly    bool    `gorm:"column:anomaly" json:"anomaly"`
	Status     string  `gorm:"column:status;default:terbuka" json:"status"`
}

func (DataQualityIssue) TableName() string { return "data_quality_issues" }

// TrustFactor memetakan tabel trust_factors.
type TrustFactor struct {
	ID         string  `gorm:"column:id;primaryKey" json:"id"`
	HospitalID string  `gorm:"column:hospital_id" json:"hospital_id"`
	Factor     string  `gorm:"column:factor" json:"factor"`
	Score      float64 `gorm:"column:score" json:"score"`
	Weight     float64 `gorm:"column:weight" json:"weight"`
}

func (TrustFactor) TableName() string { return "trust_factors" }

// ScoreSnapshot memetakan tabel score_snapshots.
type ScoreSnapshot struct {
	ID             string    `gorm:"column:id;primaryKey" json:"id"`
	HospitalID     string    `gorm:"column:hospital_id" json:"hospital_id"`
	Period         string    `gorm:"column:period" json:"period"`
	HealthScore    *float64  `gorm:"column:health_score" json:"health_score"`
	BDCS           *float64  `gorm:"column:bdcs" json:"bdcs"`
	Trust          *float64  `gorm:"column:trust" json:"trust"`
	CCS            *float64  `gorm:"column:ccs" json:"ccs"`
	CPCS           *float64  `gorm:"column:cpcs" json:"cpcs"`
	URScore        *float64  `gorm:"column:ur_score" json:"ur_score"`
	SafetyScore    *float64  `gorm:"column:safety_score" json:"safety_score"`
	FormulaVersion string    `gorm:"column:formula_version;default:medcredix-scorer/1.0" json:"formula_version"`
	CreatedAt      time.Time `gorm:"column:created_at;default:now()" json:"created_at"`
}

func (ScoreSnapshot) TableName() string { return "score_snapshots" }

// EvidenceItem memetakan tabel evidence_items.
type EvidenceItem struct {
	ID               string  `gorm:"column:id;primaryKey" json:"id"`
	HospitalID       string  `gorm:"column:hospital_id" json:"hospital_id"`
	RefType          string  `gorm:"column:ref_type" json:"ref_type"`
	RefID            *string `gorm:"column:ref_id" json:"ref_id"`
	Metric           *string `gorm:"column:metric" json:"metric"`
	MetricValue      *string `gorm:"column:metric_value" json:"metric_value"`
	SourceSystem     *string `gorm:"column:source_system" json:"source_system"`
	FHIRResource     *string `gorm:"column:fhir_resource" json:"fhir_resource"`
	RecordCount      *int    `gorm:"column:record_count" json:"record_count"`
	ValidationStatus *string `gorm:"column:validation_status" json:"validation_status"`
}

func (EvidenceItem) TableName() string { return "evidence_items" }

// DigitalIndicator memetakan tabel digital_indicators.
type DigitalIndicator struct {
	ID           string   `gorm:"column:id;primaryKey" json:"id"`
	HospitalID   string   `gorm:"column:hospital_id" json:"hospital_id"`
	Dimension    *string  `gorm:"column:dimension" json:"dimension"`
	Indicator    string   `gorm:"column:indicator" json:"indicator"`
	MetricValue  *float64 `gorm:"column:metric_value" json:"metric_value"`
	Threshold    *float64 `gorm:"column:threshold" json:"threshold"`
	Weight       *float64 `gorm:"column:weight" json:"weight"`
	Direction    *string  `gorm:"column:direction" json:"direction"`
	FHIRResource *string  `gorm:"column:fhir_resource" json:"fhir_resource"`
	Unit         *string  `gorm:"column:unit" json:"unit"`                   // %, ms, jam; kosong = %
	SourceSystem *string  `gorm:"column:source_system" json:"source_system"` // sistem asal, mis. EMR/RME, LIS
}

func (DigitalIndicator) TableName() string { return "digital_indicators" }

// ComplaintStat memetakan tabel complaint_stats.
type ComplaintStat struct {
	ID         string `gorm:"column:id;primaryKey" json:"id"`
	HospitalID string `gorm:"column:hospital_id" json:"hospital_id"`
	Month      string `gorm:"column:month" json:"month"`
	Total      *int   `gorm:"column:total" json:"total"`
	// Rincian keluhan per kategori; jumlahnya = Total.
	Antrian      *int     `gorm:"column:antrian" json:"antrian"`
	Pelayanan    *int     `gorm:"column:pelayanan" json:"pelayanan"`
	Obat         *int     `gorm:"column:obat" json:"obat"`
	Administrasi *int     `gorm:"column:administrasi" json:"administrasi"`
	Fasilitas    *int     `gorm:"column:fasilitas" json:"fasilitas"`
	Lainnya      *int     `gorm:"column:lainnya" json:"lainnya"`
	Satisfaction *float64 `gorm:"column:satisfaction" json:"satisfaction"`
	PeerAvg      *float64 `gorm:"column:peer_avg" json:"peer_avg"`
}

func (ComplaintStat) TableName() string { return "complaint_stats" }

// DataSource memetakan tabel data_sources.
type DataSource struct {
	ID          string     `gorm:"column:id;primaryKey" json:"id"`
	HospitalID  string     `gorm:"column:hospital_id" json:"hospital_id"`
	Name        string     `gorm:"column:name" json:"name"`
	Type        *string    `gorm:"column:type" json:"type"`
	Status      *string    `gorm:"column:status" json:"status"`
	AuthMethod  *string    `gorm:"column:auth_method" json:"auth_method"`
	SuccessRate *float64   `gorm:"column:success_rate" json:"success_rate"`
	CoveragePct *float64   `gorm:"column:coverage_pct" json:"coverage_pct"`
	LastSyncAt  *time.Time `gorm:"column:last_sync_at" json:"last_sync_at"`
	// Detail kanal untuk halaman Interoperabilitas & Data Sources.
	ConfigSummary *string  `gorm:"column:config_summary" json:"config_summary"`
	AvgLatencyMs  *int     `gorm:"column:avg_latency_ms" json:"avg_latency_ms"`
	Uptime30d     *float64 `gorm:"column:uptime_30d" json:"uptime_30d"`
	FHIRResources *int     `gorm:"column:fhir_resources" json:"fhir_resources"`
	Notes         *string  `gorm:"column:notes" json:"notes"`
}

func (DataSource) TableName() string { return "data_sources" }

// IntegrationJob memetakan tabel integration_jobs.
type IntegrationJob struct {
	ID            string  `gorm:"column:id;primaryKey" json:"id"`
	DataSourceID  string  `gorm:"column:data_source_id" json:"data_source_id"`
	JobType       *string `gorm:"column:job_type" json:"job_type"`
	Status        *string `gorm:"column:status" json:"status"`
	RecordsPulled *int    `gorm:"column:records_pulled" json:"records_pulled"`
	DurationMs    *int    `gorm:"column:duration_ms" json:"duration_ms"`
}

func (IntegrationJob) TableName() string { return "integration_jobs" }

// Facility memetakan tabel facilities.
type Facility struct {
	ID           string  `gorm:"column:id;primaryKey" json:"id"`
	HospitalID   string  `gorm:"column:hospital_id" json:"hospital_id"`
	Name         string  `gorm:"column:name" json:"name"`
	Category     *string `gorm:"column:category" json:"category"`
	Availability *bool   `gorm:"column:availability" json:"availability"`
	Functional   *bool   `gorm:"column:functional" json:"functional"`
	Calibration  *string `gorm:"column:calibration" json:"calibration"`
}

func (Facility) TableName() string { return "facilities" }

// BedStat memetakan tabel bed_stats.
type BedStat struct {
	ID         string  `gorm:"column:id;primaryKey" json:"id"`
	HospitalID string  `gorm:"column:hospital_id" json:"hospital_id"`
	Ward       *string `gorm:"column:ward" json:"ward"`
	BedClass   *string `gorm:"column:bed_class" json:"bed_class"`
	Total      *int    `gorm:"column:total" json:"total"`
	Active     *int    `gorm:"column:active" json:"active"`
}

func (BedStat) TableName() string { return "bed_stats" }

// SpecialtyStandard memetakan tabel specialty_standards.
type SpecialtyStandard struct {
	ID         string  `gorm:"column:id;primaryKey" json:"id"`
	HospitalID string  `gorm:"column:hospital_id" json:"hospital_id"`
	RsClass    *string `gorm:"column:rs_class" json:"rs_class"`
	Specialty  *string `gorm:"column:specialty" json:"specialty"`
	Required   *int    `gorm:"column:required" json:"required"`
}

func (SpecialtyStandard) TableName() string { return "specialty_standards" }

// UtilizationStat memetakan tabel utilization_stats.
type UtilizationStat struct {
	ID          string   `gorm:"column:id;primaryKey" json:"id"`
	HospitalID  string   `gorm:"column:hospital_id" json:"hospital_id"`
	Month       string   `gorm:"column:month" json:"month"`
	ALOS        *float64 `gorm:"column:alos" json:"alos"`
	Readmission *float64 `gorm:"column:readmission" json:"readmission"`
	BOR         *float64 `gorm:"column:bor" json:"bor"`
}

func (UtilizationStat) TableName() string { return "utilization_stats" }
