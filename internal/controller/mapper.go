package controller

import (
	"encoding/json"
	"math"
	"strings"

	"gorm.io/gorm"

	"github.com/spkd/medcredix/internal/auth"
	"github.com/spkd/medcredix/internal/model"
)

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
		"province": sv(h.Province), "district": sv(h.District), "village": sv(h.Village), "bedLicensed": h.BedLicensed, "bedActive": bedActive,
		"bedNotOperating": maxInt(h.BedLicensed-bedActive, 0), "notOperatingNote": nil,
		"bpjsFacilityCode": sv(h.BPJSFacilityCode), "satusehatOrgId": sv(h.SatusehatOrgID),
		"tier": sv(h.Tier), "onboarding": h.Onboarding != nil && *h.Onboarding != "" && *h.Onboarding != "selesai",
		"onboardingStatus": sv(h.Onboarding), "createdAt": h.CreatedAt,
	}
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

// viewOnlyRoles: peran yang hanya melihat, jadi tidak bisa menjadi penanggung jawab aksi.
var viewOnlyRoles = []string{"auditor", "viewer"}

// userBriefs: daftar calon penanggung jawab aksi di RS ini.
// Hanya akun aktif milik RS tersebut, tanpa Auditor dan Viewer (peran lihat saja).
func userBriefs(db *gorm.DB, hid string) []J {
	var users []model.User
	db.Where("hospital_id = ? AND active = ? AND role NOT IN ?", hid, true, viewOnlyRoles).Order("name").Find(&users)
	out := make([]J, 0, len(users))
	for _, u := range users {
		out = append(out, J{"id": u.ID, "name": u.Name, "role": u.Role})
	}
	return out
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
		"triggerValue": str(w.TriggerValue), "detectedAt": tv(w.DetectedAt), "status": label(w.Status),
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

func feDocument(d model.CredentialDocument, hid string) J {
	return J{
		"id": d.ID, "hospitalId": hid, "credentialId": d.CredentialID, "fileName": d.FileName,
		"fileType": sv(d.FileType), "sizeKb": iv(d.SizeKb), "uploadedById": sv(d.UploadedByID),
		"uploadedAt": tv(d.UploadedAt), "status": labelP(d.Status), "notes": nil,
		"hasFile": d.StorageKey != nil && *d.StorageKey != "",
	}
}

func feRequirement(r model.Requirement) J {
	return J{
		"id": r.ID, "hospitalId": r.HospitalID, "code": r.Code, "name": r.Name, "category": sv(r.Category),
		"regulatorySource": sv(r.RegulatorySource), "version": nil, "required": r.Code <= "P3",
		"docType": nil, "validationRule": nil, "evidenceRequired": true, "approvalRequired": true,
		"responsibleRole": nil, "status": label(r.Status), "docNumber": sv(r.DocNumber),
		"issuedAt": tv(r.IssuedAt), "docExpiry": tv(r.DocExpiry), "fileName": sv(r.FileName),
		"hasFile": r.FileKey != nil && *r.FileKey != "", "fileType": sv(r.FileType), "fileSize": iv(r.FileSize),
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
	losN, losOK := 0, 0
	for _, e := range episodes {
		if e.PathwayID == p.ID {
			n++
			if e.Documented != nil {
				dev++
			}
			// Kepatuhan LOS bisa dihitung dari data yang ada: episode dengan LOS aktual ≤ LOS standar.
			if e.LOSActual != nil && p.StandardLOS != nil {
				losN++
				if *e.LOSActual <= *p.StandardLOS {
					losOK++
				}
			}
		}
	}
	// Kepatuhan obat, lab, prosedur, dan cost ratio belum ada sumber datanya (butuh integrasi
	// SIMRS: e-resep, LIS, order tindakan, billing), jadi dikirim nil dan FE menampilkan "Belum tersedia".
	var losAdherence interface{}
	if losN > 0 {
		losAdherence = math.Round(float64(losOK)/float64(losN)*1000) / 10
	}
	return J{
		"id": p.ID, "hospitalId": p.HospitalID, "diagnosis": p.Diagnosis, "icd10": sv(p.ICD10),
		"standardLos": fv(p.StandardLOS), "targetAdherence": 85, "episodeCount": n,
		"avgAdherence": fv(p.AvgAdherence), "medicationAdherence": nil, "labAdherence": nil,
		"procedureAdherence": nil, "losAdherence": losAdherence, "deviationCount": dev,
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

func feUtil(u model.UtilizationStat) J {
	return J{
		"id": u.ID, "hospitalId": u.HospitalID, "month": u.Month, "monthLabel": monthLabel(u.Month),
		"alos": fv(u.ALOS), "readmission": fv(u.Readmission), "bor": fv(u.BOR), "score": 0,
	}
}

func feComplaint(c model.ComplaintStat) J {
	return J{
		"id": c.ID, "hospitalId": c.HospitalID, "month": c.Month, "total": iv(c.Total),
		"antrian": iv(c.Antrian), "pelayanan": iv(c.Pelayanan), "obat": iv(c.Obat),
		"administrasi": iv(c.Administrasi), "fasilitas": iv(c.Fasilitas), "lainnya": iv(c.Lainnya),
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

func feAudit(l model.AuditLog) J {
	return J{
		"id": l.ID, "hospitalId": sv(l.HospitalID), "actorName": l.ActorName, "actorRole": l.ActorRole,
		"action": l.Action, "entityType": strOr(l.EntityType, "—"), "entityId": strOr(l.EntityID, "-"),
		"description": str(l.Description), "before": rawStr(l.BeforeValue), "after": rawStr(l.AfterValue),
		"ip": sv(l.IP), "correlationId": sv(l.CorrelationID), "createdAt": l.CreatedAt,
	}
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
		"severity": labelP(n.Severity), "channel": sv(n.Channel), "read": n.IsRead, "link": notificationLink(n.Title), "createdAt": nil,
	}
}

// notificationLink: halaman tujuan saat notifikasi lonceng diklik, ditentukan dari isi judulnya.
// Tabel notifications belum punya kolom tautan, jadi dicocokkan per kata kunci (urutan = prioritas).
var notificationRoutes = []struct {
	keywords []string
	link     string
}{
	{[]string{"menunggu verifikasi"}, "#/cred-verification"},
	{[]string{"kredensial", "sip ", "str ", "kedaluwarsa"}, "#/cred-credentials"},
	{[]string{"dokumen"}, "#/cred-documents"},
	{[]string{"anomali"}, "#/claim-anomalies"},
	{[]string{"ditugaskan", "penugasan", "tugas", "tindakan", "aksi"}, "#/actions"},
	{[]string{"akreditasi", "kars", "regulasi"}, "#/compliance"},
	{[]string{"sinkronisasi", "integrasi", "p-care", "simrs", "fhir"}, "#/data-sources"},
	{[]string{"risiko"}, "#/risk-radar"},
}

func notificationLink(title string) string {
	t := strings.ToLower(title) + " "
	for _, r := range notificationRoutes {
		for _, k := range r.keywords {
			if strings.Contains(t, k) {
				return r.link
			}
		}
	}
	// Selain itu notifikasi berasal dari aturan ambang (mis. "Sinyal klaim naik 18%"): buka Early Warnings.
	return "#/early-warnings"
}

func feDataSource(s model.DataSource) J {
	return J{
		"id": s.ID, "hospitalId": s.HospitalID, "name": s.Name, "type": sv(s.Type), "status": labelP(s.Status),
		"authMethod": sv(s.AuthMethod), "configSummary": sv(s.ConfigSummary), "successRate": fv(s.SuccessRate),
		"avgLatencyMs": iv(s.AvgLatencyMs), "lastSyncAt": tv(s.LastSyncAt), "fhirResources": iv(s.FHIRResources),
		"coveragePct": fv(s.CoveragePct), "uptime30d": fv(s.Uptime30d), "notes": sv(s.Notes),
	}
}

func feJob(j model.IntegrationJob, hid string) J {
	return J{
		"id": j.ID, "hospitalId": hid, "dataSourceId": j.DataSourceID, "jobType": sv(j.JobType),
		"startedAt": nil, "finishedAt": nil, "status": labelP(j.Status), "recordsPulled": iv(j.RecordsPulled),
		"durationMs": iv(j.DurationMs), "message": nil,
	}
}

func feFacility(f model.Facility) J {
	// Laik pakai & kesesuaian belum punya kolom sendiri, jadi diturunkan dari ada/berfungsi.
	avail := f.Availability != nil && *f.Availability
	fn := f.Functional != nil && *f.Functional
	feasible, suitable := "Tidak Laik", "Kurang"
	switch {
	case avail && fn:
		feasible, suitable = "Laik Pakai", "Sesuai Kebutuhan"
		if f.Category != nil && strings.Contains(strings.ToLower(*f.Category), "opsional") {
			suitable = "Melebihi Kebutuhan"
		}
	case avail:
		feasible, suitable = "Perlu Perbaikan", "Sesuai Kebutuhan"
	}
	return J{
		"id": f.ID, "hospitalId": f.HospitalID, "name": f.Name, "category": strOr(f.Category, "Lainnya"),
		"availability": yesNo(f.Availability, "Ada", "Tidak Ada"),
		"functional":   yesNo(f.Functional, "Berfungsi", "Tidak Berfungsi"),
		"feasible":     feasible, "calibration": labelP(f.Calibration), "suitable": suitable, "standardRef": nil, "notes": nil,
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
