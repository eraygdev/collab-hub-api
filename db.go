package main

import (
	"context"
	"log"
	"os"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
)

var db *pgxpool.Pool

// Veritabanına bağlanır ve havuzu global db değişkenine atar.
func connectDB() {
	connString := os.Getenv("DATABASE_URL")
	if connString == "" {
		log.Fatal("DATABASE_URL environment variable is not set")
	}

	config, err := pgxpool.ParseConfig(connString)
	if err != nil {
		log.Fatalf("Failed to parse database config: %v", err)
	}

	// Bağlantı havuzu ayarları
	config.MaxConns = 10                      // maksimum açık bağlantı
	config.MinConns = 2                       // minimum açık tutulan
	config.MaxConnLifetime = time.Hour        // 1 saat sonra bağlantıyı yenile
	config.MaxConnIdleTime = 30 * time.Minute // 30 dk boşta kalırsa kapat
	config.HealthCheckPeriod = time.Minute    // her dakika sağlık kontrolü

	db, err = pgxpool.NewWithConfig(context.Background(), config)
	if err != nil {
		log.Fatalf("Failed to create connection pool: %v", err)
	}

	// Bağlantının gerçekten çalıştığını doğrula
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if err := db.Ping(ctx); err != nil {
		log.Fatalf("Database ping failed: %v", err)
	}

	log.Println("Database connected successfully")
}
