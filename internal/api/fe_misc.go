package api

import (
	"net/http"
	"strings"

	"github.com/gin-gonic/gin"
	"gorm.io/gorm"

	"github.com/spkd/medcredix/internal/audit"
	"github.com/spkd/medcredix/internal/auth"
	"github.com/spkd/medcredix/internal/model"
)

// ---------- Klaim BPJS ----------

func claimsGetHandler(db *gorm.DB) gin.HandlerFunc {
	return func(c *gin.Context) {
		_, hid, good := feCtx(c, db)
		if !good {
			return
		}
		if id := c.Query("id"); id != "" {
			var cl model.Claim
			if notFound(c, db.Where("id = ? AND hospital_id = ?", id, hid).First(&cl).Error, "Klaim tidak ditemukan.") {
				return
			}
			var an []model.ClaimAnomaly
			db.Where("claim_id = ? AND hospital_id = ?", id, hid).Find(&an)
			anomalies := make([]J, 0, len(an))
			for _, a := range an {
				anomalies = append(anomalies, feAnomaly(a, cl.ClaimNumber))
			}
			var ev []model.EvidenceItem
			db.Where("hospital_id = ? AND ref_id = ?", hid, cl.ClaimNumber).Find(&ev)
			evidence := make([]J, 0, len(ev))
			for _, e := range ev {
				evidence = append(evidence, feEvidence(e))
			}
			ok(c, J{"claim": feClaim(cl), "anomalies": anomalies, "evidence": evidence})
			return
		}

		q := db.Where("hospital_id = ?", hid)
		if v := c.Query("status"); v != "" {
			q = q.Where("status = ?", codeOf(v))
		}
		if v := c.Query("careType"); v != "" {
			q = q.Where("LOWER(REPLACE(care_type, '_', ' ')) = ?", strings.ToLower(v))
		}
		if s := c.Query("q"); s != "" {
			like := "%" + s + "%"
			q = q.Where("claim_number ILIKE ? OR sep_number ILIKE ? OR inacbg_code ILIKE ?", like, like, like)
		}
		var rows []model.Claim
		if dbFail(c, q.Order("claim_number DESC").Find(&rows).Error) {
			return
		}
		claims := make([]J, 0, len(rows))
		for _, cl := range rows {
			claims = append(claims, feClaim(cl))
		}
		var all []model.Claim
		db.Select("id, claim_number").Where("hospital_id = ?", hid).Find(&all)
		num := map[string]string{}
		for _, cl := range all {
			num[cl.ID] = cl.ClaimNumber
		}
		var an []model.ClaimAnomaly
		db.Where("hospital_id = ?", hid).Find(&an)
		anomalies := make([]J, 0, len(an))
		for _, a := range an {
			anomalies = append(anomalies, feAnomaly(a, strOr(ptrOrNil(num[a.ClaimID]), "—")))
		}
		// urut: status naik, severity turun (sama dengan demo)
		sortJ(anomalies, func(a, b J) bool {
			if a["status"] != b["status"] {
				return numStr(a["status"]) < numStr(b["status"])
			}
			return rank(codeOf(numStr(a["severity"]))) > rank(codeOf(numStr(b["severity"])))
		})
		_, intel, err := computeFE(db, hid)
		if err != nil {
			intelFail(c, hid, err)
			return
		}
		ok(c, J{"claims": claims, "anomalies": anomalies, "ccs": intel["ccs"], "total": len(claims)})
	}
}

var anomalyStatus = map[string]string{"review": "ditinjau", "confirm": "terkonfirmasi_bermasalah", "clarify": "diklarifikasi"}

var claimStatuses = map[string]bool{"diverifikasi": true, "disetujui": true, "ditunda": true, "ditolak": true}

