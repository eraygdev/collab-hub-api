package main

import (
	"context"
	"log"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/getsentry/sentry-go"
	"github.com/joho/godotenv"

	"github.com/eraygdev/reporeef-api/internal/app"
)

func main() {
	if err := godotenv.Load(); err != nil {
		log.Println("Warning: .env file not found, using system environment variables")
	}

	// Sentry başlat — DSN yoksa atla (lokal dev için)
	if dsn := os.Getenv("SENTRY_DSN"); dsn != "" {
		err := sentry.Init(sentry.ClientOptions{
			Dsn:              dsn,
			Environment:      os.Getenv("ENV"),
			Release:          "reporeef-api@1.0.0",
			TracesSampleRate: 0.1, // 10% tracing sample
		})
		if err != nil {
			log.Printf("Sentry initialization failed: %v", err)
		} else {
			log.Println("Sentry initialized")
			defer sentry.Flush(2 * time.Second)
		}
	} else {
		log.Println("SENTRY_DSN not set — Sentry disabled")
	}

	app.Connect()
	defer app.Close()
	app.InitOAuth()

	// OAuth state cleanup — süresi geçmiş state'leri temizler
	go app.StartOAuthCleanup()

	router := app.NewRouter()

	port := os.Getenv("PORT")
	if port == "" {
		port = "8080"
	}

	srv := &http.Server{
		Addr:              ":" + port,
		Handler:           router,
		ReadTimeout:       15 * time.Second,
		ReadHeaderTimeout: 5 * time.Second,
		WriteTimeout:      30 * time.Second,
		IdleTimeout:       60 * time.Second,
	}

	go func() {
		log.Printf("Server running at http://localhost:%s", port)
		if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			log.Fatalf("Server error: %v", err)
		}
	}()

	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)
	<-quit
	log.Println("Shutting down server...")

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	if err := srv.Shutdown(ctx); err != nil {
		log.Fatalf("Server forced to shutdown: %v", err)
	}

	log.Println("Server exited gracefully")
}
