package main

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"strconv"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/gin-gonic/gin"
	"github.com/jackc/pgx/v5"
)

// Sunucunun ayakta olup olmadığını kontrol eden basit endpoint.
func handlePing(c *gin.Context) {
	c.JSON(http.StatusOK, gin.H{"message": "pong"})
}

// Token'daki kullanıcı bilgisini geri döner (token geçerliyse).
func handleMe(c *gin.Context) {
	userID := c.GetInt("user_id")

	var (
		email     string
		username  string
		avatarURL string
		bio       string
		isPremium bool
	)

	err := db.QueryRow(context.Background(), `
		SELECT 
			email, 
			COALESCE(username, ''), 
			COALESCE(avatar_url, ''), 
			COALESCE(bio, ''),
			is_premium
		FROM users
		WHERE id = $1
	`, userID).Scan(&email, &username, &avatarURL, &bio, &isPremium)

	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "kullanıcı bulunamadı"})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"user_id":    userID,
		"email":      email,
		"username":   username,
		"avatar_url": avatarURL,
		"bio":        bio,
		"is_premium": isPremium,
	})
}

// Kullanıcının kendi profil bilgilerini günceller (username + bio).
func handleUpdateMe(c *gin.Context) {
	userID := c.GetInt("user_id")

	var input struct {
		Username string `json:"username"`
		Bio      string `json:"bio"`
	}

	if err := c.ShouldBindJSON(&input); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "geçersiz veri"})
		return
	}

	input.Username = strings.TrimSpace(input.Username)
	input.Bio = strings.TrimSpace(input.Bio)

	if input.Username == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "username boş olamaz"})
		return
	}

	// ✅ Uzunluk + karakter filtresi
	if err := validateText("username", input.Username, MaxUsernameLen, UsernameRegex); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	if err := validateText("bio", input.Bio, MaxBioLen, BioRegex); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	// 1) Önce başka biri tarafından kullanılıyor mu?
	var existingID int
	err := db.QueryRow(context.Background(),
		`SELECT id FROM users WHERE username = $1 AND id != $2`,
		input.Username, userID,
	).Scan(&existingID)

	if err == nil {
		c.JSON(http.StatusConflict, gin.H{"error": "bu kullanıcı adı zaten alınmış"})
		return
	}
	if !errors.Is(err, pgx.ErrNoRows) {
		serverError(c, err, "")
		return
	}

	// 2) UPDATE
	_, err = db.Exec(context.Background(), `
		UPDATE users
		SET username = $1, bio = $2
		WHERE id = $3
	`, input.Username, input.Bio, userID)

	if err != nil {
		if isUniqueViolation(err) {
			c.JSON(http.StatusConflict, gin.H{"error": "bu kullanıcı adı zaten alınmış"})
			return
		}
		serverError(c, err, "")
		return
	}

	auditLog(c, userID, "update", "user", userID)

	c.JSON(http.StatusOK, gin.H{
		"message":  "profil güncellendi",
		"username": input.Username,
		"bio":      input.Bio,
	})
}

