package main

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"time"

	"github.com/gin-contrib/cors"
	"github.com/gin-gonic/gin"
	"github.com/golang-jwt/jwt/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/joho/godotenv"
	"golang.org/x/oauth2"
	"golang.org/x/oauth2/github"
	"golang.org/x/oauth2/google"
)

var db *pgxpool.Pool
var googleOauthConfig *oauth2.Config
var githubOauthConfig *oauth2.Config

// ---------- BAĞLANTI & KURULUM ----------

// Veritabanına bağlanır ve havuzu global db değişkenine atar.
func connectDB() {
	connString := os.Getenv("DATABASE_URL")
	if connString == "" {
		log.Fatal("DATABASE_URL environment variable is not set")
	}
	var err error
	db, err = pgxpool.New(context.Background(), connString)
	if err != nil {
		log.Fatalf("Failed to connect to database: %v", err)
	}
	log.Println("Database connected successfully")
}

// Google ve GitHub OAuth yapılandırmalarını .env'den okur.
func initOAuth() {
	googleOauthConfig = &oauth2.Config{
		ClientID:     os.Getenv("GOOGLE_CLIENT_ID"),
		ClientSecret: os.Getenv("GOOGLE_CLIENT_SECRET"),
		RedirectURL:  os.Getenv("GOOGLE_REDIRECT_URL"),
		Scopes:       []string{"openid", "email", "profile"},
		Endpoint:     google.Endpoint,
	}
	githubOauthConfig = &oauth2.Config{
		ClientID:     os.Getenv("GITHUB_CLIENT_ID"),
		ClientSecret: os.Getenv("GITHUB_CLIENT_SECRET"),
		RedirectURL:  os.Getenv("GITHUB_REDIRECT_URL"),
		Scopes:       []string{"read:user", "user:email"},
		Endpoint:     github.Endpoint,
	}
}

// ---------- JWT ----------

// Kullanıcı için 7 gün geçerli imzalı JWT token üretir (avatar_url dahil).
func generateJWT(userID int, email, username, avatarURL string) (string, error) {
	claims := jwt.MapClaims{
		"user_id":    userID,
		"email":      email,
		"username":   username,
		"avatar_url": avatarURL,
		"exp":        time.Now().Add(7 * 24 * time.Hour).Unix(),
		"iat":        time.Now().Unix(),
	}
	token := jwt.NewWithClaims(jwt.SigningMethodHS256, claims)
	return token.SignedString([]byte(os.Getenv("JWT_SECRET")))
}

// Authorization header'ındaki JWT'yi doğrular, kullanıcı bilgisini context'e koyar.
func authMiddleware() gin.HandlerFunc {
	return func(c *gin.Context) {
		authHeader := c.GetHeader("Authorization")
		if authHeader == "" || len(authHeader) < 8 {
			c.JSON(http.StatusUnauthorized, gin.H{"error": "missing token"})
			c.Abort()
			return
		}
		tokenString := authHeader[7:]
		token, err := jwt.Parse(tokenString, func(t *jwt.Token) (interface{}, error) {
			if _, ok := t.Method.(*jwt.SigningMethodHMAC); !ok {
				return nil, fmt.Errorf("unexpected signing method")
			}
			return []byte(os.Getenv("JWT_SECRET")), nil
		})
		if err != nil || !token.Valid {
			c.JSON(http.StatusUnauthorized, gin.H{"error": "invalid token"})
			c.Abort()
			return
		}
		claims, ok := token.Claims.(jwt.MapClaims)
		if !ok {
			c.JSON(http.StatusUnauthorized, gin.H{"error": "invalid claims"})
			c.Abort()
			return
		}
		c.Set("user_id", int(claims["user_id"].(float64)))
		c.Set("email", claims["email"].(string))
		c.Set("username", claims["username"].(string))
		if av, ok := claims["avatar_url"].(string); ok {
			c.Set("avatar_url", av)
		}
		c.Next()
	}
}

// ---------- HANDLER: GENEL ----------

// Sunucunun ayakta olup olmadığını kontrol eden basit endpoint.
func handlePing(c *gin.Context) {
	c.JSON(http.StatusOK, gin.H{"message": "pong"})
}

// Token'daki kullanıcı bilgisini geri döner (token geçerliyse).
func handleMe(c *gin.Context) {
	c.JSON(http.StatusOK, gin.H{
		"user_id":    c.GetInt("user_id"),
		"email":      c.GetString("email"),
		"username":   c.GetString("username"),
		"avatar_url": c.GetString("avatar_url"),
	})
}

// ---------- HANDLER: GOOGLE ----------

// Kullanıcıyı Google giriş sayfasına yönlendirir.
func handleGoogleLogin(c *gin.Context) {
	url := googleOauthConfig.AuthCodeURL("state-google")
	c.Redirect(http.StatusTemporaryRedirect, url)
}

