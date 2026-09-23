package main

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"strings"

	"github.com/gin-gonic/gin"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
)

// Sunucunun ayakta olup olmadığını kontrol eden basit endpoint.
func handlePing(c *gin.Context) {
	c.JSON(http.StatusOK, gin.H{"message": "pong"})
}

// Token'daki kullanıcı bilgisini geri döner (token geçerliyse).
func handleMe(c *gin.Context) {
	userID := c.GetInt("user_id")

	var (
		email     string
		username  string
		avatarURL string
		bio       string
	)

	err := db.QueryRow(context.Background(), `
		SELECT email, COALESCE(username, ''), COALESCE(avatar_url, ''), COALESCE(bio, '')
		FROM users
		WHERE id = $1
	`, userID).Scan(&email, &username, &avatarURL, &bio)

	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "kullanıcı bulunamadı"})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"user_id":    userID,
		"email":      email,
		"username":   username,
		"avatar_url": avatarURL,
		"bio":        bio,
	})
}

// Kullanıcının kendi profil bilgilerini günceller (username + bio).
func handleUpdateMe(c *gin.Context) {
	userID := c.GetInt("user_id")

	var input struct {
		Username string `json:"username"`
		Bio      string `json:"bio"`
	}

	if err := c.ShouldBindJSON(&input); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "geçersiz veri"})
		return
	}

	input.Username = strings.TrimSpace(input.Username)

	if input.Username == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "username boş olamaz"})
		return
	}
	if len(input.Username) > MaxUsernameLen {
		c.JSON(http.StatusBadRequest, gin.H{
			"error": fmt.Sprintf("username en fazla %d karakter olabilir", MaxUsernameLen),
		})
		return
	}
	if len(input.Bio) > MaxBioLen {
		c.JSON(http.StatusBadRequest, gin.H{
			"error": fmt.Sprintf("bio en fazla %d karakter olabilir", MaxBioLen),
		})
		return
	}

	// 1) Önce başka biri tarafından kullanılıyor mu? (erken uyarı için)
	var existingID int
	err := db.QueryRow(context.Background(),
		`SELECT id FROM users WHERE username = $1 AND id != $2`,
		input.Username, userID,
	).Scan(&existingID)

	if err == nil {
		// Kullanıcı bulundu → çakışma
		c.JSON(http.StatusConflict, gin.H{"error": "bu kullanıcı adı zaten alınmış"})
		return
	}
	if !errors.Is(err, pgx.ErrNoRows) {
		// Beklenmedik DB hatası
		serverError(c, err, "")
		return
	}

	// 2) UPDATE — DB'de UNIQUE constraint varsa race condition burada yakalanır.
	_, err = db.Exec(context.Background(), `
		UPDATE users
		SET username = $1, bio = $2
		WHERE id = $3
	`, input.Username, input.Bio, userID)

	if err != nil {
		// UNIQUE ihlali → 409
		if isUniqueViolation(err) {
			c.JSON(http.StatusConflict, gin.H{"error": "bu kullanıcı adı zaten alınmış"})
			return
		}
		serverError(c, err, "")
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"message":  "profil güncellendi",
		"username": input.Username,
		"bio":      input.Bio,
	})
}

// PostgreSQL UNIQUE ihlalini tespit eder.
// pgconn.PgError kodu "23505" = unique_violation
func isUniqueViolation(err error) bool {
	if err == nil {
		return false
	}
	var pgErr *pgconn.PgError
	if errors.As(err, &pgErr) {
		return pgErr.Code == "23505"
	}
	return false
}
