package app

import (
	"fmt"
	"net/http"
	"strconv"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/gin-gonic/gin"
)

// handleListProjects projeleri arama + kategori + sayfalama ile listeler.
func handleListProjects(c *gin.Context) {
	userID := c.GetInt("user_id")
	ctx := c.Request.Context()

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

	search := strings.TrimSpace(c.Query("search"))

	if err := validateText("search", search, MaxSearchLen, TextRegex); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	if search != "" {
		search = strings.ReplaceAll(search, "\\", "\\\\")
		search = strings.ReplaceAll(search, "%", "\\%")
		search = strings.ReplaceAll(search, "_", "\\_")
	}

	var categoryIDs []int
	if catQuery := c.Query("categoryIds"); catQuery != "" {
		for _, s := range strings.Split(catQuery, ",") {
			idStr := strings.TrimSpace(s)
			if idStr == "" {
				continue
			}
			id, err := strconv.Atoi(idStr)
			if err != nil || id <= 0 {
				c.JSON(http.StatusBadRequest, gin.H{"error": "invalid_category_id"})
				return
			}
			categoryIDs = append(categoryIDs, id)
		}
	}

	if len(categoryIDs) > 20 {
		c.JSON(http.StatusBadRequest, gin.H{"error": "too_many_categories"})
		return
	}

	matchMode := c.DefaultQuery("mode", "or")
	sortParam := c.DefaultQuery("sort", "popular")
	conditions := []string{}
	args := []interface{}{userID}
	argIdx := 2

	if search != "" {
		conditions = append(conditions, fmt.Sprintf(
			"(p.title ILIKE $%d ESCAPE '\\' OR p.description ILIKE $%d ESCAPE '\\')",
			argIdx, argIdx,
		))
		args = append(args, "%"+search+"%")
		argIdx++
	}

	if len(categoryIDs) > 0 {
		if matchMode == "and" {
			for _, catID := range categoryIDs {
				conditions = append(conditions, fmt.Sprintf(`
					EXISTS (
						SELECT 1 FROM project_categories pc2
						WHERE pc2.project_id = p.id AND pc2.category_id = $%d
					)
				`, argIdx))
				args = append(args, catID)
				argIdx++
			}
		} else {
			conditions = append(conditions, fmt.Sprintf(`
				EXISTS (
					SELECT 1 FROM project_categories pc2
					WHERE pc2.project_id = p.id AND pc2.category_id = ANY($%d)
				)
			`, argIdx))
			args = append(args, categoryIDs)
			argIdx++
		}
	}

	whereClause := ""
	if len(conditions) > 0 {
		whereClause = "WHERE " + strings.Join(conditions, " AND ")
	}

	limitIdx := argIdx
	offsetIdx := argIdx + 1
	args = append(args, limit, offset)

	orderClause := "p.created_at DESC, p.id DESC" // default: newest

	if sortParam == "popular" {
		orderClause = "COALESCE(s.stars, 0) DESC, p.created_at DESC, p.id DESC"
	} else if sortParam == "trending" {
		orderClause = `
			(SELECT COUNT(*) FROM project_stars
			 WHERE project_id = p.id
			   AND created_at > NOW() - INTERVAL '24 hours') DESC,
			COALESCE(s.stars, 0) DESC,
			p.created_at DESC,
			p.id DESC
		`
	} else if sortParam == "hot" {
		orderClause = `
			(COALESCE(s.stars, 0) * 1.0 + COALESCE(c.contributors, 0) * 3.0 + 1)
			/ POWER(EXTRACT(EPOCH FROM (NOW() - p.created_at)) / 3600.0 + 24, 1.5)
			DESC, p.id DESC
		`
	}

	query := fmt.Sprintf(`
		SELECT 
			p.id, p.title, p.description,
			COALESCE(p.image_url, '') AS image_url,
			COALESCE(s.stars, 0) AS stars,
			COALESCE(s.starred, false) AS starred,
			COALESCE(u.username, '') AS author,
			COALESCE(u.avatar_url, '') AS author_avatar,
			COALESCE(u.id, 0) AS author_id,
			COALESCE(cat.names, ARRAY[]::varchar[]) AS categories,
			COALESCE(cat.ids, ARRAY[]::int[]) AS category_ids
		FROM projects p
		LEFT JOIN users u ON u.id = p.author_id
		LEFT JOIN LATERAL (
			SELECT COUNT(*) AS stars, BOOL_OR(user_id = $1) AS starred
			FROM project_stars WHERE project_id = p.id
		) s ON true
			LEFT JOIN LATERAL (
		SELECT COUNT(*) AS contributors
		FROM project_contributors
		WHERE project_id = p.id AND status = 'approved'
		) c ON true
		LEFT JOIN LATERAL (
			SELECT 
				ARRAY_AGG(c.name ORDER BY c.name) AS names,
				ARRAY_AGG(pc.category_id ORDER BY pc.category_id) AS ids
			FROM project_categories pc
			JOIN categories c ON c.id = pc.category_id
			WHERE pc.project_id = p.id
		) cat ON true
		%s
		ORDER BY %s
		LIMIT $%d OFFSET $%d
	`, whereClause, orderClause, limitIdx, offsetIdx)

	rows, err := db.Query(ctx, query, args...)
	if err != nil {
		serverError(c, err, "")
		return
	}
	defer rows.Close()

	projects := []map[string]interface{}{}
	for rows.Next() {
		var id, stars, authorID int
		var starred bool
		var title, description, imageURL, author, authorAvatar string
		var categories []string
		var catIDs []int

		err := rows.Scan(&id, &title, &description, &imageURL, &stars, &starred, &author, &authorAvatar, &authorID, &categories, &catIDs)
		if err != nil {
			serverError(c, err, "")
			return
		}

		if categories == nil {
			categories = []string{}
		}
		if catIDs == nil {
			catIDs = []int{}
		}

		projects = append(projects, map[string]interface{}{
			"id":           id,
			"title":        title,
			"description":  description,
			"imageUrl":     imageURL,
			"stars":        stars,
			"starred":      starred,
			"author":       author,
			"authorAvatar": authorAvatar,
			"authorId":     authorID,
			"categories":   categories,
			"categoryIds":  catIDs,
		})
	}

	c.JSON(http.StatusOK, projects)
}

