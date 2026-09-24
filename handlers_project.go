package main

import (
	"context"
	"fmt"
	"net/http"
	"strconv"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/gin-gonic/gin"
)

// Tüm projeleri hafif liste olarak döner (arama + kategori ID + sayfalama).
func handleListProjects(c *gin.Context) {
	userID := c.GetInt("user_id")

	// Sayfalama
	limit := 20
	if l := c.Query("limit"); l != "" {
		if n, err := strconv.Atoi(l); err == nil && n > 0 && n <= 100 {
			limit = n
		}
	}
	offset := 0
	if o := c.Query("offset"); o != "" {
		if n, err := strconv.Atoi(o); err == nil && n >= 0 && n <= 10000 {
			offset = n
		}
	}

	// Arama
	search := strings.TrimSpace(c.Query("search"))

	// ✅ Rune bazlı sayım (Türkçe karakterler 1 sayılsın)
	if utf8.RuneCountInString(search) > MaxSearchLen {
		c.JSON(http.StatusBadRequest, gin.H{
			"error": fmt.Sprintf("arama metni en fazla %d karakter olabilir", MaxSearchLen),
		})
		return
	}

	if search != "" {
		search = strings.ReplaceAll(search, "\\", "\\\\")
		search = strings.ReplaceAll(search, "%", "\\%")
		search = strings.ReplaceAll(search, "_", "\\_")
	}

	// Kategoriler
	var categoryIDs []int
	if catQuery := c.Query("categoryIds"); catQuery != "" {
		for _, s := range strings.Split(catQuery, ",") {
			idStr := strings.TrimSpace(s)
			if idStr == "" {
				continue
			}
			id, err := strconv.Atoi(idStr)
			if err != nil || id <= 0 {
				c.JSON(http.StatusBadRequest, gin.H{"error": "geçersiz kategori id"})
				return
			}
			categoryIDs = append(categoryIDs, id)
		}
	}

	if len(categoryIDs) > 20 {
		c.JSON(http.StatusBadRequest, gin.H{"error": "en fazla 20 kategori seçilebilir"})
		return
	}

	matchMode := c.DefaultQuery("mode", "or")

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

	// LATERAL join: subquery'leri tek seferde topla
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
			SELECT 
				COUNT(*) AS stars,
				BOOL_OR(user_id = $1) AS starred
			FROM project_stars
			WHERE project_id = p.id
		) s ON true
		LEFT JOIN LATERAL (
			SELECT 
				ARRAY_AGG(c.name ORDER BY c.name) AS names,
				ARRAY_AGG(pc.category_id ORDER BY pc.category_id) AS ids
			FROM project_categories pc
			JOIN categories c ON c.id = pc.category_id
			WHERE pc.project_id = p.id
		) cat ON true
		%s
		ORDER BY p.created_at DESC
		LIMIT $%d OFFSET $%d
	`, whereClause, limitIdx, offsetIdx)

	rows, err := db.Query(context.Background(), query, args...)
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

		err := rows.Scan(
			&id, &title, &description, &imageURL,
			&stars, &starred, &author, &authorAvatar, &authorID,
			&categories, &catIDs,
		)
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

// Tek bir projeyi tam detaylarıyla döner.
func handleGetProject(c *gin.Context) {
	id, err := strconv.Atoi(c.Param("id"))
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "geçersiz proje id"})
		return
	}

	userID := c.GetInt("user_id")

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
		AuthorID        int       `json:"authorId"`
		Categories      []string  `json:"categories"`
		CategoryIDs     []int     `json:"categoryIds"`
	}

	err = db.QueryRow(context.Background(), `
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
			COALESCE(u.id, 0) AS author_id,
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
		LEFT JOIN users u ON u.id = p.author_id
		WHERE p.id = $1
	`, id, userID).Scan(
		&project.ID, &project.Title, &project.Description, &project.LongDescription,
		&project.GithubURL, &project.DemoURL, &project.ImageURL,
		&project.Stars, &project.Starred, &project.Contributors, &project.Status, &project.CreatedAt,
		&project.Author, &project.AuthorAvatar, &project.AuthorID,
		&project.Categories, &project.CategoryIDs,
	)

	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "proje bulunamadı"})
		return
	}

	if project.Categories == nil {
		project.Categories = []string{}
	}
	if project.CategoryIDs == nil {
		project.CategoryIDs = []int{}
	}

	c.JSON(http.StatusOK, project)
}

// Kategori ID'lerinin geçerli olduğunu doğrular.
func validateCategoryIDs(categoryIDs []int) bool {
	if len(categoryIDs) == 0 {
		return true
	}
	var count int
	err := db.QueryRow(context.Background(),
		`SELECT COUNT(*) FROM categories WHERE id = ANY($1)`,
		categoryIDs,
	).Scan(&count)
	if err != nil {
		return false
	}
	return count == len(categoryIDs)
}

// Yeni proje oluşturur.
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

	if input.Title == "" || input.Description == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "title ve description zorunlu"})
		return
	}

	input.Title = sanitizeText(input.Title)
	input.Description = sanitizeText(input.Description)
	input.LongDescription = sanitizeText(input.LongDescription)

	// ✅ Rune bazlı sayım (Türkçe karakterler 1 sayılsın)
	if utf8.RuneCountInString(input.Title) > MaxTitleLen {
		c.JSON(http.StatusBadRequest, gin.H{
			"error": fmt.Sprintf("title en fazla %d karakter olabilir", MaxTitleLen),
		})
		return
	}
	if utf8.RuneCountInString(input.Description) > MaxDescriptionLen {
		c.JSON(http.StatusBadRequest, gin.H{
			"error": fmt.Sprintf("description en fazla %d karakter olabilir", MaxDescriptionLen),
		})
		return
	}
	if utf8.RuneCountInString(input.LongDescription) > MaxLongDescLen {
		c.JSON(http.StatusBadRequest, gin.H{
			"error": fmt.Sprintf("longDescription en fazla %d karakter olabilir", MaxLongDescLen),
		})
		return
	}
	if utf8.RuneCountInString(input.GithubURL) > MaxGithubURLLen {
		c.JSON(http.StatusBadRequest, gin.H{
			"error": fmt.Sprintf("githubUrl en fazla %d karakter olabilir", MaxGithubURLLen),
		})
		return
	}
	if utf8.RuneCountInString(input.DemoURL) > MaxDemoURLLen {
		c.JSON(http.StatusBadRequest, gin.H{
			"error": fmt.Sprintf("demoUrl en fazla %d karakter olabilir", MaxDemoURLLen),
		})
		return
	}
	if utf8.RuneCountInString(input.ImageURL) > MaxImageURLLen {
		c.JSON(http.StatusBadRequest, gin.H{
			"error": fmt.Sprintf("imageUrl en fazla %d karakter olabilir", MaxImageURLLen),
		})
		return
	}

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

	if len(input.CategoryIDs) > MaxCategories {
		c.JSON(http.StatusBadRequest, gin.H{
			"error": fmt.Sprintf("en fazla %d kategori seçilebilir", MaxCategories),
		})
		return
	}

	if !validateCategoryIDs(input.CategoryIDs) {
		c.JSON(http.StatusBadRequest, gin.H{"error": "geçersiz kategori id"})
		return
	}

	tx, err := db.Begin(context.Background())
	if err != nil {
		serverError(c, err, "")
		return
	}
	defer tx.Rollback(context.Background())

	var projectID int
	err = tx.QueryRow(context.Background(), `
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
		serverError(c, err, "")
		return
	}

	if len(input.CategoryIDs) > 0 {
		_, err = tx.Exec(context.Background(), `
			INSERT INTO project_categories (project_id, category_id)
			SELECT $1, UNNEST($2::int[])
			ON CONFLICT DO NOTHING
		`, projectID, input.CategoryIDs)
		if err != nil {
			serverError(c, err, "")
			return
		}
	}

	if err := tx.Commit(context.Background()); err != nil {
		serverError(c, err, "")
		return
	}

	auditLog(c, userID, "create", "project", projectID)

	c.JSON(http.StatusCreated, gin.H{
		"id":      projectID,
		"message": "proje oluşturuldu",
	})
}

