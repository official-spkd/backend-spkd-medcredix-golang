package api

import (
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	"gorm.io/gorm"

	"github.com/spkd/medcredix/internal/audit"
	"github.com/spkd/medcredix/internal/auth"
	"github.com/spkd/medcredix/internal/model"
)

// Helper untuk kontrak API frontend Vue: field camelCase, status berlabel ("Dalam Pengerjaan"),
// sedangkan database menyimpan kode ("dalam_pengerjaan").

type J = map[string]interface{}

// label mengubah kode DB menjadi label tampilan: "dalam_pengerjaan" -> "Dalam Pengerjaan".
func label(code string) string {
	if code == "" {
		return ""
	}
	words := strings.Fields(strings.ReplaceAll(code, "_", " "))
	for i, w := range words {
		words[i] = strings.ToUpper(w[:1]) + w[1:]
	}
	return strings.Join(words, " ")
}

// codeOf mengubah label tampilan menjadi kode DB: "Dalam Pengerjaan" -> "dalam_pengerjaan".
func codeOf(lbl string) string {
	return strings.ReplaceAll(strings.ToLower(strings.TrimSpace(lbl)), " ", "_")
}

func labelP(p *string) interface{} {
	if p == nil {
		return nil
	}
	return label(*p)
}

func str(p *string) string {
	if p == nil {
		return ""
	}
	return *p
}

func strOr(p *string, def string) string {
	if p == nil || *p == "" {
		return def
	}
	return *p
}

func sv(p *string) interface{} {
	if p == nil {
		return nil
	}
	return *p
}

func fv(p *float64) interface{} {
	if p == nil {
		return nil
	}
	return *p
}

func iv(p *int) interface{} {
	if p == nil {
		return nil
	}
	return *p
}

func tv(p *time.Time) interface{} {
	if p == nil {
		return nil
	}
	return *p
}

func ymd(p *time.Time) string {
	if p == nil {
		return "—"
	}
	return p.Format("2006-01-02")
}

// parseDate menerima "2026-10-15" atau RFC3339.
func parseDate(s string) *time.Time {
	s = strings.TrimSpace(s)
	if s == "" {
		return nil
	}
	if t, err := time.Parse("2006-01-02", s); err == nil {
		return &t
	}
	if t, err := time.Parse(time.RFC3339, s); err == nil {
		return &t
	}
	return nil
}

func likeAny(term string, vals ...string) bool {
	term = strings.ToLower(term)
	for _, v := range vals {
		if strings.Contains(strings.ToLower(v), term) {
			return true
		}
	}
	return false
}

// rank level/prioritas/keparahan untuk pengurutan: kritis > tinggi > sedang > rendah.
func rank(code string) int {
	switch strings.ToLower(code) {
	case "kritis":
		return 4
	case "tinggi":
		return 3
	case "sedang":
		return 2
	case "rendah":
		return 1
	}
	return 0
}

func timeAfter(a, b *time.Time) bool {
	if a == nil {
		return false
	}
	if b == nil {
		return true
	}
	return a.After(*b)
}

// ---------- konteks request ----------

// feCtx: user + RS hasil resolve (isolasi tenant). Mengirim respons error sendiri bila gagal.
func feCtx(c *gin.Context, db *gorm.DB) (model.User, string, bool) {
	return tenantID(c, db)
}

func requirePerm(c *gin.Context, db *gorm.DB, user model.User, hid string, p auth.Permission, entity, entityID string) bool {
	if auth.HasPermission(user.Role, p) {
		return true
	}
	_ = audit.Log(db, user, hid, "akses_ditolak", entity, entityID,
		fmt.Sprintf("Aksi ditolak: butuh izin %s", p), metaOf(c))
	fail(c, http.StatusForbidden, fmt.Sprintf("Peran %s tidak memiliki izin %s.", auth.RoleLabels[user.Role], p))
	return false
}

func bindBody(c *gin.Context, dst interface{}) bool {
	if err := c.ShouldBindJSON(dst); err != nil {
		fail(c, http.StatusBadRequest, "Payload tidak valid.")
		return false
	}
	return true
}

func notFound(c *gin.Context, err error, msg string) bool {
	if errors.Is(err, gorm.ErrRecordNotFound) {
		fail(c, http.StatusNotFound, msg)
		return true
	}
	if err != nil {
		fail(c, http.StatusInternalServerError, "Gagal membaca data.")
		return true
	}
	return false
}

func dbFail(c *gin.Context, err error) bool {
	if err != nil {
		fail(c, http.StatusInternalServerError, "Gagal memproses data.")
		return true
	}
	return false
}