// handleGetProject tek projenin tüm detayını döner.
func handleGetProject(c *gin.Context) {
	id, err := strconv.Atoi(c.Param("id"))
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid_project_id"})
		return
	}

	userID := c.GetInt("user_id")
	ctx := c.Request.Context()

	var project struct {
		ID               int                      `json:"id"`
		Title            string                   `json:"title"`
		Description      string                   `json:"description"`
		LongDescription  string                   `json:"longDescription"`
		GithubURL        string                   `json:"githubUrl"`
		DemoURL          string                   `json:"demoUrl"`
		ImageURL         string                   `json:"imageUrl"`
		Stars            int                      `json:"stars"`
		Starred          bool                     `json:"starred"`
		Contributors     int                      `json:"contributors"`
		MaxContributors  int                      `json:"maxContributors"` // [YENİ]
		Status           string                   `json:"status"`
		CreatedAt        time.Time                `json:"createdAt"`
		Author           string                   `json:"author"`
		AuthorAvatar     string                   `json:"authorAvatar"`
		AuthorID         int                      `json:"authorId"`
		Categories       []string                 `json:"categories"`
		CategoryIDs      []int                    `json:"categoryIds"`
		ContributorsList []map[string]interface{} `json:"contributorsList"`
	}

	err = db.QueryRow(ctx, `
		SELECT 
			p.id, p.title, p.description,
			COALESCE(p.long_description, '') AS long_description,
			COALESCE(p.github_url, '') AS github_url,
			COALESCE(p.demo_url, '') AS demo_url,
			COALESCE(p.image_url, '') AS image_url,
			(SELECT COUNT(*) FROM project_stars WHERE project_id = p.id) AS stars,
			EXISTS(SELECT 1 FROM project_stars WHERE project_id = p.id AND user_id = $2) AS starred,
			(SELECT COUNT(*) FROM project_contributors WHERE project_id = p.id AND status = 'approved') AS contributors,
			p.max_contributors,
			p.status, p.created_at,
			COALESCE(u.username, '') AS author,
			COALESCE(u.avatar_url, '') AS author_avatar,
			COALESCE(u.id, 0) AS author_id,
			COALESCE((SELECT ARRAY_AGG(c.name ORDER BY c.name) FROM project_categories pc JOIN categories c ON c.id = pc.category_id WHERE pc.project_id = p.id), ARRAY[]::varchar[]) AS categories,
			COALESCE((SELECT ARRAY_AGG(pc.category_id ORDER BY pc.category_id) FROM project_categories pc WHERE pc.project_id = p.id), ARRAY[]::int[]) AS category_ids
		FROM projects p
		LEFT JOIN users u ON u.id = p.author_id
		WHERE p.id = $1
	`, id, userID).Scan(
		&project.ID, &project.Title, &project.Description, &project.LongDescription,
		&project.GithubURL, &project.DemoURL, &project.ImageURL,
		&project.Stars, &project.Starred, &project.Contributors, &project.MaxContributors, &project.Status, &project.CreatedAt,
		&project.Author, &project.AuthorAvatar, &project.AuthorID,
		&project.Categories, &project.CategoryIDs,
	)

	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "project_not_found"})
		return
	}

	if project.Categories == nil {
		project.Categories = []string{}
	}
	if project.CategoryIDs == nil {
		project.CategoryIDs = []int{}
	}

	contribRows, err := db.Query(ctx, `
		SELECT u.id AS user_id, u.username, COALESCE(u.avatar_url, '') AS avatar_url
		FROM project_contributors pc
		JOIN users u ON u.id = pc.user_id
		WHERE pc.project_id = $1 AND pc.status = 'approved'
		ORDER BY pc.approved_at ASC
	`, id)

	if err == nil {
		defer contribRows.Close()
		contributors := []map[string]interface{}{}
		for contribRows.Next() {
			var uid int
			var username, avatarURL string
			if err := contribRows.Scan(&uid, &username, &avatarURL); err == nil {
				contributors = append(contributors, map[string]interface{}{
					"user_id":    uid,
					"username":   username,
					"avatar_url": avatarURL,
				})
			}
		}
		project.ContributorsList = contributors
	} else {
		project.ContributorsList = []map[string]interface{}{}
	}

	c.JSON(http.StatusOK, project)
}

