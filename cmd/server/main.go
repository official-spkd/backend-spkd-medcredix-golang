package main

import (
	"log"
	"net/http"
	"os"

	"github.com/gin-gonic/gin"
	"github.com/joho/godotenv"

	"github.com/spkd/medcredix/internal/db"
	"github.com/spkd/medcredix/internal/seed"
)

func main() {
	if err := godotenv.Load(); err != nil {
		log.Println("file .env tidak ditemukan, memakai environment bawaan")
	}

	database, err := db.Open()
	if err != nil {
		log.Fatal("gagal konek database: ", err)
	}
	log.Println("database terhubung")

	if err := seed.Run(database); err != nil {
		log.Fatal("gagal seed: ", err)
	}

	r := gin.Default()

	r.GET("/api/health", func(c *gin.Context) {
		c.JSON(http.StatusOK, gin.H{"ok": true, "data": gin.H{"status": "up"}})
	})

	r.GET("/", func(c *gin.Context) {
		c.JSON(http.StatusOK, gin.H{"ok": true, "data": gin.H{"app": "MedCredix API", "health": "/api/health"}})
	})

	port := "8080"
	if p := os.Getenv("MEDCREDIX_PORT"); p != "" {
		port = p
	}
	log.Fatal(r.Run(":" + port))
}
