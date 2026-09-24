package main

import (
	"log"
	"os"

	"github.com/gin-contrib/cors"
	"github.com/gin-gonic/gin"
)

// Tüm HTTP route'larını router'a bağlar.
func registerRoutes(router *gin.Engine) {
	router.Use(rateLimitMiddleware())
	router.Use(securityHeadersMiddleware())

	// --- CORS yapılandırması ---
	frontendURL := os.Getenv("FRONTEND_URL")
	env := os.Getenv("ENV")

	if frontendURL == "" {
		log.Println("[CORS WARNING] FRONTEND_URL is not set — frontend origin'i izinli değil")
	}

	origins := []string{}
	if frontendURL != "" {
		origins = append(origins, frontendURL)
	}

	// Sadece açıkça development/dev ise localhost ekle.
	// Default: production (güvenli taraf).
	if env == "development" || env == "dev" {
		origins = append(origins, "http://localhost:5173")
		log.Println("[CORS] development mode: localhost:5173 izinli")
	} else if env == "" {
		log.Println("[CORS] ENV set edilmemiş — production kabul ediliyor")
	}

	if len(origins) == 0 {
		log.Println("[CORS WARNING] Hiçbir origin izinli değil — frontend istek atamaz!")
	}

	router.Use(cors.New(cors.Config{
		AllowOrigins:     origins,
		AllowMethods:     []string{"GET", "POST", "PUT", "DELETE", "OPTIONS"},
		AllowHeaders:     []string{"Origin", "Content-Type", "Authorization"},
		AllowCredentials: true,
	}))

	router.GET("/ping", handlePing)

	// Auth
	router.GET("/api/auth/google/login", handleGoogleLogin)
	router.GET("/api/auth/google/callback", handleGoogleCallback)
	router.GET("/api/auth/github/login", handleGithubLogin)
	router.GET("/api/auth/github/callback", handleGithubCallback)
	router.GET("/api/auth/me", authMiddleware(), handleMe)

	// Projeler
	router.GET("/api/projects", authMiddlewareOptional(), handleListProjects)
	router.GET("/api/projects/:id", authMiddlewareOptional(), handleGetProject)
	router.POST("/api/projects", authMiddleware(), handleCreateProject)
	router.PUT("/api/projects/:id", authMiddleware(), handleUpdateProject)
	router.DELETE("/api/projects/:id", authMiddleware(), handleDeleteProject)
	router.GET("/api/me/projects", authMiddleware(), handleMyProjects)
	router.GET("/api/me/projects/detailed", authMiddleware(), handleMyProjectsDetailed)

	// Kategoriler (public)
	router.GET("/api/categories", handleListCategories)

	// Yıldız
	router.POST("/api/projects/:id/star", authMiddleware(), handleStarProject)
	router.DELETE("/api/projects/:id/star", authMiddleware(), handleUnstarProject)

	// Kullanıcı
	router.PUT("/api/me", authMiddleware(), handleUpdateMe)
}
