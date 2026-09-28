package main

import (
	"context"
	"net/http"
	"strconv"
	"strings"
	"unicode/utf8"

	"github.com/gin-gonic/gin"
)

const MaxContributorMessageLen = 300

// Projeye katılma başvurusu gönderir.
func handleJoinProject(c *gin.Context) {
	userID := c.GetInt("user_id")

	projectID, err := strconv.Atoi(c.Param("id"))
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "geçersiz proje id"})
		return
	}

	// Proje var mı + yazarı kim?
	var authorID int
	err = db.QueryRow(context.Background(),
		`SELECT author_id FROM projects WHERE id = $1`, projectID,
	).Scan(&authorID)
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "proje bulunamadı"})
		return
	}

	// Kendi projesine katılamaz
	if authorID == userID {
		c.JSON(http.StatusForbidden, gin.H{"error": "kendi projenize katılamazsınız"})
		return
	}

	// Kullanıcı premium mu?
	var isPremium bool
	db.QueryRow(context.Background(),
		`SELECT is_premium FROM users WHERE id = $1`, userID,
	).Scan(&isPremium)

	// Body parse
	var input struct {
		Message string `json:"message"`
	}
	c.ShouldBindJSON(&input)

	// Premium değilse mesajı yok say
	message := ""
	if isPremium {
		message = strings.TrimSpace(input.Message)
		if utf8.RuneCountInString(message) > MaxContributorMessageLen {
			c.JSON(http.StatusBadRequest, gin.H{
				"error": "mesaj en fazla 300 karakter olabilir",
			})
			return
		}
	}

	// Zaten başvurmuş mu?
	var existingID int
	var existingStatus string
	err = db.QueryRow(context.Background(),
		`SELECT id, status FROM project_contributors WHERE project_id = $1 AND user_id = $2`,
		projectID, userID,
	).Scan(&existingID, &existingStatus)

	if err == nil {
		if existingStatus == "pending" {
			c.JSON(http.StatusConflict, gin.H{"error": "zaten başvurunuz onay bekliyor"})
			return
		}
		if existingStatus == "approved" {
			c.JSON(http.StatusConflict, gin.H{"error": "zaten bu projenin katkıcısısınız"})
			return
		}
		// rejected ise tekrar başvurabilir → UPDATE
		_, err = db.Exec(context.Background(), `
			UPDATE project_contributors
			SET status = 'pending', message = $1, created_at = NOW(), approved_at = NULL
			WHERE id = $2
		`, nullableString(message), existingID)
		if err != nil {
			serverError(c, err, "")
			return
		}
		c.JSON(http.StatusOK, gin.H{"message": "başvuru tekrar gönderildi", "status": "pending"})
		return
	}

	// Yeni başvuru
	var msg interface{} = nil
	if message != "" {
		msg = message
	}

	_, err = db.Exec(context.Background(), `
		INSERT INTO project_contributors (project_id, user_id, status, message)
		VALUES ($1, $2, 'pending', $3)
	`, projectID, userID, msg)
	if err != nil {
		serverError(c, err, "")
		return
	}

	auditLog(c, userID, "join_request", "project", projectID)

	c.JSON(http.StatusCreated, gin.H{
		"message": "başvuru gönderildi",
		"status":  "pending",
	})
}

// Kullanıcının bu projedeki başvuru durumunu döner.
func handleMyJoinStatus(c *gin.Context) {
	userID := c.GetInt("user_id")

	projectID, err := strconv.Atoi(c.Param("id"))
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "geçersiz proje id"})
		return
	}

	var status string
	err = db.QueryRow(context.Background(),
		`SELECT status FROM project_contributors WHERE project_id = $1 AND user_id = $2`,
		projectID, userID,
	).Scan(&status)

	if err != nil {
		c.JSON(http.StatusOK, gin.H{"status": "none"})
		return
	}

	c.JSON(http.StatusOK, gin.H{"status": status})
}

// Proje sahibinin aldığı bekleyen başvuruları döner.
func handleContributorRequests(c *gin.Context) {
	userID := c.GetInt("user_id")

	rows, err := db.Query(context.Background(), `
		SELECT 
			pc.id,
			pc.project_id,
			p.title AS project_title,
			pc.user_id,
			u.username,
			COALESCE(u.avatar_url, '') AS avatar_url,
			COALESCE(pc.message, '') AS message,
			pc.created_at
		FROM project_contributors pc
		JOIN projects p ON p.id = pc.project_id
		JOIN users u ON u.id = pc.user_id
		WHERE p.author_id = $1 AND pc.status = 'pending'
		ORDER BY pc.created_at DESC
	`, userID)
	if err != nil {
		serverError(c, err, "")
		return
	}
	defer rows.Close()

	requests := []map[string]interface{}{}
	for rows.Next() {
		var id, projectID, requesterID int
		var projectTitle, username, avatarURL, message string
		var createdAt interface{}

		if err := rows.Scan(
			&id, &projectID, &projectTitle, &requesterID,
			&username, &avatarURL, &message, &createdAt,
		); err != nil {
			serverError(c, err, "")
			return
		}

		requests = append(requests, map[string]interface{}{
			"id":            id,
			"project_id":    projectID,
			"project_title": projectTitle,
			"user_id":       requesterID,
			"username":      username,
			"avatar_url":    avatarURL,
			"message":       message,
			"created_at":    createdAt,
		})
	}

	c.JSON(http.StatusOK, requests)
}