// ---------- pemetaan baris DB -> bentuk frontend ----------

func sessionOf(u model.User) J {
	perms := auth.RolePermissions[u.Role]
	if perms == nil {
		perms = []auth.Permission{auth.PermView}
	}
	lbl := auth.RoleLabels[u.Role]
	if lbl == "" {
		lbl = u.Role
	}
	return J{
		"id": u.ID, "name": u.Name, "email": u.Email, "role": u.Role, "roleLabel": lbl,
		"title": sv(u.Title), "hospitalId": sv(u.HospitalID), "permissions": perms,
	}
}

func feHospital(h model.Hospital, bedActive int) J {
	return J{
		"id": h.ID, "name": h.Name, "code": h.Code, "class": sv(h.HospitalClass), "city": sv(h.City),
		"province": sv(h.Province), "bedLicensed": h.BedLicensed, "bedActive": bedActive,
		"bedNotOperating": maxInt(h.BedLicensed-bedActive, 0), "notOperatingNote": nil,
		"bpjsFacilityCode": sv(h.BPJSFacilityCode), "satusehatOrgId": sv(h.SatusehatOrgID),
		"tier": sv(h.Tier), "onboarding": h.Onboarding != nil && *h.Onboarding != "" && *h.Onboarding != "selesai",
		"onboardingStatus": sv(h.Onboarding), "createdAt": h.CreatedAt,
	}
}

func maxInt(a, b int) int {
	if a > b {
		return a
	}
	return b
}

func activeBeds(db *gorm.DB, hid string) int {
	var n int64
	db.Model(&model.BedStat{}).Where("hospital_id = ?", hid).Select("COALESCE(SUM(active),0)").Scan(&n)
	return int(n)
}

func loadHospital(db *gorm.DB, hid string) (J, error) {
	var h model.Hospital
	if err := db.First(&h, "id = ?", hid).Error; err != nil {
		return nil, err
	}
	return feHospital(h, activeBeds(db, hid)), nil
}

func feRisk(r model.Risk) J {
	return J{
		"id": r.ID, "hospitalId": r.HospitalID, "code": r.Code, "category": label(r.Category),
		"level": label(r.Level), "title": r.Title, "description": str(r.Description),
		"businessImpact": str(r.BusinessImpact), "recommendedAction": str(r.Recommendation),
		"suggestedOwner": str(r.SuggestedOwner), "deadline": tv(r.Deadline), "status": label(r.Status),
		"domain": riskDomain(r.Category), "detectedAt": nil, "affectedRecords": "[]", "evidenceJson": "[]",
	}
}

// riskDomain memetakan kategori risiko ke label domain skor (dipakai panel "Mengapa").
func riskDomain(cat string) string {
	switch cat {
	case "kredensial":
		return "Credentialing Readiness"
	case "klaim":
		return "Claim Integrity"
	case "clinical_pathway":
		return "Clinical Pathway"
	case "medikasi":
		return "Medication Management"
	case "interoperabilitas":
		return "Interoperability"
	case "workforce":
		return "Clinical Workforce"
	case "kualitas_data":
		return "Data Trust"
	case "keselamatan_pasien", "keselamatan":
		return "Patient Safety"
	}
	return label(cat)
}

func feWarning(w model.EarlyWarning) J {
	return J{
		"id": w.ID, "hospitalId": w.HospitalID, "code": w.Code, "title": w.Title, "description": w.Title,
		"severity": label(w.Severity), "domain": nil, "thresholdRule": str(w.ThresholdRule),
		"triggerValue": str(w.TriggerValue), "detectedAt": nil, "status": label(w.Status),
		"acknowledgedBy": nil, "acknowledgedAt": nil, "assignedTo": sv(w.AssignedTo), "dueDate": tv(w.DueDate),
	}
}

func feAction(a model.Action) J {
	return J{
		"id": a.ID, "hospitalId": a.HospitalID, "code": a.Code, "title": a.Title, "description": str(a.Description),
		"priority": label(a.Priority), "status": label(a.Status), "sourceType": label(strOr(a.SourceType, "manual")),
		"sourceRef": sv(a.SourceRef), "owner": strOr(a.Owner, "Belum ditugaskan"), "ownerRole": "",
		"dueDate": tv(a.DueDate), "expectedOutcome": sv(a.ExpectedResult), "progressNote": nil,
		"resolutionNote": sv(a.ResolutionNote), "resolvedAt": tv(a.ResolvedAt),
		"createdAt": a.CreatedAt, "updatedAt": a.CreatedAt,
	}
}

var actionEventTypes = map[string]string{"dibuat": "created", "ditugaskan": "assigned", "komentar": "comment"}

