package controller

import (
	"context"
	"fmt"
	"log"
	"net/http"
	"os"
	"sync"
	"time"

	"github.com/gin-gonic/gin"
	"gorm.io/gorm"

	"github.com/spkd/medcredix/internal/audit"
	"github.com/spkd/medcredix/internal/integration"
	"github.com/spkd/medcredix/internal/model"
)

// GetData menampilkan isu kualitas data, trust score, sumber data, fasilitas, dan tempat tidur (GET /api/data).
func GetData(db *gorm.DB) gin.HandlerFunc {
	return func(c *gin.Context) {
		_, hid, good := feCtx(c, db)
		if !good {
			return
		}
		typ := c.DefaultQuery("type", "all")

		var iss []model.DataQualityIssue
		db.Where("hospital_id = ?", hid).Order("status, id").Find(&iss)
		issues := make([]J, 0, len(iss))
		terbuka, anomaly := 0, 0
		for _, i := range iss {
			issues = append(issues, feDQIssue(i))
			if i.Status == "terbuka" {
				terbuka++
			}
			if i.Anomaly {
				anomaly++
			}
		}
		sortJ(issues, func(a, b J) bool {
			if a["status"] != b["status"] {
				return numStr(a["status"]) > numStr(b["status"]) // Terbuka dulu
			}
			return rank(codeOf(numStr(a["severity"]))) > rank(codeOf(numStr(b["severity"])))
		})
		if typ == "quality" {
			ok(c, J{"issues": issues, "summary": J{"terbuka": terbuka, "total": len(issues), "anomaly": anomaly}})
			return
		}

		_, intel, err := computeFE(db, hid)
		if err != nil {
			intelFail(c, hid, err)
			return
		}
		if typ == "trust" {
			ok(c, J{"trust": intel["trust"], "bands": trustBands, "bcds": intel["bcds"]})
			return
		}

		var ss []model.DataSource
		db.Where("hospital_id = ?", hid).Order("name").Find(&ss)
		sources := make([]J, 0, len(ss))
		srcIDs := make([]string, 0, len(ss))
		for _, s := range ss {
			sources = append(sources, feDataSource(s))
			srcIDs = append(srcIDs, s.ID)
		}
		jobs := []J{}
		if len(srcIDs) > 0 {
			var js []model.IntegrationJob
			db.Where("data_source_id IN ?", srcIDs).Order("id DESC").Limit(24).Find(&js)
			for _, j := range js {
				jobs = append(jobs, feJob(j, hid))
			}
		}
		var fs []model.Facility
		db.Where("hospital_id = ?", hid).Order("category, name").Find(&fs)
		facilities := make([]J, 0, len(fs))
		for _, f := range fs {
			facilities = append(facilities, feFacility(f))
		}
		var bs []model.BedStat
		db.Where("hospital_id = ?", hid).Order("bed_class, ward").Find(&bs)
		beds := make([]J, 0, len(bs))
		for _, b := range bs {
			beds = append(beds, feBed(b))
		}
		hospital, _ := loadHospital(db, hid)

		if typ == "sources" {
			ok(c, J{"sources": sources, "jobs": jobs, "facilities": facilities, "beds": beds, "hospital": hospital})
			return
		}
		ok(c, J{"issues": issues, "sources": sources, "jobs": jobs, "facilities": facilities, "beds": beds,
			"hospital": hospital, "trust": intel["trust"]})
	}
}

// 24 resource FHIR yang diuji. CATATAN: daftar ini asumsi umum; samakan dengan
// daftar resmi "FHIR TDS v2" sebelum dipakai untuk UAT.
var fhirResources = []string{
	"Patient", "Encounter", "Practitioner", "PractitionerRole", "Organization", "Location",
	"Condition", "Observation", "Procedure", "MedicationRequest", "MedicationDispense",
	"MedicationStatement", "Medication", "AllergyIntolerance", "DiagnosticReport",
	"ServiceRequest", "Specimen", "ImagingStudy", "Composition", "Claim", "Coverage",
	"Immunization", "CarePlan", "Device",
}

type resourceResult struct {
	Resource string `json:"resource"`
	Status   string `json:"status"` // OK | Perlu Perbaikan | Gagal
	Records  int    `json:"count"`
	Message  string `json:"message"`
}

