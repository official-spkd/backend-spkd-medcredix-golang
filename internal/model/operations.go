package model

import (
	"encoding/json"
	"time"
)

// Risk memetakan tabel risks.
type Risk struct {
	ID             string     `gorm:"column:id;primaryKey" json:"id"`
	HospitalID     string     `gorm:"column:hospital_id" json:"hospital_id"`
	Code           string     `gorm:"column:code" json:"code"`
	Category       string     `gorm:"column:category" json:"category"`
	Level          string     `gorm:"column:level" json:"level"`
	Title          string     `gorm:"column:title" json:"title"`
	Description    *string    `gorm:"column:description" json:"description"`
	BusinessImpact *string    `gorm:"column:business_impact" json:"business_impact"`
	Recommendation *string    `gorm:"column:recommendation" json:"recommendation"`
	SuggestedOwner *string    `gorm:"column:suggested_owner" json:"suggested_owner"`
	Deadline       *time.Time `gorm:"column:deadline" json:"deadline"`
	Status         string     `gorm:"column:status;default:baru" json:"status"`
}

func (Risk) TableName() string { return "risks" }

// EarlyWarning memetakan tabel early_warnings.
type EarlyWarning struct {
	ID            string     `gorm:"column:id;primaryKey" json:"id"`
	HospitalID    string     `gorm:"column:hospital_id" json:"hospital_id"`
	Code          string     `gorm:"column:code" json:"code"`
	Title         string     `gorm:"column:title" json:"title"`
	Severity      string     `gorm:"column:severity" json:"severity"`
	ThresholdRule *string    `gorm:"column:threshold_rule" json:"threshold_rule"`
	TriggerValue  *string    `gorm:"column:trigger_value" json:"trigger_value"`
	Status        string     `gorm:"column:status;default:baru" json:"status"`
	AssignedTo    *string    `gorm:"column:assigned_to" json:"assigned_to"`
	DueDate       *time.Time `gorm:"column:due_date" json:"due_date"`
}

func (EarlyWarning) TableName() string { return "early_warnings" }

// AlertRule memetakan tabel alert_rules.
type AlertRule struct {
	ID            string  `gorm:"column:id;primaryKey" json:"id"`
	HospitalID    string  `gorm:"column:hospital_id" json:"hospital_id"`
	Name          string  `gorm:"column:name" json:"name"`
	Domain        *string `gorm:"column:domain" json:"domain"`
	Metric        *string `gorm:"column:metric" json:"metric"`
	RuleCondition *string `gorm:"column:rule_condition" json:"rule_condition"`
	Severity      *string `gorm:"column:severity" json:"severity"`
	Channels      *string `gorm:"column:channels" json:"channels"`
	Active        bool    `gorm:"column:active" json:"active"`
}

func (AlertRule) TableName() string { return "alert_rules" }

// ReportSnapshot memetakan tabel report_snapshots.
type ReportSnapshot struct {
	ID          string          `gorm:"column:id;primaryKey" json:"id"`
	HospitalID  string          `gorm:"column:hospital_id" json:"hospital_id"`
	Type        string          `gorm:"column:type" json:"type"`
	Title       string          `gorm:"column:title" json:"title"`
	Period      *string         `gorm:"column:period" json:"period"`
	Version     string          `gorm:"column:version" json:"version"`
	AuditRef    *string         `gorm:"column:audit_ref" json:"audit_ref"`
	ContentJSON json.RawMessage `gorm:"column:content_json" json:"content_json"`
	CreatedBy   *string         `gorm:"column:created_by" json:"created_by"`
	CreatedAt   time.Time       `gorm:"column:created_at;default:now()" json:"created_at"`
}

func (ReportSnapshot) TableName() string { return "report_snapshots" }

// Notification memetakan tabel notifications.
type Notification struct {
	ID         string  `gorm:"column:id;primaryKey" json:"id"`
	HospitalID *string `gorm:"column:hospital_id" json:"hospital_id"`
	UserID     string  `gorm:"column:user_id" json:"user_id"`
	Title      string  `gorm:"column:title" json:"title"`
	Severity   *string `gorm:"column:severity" json:"severity"`
	Channel    *string `gorm:"column:channel" json:"channel"`
	IsRead     bool    `gorm:"column:is_read" json:"is_read"`
}

func (Notification) TableName() string { return "notifications" }