func feActionEvent(e model.ActionEvent) J {
	typ := e.Type
	if t, ok := actionEventTypes[typ]; ok {
		typ = t
	}
	from, to := sv(e.FromValue), sv(e.ToValue)
	if typ == "status" || typ == "created" {
		from, to = labelP(e.FromValue), labelP(e.ToValue)
	}
	return J{
		"id": e.ID, "actionId": e.ActionID, "type": typ, "fromValue": from, "toValue": to,
		"byName": sv(e.ByName), "note": sv(e.Note), "createdAt": e.CreatedAt,
	}
}

// practitionerStatus diturunkan dari kredensial (kolom status praktisi tidak ada di skema).
func practitionerStatus(creds []model.PractitionerCredential) string {
	for _, c := range creds {
		if c.Status == "dicabut" {
			return "Nonaktif"
		}
	}
	for _, c := range creds {
		if c.Status == "ditangguhkan" {
			return "Ditangguhkan"
		}
	}
	return "Aktif"
}

func fePractitioner(p model.Practitioner, status string) J {
	return J{
		"id": p.ID, "hospitalId": p.HospitalID, "fullName": p.FullName, "nikMasked": nil,
		"profession": strOr(p.Profession, "—"), "specialty": sv(p.Specialty), "subSpecialty": nil,
		"department": strOr(p.Department, "—"), "employmentType": nil, "status": status,
		"strNumber": sv(p.STRNumber), "strExpiry": tv(p.STRExpiry), "sipNumber": sv(p.SIPNumber),
		"sipExpiry": tv(p.SIPExpiry), "phone": nil, "joinDate": nil, "fhirId": sv(p.FHIRID), "createdAt": nil,
	}
}

func feCredential(c model.PractitionerCredential, practitionerName string) J {
	name := c.Type
	if practitionerName != "" {
		name = c.Type + " — " + practitionerName
	}
	return J{
		"id": c.ID, "hospitalId": c.HospitalID, "practitionerId": c.PractitionerID, "type": c.Type,
		"name": name, "number": sv(c.Number), "issueDate": tv(c.IssueDate), "expiryDate": tv(c.ExpiryDate),
		"status": label(c.Status), "version": c.Version, "verifiedById": sv(c.VerifiedByID),
		"verifiedAt": tv(c.VerifiedAt), "approvedById": sv(c.ApprovedByID), "approvedAt": tv(c.ApprovedAt),
		"notes": nil, "createdAt": nil, "updatedAt": tv(latest(c.ApprovedAt, c.VerifiedAt)),
	}
}

func latest(a, b *time.Time) *time.Time {
	if timeAfter(a, b) {
		return a
	}
	return b
}

func feDocument(d model.CredentialDocument, hid string) J {
	return J{
		"id": d.ID, "hospitalId": hid, "credentialId": d.CredentialID, "fileName": d.FileName,
		"fileType": sv(d.FileType), "sizeKb": iv(d.SizeKb), "uploadedById": sv(d.UploadedByID),
		"uploadedAt": nil, "status": labelP(d.Status), "notes": nil,
	}
}

func feRequirement(r model.Requirement) J {
	return J{
		"id": r.ID, "hospitalId": r.HospitalID, "code": r.Code, "name": r.Name, "category": sv(r.Category),
		"regulatorySource": sv(r.RegulatorySource), "version": nil, "required": r.Code <= "P3",
		"docType": nil, "validationRule": nil, "evidenceRequired": true, "approvalRequired": true,
		"responsibleRole": nil, "status": label(r.Status), "docNumber": sv(r.DocNumber),
		"issuedAt": tv(r.IssuedAt), "docExpiry": tv(r.DocExpiry), "fileName": sv(r.FileName),
		"notes": sv(r.Note), "updatedAt": nil,
	}
}

// flagsJSON: kunci flag klaim dibuat Title Case ("diagnosis" -> "Diagnosis") seperti frontend.
func flagsJSON(raw json.RawMessage) string {
	var m map[string]string
	if len(raw) == 0 || json.Unmarshal(raw, &m) != nil {
		return "{}"
	}
	out := map[string]string{}
	for k, v := range m {
		out[label(strings.ToLower(k))] = strings.ToLower(v)
	}
	b, _ := json.Marshal(out)
	return string(b)
}