// Başvuruyu onaylar.
func handleApproveRequest(c *gin.Context) {
	handleRequestAction(c, "approve")
}

// Başvuruyu reddeder.
func handleRejectRequest(c *gin.Context) {
	handleRequestAction(c, "reject")
}

func handleRequestAction(c *gin.Context, action string) {
	userID := c.GetInt("user_id")

	requestID, err := strconv.Atoi(c.Param("id"))
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "geçersiz başvuru id"})
		return
	}

	// Başvuruyu ve projenin sahibini bul
	var projectAuthorID int
	err = db.QueryRow(context.Background(), `
		SELECT p.author_id
		FROM project_contributors pc
		JOIN projects p ON p.id = pc.project_id
		WHERE pc.id = $1 AND pc.status = 'pending'
	`, requestID).Scan(&projectAuthorID)

	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "başvuru bulunamadı"})
		return
	}

	if projectAuthorID != userID {
		c.JSON(http.StatusForbidden, gin.H{"error": "bu başvuruyu yönetme yetkiniz yok"})
		return
	}

	if action == "approve" {
		_, err = db.Exec(context.Background(), `
			UPDATE project_contributors
			SET status = 'approved', approved_at = NOW()
			WHERE id = $1
		`, requestID)
	} else {
		_, err = db.Exec(context.Background(), `
			UPDATE project_contributors
			SET status = 'rejected'
			WHERE id = $1
		`, requestID)
	}

	if err != nil {
		serverError(c, err, "")
		return
	}

	c.JSON(http.StatusOK, gin.H{"message": "işlem tamam"})
}

// Katkıcıyı projeden çıkarır (sadece proje sahibi).
func handleRemoveContributor(c *gin.Context) {
	userID := c.GetInt("user_id")

	projectID, err := strconv.Atoi(c.Param("id"))
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "geçersiz proje id"})
		return
	}
	contributorUserID, err := strconv.Atoi(c.Param("userId"))
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "geçersiz kullanıcı id"})
		return
	}

	var authorID int
	err = db.QueryRow(context.Background(),
		`SELECT author_id FROM projects WHERE id = $1`, projectID,
	).Scan(&authorID)
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "proje bulunamadı"})
		return
	}
	if authorID != userID {
		c.JSON(http.StatusForbidden, gin.H{"error": "yetkiniz yok"})
		return
	}

	_, err = db.Exec(context.Background(), `
		DELETE FROM project_contributors
		WHERE project_id = $1 AND user_id = $2
	`, projectID, contributorUserID)
	if err != nil {
		serverError(c, err, "")
		return
	}

	c.JSON(http.StatusOK, gin.H{"message": "katkıcı çıkarıldı"})
}

// Kullanıcının katkıcı olduğu projeleri döner.
func handleMyContributions(c *gin.Context) {
	userID := c.GetInt("user_id")

	rows, err := db.Query(context.Background(), `
		SELECT 
			p.id, p.title, p.description,
			COALESCE(p.image_url, '') AS image_url,
			(SELECT COUNT(*) FROM project_stars WHERE project_id = p.id) AS stars,
			COALESCE(u.username, '') AS author,
			COALESCE(u.avatar_url, '') AS author_avatar
		FROM project_contributors pc
		JOIN projects p ON p.id = pc.project_id
		LEFT JOIN users u ON u.id = p.author_id
		WHERE pc.user_id = $1 AND pc.status = 'approved'
		ORDER BY pc.approved_at DESC
	`, userID)
	if err != nil {
		serverError(c, err, "")
		return
	}
	defer rows.Close()

	projects := []map[string]interface{}{}
	for rows.Next() {
		var id, stars int
		var title, description, imageURL, author, authorAvatar string

		if err := rows.Scan(&id, &title, &description, &imageURL, &stars, &author, &authorAvatar); err != nil {
			serverError(c, err, "")
			return
		}

		projects = append(projects, map[string]interface{}{
			"id":           id,
			"title":        title,
			"description":  description,
			"imageUrl":     imageURL,
			"stars":        stars,
			"author":       author,
			"authorAvatar": authorAvatar,
		})
	}

	c.JSON(http.StatusOK, projects)
}

// nullableString: boş string ise nil döner (DB'ye NULL yazmak için).
func nullableString(s string) interface{} {
	if s == "" {
		return nil
	}
	return s
}
