package app

import (
	"context"
	"errors"
	"log"

	"github.com/gin-gonic/gin"
	"github.com/jackc/pgx/v5/pgconn"
)

// serverError sunucu hatası döner ve detayı log'a yazar.
// Client bağlantıyı kestiyse (context canceled / deadline exceeded) sessizce çıkar.
func serverError(c *gin.Context, err error, publicMsg string) {
	// Client isteği iptal ettiyse — normal durum, log etme, cevap yazma.
	// (React StrictMode, sayfa değişimi, mobil ağ kopması vb.)
	if errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
		c.Abort()
		return
	}

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

// isUniqueViolationOn — belirli bir constraint'te UNIQUE ihlali mi?
func isUniqueViolationOn(err error, constraintName string) bool {
	if err == nil {
		return false
	}
	var pgErr *pgconn.PgError
	if errors.As(err, &pgErr) {
		return pgErr.Code == "23505" && pgErr.ConstraintName == constraintName
	}
	return false
}
