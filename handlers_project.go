package main

import (
	"context"
	"net/http"
	"os"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/golang-jwt/jwt/v5"
)

// Tüm projeleri hafif liste olarak döner (kullanıcı yıldızladı mı bilgisiyle).
func handleListProjects(c *gin.Context) {
	userID := 0
	authHeader := c.GetHeader("Authorization")
	if authHeader != "" && len(authHeader) > 7 {
		tokenString := authHeader[7:]
		token, err := jwt.Parse(tokenString, func(t *jwt.Token) (interface{}, error) {
			return []byte(os.Getenv("JWT_SECRET")), nil
		})
		if err == nil && token.Valid {
			if claims, ok := token.Claims.(jwt.MapClaims); ok {
				if uid, ok := claims["user_id"].(float64); ok {
					userID = int(uid)
				}
			}
		}
	}

	rows, err := db.Query(context.Background(), `
		SELECT 
			p.id, p.title, p.description,
			COALESCE(p.image_url, '') AS image_url,
			(SELECT COUNT(*) FROM project_stars WHERE project_id = p.id) AS stars,
			EXISTS(SELECT 1 FROM project_stars WHERE project_id = p.id AND user_id = $1) AS starred,
			COALESCE(u.username, '') AS author,
			COALESCE(
				ARRAY_AGG(c.name ORDER BY c.name) FILTER (WHERE c.id IS NOT NULL),
				ARRAY[]::varchar[]
			) AS categories
		FROM projects p
		LEFT JOIN users u ON u.id = p.author_id
		LEFT JOIN project_categories pc ON pc.project_id = p.id
		LEFT JOIN categories c ON c.id = pc.category_id
		GROUP BY 
			p.id, p.title, p.description, p.image_url,
			u.username
		ORDER BY p.created_at DESC
	`, userID)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	defer rows.Close()

	projects := []map[string]interface{}{}
	for rows.Next() {
		var id, stars int
		var starred bool
		var title, description, imageURL, author string
		var categories []string

		err := rows.Scan(
			&id, &title, &description, &imageURL,
			&stars, &starred, &author,
			&categories,
		)
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
			return
		}

		if categories == nil {
			categories = []string{}
		}

		projects = append(projects, map[string]interface{}{
			"id":          id,
			"title":       title,
			"description": description,
			"imageUrl":    imageURL,
			"stars":       stars,
			"starred":     starred,
			"author":      author,
			"categories":  categories,
		})
	}

	c.JSON(http.StatusOK, projects)
}

// Tek bir projeyi tam detaylarıyla döner (kategoriler ve yıldız bilgisi dahil).
func handleGetProject(c *gin.Context) {
	id := c.Param("id")

	userID := 0
	authHeader := c.GetHeader("Authorization")
	if authHeader != "" && len(authHeader) > 7 {
		tokenString := authHeader[7:]
		token, err := jwt.Parse(tokenString, func(t *jwt.Token) (interface{}, error) {
			return []byte(os.Getenv("JWT_SECRET")), nil
		})
		if err == nil && token.Valid {
			if claims, ok := token.Claims.(jwt.MapClaims); ok {
				if uid, ok := claims["user_id"].(float64); ok {
					userID = int(uid)
				}
			}
		}
	}

	var project struct {
		ID              int       `json:"id"`
		Title           string    `json:"title"`
		Description     string    `json:"description"`
		LongDescription string    `json:"longDescription"`
		GithubURL       string    `json:"githubUrl"`
		DemoURL         string    `json:"demoUrl"`
		ImageURL        string    `json:"imageUrl"`
		Stars           int       `json:"stars"`
		Starred         bool      `json:"starred"`
		Contributors    int       `json:"contributors"`
		Status          string    `json:"status"`
		CreatedAt       time.Time `json:"createdAt"`
		Author          string    `json:"author"`
		AuthorAvatar    string    `json:"authorAvatar"`
		Categories      []string  `json:"categories"`
	}

	err := db.QueryRow(context.Background(), `
		SELECT 
			p.id, p.title, p.description,
			COALESCE(p.long_description, '') AS long_description,
			COALESCE(p.github_url, '') AS github_url,
			COALESCE(p.demo_url, '') AS demo_url,
			COALESCE(p.image_url, '') AS image_url,
			(SELECT COUNT(*) FROM project_stars WHERE project_id = p.id) AS stars,
			EXISTS(SELECT 1 FROM project_stars WHERE project_id = p.id AND user_id = $2) AS starred,
			p.contributors, p.status, p.created_at,
			COALESCE(u.username, '') AS author,
			COALESCE(u.avatar_url, '') AS author_avatar,
			COALESCE(
				(SELECT ARRAY_AGG(c.name ORDER BY c.name)
				 FROM project_categories pc
				 JOIN categories c ON c.id = pc.category_id
				 WHERE pc.project_id = p.id),
				ARRAY[]::varchar[]
			) AS categories
		FROM projects p
		LEFT JOIN users u ON u.id = p.author_id
		WHERE p.id = $1
	`, id, userID).Scan(
		&project.ID, &project.Title, &project.Description, &project.LongDescription,
		&project.GithubURL, &project.DemoURL, &project.ImageURL,
		&project.Stars, &project.Starred, &project.Contributors, &project.Status, &project.CreatedAt,
		&project.Author, &project.AuthorAvatar,
		&project.Categories,
	)

	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "proje bulunamadı"})
		return
	}

	if project.Categories == nil {
		project.Categories = []string{}
	}

	c.JSON(http.StatusOK, project)
}