func feClaim(c model.Claim) J {
	billed := 0.0
	if c.BilledAmount != nil {
		billed = *c.BilledAmount
	}
	return J{
		"id": c.ID, "hospitalId": c.HospitalID, "claimNumber": c.ClaimNumber, "sepNumber": sv(c.SEPNumber),
		"patientInit": nil, "inacbgCode": strOr(c.INACBGCode, "—"), "inacbgDesc": "",
		"careType": labelP(c.CareType), "admissionDate": nil, "dischargeDate": nil, "billedAmount": billed,
		"status": labelP(c.Status), "flagsJson": flagsJSON(c.FlagsJSON), "createdAt": nil,
	}
}

func feAnomaly(a model.ClaimAnomaly, claimNumber string) J {
	return J{
		"id": a.ID, "claimId": a.ClaimID, "hospitalId": a.HospitalID, "category": labelP(a.Category),
		"severity": labelP(a.Severity), "title": a.Title, "description": a.Title,
		"evidenceJson": `{"fhir":[],"source":[]}`, "recommendation": "Verifikasi manual terhadap dokumen klaim.",
		"status": label(a.Status), "reviewedBy": sv(a.ReviewedBy), "detectedAt": nil, "claimNumber": claimNumber,
	}
}

func fePathway(p model.ClinicalPathway, episodes []model.PathwayEpisode) J {
	n, dev := 0, 0
	for _, e := range episodes {
		if e.PathwayID == p.ID {
			n++
			if e.Documented != nil {
				dev++
			}
		}
	}
	return J{
		"id": p.ID, "hospitalId": p.HospitalID, "diagnosis": p.Diagnosis, "icd10": sv(p.ICD10),
		"standardLos": fv(p.StandardLOS), "targetAdherence": 85, "episodeCount": n,
		"avgAdherence": fv(p.AvgAdherence), "medicationAdherence": nil, "labAdherence": nil,
		"procedureAdherence": nil, "losAdherence": nil, "deviationCount": dev,
		"documentedDeviation": fv(p.DocumentedDeviation), "costRatio": nil, "status": "Aktif",
	}
}

func feEpisode(e model.PathwayEpisode, hid string, std *float64) J {
	devType := "Tidak Ada"
	if e.Documented != nil {
		devType = "Deviasi"
	}
	documented := false
	if e.Documented != nil {
		documented = *e.Documented
	}
	return J{
		"id": e.ID, "pathwayId": e.PathwayID, "hospitalId": hid, "episodeNumber": e.EpisodeNumber,
		"patientInit": nil, "dpjp": nil, "adherence": fv(e.Adherence), "losActual": fv(e.LOSActual),
		"losStandard": fv(std), "deviationType": devType, "deviationReason": nil, "documented": documented,
		"costRatio": nil, "period": nil,
	}
}

var monthsID = []string{"Jan", "Feb", "Mar", "Apr", "Mei", "Jun", "Jul", "Agu", "Sep", "Okt", "Nov", "Des"}

func monthLabel(m string) string {
	if len(m) == 7 {
		if i, err := strconv.Atoi(m[5:]); err == nil && i >= 1 && i <= 12 {
			return monthsID[i-1]
		}
	}
	return m
}

func feUtil(u model.UtilizationStat) J {
	return J{
		"id": u.ID, "hospitalId": u.HospitalID, "month": u.Month, "monthLabel": monthLabel(u.Month),
		"alos": fv(u.ALOS), "readmission": fv(u.Readmission), "bor": fv(u.BOR), "score": 0,
	}
}

func feComplaint(c model.ComplaintStat) J {
	return J{
		"id": c.ID, "hospitalId": c.HospitalID, "month": c.Month, "total": iv(c.Total),
		"antrian": nil, "pelayanan": nil, "obat": nil, "administrasi": nil, "fasilitas": nil, "lainnya": nil,
		"satisfaction": fv(c.Satisfaction), "per1000": nil, "peerAvg": fv(c.PeerAvg),
	}
}

func feEvidence(e model.EvidenceItem) J {
	title := strOr(e.Metric, "Bukti data")
	return J{
		"id": e.ID, "hospitalId": e.HospitalID, "refType": label(e.RefType), "refId": sv(e.RefID),
		"title": title, "metric": strOr(e.Metric, "—"), "value": sv(e.MetricValue), "unit": nil,
		"sourceSystem": strOr(e.SourceSystem, "—"), "fhirResource": sv(e.FHIRResource),
		"recordCount": iv(e.RecordCount), "lastSyncAt": nil,
		"validationStatus": strOr(labelStr(e.ValidationStatus), "Belum Divalidasi"), "timestamp": nil,
		"demoData": false,
	}
}

func labelStr(p *string) *string {
	if p == nil {
		return nil
	}
	s := label(*p)
	return &s
}