func claimsPatchHandler(db *gorm.DB) gin.HandlerFunc {
	return func(c *gin.Context) {
		user, hid, good := feCtx(c, db)
		if !good {
			return
		}
		var body struct {
			AnomalyID string `json:"anomalyId"`
			ClaimID   string `json:"claimId"`
			Action    string `json:"action"`
			Status    string `json:"status"`
		}
		if !bindBody(c, &body) {
			return
		}
		switch {
		case body.AnomalyID != "":
			var a model.ClaimAnomaly
			if notFound(c, db.Where("id = ? AND hospital_id = ?", body.AnomalyID, hid).First(&a).Error, "Anomali tidak ditemukan.") {
				return
			}
			if !requirePerm(c, db, user, hid, auth.PermEdit, "claim_anomalies", a.ID) {
				return
			}
			st, known := anomalyStatus[body.Action]
			if !known {
				fail(c, http.StatusBadRequest, "Aksi tidak dikenal.")
				return
			}
			upd := map[string]interface{}{"status": st, "reviewed_by": user.Name}
			if dbFail(c, audit.UpdateWithAudit(db, user, hid, "claim_anomalies", a.ID, upd, metaOf(c))) {
				return
			}
			db.First(&a, "id = ?", a.ID)
			var cl model.Claim
			db.Select("claim_number").First(&cl, "id = ?", a.ClaimID)
			ok(c, feAnomaly(a, cl.ClaimNumber))
		case body.ClaimID != "":
			var cl model.Claim
			if notFound(c, db.Where("id = ? AND hospital_id = ?", body.ClaimID, hid).First(&cl).Error, "Klaim tidak ditemukan.") {
				return
			}
			if !requirePerm(c, db, user, hid, auth.PermEdit, "claims", cl.ID) {
				return
			}
			st := codeOf(body.Status)
			if !claimStatuses[st] {
				fail(c, http.StatusBadRequest, "Status klaim tidak dikenal.")
				return
			}
			if dbFail(c, audit.UpdateWithAudit(db, user, hid, "claims", cl.ID, map[string]interface{}{"status": st}, metaOf(c))) {
				return
			}
			db.First(&cl, "id = ?", cl.ID)
			ok(c, feClaim(cl))
		default:
			fail(c, http.StatusBadRequest, "Payload tidak valid.")
		}
	}
}

// ---------- Klinis: pathway, utilisasi, keselamatan ----------

func clinicalGetHandler(db *gorm.DB) gin.HandlerFunc {
	return func(c *gin.Context) {
		_, hid, good := feCtx(c, db)
		if !good {
			return
		}
		in, intel, err := computeFE(db, hid)
		if err != nil {
			intelFail(c, hid, err)
			return
		}
		var pws []model.ClinicalPathway
		db.Where("hospital_id = ?", hid).Order("avg_adherence ASC NULLS FIRST").Find(&pws)
		pwIDs := make([]string, 0, len(pws))
		stdLOS := map[string]*float64{}
		for _, p := range pws {
			pwIDs = append(pwIDs, p.ID)
			stdLOS[p.ID] = p.StandardLOS
		}
		var eps []model.PathwayEpisode
		if len(pwIDs) > 0 {
			db.Where("pathway_id IN ?", pwIDs).Order("adherence ASC NULLS FIRST").Find(&eps)
		}
		pathways := make([]J, 0, len(pws))
		for _, p := range pws {
			pathways = append(pathways, fePathway(p, eps))
		}

		typ := c.DefaultQuery("type", "all")
		if typ == "pathway" {
			episodes := make([]J, 0, len(eps))
			for _, e := range eps {
				episodes = append(episodes, feEpisode(e, hid, stdLOS[e.PathwayID]))
			}
			ok(c, J{"pathways": pathways, "episodes": episodes, "cpcs": intel["cpcs"],
				"coverage": J{"total": in.Pathway.CoverageTarget, "covered": in.Pathway.Covered}})
			return
		}

		var us []model.UtilizationStat
		db.Where("hospital_id = ?", hid).Order("month").Find(&us)
		utils := make([]J, 0, len(us))
		for _, u := range us {
			utils = append(utils, feUtil(u))
		}
		var cs []model.ComplaintStat
		db.Where("hospital_id = ?", hid).Order("month").Find(&cs)
		complaints := make([]J, 0, len(cs))
		for _, x := range cs {
			complaints = append(complaints, feComplaint(x))
		}

		switch typ {
		case "utilization":
			ok(c, J{"utilization": utils, "complaints": complaints, "ur": intel["ur"]})
		case "safety":
			var dom interface{}
			for _, d := range intel["domains"].([]J) {
				if d["key"] == "safety" {
					dom = d
				}
			}
			ok(c, J{"safety": intel["safety"], "domain": dom})
		default:
			ok(c, J{"pathways": pathways, "utilization": utils, "complaints": complaints,
				"cpcs": intel["cpcs"], "ur": intel["ur"], "safety": intel["safety"]})
		}
	}
}

// ---------- Kepatuhan regulasi ----------

func regStatus(s string) string {
	if s == "active" {
		return "Berlaku"
	}
	return label(s)
}