// handleCreateProject yeni proje oluşturur. Proje limiti kontrolü içerir.
func handleCreateProject(c *gin.Context) {
	userID := c.GetInt("user_id")
	ctx := c.Request.Context()

	var input struct {
		Title           string `json:"title"`
		Description     string `json:"description"`
		LongDescription string `json:"longDescription"`
		GithubURL       string `json:"githubUrl"`
		DemoURL         string `json:"demoUrl"`
		ImageURL        string `json:"imageUrl"`
		CategoryIDs     []int  `json:"categoryIds"`
		MaxContributors int    `json:"maxContributors"` // [YENİ]
	}

	if err := c.ShouldBindJSON(&input); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid_data"})
		return
	}

	if input.Title == "" || input.Description == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "title_and_description_required"})
		return
	}

	// [YENİ] Katkıcı limiti validasyonu
	if input.MaxContributors == 0 {
		input.MaxContributors = DefaultContributorLimit
	}
	if !isAllowedContributorLimit(input.MaxContributors) {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid_contributor_limit"})
		return
	}

	// Sanitize
	input.Title = sanitizeText(input.Title)
	input.Description = sanitizeText(input.Description)
	input.LongDescription = sanitizeText(input.LongDescription)

	// Metin validasyonları
	if err := validateTextMinMax("title", input.Title, MinTitleLen, MaxTitleLen, TitleRegex, true); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	if err := validateTextMinMax("description", input.Description, MinDescriptionLen, MaxDescriptionLen, TextRegex, true); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	if err := validateTextMinMax("longDescription", input.LongDescription, 0, MaxLongDescLen, TextRegex, false); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	// URL kontrolleri
	if utf8.RuneCountInString(input.GithubURL) > MaxGithubURLLen {
		c.JSON(http.StatusBadRequest, gin.H{"error": "github_url_too_long"})
		return
	}
	if utf8.RuneCountInString(input.DemoURL) > MaxDemoURLLen {
		c.JSON(http.StatusBadRequest, gin.H{"error": "demo_url_too_long"})
		return
	}
	if utf8.RuneCountInString(input.ImageURL) > MaxImageURLLen {
		c.JSON(http.StatusBadRequest, gin.H{"error": "image_url_too_long"})
		return
	}

	// GitHub URL zorunlu + public olmalı
	if input.GithubURL == "" || strings.TrimSpace(input.GithubURL) == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "github_url_required"})
		return
	}
	normalizedRepo := normalizeGithubRepoURL(input.GithubURL)
	if normalizedRepo == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid_github_url"})
		return
	}
	if err := checkGithubRepoPublic(normalizedRepo); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	input.GithubURL = normalizedRepo
	if input.DemoURL != "" &&
		!strings.HasPrefix(input.DemoURL, "http://") &&
		!strings.HasPrefix(input.DemoURL, "https://") {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid_demo_url"})
		return
	}
	if input.ImageURL != "" {
		normalized := normalizeGithubImageURL(input.ImageURL)
		if normalized == "" || !isValidGithubImageURL(normalized) {
			c.JSON(http.StatusBadRequest, gin.H{"error": "image_url_must_be_github"})
			return
		}
		if err := checkGithubImageSize(normalized); err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
			return
		}
		input.ImageURL = normalized
	}

	if len(input.CategoryIDs) > MaxCategories {
		c.JSON(http.StatusBadRequest, gin.H{"error": "too_many_categories"})
		return
	}

	if !validateCategoryIDs(ctx, input.CategoryIDs) {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid_category_id"})
		return
	}

	// Transaction
	tx, err := db.Begin(ctx)
	if err != nil {
		serverError(c, err, "")
		return
	}
	defer func() { _ = tx.Rollback(ctx) }()

	// Advisory lock
	if _, err := tx.Exec(ctx, `SELECT pg_advisory_xact_lock($1)`, userID); err != nil {
		serverError(c, err, "")
		return
	}

	// Proje limiti kontrolü
	var currentCount int
	err = tx.QueryRow(ctx,
		`SELECT COUNT(*) FROM projects WHERE author_id = $1`, userID,
	).Scan(&currentCount)
	if err != nil {
		serverError(c, err, "")
		return
	}

	if currentCount >= MaxProjectsPerUser {
		c.JSON(http.StatusForbidden, gin.H{
			"error":       "project_limit_reached",
			"current":     currentCount,
			"maxProjects": MaxProjectsPerUser,
		})
		return
	}

	var projectID int
	err = tx.QueryRow(ctx, `
		INSERT INTO projects (title, description, long_description, github_url, demo_url, image_url, author_id, max_contributors)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8)
		RETURNING id
	`,
		input.Title, input.Description, input.LongDescription,
		input.GithubURL, input.DemoURL, input.ImageURL, userID,
		input.MaxContributors, // [YENİ]
	).Scan(&projectID)
	if err != nil {
		if isUniqueViolationOn(err, "idx_projects_github_url_unique") {
			c.JSON(http.StatusConflict, gin.H{"error": "github_url_taken"})
			return
		}
		serverError(c, err, "")
		return
	}

	if len(input.CategoryIDs) > 0 {
		_, err = tx.Exec(ctx, `
			INSERT INTO project_categories (project_id, category_id)
			SELECT $1, UNNEST($2::int[])
			ON CONFLICT DO NOTHING
		`, projectID, input.CategoryIDs)
		if err != nil {
			serverError(c, err, "")
			return
		}
	}

	if err := tx.Commit(ctx); err != nil {
		serverError(c, err, "")
		return
	}

	auditLog(c, userID, "create", "project", projectID)

	c.JSON(http.StatusCreated, gin.H{
		"id":           projectID,
		"message":      "project_created",
		"projectCount": currentCount + 1,
		"maxProjects":  MaxProjectsPerUser,
	})
}

