package main

import (
	"context"
	"net/http"
	"time"

	"github.com/gin-gonic/gin"
)

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
