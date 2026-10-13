package controller

import "github.com/gin-gonic/gin"

// Home menampilkan info singkat bahwa API hidup (GET /).
func Home(c *gin.Context) {
	ok(c, gin.H{"app": "MedCredix API", "health": "/api/health"})
}

// Health dipakai Docker dan monitoring untuk cek server (GET /api/health).
func Health(c *gin.Context) {
	ok(c, gin.H{"status": "up"})
}