func complianceGetHandler(db *gorm.DB) gin.HandlerFunc {
	return func(c *gin.Context) {
		_, hid, good := feCtx(c, db)
		if !good {
			return
		}
		var regs []model.Regulation
		db.Order("year DESC NULLS LAST, code").Find(&regs)
		var vers []model.RegulationVersion
		db.Order("issued_at").Find(&vers)
		byReg := map[string][]J{}
		for _, v := range vers {
			byReg[v.RegulationID] = append(byReg[v.RegulationID], J{
				"id": v.ID, "regulationId": v.RegulationID, "version": v.Version, "issuedAt": tv(v.IssuedAt),
				"status": nil, "changeSummary": nil, "impactLevel": labelP(v.ImpactLevel), "supersededBy": nil,
			})
		}
		regMap := map[string]model.Regulation{}
		regulations := make([]J, 0, len(regs))
		for _, r := range regs {
			regMap[r.ID] = r
			vs := byReg[r.ID]
			if vs == nil {
				vs = []J{}
			}
			regulations = append(regulations, J{
				"id": r.ID, "code": r.Code, "title": r.Title, "number": str(r.Number), "authority": sv(r.Authority),
				"year": iv(r.Year), "category": nil, "status": regStatus(r.Status), "summary": nil,
				"source": nil, "effectiveAt": nil, "versions": vs,
			})
		}

		var ms []model.ComplianceMapping
		db.Where("hospital_id = ?", hid).Order("status, article").Find(&ms)
		count := map[string]int{}
		mappings := make([]J, 0, len(ms))
		for _, m := range ms {
			st := str(m.Status)
			count[st]++
			var reg interface{}
			if r, has := regMap[m.RegulationID]; has {
				reg = J{"code": r.Code, "title": r.Title, "number": str(r.Number), "category": nil}
			}
			mappings = append(mappings, J{
				"id": m.ID, "regulationId": m.RegulationID, "hospitalId": m.HospitalID, "article": str(m.Article),
				"requirement": strOr(m.Feature, "—"), "module": str(m.Module), "feature": sv(m.Feature),
				"dataField": nil, "validation": nil, "evidenceRef": nil, "auditRequired": true,
				"owner": str(m.Owner), "status": label(st), "lastReviewedAt": tv(m.ReviewedAt), "reviewer": nil,
				"regulation": reg,
			})
		}
		ok(c, J{
			"regulations": regulations, "mappings": mappings,
			"summary": J{"patuh": count["patuh"], "sebagian": count["sebagian"], "belum": count["belum"],
				"ditinjau": count["sedang_ditinjau"]},
			"total": len(mappings),
		})
	}
}

// ---------- Evidence ----------

func evidenceGetHandler(db *gorm.DB) gin.HandlerFunc {
	return func(c *gin.Context) {
		_, hid, good := feCtx(c, db)
		if !good {
			return
		}
		q := db.Where("hospital_id = ?", hid)
		if v := c.Query("refType"); v != "" {
			q = q.Where("ref_type = ?", codeOf(v))
		}
		if v := c.Query("refId"); v != "" {
			q = q.Where("ref_id = ?", v)
		}
		if s := c.Query("q"); s != "" {
			like := "%" + s + "%"
			q = q.Where("metric ILIKE ? OR source_system ILIKE ? OR ref_id ILIKE ?", like, like, like)
		}
		var rows []model.EvidenceItem
		if dbFail(c, q.Order("ref_type, id").Find(&rows).Error) {
			return
		}
		items := make([]J, 0, len(rows))
		byType := map[string]int{}
		for _, e := range rows {
			j := feEvidence(e)
			items = append(items, j)
			byType[j["refType"].(string)]++
		}
		ok(c, J{"items": items, "byType": byType, "total": len(items)})
	}
}

// ---------- Audit trail ----------

func auditGetHandler(db *gorm.DB) gin.HandlerFunc {
	return func(c *gin.Context) {
		user, hid, good := feCtx(c, db)
		if !good {
			return
		}
		q := db.Model(&model.AuditLog{})
		if user.Role != "super_admin" {
			q = q.Where("hospital_id = ? OR (hospital_id IS NULL AND actor_role IN ?)", hid, []string{"super_admin", "system"})
		}
		if v := c.Query("action"); v != "" {
			q = q.Where("action = ?", v)
		}
		if v := c.Query("entityType"); v != "" {
			q = q.Where("entity_type = ?", v)
		}
		if s := c.Query("q"); s != "" {
			like := "%" + s + "%"
			q = q.Where("actor_name ILIKE ? OR description ILIKE ? OR entity_id ILIKE ?", like, like, like)
		}
		var rows []model.AuditLog
		if dbFail(c, q.Order("created_at DESC").Limit(200).Find(&rows).Error) {
			return
		}
		logs := make([]J, 0, len(rows))
		for _, l := range rows {
			logs = append(logs, feAudit(l))
		}
		ok(c, J{"logs": logs, "total": len(logs)})
	}
}

