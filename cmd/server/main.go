package main

import (
	"context"
	"log"
	"net/http"
	"os"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/joho/godotenv"

	"github.com/spkd/medcredix/internal/auth"
	"github.com/spkd/medcredix/internal/config"
	"github.com/spkd/medcredix/internal/database"
	"github.com/spkd/medcredix/internal/router"
	"github.com/spkd/medcredix/internal/seed"
	"github.com/spkd/medcredix/internal/service"
	"github.com/spkd/medcredix/internal/storage"
)

func main() {
	if err := godotenv.Load(); err != nil {
		log.Println("file .env tidak ditemukan, memakai environment bawaan")
	}

	prod := config.IsProduction()
	if prod {
		gin.SetMode(gin.ReleaseMode)
	}

	// Cek JWT_SECRET sebelum server jalan.
	if err := auth.CheckConfig(); err != nil {
		log.Fatal("konfigurasi keamanan: ", err)
	}

	// Penyimpanan berkas unggahan (terenkripsi).
	if err := storage.Init(); err != nil {
		log.Fatal("penyimpanan berkas: ", err)
	}

	conn, err := database.Open()
	if err != nil {
		log.Fatal("gagal konek database: ", err)
	}
	log.Println("database terhubung")

	// Seed demo hanya bila SEED_DEMO=true, tidak pernah di produksi.
	switch {
	case prod && config.Bool("SEED_DEMO"):
		log.Fatal("SEED_DEMO=true tidak boleh dipakai saat APP_ENV=production (akun demo bersandi publik)")
	case config.Bool("SEED_DEMO"):
		if err := seed.Run(conn); err != nil {
			log.Fatal("gagal seed: ", err)
		}
	default:
		log.Println("seed demo dilewati (set SEED_DEMO=true untuk mengisi data uji di lokal)")
	}

	// Status kredensial (segera berakhir / kedaluwarsa) diperbarui otomatis tiap jam.
	service.StartCredentialJob(context.Background(), conn)

	r, err := router.NewRouter(conn)
	if err != nil {
		log.Fatal("gagal menyiapkan router: ", err)
	}

	port := os.Getenv("MEDCREDIX_PORT")
	if port == "" {
		port = "8080"
	}

	// Server dengan timeout.
	srv := &http.Server{
		Addr:              ":" + port,
		Handler:           r,
		ReadHeaderTimeout: 10 * time.Second,
		ReadTimeout:       30 * time.Second,
		WriteTimeout:      120 * time.Second,
		IdleTimeout:       120 * time.Second,
		MaxHeaderBytes:    1 << 20,
	}
	log.Printf("MedCredix API jalan di :%s (produksi=%v)", port, prod)
	log.Fatal(srv.ListenAndServe())
}
