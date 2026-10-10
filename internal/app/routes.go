package app

import (
	"log"
	"os"

	sentrygin "github.com/getsentry/sentry-go/gin"
	"github.com/gin-contrib/cors"
	"github.com/gin-gonic/gin"
)

// NewRouter tüm middleware ve route'ları bağlar, hazır *gin.Engine döner.
func NewRouter() *gin.Engine {
	var router *gin.Engine
	if os.Getenv("ENV") == "production" {
		router = gin.New()
		router.Use(gin.Recovery())
	} else {
		router = gin.Default()
	}

	// Sentry middleware — panikleri yakalar
	// Repanic: true → Gin'in kendi Recovery'si hâlâ çalışsın
	router.Use(sentrygin.New(sentrygin.Options{Repanic: true}))

	// Güvenilir proxy ayarı — X-Forwarded-For spoofing'i engeller
	// Production'da sadece Cloudflare IP'lerine güven
	if os.Getenv("ENV") == "production" {
		_ = router.SetTrustedProxies([]string{
			// Cloudflare IP aralıkları
			"173.245.48.0/20",
			"103.21.244.0/22",
			"103.22.200.0/22",
			"103.31.4.0/22",
			"141.101.64.0/18",
			"108.162.192.0/18",
			"190.93.240.0/20",
			"188.114.96.0/20",
			"197.234.240.0/22",
			"198.41.128.0/17",
			"162.158.0.0/15",
			"104.16.0.0/13",
			"104.24.0.0/14",
			"172.64.0.0/13",
			"131.0.72.0/22",
		})
	} else {
		// Development'ta X-Forwarded-For'a hiç güvenme
		_ = router.SetTrustedProxies(nil)
	}

	router.Use(rateLimitMiddleware())
	router.Use(securityHeadersMiddleware())

	setupCORS(router)

	router.GET("/ping", handlePing)

	api := router.Group("/api/" + APIVersion)

	// Auth
	auth := api.Group("/auth")
	{
		auth.GET("/github/login", handleGithubLogin)
		auth.GET("/github/callback", handleGithubCallback)
		auth.GET("/me", authMiddleware(), handleMe)
	}

	// Projects
	projects := api.Group("/projects")
	{
		projects.GET("", authMiddlewareOptional(), handleListProjects)
		projects.GET("/:id", authMiddlewareOptional(), handleGetProject)
		projects.POST("", authMiddleware(), handleCreateProject)
		projects.PUT("/:id", authMiddleware(), handleUpdateProject)
		projects.DELETE("/:id", authMiddleware(), handleDeleteProject)

		projects.POST("/:id/star", authMiddleware(), handleStarProject)
		projects.DELETE("/:id/star", authMiddleware(), handleUnstarProject)

		projects.POST("/:id/join", authMiddleware(), handleJoinProject)
		projects.DELETE("/:id/leave", authMiddleware(), handleLeaveProject)
		projects.GET("/:id/my-join-status", authMiddleware(), handleMyJoinStatus)
		projects.DELETE("/:id/contributors/:userId", authMiddleware(), handleRemoveContributor)
	}

	// Me — kendi kaynaklarım
	me := api.Group("/me", authMiddleware())
	{
		me.PUT("", handleUpdateMe)
		me.GET("/projects", handleMyProjects)
		me.GET("/contributions", handleMyContributions)
		me.GET("/contributor-requests", handleContributorRequests)
	}

	// Users — search önce tanımlı olmalı
	users := api.Group("/users")
	{
		users.GET("/search", handleSearchUsers)
		users.GET("/:username", handleGetUserByUsername)
	}

	api.GET("/categories", handleListCategories)
	api.GET("/config", handleConfig)

	// Contributor requests
	requests := api.Group("/contributor-requests", authMiddleware())
	{
		requests.PUT("/:id/approve", handleApproveRequest)
		requests.PUT("/:id/reject", handleRejectRequest)
	}

	return router
}

// setupCORS CORS middleware'ini yapılandırır.
func setupCORS(router *gin.Engine) {
	frontendURL := os.Getenv("FRONTEND_URL")
	env := os.Getenv("ENV")

	if frontendURL == "" {
		log.Println("[CORS WARNING] FRONTEND_URL is not set")
	}

	origins := []string{}
	if frontendURL != "" {
		origins = append(origins, frontendURL)
	}

	if env == "development" || env == "dev" {
		origins = append(origins, "http://localhost:5173")
		log.Println("[CORS] development mode: localhost:5173 izinli")
	} else if env == "" {
		log.Println("[CORS] ENV set edilmemiş — production kabul ediliyor")
	}

	if len(origins) == 0 {
		log.Println("[CORS WARNING] Hiçbir origin izinli değil")
	}

	router.Use(cors.New(cors.Config{
		AllowOrigins:     origins,
		AllowMethods:     []string{"GET", "POST", "PUT", "DELETE", "OPTIONS"},
		AllowHeaders:     []string{"Origin", "Content-Type", "Authorization"},
		AllowCredentials: true,
	}))
}
