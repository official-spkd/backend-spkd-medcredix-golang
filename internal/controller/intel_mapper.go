package controller

import (
	"fmt"
	"log"
	"math"
	"net/http"
	"sort"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	"gorm.io/gorm"

	"github.com/spkd/medcredix/internal/model"
	"github.com/spkd/medcredix/internal/scoring"
	"github.com/spkd/medcredix/internal/service"
)

// Pemetaan hasil scoring engine (scoring.Intelligence) ke bentuk "intel" yang dipakai frontend.
var stageTable = []J{
	{"min": 90, "stage": 7}, {"min": 80, "stage": 6}, {"min": 66, "stage": 5}, {"min": 60, "stage": 4},
	{"min": 50, "stage": 3}, {"min": 35, "stage": 2}, {"min": 20, "stage": 1}, {"min": 0, "stage": 0},
}

var trustBands = []J{
	{"label": "×1,00 — penuh", "minTrust": 90, "multiplier": 1},
	{"label": "×0,95", "minTrust": 70, "multiplier": 0.95},
	{"label": "×0,85", "minTrust": 50, "multiplier": 0.85},
	{"label": "×0,70", "minTrust": 0, "multiplier": 0.7},
}

var domainOwner = map[string]string{
	"workforce": "HR / SDM", "credentialing": "Petugas Kredensial", "digital": "IT / SIMRS",
	"medication": "Farmasi Klinik", "diagnostic": "Kepala Penunjang Medis", "interop": "IT / SIMRS",
	"claim": "Tim Klaim", "pathway": "Komite Medis", "safety": "Komite Mutu & Keselamatan Pasien",
}

// Kunci domain di frontend memakai "interop", engine memakai "interoperability".
func feDomainKey(k scoring.DomainKey) string {
	if k == scoring.DomInteroperability {
		return "interop"
	}
	return string(k)
}

func engineDomainKey(k string) string {
	if k == "interop" {
		return string(scoring.DomInteroperability)
	}
	return k
}

func indicatorStatusLabel(s string) string {
	switch s {
	case "ok":
		return "Baik"
	case "warn":
		return "Perlu Perhatian"
	case "bad":
		return "Kritis"
	}
	return "Tidak Ada Data"
}

func healthStatus(v float64) string {
	switch {
	case v >= 90:
		return "excellent"
	case v >= 75:
		return "good"
	case v >= 60:
		return "fair"
	}
	return "poor"
}

func priorityOf(score float64) string {
	switch {
	case score < 60:
		return "Tinggi"
	case score < 75:
		return "Sedang"
	}
	return "Rendah"
}

// groupOf: label kelompok tiap domain ("A. Kapabilitas Klinis & SDM").
func groupOf(in scoring.Intelligence, k scoring.DomainKey) string {
	for _, g := range in.Groups {
		for _, d := range g.Domains {
			if d == k {
				return g.Code + ". " + g.Label
			}
		}
	}
	// Credentialing tidak masuk kelompok BDCS; ditampilkan bersama Kelompok A (SDM).
	for _, g := range in.Groups {
		if g.Code == "A" {
			return g.Code + ". " + g.Label
		}
	}
	return ""
}

func feDomain(in scoring.Intelligence, d scoring.DomainResult) J {
	score := 0.0
	if d.Score != nil {
		score = *d.Score
	}
	inds := make([]J, 0, len(d.Indicators))
	positives, negatives := []string{}, []string{}
	for _, i := range d.Indicators {
		inds = append(inds, J{
			"label": i.Name, "value": fv(i.Value), "unit": "%", "threshold": fv(i.Threshold),
			"status": indicatorStatusLabel(i.Status), "weight": i.Weight, "source": i.Source,
			"fhir": i.Source, "note": i.Note,
		})
		if i.Value == nil {
			negatives = append(negatives, i.Name+": data belum tersedia")
			continue
		}
		txt := fmt.Sprintf("%s: %s%%", i.Name, fmtNum(*i.Value))
		if i.Threshold != nil {
			txt += fmt.Sprintf(" (ambang %s%%)", fmtNum(*i.Threshold))
		}
		if i.Status == "ok" {
			positives = append(positives, txt)
		} else {
			negatives = append(negatives, txt)
		}
	}
	positives = append(positives, d.Notes...)
	if !d.HasData {
		negatives = append(negatives, "Data domain belum tersedia, skor dihitung 0.")
	}
	return J{
		"key": feDomainKey(d.Key), "label": d.Label, "score": score, "weight": d.Weight / 100,
		"group": groupOf(in, d.Key), "formula": d.Formula, "indicators": inds,
		"positives": positives, "negatives": negatives, "contribution": d.Contribution, "hasData": d.HasData,
	}
}

