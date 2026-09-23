package main

import (
	"net/http"

	"github.com/gin-gonic/gin"
)

// Sunucunun ayakta olup olmadığını kontrol eden basit endpoint.
func handlePing(c *gin.Context) {
	c.JSON(http.StatusOK, gin.H{"message": "pong"})
}

// Token'daki kullanıcı bilgisini geri döner (token geçerliyse).
func handleMe(c *gin.Context) {
	c.JSON(http.StatusOK, gin.H{
		"user_id":    c.GetInt("user_id"),
		"email":      c.GetString("email"),
		"username":   c.GetString("username"),
		"avatar_url": c.GetString("avatar_url"),
	})
}
