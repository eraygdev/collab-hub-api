package main

import (
	"net/http"
	"strconv"
	"strings"
	"unicode/utf8"

	"github.com/gin-gonic/gin"
)

const MaxContributorMessageLen = 300

// handleJoinProject projeye katılma başvurusu gönderir.
func handleJoinProject(c *gin.Context) {
	userID := c.GetInt("user_id")
	ctx := c.Request.Context()

	projectID, err := strconv.Atoi(c.Param("id"))
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid_project_id"})
		return
	}

	// [YENİ] Tek sorguda author + max_contributors + mevcut katkıcı sayısı
	var authorID, maxContributors, currentCount int
	err = db.QueryRow(ctx, `
		SELECT 
			p.author_id,
			p.max_contributors,
			(SELECT COUNT(*) FROM project_contributors WHERE project_id = p.id AND status = 'approved')
		FROM projects p WHERE p.id = $1
	`, projectID).Scan(&authorID, &maxContributors, &currentCount)
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "project_not_found"})
		return
	}

	if authorID == userID {
		c.JSON(http.StatusForbidden, gin.H{"error": "cannot_join_own_project"})
		return
	}

	// [YENİ] Katkıcı limiti kontrolü
	if currentCount >= maxContributors {
		c.JSON(http.StatusForbidden, gin.H{"error": "contributor_limit_reached"})
		return
	}

	var isPremium bool
	db.QueryRow(ctx, `SELECT is_premium FROM users WHERE id = $1`, userID).Scan(&isPremium)

	var input struct {
		Message string `json:"message"`
	}
	c.ShouldBindJSON(&input)

	message := ""
	if isPremium {
		message = strings.TrimSpace(input.Message)
		if utf8.RuneCountInString(message) > MaxContributorMessageLen {
			c.JSON(http.StatusBadRequest, gin.H{"error": "message_too_long"})
			return
		}
	}

	var existingID int
	var existingStatus string
	err = db.QueryRow(ctx,
		`SELECT id, status FROM project_contributors WHERE project_id = $1 AND user_id = $2`,
		projectID, userID,
	).Scan(&existingID, &existingStatus)

	if err == nil {
		if existingStatus == "pending" {
			c.JSON(http.StatusConflict, gin.H{"error": "already_pending"})
			return
		}
		if existingStatus == "approved" {
			c.JSON(http.StatusConflict, gin.H{"error": "already_contributor"})
			return
		}
		_, err = db.Exec(ctx, `
			UPDATE project_contributors
			SET status = 'pending', message = $1, created_at = NOW(), approved_at = NULL
			WHERE id = $2
		`, nullableString(message), existingID)
		if err != nil {
			serverError(c, err, "")
			return
		}
		c.JSON(http.StatusOK, gin.H{"message": "application_resent", "status": "pending"})
		return
	}

	var msg interface{} = nil
	if message != "" {
		msg = message
	}

	_, err = db.Exec(ctx, `
		INSERT INTO project_contributors (project_id, user_id, status, message)
		VALUES ($1, $2, 'pending', $3)
	`, projectID, userID, msg)
	if err != nil {
		serverError(c, err, "")
		return
	}

	auditLog(c, userID, "join_request", "project", projectID)

	c.JSON(http.StatusCreated, gin.H{
		"message": "application_sent",
		"status":  "pending",
	})
}

// handleLeaveProject kullanıcının kendi isteğiyle projeden ayrılmasını sağlar.
func handleLeaveProject(c *gin.Context) {
	userID := c.GetInt("user_id")
	ctx := c.Request.Context()

	projectID, err := strconv.Atoi(c.Param("id"))
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid_project_id"})
		return
	}

	var authorID int
	err = db.QueryRow(ctx,
		`SELECT author_id FROM projects WHERE id = $1`, projectID,
	).Scan(&authorID)
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "project_not_found"})
		return
	}

	if authorID == userID {
		c.JSON(http.StatusForbidden, gin.H{"error": "owner_cannot_leave"})
		return
	}

	var existingID int
	err = db.QueryRow(ctx, `
		SELECT id FROM project_contributors
		WHERE project_id = $1 AND user_id = $2 AND status = 'approved'
	`, projectID, userID).Scan(&existingID)

	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "not_a_contributor"})
		return
	}

	_, err = db.Exec(ctx,
		`DELETE FROM project_contributors WHERE id = $1`, existingID)
	if err != nil {
		serverError(c, err, "")
		return
	}

	auditLog(c, userID, "leave", "project", projectID)

	c.JSON(http.StatusOK, gin.H{"message": "left_project"})
}

// handleMyJoinStatus kullanıcının bu projedeki başvuru durumunu döner.
func handleMyJoinStatus(c *gin.Context) {
	userID := c.GetInt("user_id")

	projectID, err := strconv.Atoi(c.Param("id"))
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid_project_id"})
		return
	}

	var status string
	err = db.QueryRow(c.Request.Context(),
		`SELECT status FROM project_contributors WHERE project_id = $1 AND user_id = $2`,
		projectID, userID,
	).Scan(&status)

	if err != nil {
		c.JSON(http.StatusOK, gin.H{"status": "none"})
		return
	}

	c.JSON(http.StatusOK, gin.H{"status": status})
}

