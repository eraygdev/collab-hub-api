package main

import (
	"log"

	"github.com/gin-gonic/gin"
)

// Sunucu hatası döner ve detayı log'a yazar.
func serverError(c *gin.Context, err error, publicMsg string) {
	if publicMsg == "" {
		publicMsg = "sunucu hatası, lütfen tekrar deneyin"
	}
	log.Printf("[ERROR] %s %s: %v", c.Request.Method, c.Request.URL.Path, err)
	c.JSON(500, gin.H{"error": publicMsg})
}
