package main

import (
	"log"
	"os"

	"github.com/gin-gonic/gin"
	"github.com/joho/godotenv"
)

// Uygulamayı başlatır: env yükle, DB bağlan, OAuth kur, route'ları bağla, sunucuyu çalıştır.
func main() {
	if err := godotenv.Load(); err != nil {
		log.Println("Warning: .env file not found, using system environment variables")
	}

	connectDB()
	defer db.Close()
	initOAuth()

	router := gin.Default()
	registerRoutes(router)

	port := os.Getenv("PORT")
	if port == "" {
		port = "8080"
	}
	log.Printf("Server running at http://localhost:%s", port)
	router.Run(":" + port)
}