// Kullanıcının kendi projelerini döner.
func handleMyProjects(c *gin.Context) {
	userID := c.GetInt("user_id")

	rows, err := db.Query(context.Background(), `
		SELECT 
			p.id, p.title, p.description,
			COALESCE(p.image_url, '') AS image_url,
			(SELECT COUNT(*) FROM project_stars WHERE project_id = p.id) AS stars,
			p.contributors,
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
		ORDER BY p.created_at DESC
	`, userID)
	if err != nil {
		serverError(c, err, "")
		return
	}
	defer rows.Close()

	projects := []map[string]interface{}{}
	for rows.Next() {
		var id, stars, contributors int
		var title, description, imageURL string
		var categories []string
		var categoryIDs []int

		if err := rows.Scan(&id, &title, &description, &imageURL, &stars, &contributors, &categories, &categoryIDs); err != nil {
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
			"contributors": contributors,
			"categories":   categories,
			"categoryIds":  categoryIDs,
		})
	}

	c.JSON(http.StatusOK, projects)
}

// Yıldızla.
func handleStarProject(c *gin.Context) {
	userID := c.GetInt("user_id")

	projectID, err := strconv.Atoi(c.Param("id"))
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "geçersiz proje id"})
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
	if authorID == userID {
		c.JSON(http.StatusForbidden, gin.H{"error": "kendi projeni yıldızlayamazsın"})
		return
	}

	_, err = db.Exec(context.Background(), `
		INSERT INTO project_stars (project_id, user_id)
		VALUES ($1, $2)
		ON CONFLICT DO NOTHING
	`, projectID, userID)
	if err != nil {
		serverError(c, err, "")
		return
	}

	var count int
	db.QueryRow(context.Background(),
		`SELECT COUNT(*) FROM project_stars WHERE project_id = $1`, projectID,
	).Scan(&count)

	c.JSON(http.StatusOK, gin.H{"stars": count, "starred": true})
}