// trend 6 bulan terakhir dari score_snapshots; bila kosong pakai hasil hitung saat ini.
func loadTrend(db *gorm.DB, hid string, in scoring.Intelligence) []J {
	var snaps []model.ScoreSnapshot
	db.Where("hospital_id = ?", hid).Order("period DESC").Limit(6).Find(&snaps)
	sort.Slice(snaps, func(i, j int) bool { return snaps[i].Period < snaps[j].Period })
	out := make([]J, 0, len(snaps)+1)
	for _, s := range snaps {
		out = append(out, J{
			"period": s.Period, "health": fv(s.HealthScore), "bcds": fv(s.BDCS), "trust": fv(s.Trust),
			"ccs": fv(s.CCS), "cpcs": fv(s.CPCS), "safety": fv(s.SafetyScore),
		})
	}
	if len(out) == 0 {
		out = append(out, J{
			"period": time.Now().Format("2006-01"), "health": in.HealthScore, "bcds": in.BDCS,
			"trust": in.Trust.Score, "ccs": fv(in.Claim.CCS), "cpcs": fv(in.Pathway.CPCS), "safety": fv(in.SafetyScore),
		})
	}
	return out
}

func loadPrereq(db *gorm.DB, hid string) []J {
	var reqs []model.Requirement
	db.Where("hospital_id = ?", hid).Order("code").Find(&reqs)
	out := make([]J, 0, len(reqs))
	for _, r := range reqs {
		var days interface{}
		if r.DocExpiry != nil {
			days = int(math.Ceil(time.Until(*r.DocExpiry).Hours() / 24))
		}
		out = append(out, J{"code": r.Code, "name": r.Name, "status": label(r.Status), "docExpiry": tv(r.DocExpiry), "daysLeft": days})
	}
	return out
}

func feIntel(db *gorm.DB, hid string, in scoring.Intelligence) J {
	domains := make([]J, 0, len(in.Domains))
	for _, d := range in.Domains {
		domains = append(domains, feDomain(in, d))
	}

	kelompok := J{"A": 0.0, "B": 0.0, "C": 0.0, "D": 0.0}
	for _, g := range in.Groups {
		if g.Score != nil {
			kelompok[g.Code] = *g.Score
		}
	}

	subs := make([]J, 0, len(in.Claim.Categories))
	for _, c := range in.Claim.Categories {
		subs = append(subs, J{
			"key": label(c.Key), "label": c.Label, "weight": c.Weight / 100,
			"ok": c.Ok, "warn": c.Warn, "bad": c.Bad, "score": c.Score,
		})
	}

	ur := J{
		"alos": 0.0, "readmission": 0.0, "bor": 0.0, "score": 0.0, "alosScore": 0.0, "readmScore": 0.0,
		"borScore": 0.0, "targetAlos": 5, "targetReadmission": 5, "targetBorMin": 60, "targetBorMax": 85,
	}
	if in.Utilization != nil {
		ur["score"] = in.Utilization.Score
		keys := [][2]string{{"alos", "alosScore"}, {"readmission", "readmScore"}, {"bor", "borScore"}}
		for i, comp := range in.Utilization.Components {
			if i < len(keys) {
				ur[keys[i][0]], ur[keys[i][1]] = comp.Value, comp.Score
			}
		}
	}

	var factors []model.TrustFactor
	db.Where("hospital_id = ?", hid).Order("weight DESC").Find(&factors)
	trustFactors := make([]J, 0, len(factors))
	for _, f := range factors {
		trustFactors = append(trustFactors, feTrustFactor(f))
	}

	specCov := make([]J, 0, len(in.Workforce.Specialists))
	for _, s := range in.Workforce.Specialists {
		specCov = append(specCov, J{
			"specialty": s.Specialty, "required": s.Required, "available": s.Available,
			"coverage": s.Coverage, "note": nil,
		})
	}

	h, _ := loadHospital(db, hid)
	hospital := J{}
	if h != nil {
		hospital = J{"id": h["id"], "name": h["name"], "class": h["class"], "city": h["city"], "province": h["province"]}
	}

	ccs := 0.0
	if in.Claim.CCS != nil {
		ccs = *in.Claim.CCS
	}
	cpcs := 0.0
	if in.Pathway.CPCS != nil {
		cpcs = *in.Pathway.CPCS
	}

	return J{
		"formulaVersion": in.FormulaVersion, "computedAt": in.ComputedAt, "dataAt": in.ComputedAt,
		"hospital": hospital, "domains": domains,
		"healthScore": in.HealthScore, "healthStatus": healthStatus(in.HealthScore),
		"bcds": J{
			"raw": in.BDCSRaw, "trust": in.Trust.Score, "multiplier": in.Trust.Multiplier, "final": in.BDCS,
			"stage": in.StageEffective, "lulus": in.Pass, "prereqOk": !in.GateBlocked,
		},
		"kelompok": kelompok,
		"ccs": J{
			"score": ccs, "grade": in.Claim.Grade, "subs": subs,
			"flaggedClaims": in.Claim.FlaggedClaims, "totalClaims": in.Claim.TotalClaims,
		},
		"cpcs": J{
			"score": cpcs, "coverage": in.Pathway.Coverage, "adherence": in.Pathway.Adherence,
			"documentation": in.Pathway.Documentation,
		},
		"ur": ur,
		"safety": J{
			"score": in.Safety.Score, "allergy": in.Safety.Allergy, "closedLoop": in.Safety.ClosedLoop,
			"discharge": in.Safety.Discharge,
		},
		"trust": J{
			"score": in.Trust.Score, "multiplier": in.Trust.Multiplier, "factors": trustFactors,
			"anomalyFlags": in.AnomalyFlags, "underReview": in.UnderReview,
		},
		"workforce": J{
			"specialistCov": specCov, "coverage": in.Workforce.Coverage, "practitioners": in.Workforce.Practitioners,
		},
		"credentialing": J{
			"completeness": in.Credentialing.Completeness, "licenseActive": in.Credentialing.LicenseActive,
			"expired": in.Credentialing.Expired, "within30": in.Credentialing.Within30,
			"within90": in.Credentialing.Within90, "pipelineHealth": in.Credentialing.PipelineHealth,
		},
		"sarana": J{"score": in.Sarana.Score, "total": in.Sarana.Total, "wajib": in.Sarana.Wajib},
		"trend":  loadTrend(db, hid, in),
		"prereq": loadPrereq(db, hid),
	}
}