// Google'dan dönen code'u token'a çevirir, kullanıcıyı DB'ye yazar, JWT ile frontend'e yollar.
func handleGoogleCallback(c *gin.Context) {
	code := c.Query("code")
	token, err := googleOauthConfig.Exchange(c.Request.Context(), code)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	// Google'dan kullanıcı bilgisini çek.
	resp, err := http.Get("https://www.googleapis.com/oauth2/v2/userinfo?access_token=" + token.AccessToken)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	defer resp.Body.Close()

	body, _ := io.ReadAll(resp.Body)
	var info map[string]interface{}
	json.Unmarshal(body, &info)

	googleID := fmt.Sprintf("%v", info["id"])
	email, _ := info["email"].(string)
	name, _ := info["name"].(string)
	picture, _ := info["picture"].(string)

	if email == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Google did not return email"})
		return
	}
	if name == "" {
		name = email
	}

	// Kullanıcıyı ekle veya güncelle.
	var userID int
	err = db.QueryRow(context.Background(), `
		INSERT INTO users (username, email, google_id, avatar_url)
		VALUES ($1, $2, $3, $4)
		ON CONFLICT (email) DO UPDATE
		SET google_id = EXCLUDED.google_id,
		    avatar_url = COALESCE(EXCLUDED.avatar_url, users.avatar_url)
		RETURNING id
	`, name, email, googleID, picture).Scan(&userID)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	jwtToken, err := generateJWT(userID, email, name, picture)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	frontend := os.Getenv("FRONTEND_URL")
	c.Redirect(http.StatusTemporaryRedirect, frontend+"/auth/callback?token="+jwtToken)
}

// ---------- HANDLER: GITHUB ----------

// Kullanıcıyı GitHub giriş sayfasına yönlendirir.
func handleGithubLogin(c *gin.Context) {
	url := githubOauthConfig.AuthCodeURL("state-github")
	c.Redirect(http.StatusTemporaryRedirect, url)
}

// GitHub'dan dönen code'u token'a çevirir, kullanıcıyı DB'ye yazar, JWT ile frontend'e yollar.
func handleGithubCallback(c *gin.Context) {
	code := c.Query("code")
	token, err := githubOauthConfig.Exchange(c.Request.Context(), code)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	client := &http.Client{}

	// GitHub kullanıcı bilgisini çek.
	req, _ := http.NewRequest("GET", "https://api.github.com/user", nil)
	req.Header.Set("Authorization", "Bearer "+token.AccessToken)
	req.Header.Set("Accept", "application/vnd.github+json")

	resp, err := client.Do(req)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	defer resp.Body.Close()

	body, _ := io.ReadAll(resp.Body)
	var info map[string]interface{}
	json.Unmarshal(body, &info)

	login, _ := info["login"].(string)
	avatar, _ := info["avatar_url"].(string)
	bio, _ := info["bio"].(string)

	var githubID int
	if v, ok := info["id"].(float64); ok {
		githubID = int(v)
	}

	// Email gizliyse ayrı endpoint'ten çek.
	email, _ := info["email"].(string)
	if email == "" {
		emailReq, _ := http.NewRequest("GET", "https://api.github.com/user/emails", nil)
		emailReq.Header.Set("Authorization", "Bearer "+token.AccessToken)
		emailReq.Header.Set("Accept", "application/vnd.github+json")

		emailResp, err := client.Do(emailReq)
		if err == nil {
			defer emailResp.Body.Close()
			emailBody, _ := io.ReadAll(emailResp.Body)
			var emails []map[string]interface{}
			json.Unmarshal(emailBody, &emails)
			for _, e := range emails {
				if primary, _ := e["primary"].(bool); primary {
					email, _ = e["email"].(string)
					break
				}
			}
		}
	}

	// Email ve login boşsa fallback üret.
	if email == "" {
		email = fmt.Sprintf("%s@users.noreply.github.com", login)
	}
	if login == "" {
		login = fmt.Sprintf("github_%d", githubID)
	}

	// Kullanıcıyı ekle veya güncelle.
	var userID int
	err = db.QueryRow(context.Background(), `
		INSERT INTO users (username, email, github_id, avatar_url, bio)
		VALUES ($1, $2, $3, $4, $5)
		ON CONFLICT (email) DO UPDATE
		SET github_id = EXCLUDED.github_id,
		    avatar_url = COALESCE(EXCLUDED.avatar_url, users.avatar_url),
		    bio = COALESCE(EXCLUDED.bio, users.bio)
		RETURNING id
	`, login, email, githubID, avatar, bio).Scan(&userID)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	jwtToken, err := generateJWT(userID, email, login, avatar)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	frontend := os.Getenv("FRONTEND_URL")
	c.Redirect(http.StatusTemporaryRedirect, frontend+"/auth/callback?token="+jwtToken)
}

