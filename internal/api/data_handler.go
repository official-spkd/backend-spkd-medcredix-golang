package api

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

// 24 resource FHIR yang diuji. CATATAN: daftar ini asumsi umum; samakan dengan
// daftar resmi "FHIR TDS v2" di SRS sebelum dipakai untuk UAT.
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

// POST /api/data  body: {"action":"test-connection"}   (izin CONFIGURE dicek di router)
//
// Mode nyata   : bila env SIMRS_FHIR_BASE_URL terisi -> memanggil server FHIR sungguhan.
// Mode simulasi: bila kosong -> status diturunkan dari tabel data_sources (seperti versi demo)
//
//	dan hasilnya diberi penanda "simulated": true agar tidak menyesatkan.
func dataPostHandler(db *gorm.DB) gin.HandlerFunc {
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