// Action memetakan tabel actions.
type Action struct {
	ID             string     `gorm:"column:id;primaryKey" json:"id"`
	HospitalID     string     `gorm:"column:hospital_id" json:"hospital_id"`
	Code           string     `gorm:"column:code" json:"code"`
	Title          string     `gorm:"column:title" json:"title"`
	Description    *string    `gorm:"column:description" json:"description"`
	Priority       string     `gorm:"column:priority;default:sedang" json:"priority"`
	Status         string     `gorm:"column:status;default:baru" json:"status"`
	SourceType     *string    `gorm:"column:source_type" json:"source_type"`
	SourceRef      *string    `gorm:"column:source_ref" json:"source_ref"`
	Owner          *string    `gorm:"column:owner" json:"owner"`
	DueDate        *time.Time `gorm:"column:due_date" json:"due_date"`
	ExpectedResult *string    `gorm:"column:expected_result" json:"expected_result"`
	ResolutionNote *string    `gorm:"column:resolution_note" json:"resolution_note"`
	ResolvedAt     *time.Time `gorm:"column:resolved_at" json:"resolved_at"`
	CreatedAt      time.Time  `gorm:"column:created_at;default:now()" json:"created_at"`
}

func (Action) TableName() string { return "actions" }

// ActionEvent memetakan tabel action_events.
type ActionEvent struct {
	ID        string    `gorm:"column:id;primaryKey" json:"id"`
	ActionID  string    `gorm:"column:action_id" json:"action_id"`
	Type      string    `gorm:"column:type" json:"type"`
	FromValue *string   `gorm:"column:from_value" json:"from_value"`
	ToValue   *string   `gorm:"column:to_value" json:"to_value"`
	ByName    *string   `gorm:"column:by_name" json:"by_name"`
	Note      *string   `gorm:"column:note" json:"note"`
	CreatedAt time.Time `gorm:"column:created_at;default:now()" json:"created_at"`
}

func (ActionEvent) TableName() string { return "action_events" }

// Claim memetakan tabel claims.
type Claim struct {
	ID           string          `gorm:"column:id;primaryKey" json:"id"`
	HospitalID   string          `gorm:"column:hospital_id" json:"hospital_id"`
	ClaimNumber  string          `gorm:"column:claim_number" json:"claim_number"`
	SEPNumber    *string         `gorm:"column:sep_number" json:"sep_number"`
	INACBGCode   *string         `gorm:"column:inacbg_code" json:"inacbg_code"`
	CareType     *string         `gorm:"column:care_type" json:"care_type"`
	BilledAmount *float64        `gorm:"column:billed_amount" json:"billed_amount"`
	Status       *string         `gorm:"column:status" json:"status"`
	FlagsJSON    json.RawMessage `gorm:"column:flags_json" json:"flags_json"`
}

func (Claim) TableName() string { return "claims" }

// ClaimAnomaly memetakan tabel claim_anomalies.
type ClaimAnomaly struct {
	ID         string  `gorm:"column:id;primaryKey" json:"id"`
	ClaimID    string  `gorm:"column:claim_id" json:"claim_id"`
	HospitalID string  `gorm:"column:hospital_id" json:"hospital_id"`
	Category   *string `gorm:"column:category" json:"category"`
	Severity   *string `gorm:"column:severity" json:"severity"`
	Title      string  `gorm:"column:title" json:"title"`
	Status     string  `gorm:"column:status;default:baru" json:"status"`
	ReviewedBy *string `gorm:"column:reviewed_by" json:"reviewed_by"`
}

func (ClaimAnomaly) TableName() string { return "claim_anomalies" }

// ClinicalPathway memetakan tabel clinical_pathways.
type ClinicalPathway struct {
	ID                  string   `gorm:"column:id;primaryKey" json:"id"`
	HospitalID          string   `gorm:"column:hospital_id" json:"hospital_id"`
	Diagnosis           string   `gorm:"column:diagnosis" json:"diagnosis"`
	ICD10               *string  `gorm:"column:icd10" json:"icd10"`
	StandardLOS         *float64 `gorm:"column:standard_los" json:"standard_los"`
	AvgAdherence        *float64 `gorm:"column:avg_adherence" json:"avg_adherence"`
	DocumentedDeviation *float64 `gorm:"column:documented_deviation" json:"documented_deviation"`
}

func (ClinicalPathway) TableName() string { return "clinical_pathways" }

// PathwayEpisode memetakan tabel pathway_episodes.
type PathwayEpisode struct {
	ID            string   `gorm:"column:id;primaryKey" json:"id"`
	PathwayID     string   `gorm:"column:pathway_id" json:"pathway_id"`
	EpisodeNumber string   `gorm:"column:episode_number" json:"episode_number"`
	Adherence     *float64 `gorm:"column:adherence" json:"adherence"`
	LOSActual     *float64 `gorm:"column:los_actual" json:"los_actual"`
	Documented    *bool    `gorm:"column:documented" json:"documented"`
}

func (PathwayEpisode) TableName() string { return "pathway_episodes" }
