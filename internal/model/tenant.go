// Kode dihasilkan dari medcredix_erd.sql (SRS Appendix B). Skema asli ada di internal/db/schema.sql.
package model

import (
	"encoding/json"
	"time"
)

// Regulation memetakan tabel regulations.
type Regulation struct {
	ID        string  `gorm:"column:id;primaryKey" json:"id"`
	Code      string  `gorm:"column:code" json:"code"`
	Title     string  `gorm:"column:title" json:"title"`
	Number    *string `gorm:"column:number" json:"number"`
	Authority *string `gorm:"column:authority" json:"authority"`
	Year      *int    `gorm:"column:year" json:"year"`
	Status    string  `gorm:"column:status;default:active" json:"status"`
}

func (Regulation) TableName() string { return "regulations" }

// RegulationVersion memetakan tabel regulation_versions.
type RegulationVersion struct {
	ID           string     `gorm:"column:id;primaryKey" json:"id"`
	RegulationID string     `gorm:"column:regulation_id" json:"regulation_id"`
	Version      string     `gorm:"column:version" json:"version"`
	IssuedAt     *time.Time `gorm:"column:issued_at" json:"issued_at"`
	ImpactLevel  *string    `gorm:"column:impact_level" json:"impact_level"`
}

func (RegulationVersion) TableName() string { return "regulation_versions" }

// Hospital memetakan tabel hospitals.
type Hospital struct {
	ID               string    `gorm:"column:id;primaryKey" json:"id"`
	Name             string    `gorm:"column:name" json:"name"`
	Code             string    `gorm:"column:code" json:"code"`
	HospitalClass    *string   `gorm:"column:hospital_class" json:"hospital_class"`
	Province         *string   `gorm:"column:province" json:"province"`
	City             *string   `gorm:"column:city" json:"city"`
	BedLicensed      int       `gorm:"column:bed_licensed" json:"bed_licensed"`
	BPJSFacilityCode *string   `gorm:"column:bpjs_facility_code" json:"bpjs_facility_code"`
	SatusehatOrgID   *string   `gorm:"column:satusehat_org_id" json:"satusehat_org_id"`
	Tier             *string   `gorm:"column:tier" json:"tier"`
	Onboarding       *string   `gorm:"column:onboarding" json:"onboarding"`
	CreatedAt        time.Time `gorm:"column:created_at;default:now()" json:"created_at"`
}

func (Hospital) TableName() string { return "hospitals" }

// User memetakan tabel users.
type User struct {
	ID           string     `gorm:"column:id;primaryKey" json:"id"`
	HospitalID   *string    `gorm:"column:hospital_id" json:"hospital_id"`
	Email        string     `gorm:"column:email" json:"email"`
	PasswordHash string     `gorm:"column:password_hash" json:"-"`
	Name         string     `gorm:"column:name" json:"name"`
	Title        *string    `gorm:"column:title" json:"title"`
	Role         string     `gorm:"column:role" json:"role"`
	Active       bool       `gorm:"column:active" json:"active"`
	LastLogin    *time.Time `gorm:"column:last_login" json:"last_login"`
}

func (User) TableName() string { return "users" }

// Practitioner memetakan tabel practitioners.
type Practitioner struct {
	ID         string     `gorm:"column:id;primaryKey" json:"id"`
	HospitalID string     `gorm:"column:hospital_id" json:"hospital_id"`
	FullName   string     `gorm:"column:full_name" json:"full_name"`
	Profession *string    `gorm:"column:profession" json:"profession"`
	Specialty  *string    `gorm:"column:specialty" json:"specialty"`
	Department *string    `gorm:"column:department" json:"department"`
	STRNumber  *string    `gorm:"column:str_number" json:"str_number"`
	STRExpiry  *time.Time `gorm:"column:str_expiry" json:"str_expiry"`
	SIPNumber  *string    `gorm:"column:sip_number" json:"sip_number"`
	SIPExpiry  *time.Time `gorm:"column:sip_expiry" json:"sip_expiry"`
	FHIRID     *string    `gorm:"column:fhir_id" json:"fhir_id"`
}

func (Practitioner) TableName() string { return "practitioners" }

// PractitionerCredential memetakan tabel practitioner_credentials.
type PractitionerCredential struct {
	ID             string     `gorm:"column:id;primaryKey" json:"id"`
	PractitionerID string     `gorm:"column:practitioner_id" json:"practitioner_id"`
	HospitalID     string     `gorm:"column:hospital_id" json:"hospital_id"`
	Type           string     `gorm:"column:type" json:"type"`
	Number         *string    `gorm:"column:number" json:"number"`
	IssueDate      *time.Time `gorm:"column:issue_date" json:"issue_date"`
	ExpiryDate     *time.Time `gorm:"column:expiry_date" json:"expiry_date"`
	Status         string     `gorm:"column:status;default:diajukan" json:"status"`
	Version        int        `gorm:"column:version;default:1" json:"version"`
	VerifiedByID   *string    `gorm:"column:verified_by_id" json:"verified_by_id"`
	VerifiedAt     *time.Time `gorm:"column:verified_at" json:"verified_at"`
	ApprovedByID   *string    `gorm:"column:approved_by_id" json:"approved_by_id"`
	ApprovedAt     *time.Time `gorm:"column:approved_at" json:"approved_at"`
}

