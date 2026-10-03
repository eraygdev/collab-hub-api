package main

import (
	"errors"
	"log"

	"github.com/gin-gonic/gin"
	"github.com/jackc/pgx/v5/pgconn"
)

// serverError sunucu hatası döner ve detayı log'a yazar.
func serverError(c *gin.Context, err error, publicMsg string) {
	if publicMsg == "" {
		publicMsg = "server_error"
	}
	log.Printf("[ERROR] %s %s: %v", c.Request.Method, c.Request.URL.Path, err)
	c.JSON(500, gin.H{"error": publicMsg})
}

// isUniqueViolation PostgreSQL UNIQUE ihlalini tespit eder.
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