// ---------- HANDLER: PROJELER ----------

// Tüm projeleri yazar bilgisiyle birlikte listeler.
func handleListProjects(c *gin.Context) {
	rows, err := db.Query(context.Background(), `
		SELECT p.id, p.title, p.description, p.long_description,
		       p.stars, p.contributors, p.status, p.created_at,
		       u.username AS author,
		       COALESCE(u.avatar_url, '') AS author_avatar
		FROM projects p
		LEFT JOIN users u ON u.id = p.author_id
		ORDER BY p.created_at DESC
	`)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	defer rows.Close()

	var projects []map[string]interface{}
	for rows.Next() {
		var id, stars, contributors int
		var title, description, longDesc, status, author, authorAvatar string
		var createdAt time.Time

		err := rows.Scan(
			&id, &title, &description, &longDesc,
			&stars, &contributors, &status, &createdAt,
			&author, &authorAvatar,
		)
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
			return
		}

		projects = append(projects, map[string]interface{}{
			"id":               id,
			"title":            title,
			"description":      description,
			"long_description": longDesc,
			"stars":            stars,
			"contributors":     contributors,
			"status":           status,
			"created_at":       createdAt,
			"author":           author,
			"author_avatar":    authorAvatar,
		})
	}

	if projects == nil {
		projects = []map[string]interface{}{}
	}
	c.JSON(http.StatusOK, projects)
}

// Yeni proje oluşturur (giriş yapmış kullanıcı için).
func handleCreateProject(c *gin.Context) {
	userID := c.GetInt("user_id")

	var input struct {
		Title           string `json:"title"`
		Description     string `json:"description"`
		LongDescription string `json:"long_description"`
		GithubURL       string `json:"github_url"`
		DemoURL         string `json:"demo_url"`
		ImageURL        string `json:"image_url"`
	}

	if err := c.ShouldBindJSON(&input); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "geçersiz veri"})
		return
	}

	if input.Title == "" || input.Description == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "title ve description zorunlu"})
		return
	}

	var projectID int
	err := db.QueryRow(context.Background(), `
		INSERT INTO projects (
			title, description, long_description,
			github_url, demo_url, image_url, author_id
		)
		VALUES ($1, $2, $3, $4, $5, $6, $7)
		RETURNING id
	`,
		input.Title, input.Description, input.LongDescription,
		input.GithubURL, input.DemoURL, input.ImageURL, userID,
	).Scan(&projectID)

	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusCreated, gin.H{
		"id":      projectID,
		"message": "proje oluşturuldu",
	})
}

// Kullanıcının kendi projelerini listeler.
func handleMyProjects(c *gin.Context) {
	userID := c.GetInt("user_id")

	rows, err := db.Query(context.Background(), `
		SELECT id, title, description, stars, contributors, status, created_at
		FROM projects
		WHERE author_id = $1
		ORDER BY created_at DESC
	`, userID)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	defer rows.Close()

	var projects []map[string]interface{}
	for rows.Next() {
		var id, stars, contributors int
		var title, description, status string
		var createdAt time.Time

		if err := rows.Scan(&id, &title, &description, &stars, &contributors, &status, &createdAt); err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
			return
		}

		projects = append(projects, map[string]interface{}{
			"id":           id,
			"title":        title,
			"description":  description,
			"stars":        stars,
			"contributors": contributors,
			"status":       status,
			"created_at":   createdAt,
		})
	}

	if projects == nil {
		projects = []map[string]interface{}{}
	}
	c.JSON(http.StatusOK, projects)
}

// ---------- ROUTE KAYIT ----------

// Tüm HTTP route'larını router'a bağlar.
func registerRoutes(router *gin.Engine) {
	// CORS ayarı
	router.Use(cors.New(cors.Config{
		AllowOrigins:     []string{os.Getenv("FRONTEND_URL"), "http://localhost:5173"},
		AllowMethods:     []string{"GET", "POST", "PUT", "DELETE", "OPTIONS"},
		AllowHeaders:     []string{"Origin", "Content-Type", "Authorization"},
		AllowCredentials: true,
	}))

	// Sağlık kontrolü
	router.GET("/ping", handlePing)

	// Auth route'ları
	router.GET("/api/auth/google/login", handleGoogleLogin)
	router.GET("/api/auth/google/callback", handleGoogleCallback)
	router.GET("/api/auth/github/login", handleGithubLogin)
	router.GET("/api/auth/github/callback", handleGithubCallback)
	router.GET("/api/auth/me", authMiddleware(), handleMe)

	// Proje route'ları
	router.GET("/api/projects", handleListProjects)
	router.POST("/api/projects", authMiddleware(), handleCreateProject)
	router.GET("/api/me/projects", authMiddleware(), handleMyProjects)
}

// ---------- MAIN ----------

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