// handleMyProjects kullanıcının kendi projelerini döner.
func handleMyProjects(c *gin.Context) {
	userID := c.GetInt("user_id")
	ctx := c.Request.Context()

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

	sortParam := c.DefaultQuery("sort", "newest")

	orderClause := "p.created_at DESC, p.id DESC"
	if sortParam == "popular" {
		orderClause = "stars DESC, p.created_at DESC, p.id DESC"
	}

	var totalProjects, totalStars, totalContributors int
	_ = db.QueryRow(ctx, `
		SELECT 
			(SELECT COUNT(*) FROM projects WHERE author_id = $1),
			COALESCE((SELECT SUM((SELECT COUNT(*) FROM project_stars WHERE project_id = p.id)) FROM projects p WHERE p.author_id = $1), 0),
			COALESCE((SELECT COUNT(*) FROM project_contributors pc JOIN projects p ON p.id = pc.project_id WHERE p.author_id = $1 AND pc.status = 'approved'), 0)
	`, userID).Scan(&totalProjects, &totalStars, &totalContributors)

	query := fmt.Sprintf(`
		SELECT 
			p.id, p.title, p.description,
			COALESCE(p.image_url, '') AS image_url,
			(SELECT COUNT(*) FROM project_stars WHERE project_id = p.id) AS stars,
			(SELECT COUNT(*) FROM project_contributors WHERE project_id = p.id AND status = 'approved') AS contributors,
			p.created_at,
			COALESCE((SELECT ARRAY_AGG(c.name ORDER BY c.name) FROM project_categories pc JOIN categories c ON c.id = pc.category_id WHERE pc.project_id = p.id), ARRAY[]::varchar[]) AS categories,
			COALESCE((SELECT ARRAY_AGG(pc.category_id ORDER BY pc.category_id) FROM project_categories pc WHERE pc.project_id = p.id), ARRAY[]::int[]) AS category_ids
		FROM projects p
		WHERE p.author_id = $1
		ORDER BY %s
		LIMIT $2 OFFSET $3
	`, orderClause)

	rows, err := db.Query(ctx, query, userID, limit+1, offset)
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

		if err := rows.Scan(&id, &title, &description, &imageURL, &stars, &contributors, &projCreatedAt, &categories, &categoryIDs); err != nil {
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
		})
	}

	hasMore := count > limit

	c.JSON(http.StatusOK, gin.H{
		"projects":          projects,
		"hasMore":           hasMore,
		"totalProjects":     totalProjects,
		"totalStars":        totalStars,
		"totalContributors": totalContributors,
	})
}