func (PractitionerCredential) TableName() string { return "practitioner_credentials" }

// CredentialDocument memetakan tabel credential_documents.
type CredentialDocument struct {
	ID           string     `gorm:"column:id;primaryKey" json:"id"`
	CredentialID string     `gorm:"column:credential_id" json:"credential_id"`
	FileName     string     `gorm:"column:file_name" json:"file_name"`
	FileType     *string    `gorm:"column:file_type" json:"file_type"`
	SizeKb       *int       `gorm:"column:size_kb" json:"size_kb"`
	UploadedByID *string    `gorm:"column:uploaded_by_id" json:"uploaded_by_id"`
	Status       *string    `gorm:"column:status" json:"status"`
	StorageKey   *string    `gorm:"column:storage_key" json:"-"`
	UploadedAt   *time.Time `gorm:"column:uploaded_at" json:"uploaded_at"`
}

func (CredentialDocument) TableName() string { return "credential_documents" }

// Requirement memetakan tabel requirements.
type Requirement struct {
	ID               string     `gorm:"column:id;primaryKey" json:"id"`
	HospitalID       string     `gorm:"column:hospital_id" json:"hospital_id"`
	Code             string     `gorm:"column:code" json:"code"`
	Name             string     `gorm:"column:name" json:"name"`
	Category         *string    `gorm:"column:category" json:"category"`
	RegulatorySource *string    `gorm:"column:regulatory_source" json:"regulatory_source"`
	Status           string     `gorm:"column:status;default:belum_dilengkapi" json:"status"`
	DocNumber        *string    `gorm:"column:doc_number" json:"doc_number"`
	IssuedAt         *time.Time `gorm:"column:issued_at" json:"issued_at"`
	DocExpiry        *time.Time `gorm:"column:doc_expiry" json:"doc_expiry"`
	FileName         *string    `gorm:"column:file_name" json:"file_name"`
	FileKey          *string    `gorm:"column:file_key" json:"-"`
	FileType         *string    `gorm:"column:file_type" json:"file_type"`
	FileSize         *int       `gorm:"column:file_size" json:"file_size"`
	Note             *string    `gorm:"column:note" json:"note"`
}

func (Requirement) TableName() string { return "requirements" }

// AuditLog memetakan tabel audit_logs.
type AuditLog struct {
	ID            string          `gorm:"column:id;primaryKey" json:"id"`
	HospitalID    *string         `gorm:"column:hospital_id" json:"hospital_id"`
	ActorName     string          `gorm:"column:actor_name" json:"actor_name"`
	ActorRole     string          `gorm:"column:actor_role" json:"actor_role"`
	Action        string          `gorm:"column:action" json:"action"`
	EntityType    *string         `gorm:"column:entity_type" json:"entity_type"`
	EntityID      *string         `gorm:"column:entity_id" json:"entity_id"`
	Description   *string         `gorm:"column:description" json:"description"`
	BeforeValue   json.RawMessage `gorm:"column:before_value" json:"before_value"`
	AfterValue    json.RawMessage `gorm:"column:after_value" json:"after_value"`
	IP            *string         `gorm:"column:ip" json:"ip"`
	CorrelationID *string         `gorm:"column:correlation_id" json:"correlation_id"`
	CreatedAt     time.Time       `gorm:"column:created_at;default:now()" json:"created_at"`
}

func (AuditLog) TableName() string { return "audit_logs" }

// ComplianceMapping memetakan tabel compliance_mappings.
type ComplianceMapping struct {
	ID           string     `gorm:"column:id;primaryKey" json:"id"`
	RegulationID string     `gorm:"column:regulation_id" json:"regulation_id"`
	HospitalID   string     `gorm:"column:hospital_id" json:"hospital_id"`
	Article      *string    `gorm:"column:article" json:"article"`
	Module       *string    `gorm:"column:module" json:"module"`
	Feature      *string    `gorm:"column:feature" json:"feature"`
	Status       *string    `gorm:"column:status" json:"status"`
	Owner        *string    `gorm:"column:owner" json:"owner"`
	ReviewedAt   *time.Time `gorm:"column:reviewed_at" json:"reviewed_at"`
}

func (ComplianceMapping) TableName() string { return "compliance_mappings" }
