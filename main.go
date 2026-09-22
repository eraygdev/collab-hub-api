package main

import (
	"fmt"
	"net/http"

	"github.com/gin-gonic/gin"
)

func main() {
	router := gin.Default()

	router.GET("/ping", func(c *gin.Context) {
		c.JSON(http.StatusOK, gin.H{
			"message": "pong",
		})
	})

	fmt.Println("🚀 Sunucu http://localhost:8080 adresinde çalışıyor")
	router.Run(":8080")
}