// handleStarProject projeyi yıldızlar.
func handleStarProject(c *gin.Context) {
	userID := c.GetInt("user_id")
	ctx := c.Request.Context()

	projectID, err := strconv.Atoi(c.Param("id"))
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid_project_id"})
		return
	}

	var authorID int
	err = db.QueryRow(ctx, `SELECT author_id FROM projects WHERE id = $1`, projectID).Scan(&authorID)
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "project_not_found"})
		return
	}
	if authorID == userID {
		c.JSON(http.StatusForbidden, gin.H{"error": "cannot_star_own_project"})
		return
	}

	_, err = db.Exec(ctx, `
		INSERT INTO project_stars (project_id, user_id)
		VALUES ($1, $2)
		ON CONFLICT DO NOTHING
	`, projectID, userID)
	if err != nil {
		serverError(c, err, "")
		return
	}

	var count int
	_ = db.QueryRow(ctx, `SELECT COUNT(*) FROM project_stars WHERE project_id = $1`, projectID).Scan(&count)

	c.JSON(http.StatusOK, gin.H{"stars": count, "starred": true})
}

// handleUnstarProject yıldızı geri alır.
func handleUnstarProject(c *gin.Context) {
	userID := c.GetInt("user_id")
	ctx := c.Request.Context()

	projectID, err := strconv.Atoi(c.Param("id"))
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid_project_id"})
		return
	}

	_, err = db.Exec(ctx, `
		DELETE FROM project_stars WHERE project_id = $1 AND user_id = $2
	`, projectID, userID)
	if err != nil {
		serverError(c, err, "")
		return
	}

	var count int
	_ = db.QueryRow(ctx, `SELECT COUNT(*) FROM project_stars WHERE project_id = $1`, projectID).Scan(&count)

	c.JSON(http.StatusOK, gin.H{"stars": count, "starred": false})
}