// Yeni proje oluşturur (giriş yapmış kullanıcı için, kategori desteğiyle).
func handleCreateProject(c *gin.Context) {
	userID := c.GetInt("user_id")

	var input struct {
		Title           string `json:"title"`
		Description     string `json:"description"`
		LongDescription string `json:"longDescription"`
		GithubURL       string `json:"githubUrl"`
		DemoURL         string `json:"demoUrl"`
		ImageURL        string `json:"imageUrl"`
		CategoryIDs     []int  `json:"categoryIds"`
	}

	if err := c.ShouldBindJSON(&input); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "geçersiz veri"})
		return
	}

	// Zorunlu alanlar
	if input.Title == "" || input.Description == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "title ve description zorunlu"})
		return
	}

	// Uzunluk sınırları
	if len(input.Title) > 80 {
		c.JSON(http.StatusBadRequest, gin.H{"error": "title en fazla 80 karakter olabilir"})
		return
	}
	if len(input.Description) > 150 {
		c.JSON(http.StatusBadRequest, gin.H{"error": "description en fazla 150 karakter olabilir"})
		return
	}
	if len(input.LongDescription) > 500 {
		c.JSON(http.StatusBadRequest, gin.H{"error": "longDescription en fazla 500 karakter olabilir"})
		return
	}
	if len(input.GithubURL) > 200 {
		c.JSON(http.StatusBadRequest, gin.H{"error": "githubUrl en fazla 200 karakter olabilir"})
		return
	}
	if len(input.DemoURL) > 200 {
		c.JSON(http.StatusBadRequest, gin.H{"error": "demoUrl en fazla 200 karakter olabilir"})
		return
	}
	if len(input.ImageURL) > 300 {
		c.JSON(http.StatusBadRequest, gin.H{"error": "imageUrl en fazla 300 karakter olabilir"})
		return
	}

	// URL formatı
	if input.GithubURL != "" && !strings.HasPrefix(input.GithubURL, "http") {
		c.JSON(http.StatusBadRequest, gin.H{"error": "githubUrl geçersiz"})
		return
	}
	if input.DemoURL != "" && !strings.HasPrefix(input.DemoURL, "http") {
		c.JSON(http.StatusBadRequest, gin.H{"error": "demoUrl geçersiz"})
		return
	}
	if input.ImageURL != "" && !strings.HasPrefix(input.ImageURL, "http") {
		c.JSON(http.StatusBadRequest, gin.H{"error": "imageUrl geçersiz"})
		return
	}

	// Kategori sayısı sınırı (max 5)
	if len(input.CategoryIDs) > 5 {
		c.JSON(http.StatusBadRequest, gin.H{"error": "en fazla 5 kategori seçilebilir"})
		return
	}

	// Projeyi oluştur
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

	// Kategorileri ekle
	if len(input.CategoryIDs) > 0 {
		for _, catID := range input.CategoryIDs {
			_, err := db.Exec(context.Background(), `
				INSERT INTO project_categories (project_id, category_id)
				VALUES ($1, $2)
				ON CONFLICT DO NOTHING
			`, projectID, catID)
			if err != nil {
				continue
			}
		}
	}

	c.JSON(http.StatusCreated, gin.H{
		"id":      projectID,
		"message": "proje oluşturuldu",
	})
}

