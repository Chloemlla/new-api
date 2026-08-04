package controller

import (
	"net/http"

	"github.com/QuantumNous/new-api/middleware"
	"github.com/gin-gonic/gin"
)

func GetResponseCacheStats(c *gin.Context) {
	stats := middleware.GetResponseCacheStats()
	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"message": "",
		"data":    stats,
	})
}

func ClearResponseCache(c *gin.Context) {
	deleted, err := middleware.ClearResponseCache()
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"success": false,
			"message": err.Error(),
		})
		return
	}
	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"message": "",
		"data": gin.H{
			"deleted": deleted,
		},
	})
}