// PostData menguji koneksi ke SIMRS (POST /api/data), body: {"action":"test-connection"}.
// Izin CONFIGURE dicek di router.
//
// Mode nyata   : bila env SIMRS_FHIR_BASE_URL terisi -> memanggil server FHIR sungguhan.
// Mode simulasi: bila kosong -> status diturunkan dari tabel data_sources
//
//	dan hasilnya diberi penanda "simulated": true agar tidak menyesatkan.
func PostData(db *gorm.DB) gin.HandlerFunc {
	return func(c *gin.Context) {
		user, hid, good := tenantID(c, db)
		if !good {
			return
		}
		var body struct {
			Action string `json:"action"`
		}
		if err := c.ShouldBindJSON(&body); err != nil || body.Action != "test-connection" {
			fail(c, http.StatusBadRequest, "Aksi tidak dikenal.")
			return
		}

		var src model.DataSource
		if err := db.Where("hospital_id = ? AND type = ?", hid, "SIMRS").Order("name").First(&src).Error; err != nil {
			fail(c, http.StatusNotFound, "Belum ada sumber data SIMRS untuk rumah sakit ini.")
			return
		}

		baseURL := os.Getenv("SIMRS_FHIR_BASE_URL")
		simulated := baseURL == ""
		results := make([]resourceResult, len(fhirResources))
		started := time.Now()

		if simulated {
			status := "OK"
			if src.Status != nil && *src.Status != "terhubung" {
				status = "Perlu Perbaikan"
			}
			for i, r := range fhirResources {
				results[i] = resourceResult{Resource: r, Status: status, Message: "simulasi dari data_sources"}
			}
		} else {
			client := integration.NewFHIRClient(baseURL, os.Getenv("SIMRS_FHIR_TOKEN"))
			ctx, cancel := context.WithTimeout(c.Request.Context(), 60*time.Second)
			defer cancel()

			sem := make(chan struct{}, 6) // maksimal 6 panggilan paralel
			var wg sync.WaitGroup
			for i, r := range fhirResources {
				wg.Add(1)
				go func(i int, r string) {
					defer wg.Done()
					sem <- struct{}{}
					defer func() { <-sem }()
					okRes, total, err := client.TestFHIRResource(ctx, r)
					switch {
					case err != nil:
						// Detail error hanya ke log server.
						log.Printf("fhir test %s (%s): %v", r, hid, err)
						results[i] = resourceResult{Resource: r, Status: "Gagal", Message: "gagal menghubungi atau membaca server FHIR"}
					case okRes && total == 0:
						results[i] = resourceResult{Resource: r, Status: "Perlu Perbaikan", Message: "resource kosong"}
					default:
						results[i] = resourceResult{Resource: r, Status: "OK", Records: total, Message: "Skema & field wajib valid"}
					}
				}(i, r)
			}
			wg.Wait()
		}

		summary := map[string]int{"ok": 0, "perlu": 0, "gagal": 0}
		for _, r := range results {
			switch r.Status {
			case "OK":
				summary["ok"]++
			case "Perlu Perbaikan":
				summary["perlu"]++
			default:
				summary["gagal"]++
			}
		}

		// Catat hasil sebagai integration_job + audit log.
		jobStatus := "sukses"
		if summary["gagal"] > 0 {
			jobStatus = "gagal"
		}
		dur := int(time.Since(started).Milliseconds())
		total := 0
		for _, r := range results {
			total += r.Records
		}
		jt := "test-connection"
		_ = db.Create(&model.IntegrationJob{
			ID: model.NewID("job"), DataSourceID: src.ID, JobType: &jt,
			Status: &jobStatus, RecordsPulled: &total, DurationMs: &dur,
		}).Error
		_ = audit.Log(db, user, hid, "config", "data_sources", src.ID,
			fmt.Sprintf("Test Koneksi %d resource FHIR (simulasi=%v)", len(results), simulated), metaOf(c))

		ok(c, gin.H{
			"testedAt":  time.Now(),
			"source":    src.Name,
			"baseUrl":   strOr(ptrOrNil(baseURL), "simulasi (SIMRS_FHIR_BASE_URL belum diisi)"),
			"simulated": simulated,
			"results":   results,
			"summary":   summary,
		})
	}
}