// Kullanıcının kendi projelerini yıldız ve katkıcı bilgisiyle döner.
func handleMyProjects(c *gin.Context) {
	userID := c.GetInt("user_id")

	rows, err := db.Query(context.Background(), `
		SELECT 
			p.id, p.title, p.description,
			COALESCE(p.image_url, '') AS image_url,
			(SELECT COUNT(*) FROM project_stars WHERE project_id = p.id) AS stars,
			p.contributors,
			COALESCE(
				ARRAY_AGG(c.name ORDER BY c.name) FILTER (WHERE c.id IS NOT NULL),
				ARRAY[]::varchar[]
			) AS categories
		FROM projects p
		LEFT JOIN project_categories pc ON pc.project_id = p.id
		LEFT JOIN categories c ON c.id = pc.category_id
		WHERE p.author_id = $1
		GROUP BY 
			p.id, p.title, p.description, p.image_url,
			p.contributors
		ORDER BY p.created_at DESC
	`, userID)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	defer rows.Close()

	projects := []map[string]interface{}{}
	for rows.Next() {
		var id, stars, contributors int
		var title, description, imageURL string
		var categories []string

		if err := rows.Scan(&id, &title, &description, &imageURL, &stars, &contributors, &categories); err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
			return
		}

		if categories == nil {
			categories = []string{}
		}

		projects = append(projects, map[string]interface{}{
			"id":           id,
			"title":        title,
			"description":  description,
			"imageUrl":     imageURL,
			"stars":        stars,
			"contributors": contributors,
			"categories":   categories,
		})
	}

	c.JSON(http.StatusOK, projects)
}

// Kullanıcının bir projeyi yıldızlamasını sağlar.
func handleStarProject(c *gin.Context) {
	userID := c.GetInt("user_id")
	projectID := c.Param("id")

	_, err := db.Exec(context.Background(), `
		INSERT INTO project_stars (project_id, user_id)
		VALUES ($1, $2)
		ON CONFLICT DO NOTHING
	`, projectID, userID)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	var count int
	db.QueryRow(context.Background(),
		`SELECT COUNT(*) FROM project_stars WHERE project_id = $1`, projectID,
	).Scan(&count)

	c.JSON(http.StatusOK, gin.H{"stars": count, "starred": true})
}

// Kullanıcının yıldızını geri almasını sağlar.
func handleUnstarProject(c *gin.Context) {
	userID := c.GetInt("user_id")
	projectID := c.Param("id")

	_, err := db.Exec(context.Background(), `
		DELETE FROM project_stars
		WHERE project_id = $1 AND user_id = $2
	`, projectID, userID)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	var count int
	db.QueryRow(context.Background(),
		`SELECT COUNT(*) FROM project_stars WHERE project_id = $1`, projectID,
	).Scan(&count)

	c.JSON(http.StatusOK, gin.H{"stars": count, "starred": false})
}

// Kullanıcının kendi projelerini TÜM detaylarıyla döner (kategoriler dahil).
func handleMyProjectsDetailed(c *gin.Context) {
	userID := c.GetInt("user_id")

	rows, err := db.Query(context.Background(), `
		SELECT 
			p.id, p.title, p.description,
			COALESCE(p.long_description, '') AS long_description,
			COALESCE(p.github_url, '') AS github_url,
			COALESCE(p.demo_url, '') AS demo_url,
			COALESCE(p.image_url, '') AS image_url,
			(SELECT COUNT(*) FROM project_stars WHERE project_id = p.id) AS stars,
			p.contributors, p.status, p.created_at,
			COALESCE(u.username, '') AS author,
			COALESCE(u.avatar_url, '') AS author_avatar,
			COALESCE(
				ARRAY_AGG(c.name ORDER BY c.name) FILTER (WHERE c.id IS NOT NULL),
				ARRAY[]::varchar[]
			) AS categories
		FROM projects p
		LEFT JOIN users u ON u.id = p.author_id
		LEFT JOIN project_categories pc ON pc.project_id = p.id
		LEFT JOIN categories c ON c.id = pc.category_id
		WHERE p.author_id = $1
		GROUP BY 
			p.id, p.title, p.description, p.long_description,
			p.github_url, p.demo_url, p.image_url,
			p.contributors, p.status, p.created_at,
			u.username, u.avatar_url
		ORDER BY p.created_at DESC
	`, userID)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	defer rows.Close()

	projects := []map[string]interface{}{}
	for rows.Next() {
		var id, stars, contributors int
		var title, description, longDesc, status, author, authorAvatar, githubURL, demoURL, imageURL string
		var createdAt time.Time
		var categories []string

		err := rows.Scan(
			&id, &title, &description, &longDesc,
			&githubURL, &demoURL, &imageURL,
			&stars, &contributors, &status, &createdAt,
			&author, &authorAvatar,
			&categories,
		)
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
			return
		}

		if categories == nil {
			categories = []string{}
		}

		projects = append(projects, map[string]interface{}{
			"id":              id,
			"title":           title,
			"description":     description,
			"longDescription": longDesc,
			"githubUrl":       githubURL,
			"demoUrl":         demoURL,
			"imageUrl":        imageURL,
			"stars":           stars,
			"contributors":    contributors,
			"status":          status,
			"createdAt":       createdAt,
			"author":          author,
			"authorAvatar":    authorAvatar,
			"categories":      categories,
		})
	}

	c.JSON(http.StatusOK, projects)
}