// ---------- Pencarian global ----------

func searchGetHandler(db *gorm.DB) gin.HandlerFunc {
	return func(c *gin.Context) {
		_, hid, good := feCtx(c, db)
		if !good {
			return
		}
		term := strings.TrimSpace(c.Query("q"))
		if len([]rune(term)) < 2 {
			ok(c, J{"results": []J{}, "term": term})
			return
		}
		like := "%" + term + "%"
		results := []J{}
		add := func(typ, icon, lbl, sub, view, id string) {
			results = append(results, J{"type": typ, "icon": icon, "label": lbl, "sub": sub, "view": view, "id": id})
		}

		var ps []model.Practitioner
		db.Where("hospital_id = ? AND (full_name ILIKE ? OR specialty ILIKE ? OR sip_number ILIKE ? OR str_number ILIKE ?)",
			hid, like, like, like, like).Limit(5).Find(&ps)
		for _, p := range ps {
			sub := strOr(p.Profession, "—")
			if p.Specialty != nil {
				sub += " — " + *p.Specialty
			}
			add("Praktisi", "Users", p.FullName, sub, "cred-practitioners", p.ID)
		}
		var cs []model.PractitionerCredential
		db.Where("hospital_id = ? AND (type ILIKE ? OR number ILIKE ?)", hid, like, like).Limit(5).Find(&cs)
		for _, cr := range cs {
			add("Kredensial", "IdCard", cr.Type+" "+str(cr.Number), cr.Type+" · "+label(cr.Status), "cred-credentials", cr.ID)
		}
		var rs []model.Risk
		db.Where("hospital_id = ? AND (title ILIKE ? OR code ILIKE ?)", hid, like, like).Limit(5).Find(&rs)
		for _, r := range rs {
			add("Risiko", "Radar", r.Title, r.Code+" · "+label(r.Category)+" · "+label(r.Level), "risk-radar", r.ID)
		}
		var cls []model.Claim
		db.Where("hospital_id = ? AND (claim_number ILIKE ? OR sep_number ILIKE ? OR inacbg_code ILIKE ?)",
			hid, like, like, like).Limit(5).Find(&cls)
		for _, cl := range cls {
			add("Klaim", "FileCheck2", cl.ClaimNumber, str(cl.INACBGCode)+" · "+label(str(cl.Status)), "claim-anomalies", cl.ID)
		}
		var as []model.Action
		db.Where("hospital_id = ? AND (title ILIKE ? OR code ILIKE ? OR owner ILIKE ?)", hid, like, like, like).Limit(5).Find(&as)
		for _, a := range as {
			add("Aksi", "CheckCircle2", a.Title, a.Code+" · "+label(a.Status)+" · "+strOr(a.Owner, "Belum ditugaskan"), "actions", a.ID)
		}
		var regs []model.Regulation
		db.Where("title ILIKE ? OR number ILIKE ? OR code ILIKE ?", like, like, like).Limit(4).Find(&regs)
		for _, r := range regs {
			add("Regulasi", "Scale", r.Title, str(r.Number), "compliance", r.ID)
		}
		var ev []model.EvidenceItem
		db.Where("hospital_id = ? AND (metric ILIKE ? OR ref_id ILIKE ?)", hid, like, like).Limit(4).Find(&ev)
		for _, e := range ev {
			add("Bukti", "Fingerprint", strOr(e.Metric, "Bukti data"), strOr(e.SourceSystem, "—"), "evidence", e.ID)
		}
		var ws []model.EarlyWarning
		db.Where("hospital_id = ? AND (title ILIKE ? OR code ILIKE ?)", hid, like, like).Limit(4).Find(&ws)
		for _, w := range ws {
			add("Peringatan", "AlertTriangle", w.Title, w.Code+" · "+label(w.Severity), "early-warnings", w.ID)
		}
		ok(c, J{"results": results, "term": term})
	}
}
