package audit

import (
	"encoding/json"
	"fmt"
	"sort"
	"strings"
	"time"

	"github.com/spkd/medcredix/internal/model"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

// Meta = konteks request (diisi dari middleware).
type Meta struct {
	IP            string
	CorrelationID string
}

// auditable: tabel yang boleh diubah lewat UpdateWithAudit -> kolom pemisah tenant.
// Whitelist ini mencegah SQL injection lewat nama tabel dan kebocoran lintas RS.
var auditable = map[string]string{
	"practitioner_credentials": "hospital_id",
	"practitioners":            "hospital_id",
	"requirements":             "hospital_id",
	"risks":                    "hospital_id",
	"early_warnings":           "hospital_id",
	"alert_rules":              "hospital_id",
	"actions":                  "hospital_id",
	"claims":                   "hospital_id",
	"claim_anomalies":          "hospital_id",
	"data_sources":             "hospital_id",
	"data_quality_issues":      "hospital_id",
	"hospitals":                "id",
}

func ptr(s string) *string {
	if s == "" {
		return nil
	}
	return &s
}

// Log mencatat satu kejadian audit (login, create, akses ditolak, konfigurasi, dst).
// hospitalID = RS yang menjadi target kejadian (bukan selalu RS milik aktor).
func Log(db *gorm.DB, actor model.User, hospitalID, action, entityType, entityID, description string, meta Meta) error {
	entry := model.AuditLog{
		ID:            model.NewID("aud"),
		HospitalID:    ptr(hospitalID),
		ActorName:     actor.Name,
		ActorRole:     actor.Role,
		Action:        action,
		EntityType:    ptr(entityType),
		EntityID:      ptr(entityID),
		Description:   ptr(description),
		IP:            ptr(meta.IP),
		CorrelationID: ptr(meta.CorrelationID),
		CreatedAt:     time.Now(),
	}
	return db.Create(&entry).Error
}

// LogAnonymous untuk kejadian sebelum ada sesi (mis. login gagal) - temuan F-18.
func LogAnonymous(db *gorm.DB, actorName, action, description string, meta Meta) error {
	return Log(db, model.User{Name: actorName, Role: "anonymous"}, "", action, "users", "", description, meta)
}

// UpdateWithAudit: update + snapshot before/after + audit log dalam SATU transaksi.
// - tenantID wajib: baris dari RS lain tidak akan ditemukan (isolasi multi-RS).
// - baris dikunci (FOR UPDATE) agar nilai "before" tidak balapan dengan request lain.
func UpdateWithAudit(db *gorm.DB, actor model.User, tenantID, entityType, entityID string,
	updates map[string]interface{}, meta Meta) error {

	tenantCol, ok := auditable[entityType]
	if !ok {
		return fmt.Errorf("tabel %q tidak terdaftar untuk audit", entityType)
	}
	if tenantID == "" {
		return fmt.Errorf("tenantID wajib diisi")
	}

	return db.Transaction(func(tx *gorm.DB) error {
		scope := func() *gorm.DB {
			return tx.Table(entityType).Where("id = ?", entityID).Where(tenantCol+" = ?", tenantID)
		}

		var before map[string]interface{}
		if err := scope().Clauses(clause.Locking{Strength: "UPDATE"}).Take(&before).Error; err != nil {
			return err // termasuk gorm.ErrRecordNotFound
		}
		if err := scope().Updates(updates).Error; err != nil {
			return err
		}
		var after map[string]interface{}
		if err := scope().Take(&after).Error; err != nil {
			return err
		}

		bBytes, err := json.Marshal(before)
		if err != nil {
			return err
		}
		aBytes, err := json.Marshal(after)
		if err != nil {
			return err
		}

		keys := make([]string, 0, len(updates))
		for k := range updates {
			keys = append(keys, k)
		}
		sort.Strings(keys)
		desc := "Ubah " + entityType + ": " + strings.Join(keys, ", ")

		entry := model.AuditLog{
			ID:            model.NewID("aud"),
			HospitalID:    ptr(tenantID),
			ActorName:     actor.Name,
			ActorRole:     actor.Role,
			Action:        "update",
			EntityType:    &entityType,
			EntityID:      &entityID,
			Description:   &desc,
			BeforeValue:   bBytes,
			AfterValue:    aBytes,
			IP:            ptr(meta.IP),
			CorrelationID: ptr(meta.CorrelationID),
			CreatedAt:     time.Now(),
		}
		return tx.Create(&entry).Error
	})
}