package app

import (
	"context"
	"errors"
	"fmt"
	"log"

	"github.com/getsentry/sentry-go"
	sentrygin "github.com/getsentry/sentry-go/gin"
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

	// Sentry'ye gönder — request context'inden hub'ı al
	if hub := sentrygin.GetHubFromContext(c); hub != nil {
		hub.WithScope(func(scope *sentry.Scope) {
			scope.SetTag("path", c.Request.URL.Path)
			scope.SetTag("method", c.Request.Method)
			if uid := c.GetInt("user_id"); uid > 0 {
				scope.SetUser(sentry.User{ID: fmt.Sprintf("%d", uid)})
			}
			hub.CaptureException(err)
		})
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
