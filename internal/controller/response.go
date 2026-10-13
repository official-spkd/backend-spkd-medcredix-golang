package controller

import (
	"net/http"

	"github.com/gin-gonic/gin"
)

func ok(c *gin.Context, data interface{}) {
	c.JSON(http.StatusOK, gin.H{"ok": true, "data": data})
}

func fail(c *gin.Context, status int, msg string) {
	c.AbortWithStatusJSON(status, gin.H{"ok": false, "error": msg})
}