// handleUpdateProject var olan projeyi günceller (sadece yazar).
func handleUpdateProject(c *gin.Context) {
	userID := c.GetInt("user_id")
	ctx := c.Request.Context()

	projectID, err := strconv.Atoi(c.Param("id"))
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid_project_id"})
		return
	}

	var authorID int
	err = db.QueryRow(ctx, `SELECT author_id FROM projects WHERE id = $1`, projectID).Scan(&authorID)
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "project_not_found"})
		return
	}
	if authorID != userID {
		c.JSON(http.StatusForbidden, gin.H{"error": "forbidden"})
		return
	}

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
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid_data"})
		return
	}

	if input.Title == "" || input.Description == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "title_and_description_required"})
		return
	}

	input.Title = sanitizeText(input.Title)
	input.Description = sanitizeText(input.Description)
	input.LongDescription = sanitizeText(input.LongDescription)

	// Metin validasyonları (min + max + regex)
	if err := validateTextMinMax("title", input.Title, MinTitleLen, MaxTitleLen, TitleRegex, true); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	if err := validateTextMinMax("description", input.Description, MinDescriptionLen, MaxDescriptionLen, TextRegex, true); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	if err := validateTextMinMax("longDescription", input.LongDescription, 0, MaxLongDescLen, TextRegex, false); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	if utf8.RuneCountInString(input.GithubURL) > MaxGithubURLLen {
		c.JSON(http.StatusBadRequest, gin.H{"error": "github_url_too_long"})
		return
	}
	if utf8.RuneCountInString(input.DemoURL) > MaxDemoURLLen {
		c.JSON(http.StatusBadRequest, gin.H{"error": "demo_url_too_long"})
		return
	}
	if utf8.RuneCountInString(input.ImageURL) > MaxImageURLLen {
		c.JSON(http.StatusBadRequest, gin.H{"error": "image_url_too_long"})
		return
	}

	// GitHub URL zorunlu + public olmalı
	if input.GithubURL == "" || strings.TrimSpace(input.GithubURL) == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "github_url_required"})
		return
	}
	normalizedRepo := normalizeGithubRepoURL(input.GithubURL)
	if normalizedRepo == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid_github_url"})
		return
	}
	if err := checkGithubRepoPublic(normalizedRepo); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	input.GithubURL = normalizedRepo
	if input.DemoURL != "" && !strings.HasPrefix(input.DemoURL, "http") {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid_demo_url"})
		return
	}
	if input.ImageURL != "" {
		normalized := normalizeGithubImageURL(input.ImageURL)
		if normalized == "" || !isValidGithubImageURL(normalized) {
			c.JSON(http.StatusBadRequest, gin.H{"error": "image_url_must_be_github"})
			return
		}
		if err := checkGithubImageSize(normalized); err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
			return
		}
		input.ImageURL = normalized
	}

	if len(input.CategoryIDs) > MaxCategories {
		c.JSON(http.StatusBadRequest, gin.H{"error": "too_many_categories"})
		return
	}

	if !validateCategoryIDs(ctx, input.CategoryIDs) {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid_category_id"})
		return
	}

	tx, err := db.Begin(ctx)
	if err != nil {
		serverError(c, err, "")
		return
	}
	defer func() { _ = tx.Rollback(ctx) }()

	_, err = tx.Exec(ctx, `
		UPDATE projects
		SET title = $1, description = $2, long_description = $3,
		    github_url = $4, demo_url = $5, image_url = $6,
		    updated_at = NOW()
		WHERE id = $7
	`, input.Title, input.Description, input.LongDescription,
		input.GithubURL, input.DemoURL, input.ImageURL, projectID)
	if err != nil {
		serverError(c, err, "")
		return
	}

	_, err = tx.Exec(ctx, `DELETE FROM project_categories WHERE project_id = $1`, projectID)
	if err != nil {
		if isUniqueViolationOn(err, "idx_projects_github_url_unique") {
			c.JSON(http.StatusConflict, gin.H{"error": "github_url_taken"})
			return
		}
		serverError(c, err, "")
		return
	}

	if len(input.CategoryIDs) > 0 {
		_, err = tx.Exec(ctx, `
			INSERT INTO project_categories (project_id, category_id)
			SELECT $1, UNNEST($2::int[])
			ON CONFLICT DO NOTHING
		`, projectID, input.CategoryIDs)
		if err != nil {
			serverError(c, err, "")
			return
		}
	}

	if err := tx.Commit(ctx); err != nil {
		serverError(c, err, "")
		return
	}

	auditLog(c, userID, "update", "project", projectID)

	c.JSON(http.StatusOK, gin.H{
		"id":      projectID,
		"message": "project_updated",
	})
}

// handleDeleteProject projeyi siler (sadece yazar). projectCount döner.
func handleDeleteProject(c *gin.Context) {
	userID := c.GetInt("user_id")
	ctx := c.Request.Context()

	projectID, err := strconv.Atoi(c.Param("id"))
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid_project_id"})
		return
	}

	var authorID int
	err = db.QueryRow(ctx, `SELECT author_id FROM projects WHERE id = $1`, projectID).Scan(&authorID)
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "project_not_found"})
		return
	}
	if authorID != userID {
		c.JSON(http.StatusForbidden, gin.H{"error": "forbidden"})
		return
	}

	_, err = db.Exec(ctx, `DELETE FROM projects WHERE id = $1`, projectID)
	if err != nil {
		serverError(c, err, "")
		return
	}

	auditLog(c, userID, "delete", "project", projectID)

	// Silme sonrası güncel proje sayısı
	var newCount int
	_ = db.QueryRow(ctx, `SELECT COUNT(*) FROM projects WHERE author_id = $1`, userID).Scan(&newCount)

	c.JSON(http.StatusOK, gin.H{
		"message":      "project_deleted",
		"projectCount": newCount,
		"maxProjects":  MaxProjectsPerUser,
	})
}