// Yıldız geri al.
func handleUnstarProject(c *gin.Context) {
	userID := c.GetInt("user_id")

	projectID, err := strconv.Atoi(c.Param("id"))
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "geçersiz proje id"})
		return
	}

	_, err = db.Exec(context.Background(), `
		DELETE FROM project_stars
		WHERE project_id = $1 AND user_id = $2
	`, projectID, userID)
	if err != nil {
		serverError(c, err, "")
		return
	}

	var count int
	db.QueryRow(context.Background(),
		`SELECT COUNT(*) FROM project_stars WHERE project_id = $1`, projectID,
	).Scan(&count)

	c.JSON(http.StatusOK, gin.H{"stars": count, "starred": false})
}

// Kullanıcının kendi projelerini TÜM detaylarıyla döner.
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
			COALESCE(u.id, 0) AS author_id,
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
		LEFT JOIN users u ON u.id = p.author_id
		WHERE p.author_id = $1
		ORDER BY p.created_at DESC
	`, userID)
	if err != nil {
		serverError(c, err, "")
		return
	}
	defer rows.Close()

	projects := []map[string]interface{}{}
	for rows.Next() {
		var id, stars, contributors, authorID int
		var title, description, longDesc, status, author, authorAvatar, githubURL, demoURL, imageURL string
		var createdAt time.Time
		var categories []string
		var categoryIDs []int

		err := rows.Scan(
			&id, &title, &description, &longDesc,
			&githubURL, &demoURL, &imageURL,
			&stars, &contributors, &status, &createdAt,
			&author, &authorAvatar, &authorID,
			&categories, &categoryIDs,
		)
		if err != nil {
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
			"authorId":        authorID,
			"categories":      categories,
			"categoryIds":     categoryIDs,
		})
	}

	c.JSON(http.StatusOK, projects)
}

// Var olan projeyi günceller (sadece yazar).
func handleUpdateProject(c *gin.Context) {
	userID := c.GetInt("user_id")

	projectID, err := strconv.Atoi(c.Param("id"))
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "geçersiz proje id"})
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
		c.JSON(http.StatusForbidden, gin.H{"error": "bu projeyi düzenleme yetkin yok"})
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
		c.JSON(http.StatusBadRequest, gin.H{"error": "geçersiz veri"})
		return
	}

	if input.Title == "" || input.Description == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "title ve description zorunlu"})
		return
	}

	input.Title = sanitizeText(input.Title)
	input.Description = sanitizeText(input.Description)
	input.LongDescription = sanitizeText(input.LongDescription)

	if utf8.RuneCountInString(input.Title) > MaxTitleLen {
		c.JSON(http.StatusBadRequest, gin.H{
			"error": fmt.Sprintf("title en fazla %d karakter olabilir", MaxTitleLen),
		})
		return
	}
	if utf8.RuneCountInString(input.Description) > MaxDescriptionLen {
		c.JSON(http.StatusBadRequest, gin.H{
			"error": fmt.Sprintf("description en fazla %d karakter olabilir", MaxDescriptionLen),
		})
		return
	}
	if utf8.RuneCountInString(input.LongDescription) > MaxLongDescLen {
		c.JSON(http.StatusBadRequest, gin.H{
			"error": fmt.Sprintf("longDescription en fazla %d karakter olabilir", MaxLongDescLen),
		})
		return
	}
	if utf8.RuneCountInString(input.GithubURL) > MaxGithubURLLen {
		c.JSON(http.StatusBadRequest, gin.H{
			"error": fmt.Sprintf("githubUrl en fazla %d karakter olabilir", MaxGithubURLLen),
		})
		return
	}
	if utf8.RuneCountInString(input.DemoURL) > MaxDemoURLLen {
		c.JSON(http.StatusBadRequest, gin.H{
			"error": fmt.Sprintf("demoUrl en fazla %d karakter olabilir", MaxDemoURLLen),
		})
		return
	}
	if utf8.RuneCountInString(input.ImageURL) > MaxImageURLLen {
		c.JSON(http.StatusBadRequest, gin.H{
			"error": fmt.Sprintf("imageUrl en fazla %d karakter olabilir", MaxImageURLLen),
		})
		return
	}

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

	if len(input.CategoryIDs) > MaxCategories {
		c.JSON(http.StatusBadRequest, gin.H{
			"error": fmt.Sprintf("en fazla %d kategori seçilebilir", MaxCategories),
		})
		return
	}

	if !validateCategoryIDs(input.CategoryIDs) {
		c.JSON(http.StatusBadRequest, gin.H{"error": "geçersiz kategori id"})
		return
	}

	tx, err := db.Begin(context.Background())
	if err != nil {
		serverError(c, err, "")
		return
	}
	defer tx.Rollback(context.Background())

	_, err = tx.Exec(context.Background(), `
		UPDATE projects
		SET title = $1,
		    description = $2,
		    long_description = $3,
		    github_url = $4,
		    demo_url = $5,
		    image_url = $6,
		    updated_at = NOW()
		WHERE id = $7
	`, input.Title, input.Description, input.LongDescription,
		input.GithubURL, input.DemoURL, input.ImageURL, projectID)
	if err != nil {
		serverError(c, err, "")
		return
	}

	_, err = tx.Exec(context.Background(),
		`DELETE FROM project_categories WHERE project_id = $1`, projectID)
	if err != nil {
		serverError(c, err, "")
		return
	}

	if len(input.CategoryIDs) > 0 {
		_, err = tx.Exec(context.Background(), `
			INSERT INTO project_categories (project_id, category_id)
			SELECT $1, UNNEST($2::int[])
			ON CONFLICT DO NOTHING
		`, projectID, input.CategoryIDs)
		if err != nil {
			serverError(c, err, "")
			return
		}
	}

	if err := tx.Commit(context.Background()); err != nil {
		serverError(c, err, "")
		return
	}

	auditLog(c, userID, "update", "project", projectID)

	c.JSON(http.StatusOK, gin.H{
		"id":      projectID,
		"message": "proje güncellendi",
	})
}

// Projeyi siler (sadece yazar).
func handleDeleteProject(c *gin.Context) {
	userID := c.GetInt("user_id")

	projectID, err := strconv.Atoi(c.Param("id"))
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "geçersiz proje id"})
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
		c.JSON(http.StatusForbidden, gin.H{"error": "bu projeyi silme yetkin yok"})
		return
	}

	_, err = db.Exec(context.Background(),
		`DELETE FROM projects WHERE id = $1`, projectID)
	if err != nil {
		serverError(c, err, "")
		return
	}

	auditLog(c, userID, "delete", "project", projectID)

	c.JSON(http.StatusOK, gin.H{"message": "proje silindi"})
}