// computeFE mengambil skor satu RS dalam bentuk yang dipakai frontend (lewat cache di service).
func computeFE(db *gorm.DB, hid string) (scoring.Intelligence, J, error) {
	return service.GetIntel(db, hid, feIntel)
}

func intelFail(c *gin.Context, hid string, err error) {
	log.Printf("scoring: gagal menghitung %s: %v", hid, err)
	fail(c, http.StatusInternalServerError, "Gagal menghitung skor.")
}

// domainRecs: rekomendasi otomatis dari indikator yang belum mencapai ambang.
func domainRecs(d J) []J {
	key, _ := d["key"].(string)
	inds, _ := d["indicators"].([]J)
	out := []J{}
	for _, i := range inds {
		status, _ := i["status"].(string)
		if status == "Baik" {
			continue
		}
		name, _ := i["label"].(string)
		prio := "Sedang"
		if status == "Kritis" || status == "Tidak Ada Data" {
			prio = "Tinggi"
		}
		title := "Lengkapi data " + strings.ToLower(name)
		if thr, ok := i["threshold"].(float64); ok && status != "Tidak Ada Data" {
			title = fmt.Sprintf("Tingkatkan %s hingga ≥ %s%%", strings.ToLower(name), fmtNum(thr))
		}
		out = append(out, J{"title": title, "priority": prio, "suggestedOwner": domainOwner[key]})
		if len(out) == 3 {
			break
		}
	}
	return out
}

// healthRecs: rekomendasi dari dua domain dengan skor terendah.
func healthRecs(domains []J) []J {
	sorted := append([]J(nil), domains...)
	sort.SliceStable(sorted, func(i, j int) bool { return sorted[i]["score"].(float64) < sorted[j]["score"].(float64) })
	out := []J{}
	for _, d := range sorted {
		if len(out) == 2 {
			break
		}
		score := d["score"].(float64)
		if score >= 90 {
			continue
		}
		recs := domainRecs(d)
		title := fmt.Sprintf("Perbaiki domain %s (skor %s)", d["label"], fmtNum(score))
		if len(recs) > 0 {
			title = recs[0]["title"].(string)
		}
		out = append(out, J{"title": title, "priority": priorityOf(score), "suggestedOwner": domainOwner[d["key"].(string)]})
	}
	return out
}