// Belirli bir kullanıcının profilini ve projelerini döner (public).
func handleGetUserByUsername(c *gin.Context) {
	username := c.Param("username")

	if username == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "username gerekli"})
		return
	}

	// Sayfalama
	limit := MaxProjectsPerPage
	if l := c.Query("limit"); l != "" {
		if n, err := strconv.Atoi(l); err == nil && n > 0 && n <= MaxProjectsPerPage {
			limit = n
		}
	}
	offset := 0
	if o := c.Query("offset"); o != "" {
		if n, err := strconv.Atoi(o); err == nil && n >= 0 && n <= 10000 {
			offset = n
		}
	}

	// Sıralama
	sortParam := c.DefaultQuery("sort", "newest")

	// Kullanıcıyı bul
	var (
		userID     int
		dbUsername string
		email      string
		avatarURL  string
		bio        string
		createdAt  time.Time
	)

	err := db.QueryRow(context.Background(), `
		SELECT 
			id, 
			username, 
			COALESCE(email, '') AS email,
			COALESCE(avatar_url, '') AS avatar_url,
			COALESCE(bio, '') AS bio,
			created_at
		FROM users
		WHERE LOWER(username) = LOWER($1)
	`, username).Scan(&userID, &dbUsername, &email, &avatarURL, &bio, &createdAt)

	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "kullanıcı bulunamadı"})
		return
	}

	// İstatistikler
	var totalProjects, totalStars, totalContributors int
	db.QueryRow(context.Background(), `
		SELECT 
			(SELECT COUNT(*) FROM projects WHERE author_id = $1) AS total_projects,
			COALESCE((
				SELECT SUM((SELECT COUNT(*) FROM project_stars WHERE project_id = p.id))
				FROM projects p WHERE p.author_id = $1
			), 0) AS total_stars,
			COALESCE((
				SELECT COUNT(*)
				FROM project_contributors pc
				JOIN projects p ON p.id = pc.project_id
				WHERE p.author_id = $1 AND pc.status = 'approved'
			), 0) AS total_contributors
	`, userID).Scan(&totalProjects, &totalStars, &totalContributors)

	// Sıralama
	orderClause := "p.created_at DESC, p.id DESC"
	if sortParam == "popular" {
		orderClause = "stars DESC, p.created_at DESC, p.id DESC"
	}

	query := fmt.Sprintf(`
		SELECT 
			p.id, p.title, p.description,
			COALESCE(p.image_url, '') AS image_url,
			(SELECT COUNT(*) FROM project_stars WHERE project_id = p.id) AS stars,
			(SELECT COUNT(*) FROM project_contributors WHERE project_id = p.id AND status = 'approved') AS contributors,
			p.created_at,
			COALESCE(
				(SELECT ARRAY_AGG(c.name ORDER BY c.name)
				FROM project_categories pc
				JOIN categories c ON c.id = pc.category_id
				WHERE pc.project_id = p.id),
				ARRAY[]::varchar[]
			) AS categories,
			COALESCE(
				(SELECT ARRAY_AGG(pc.category_id ORDER BY pc.category_id)
				FROM project_categories pc
				WHERE pc.project_id = p.id),
				ARRAY[]::int[]
			) AS category_ids
		FROM projects p
		WHERE p.author_id = $1
		ORDER BY %s
		LIMIT $2 OFFSET $3
	`, orderClause)

	rows, err := db.Query(context.Background(), query, userID, limit+1, offset)
	if err != nil {
		serverError(c, err, "")
		return
	}
	defer rows.Close()

	projects := []map[string]interface{}{}
	count := 0

	for rows.Next() {
		var id, stars, contributors int
		var title, description, imageURL string
		var projCreatedAt time.Time
		var categories []string
		var categoryIDs []int

		if err := rows.Scan(
			&id, &title, &description, &imageURL,
			&stars, &contributors, &projCreatedAt,
			&categories, &categoryIDs,
		); err != nil {
			serverError(c, err, "")
			return
		}

		count++
		if count > limit {
			break
		}

		if categories == nil {
			categories = []string{}
		}
		if categoryIDs == nil {
			categoryIDs = []int{}
		}

		projects = append(projects, map[string]interface{}{
			"id":           id,
			"title":        title,
			"description":  description,
			"imageUrl":     imageURL,
			"stars":        stars,
			"contributors": contributors,
			"createdAt":    projCreatedAt,
			"categories":   categories,
			"categoryIds":  categoryIDs,
			"author":       dbUsername,
			"authorAvatar": avatarURL,
		})
	}

	hasMore := count > limit

	c.JSON(http.StatusOK, gin.H{
		"user_id":    userID,
		"username":   dbUsername,
		"email":      email,
		"avatar_url": avatarURL,
		"bio":        bio,
		"created_at": createdAt,
		"stats": gin.H{
			"totalProjects":     totalProjects,
			"totalStars":        totalStars,
			"totalContributors": totalContributors,
		},
		"projects": projects,
		"hasMore":  hasMore,
	})
}

// Kullanıcı adına göre arama (case-insensitive).
func handleSearchUsers(c *gin.Context) {
	query := strings.TrimSpace(c.Query("q"))

	if query == "" {
		c.JSON(http.StatusOK, []map[string]interface{}{})
		return
	}

	// ✅ Uzunluk + karakter filtresi (UsernameRegex — kullanıcı adı arıyoruz)
	if err := validateText("arama metni", query, MaxUsernameLen, UsernameRegex); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	// ✅ Limit — MaxUsersPerSearch (5)
	limit := MaxUsersPerSearch
	if l := c.Query("limit"); l != "" {
		if n, err := strconv.Atoi(l); err == nil && n > 0 && n <= MaxUsersPerSearch {
			limit = n
		}
	}

	// Wildcard escape
	query = strings.ReplaceAll(query, "\\", "\\\\")
	query = strings.ReplaceAll(query, "%", "\\%")
	query = strings.ReplaceAll(query, "_", "\\_")

	rows, err := db.Query(context.Background(), `
		SELECT 
			id, 
			username, 
			COALESCE(avatar_url, '') AS avatar_url,
			COALESCE(bio, '') AS bio
		FROM users
		WHERE LOWER(username) LIKE LOWER($1) ESCAPE '\'
		ORDER BY username ASC
		LIMIT $2
	`, "%"+query+"%", limit)
	if err != nil {
		serverError(c, err, "")
		return
	}
	defer rows.Close()

	users := []map[string]interface{}{}
	for rows.Next() {
		var id int
		var username, avatarURL, bio string

		if err := rows.Scan(&id, &username, &avatarURL, &bio); err != nil {
			serverError(c, err, "")
			return
		}

		// Bio'yu kısalt (max 60 karakter)
		if utf8.RuneCountInString(bio) > 60 {
			runes := []rune(bio)
			bio = string(runes[:60]) + "..."
		}

		users = append(users, map[string]interface{}{
			"user_id":    id,
			"username":   username,
			"avatar_url": avatarURL,
			"bio":        bio,
		})
	}

	c.JSON(http.StatusOK, users)
}