// handleContributorRequests proje sahibinin bekleyen başvurularını döner.
func handleContributorRequests(c *gin.Context) {
	userID := c.GetInt("user_id")

	rows, err := db.Query(c.Request.Context(), `
		SELECT 
			pc.id, pc.project_id, p.title AS project_title,
			pc.user_id, u.username, COALESCE(u.avatar_url, '') AS avatar_url,
			COALESCE(pc.message, '') AS message, pc.created_at
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

		if err := rows.Scan(&id, &projectID, &projectTitle, &requesterID, &username, &avatarURL, &message, &createdAt); err != nil {
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

// handleApproveRequest başvuruyu onaylar.
func handleApproveRequest(c *gin.Context) {
	handleRequestAction(c, "approve")
}

// handleRejectRequest başvuruyu reddeder.
func handleRejectRequest(c *gin.Context) {
	handleRequestAction(c, "reject")
}

// handleRequestAction approve/reject ortak işlemini yürütür.
func handleRequestAction(c *gin.Context, action string) {
	userID := c.GetInt("user_id")
	ctx := c.Request.Context()

	requestID, err := strconv.Atoi(c.Param("id"))
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid_request_id"})
		return
	}

	var projectAuthorID int
	err = db.QueryRow(ctx, `
		SELECT p.author_id
		FROM project_contributors pc
		JOIN projects p ON p.id = pc.project_id
		WHERE pc.id = $1 AND pc.status = 'pending'
	`, requestID).Scan(&projectAuthorID)

	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "request_not_found"})
		return
	}

	if projectAuthorID != userID {
		c.JSON(http.StatusForbidden, gin.H{"error": "forbidden"})
		return
	}

	if action == "approve" {
		_, err = db.Exec(ctx, `
			UPDATE project_contributors
			SET status = 'approved', approved_at = NOW()
			WHERE id = $1
		`, requestID)
	} else {
		_, err = db.Exec(ctx, `
			UPDATE project_contributors
			SET status = 'rejected'
			WHERE id = $1
		`, requestID)
	}

	if err != nil {
		serverError(c, err, "")
		return
	}

	c.JSON(http.StatusOK, gin.H{"message": "action_completed"})
}

// handleRemoveContributor katkıcıyı projeden çıkarır (sadece proje sahibi).
func handleRemoveContributor(c *gin.Context) {
	userID := c.GetInt("user_id")
	ctx := c.Request.Context()

	projectID, err := strconv.Atoi(c.Param("id"))
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid_project_id"})
		return
	}
	contributorUserID, err := strconv.Atoi(c.Param("userId"))
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid_user_id"})
		return
	}

	var authorID int
	err = db.QueryRow(ctx,
		`SELECT author_id FROM projects WHERE id = $1`, projectID,
	).Scan(&authorID)
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "project_not_found"})
		return
	}
	if authorID != userID {
		c.JSON(http.StatusForbidden, gin.H{"error": "forbidden"})
		return
	}

	_, err = db.Exec(ctx, `
		DELETE FROM project_contributors
		WHERE project_id = $1 AND user_id = $2
	`, projectID, contributorUserID)
	if err != nil {
		serverError(c, err, "")
		return
	}

	c.JSON(http.StatusOK, gin.H{"message": "contributor_removed"})
}

// handleMyContributions kullanıcının katkıcı olduğu projeleri döner.
func handleMyContributions(c *gin.Context) {
	userID := c.GetInt("user_id")

	rows, err := db.Query(c.Request.Context(), `
		SELECT 
			p.id, p.title, p.description,
			COALESCE(p.image_url, '') AS image_url,
			(SELECT COUNT(*) FROM project_stars WHERE project_id = p.id) AS stars,
			COALESCE(u.username, '') AS author,
			COALESCE(u.avatar_url, '') AS author_avatar,
			COALESCE((SELECT ARRAY_AGG(c.name ORDER BY c.name) FROM project_categories pc JOIN categories c ON c.id = pc.category_id WHERE pc.project_id = p.id), ARRAY[]::varchar[]) AS categories,
			COALESCE((SELECT ARRAY_AGG(pc.category_id ORDER BY pc.category_id) FROM project_categories pc WHERE pc.project_id = p.id), ARRAY[]::int[]) AS category_ids
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
		var categories []string
		var categoryIDs []int

		if err := rows.Scan(&id, &title, &description, &imageURL, &stars, &author, &authorAvatar, &categories, &categoryIDs); err != nil {
			serverError(c, err, "")
			return
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
			"author":       author,
			"authorAvatar": authorAvatar,
			"categories":   categories,
			"categoryIds":  categoryIDs,
		})
	}

	c.JSON(http.StatusOK, projects)
}

// nullableString boş string için nil döner (DB'ye NULL yazmak için).
func nullableString(s string) interface{} {
	if s == "" {
		return nil
	}
	return s
}
