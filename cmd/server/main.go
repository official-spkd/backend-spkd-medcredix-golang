package main

import (
	"log"
	"net/http"
	"os"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/joho/godotenv"

	"github.com/spkd/medcredix/internal/api"
	"github.com/spkd/medcredix/internal/appenv"
	"github.com/spkd/medcredix/internal/auth"
	"github.com/spkd/medcredix/internal/db"
	"github.com/spkd/medcredix/internal/seed"
)

func main() {
	if err := godotenv.Load(); err != nil {
		log.Println("file .env tidak ditemukan, memakai environment bawaan")
	}

	prod := appenv.IsProduction()
	if prod {
		gin.SetMode(gin.ReleaseMode)
	}

	// Cek JWT_SECRET sebelum server jalan.
	if err := auth.CheckConfig(); err != nil {
		log.Fatal("konfigurasi keamanan: ", err)
	}

	database, err := db.Open()
	if err != nil {
		log.Fatal("gagal konek database: ", err)
	}
	log.Println("database terhubung")

	// Seed demo hanya bila SEED_DEMO=true, tidak pernah di produksi.
	switch {
	case prod && appenv.Bool("SEED_DEMO"):
		log.Fatal("SEED_DEMO=true tidak boleh dipakai saat APP_ENV=production (akun demo bersandi publik)")
	case appenv.Bool("SEED_DEMO"):
		if err := seed.Run(database); err != nil {
			log.Fatal("gagal seed: ", err)
		}
	default:
		log.Println("seed demo dilewati (set SEED_DEMO=true untuk mengisi data uji di lokal)")
	}

	r, err := api.NewRouter(database)
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
