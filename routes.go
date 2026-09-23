package main

import (
	"os"

	"github.com/gin-contrib/cors"
	"github.com/gin-gonic/gin"
)

// Tüm HTTP route'larını router'a bağlar.
func registerRoutes(router *gin.Engine) {
	router.Use(rateLimitMiddleware())
	router.Use(securityHeadersMiddleware())

	origins := []string{os.Getenv("FRONTEND_URL")}
	if os.Getenv("ENV") != "production" {
		origins = append(origins, "http://localhost:5173")
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
	router.GET("/api/projects", handleListProjects)
	router.GET("/api/projects/:id", handleGetProject)
	router.POST("/api/projects", authMiddleware(), handleCreateProject)
	router.PUT("/api/projects/:id", authMiddleware(), handleUpdateProject)
	router.DELETE("/api/projects/:id", authMiddleware(), handleDeleteProject)
	router.GET("/api/me/projects", authMiddleware(), handleMyProjects)
	router.GET("/api/me/projects/detailed", authMiddleware(), handleMyProjectsDetailed)

	// Kategoriler (public)
	router.GET("/api/categories", handleListCategories)

	// Yıldız (YENİ)
	router.POST("/api/projects/:id/star", authMiddleware(), handleStarProject)
	router.DELETE("/api/projects/:id/star", authMiddleware(), handleUnstarProject)

	// Kullanıcı
	router.PUT("/api/me", authMiddleware(), handleUpdateMe)
}