func feAudit(l model.AuditLog) J {
	return J{
		"id": l.ID, "hospitalId": sv(l.HospitalID), "actorName": l.ActorName, "actorRole": l.ActorRole,
		"action": l.Action, "entityType": strOr(l.EntityType, "—"), "entityId": strOr(l.EntityID, "-"),
		"description": str(l.Description), "before": rawStr(l.BeforeValue), "after": rawStr(l.AfterValue),
		"ip": sv(l.IP), "correlationId": sv(l.CorrelationID), "createdAt": l.CreatedAt,
	}
}

func rawStr(r json.RawMessage) interface{} {
	if len(r) == 0 || string(r) == "null" {
		return nil
	}
	return string(r)
}

func feAlertRule(r model.AlertRule) J {
	return J{
		"id": r.ID, "hospitalId": r.HospitalID, "name": r.Name, "domain": sv(r.Domain), "metric": sv(r.Metric),
		"condition": str(r.RuleCondition), "severity": labelP(r.Severity), "channels": strOr(r.Channels, "in-app"),
		"active": r.Active, "triggeredCount": 0, "lastTriggeredAt": nil,
	}
}

func feNotification(n model.Notification) J {
	return J{
		"id": n.ID, "hospitalId": sv(n.HospitalID), "userId": n.UserID, "title": n.Title, "body": "",
		"severity": labelP(n.Severity), "channel": sv(n.Channel), "read": n.IsRead, "link": nil, "createdAt": nil,
	}
}

func feDataSource(s model.DataSource) J {
	return J{
		"id": s.ID, "hospitalId": s.HospitalID, "name": s.Name, "type": sv(s.Type), "status": labelP(s.Status),
		"authMethod": sv(s.AuthMethod), "configSummary": nil, "successRate": fv(s.SuccessRate),
		"avgLatencyMs": nil, "lastSyncAt": tv(s.LastSyncAt), "fhirResources": 24,
		"coveragePct": fv(s.CoveragePct), "uptime30d": nil, "notes": nil,
	}
}

func feJob(j model.IntegrationJob, hid string) J {
	return J{
		"id": j.ID, "hospitalId": hid, "dataSourceId": j.DataSourceID, "jobType": sv(j.JobType),
		"startedAt": nil, "finishedAt": nil, "status": labelP(j.Status), "recordsPulled": iv(j.RecordsPulled),
		"durationMs": iv(j.DurationMs), "message": nil,
	}
}

func yesNo(b *bool, yes, no string) interface{} {
	if b == nil {
		return nil
	}
	if *b {
		return yes
	}
	return no
}

func feFacility(f model.Facility) J {
	return J{
		"id": f.ID, "hospitalId": f.HospitalID, "name": f.Name, "category": strOr(f.Category, "Lainnya"),
		"availability": yesNo(f.Availability, "Ada", "Tidak Ada"),
		"functional": yesNo(f.Functional, "Berfungsi", "Tidak Berfungsi"),
		"feasible": nil, "calibration": labelP(f.Calibration), "suitable": nil, "standardRef": nil, "notes": nil,
	}
}

func feBed(b model.BedStat) J {
	return J{
		"id": b.ID, "hospitalId": b.HospitalID, "ward": sv(b.Ward), "bedClass": sv(b.BedClass),
		"total": iv(b.Total), "active": iv(b.Active), "usable": iv(b.Active), "nonOperatingReason": nil,
	}
}

func feDQIssue(i model.DataQualityIssue) J {
	return J{
		"id": i.ID, "hospitalId": i.HospitalID, "dimension": sv(i.Dimension), "title": i.Title,
		"description": i.Title, "affectedRecords": nil, "sourceSystem": nil, "severity": labelP(i.Severity),
		"anomaly": i.Anomaly, "status": label(i.Status), "detectedAt": nil, "resolvedAt": nil,
	}
}

func feTrustFactor(t model.TrustFactor) J {
	w := t.Weight
	if w > 1 {
		w = w / 100
	}
	return J{"id": t.ID, "hospitalId": t.HospitalID, "factor": t.Factor, "score": t.Score, "weight": w, "notes": nil}
}

func userBriefs(db *gorm.DB, hid string) []J {
	var users []model.User
	db.Where("hospital_id = ?", hid).Order("name").Find(&users)
	out := make([]J, 0, len(users))
	for _, u := range users {
		out = append(out, J{"id": u.ID, "name": u.Name, "role": u.Role})
	}
	return out
}

// sortJ mengurutkan slice J secara stabil dengan pembanding bebas.
func sortJ(rows []J, less func(a, b J) bool) {
	sort.SliceStable(rows, func(i, j int) bool { return less(rows[i], rows[j]) })
}
