package main

import (
	"context"
	"fmt"
	"net/http"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/golang-jwt/jwt/v5"
)

// Tüm projeleri hafif liste olarak döner (arama + kategori + sayfalama).
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

	// Arama uzunluğu kontrolü
	if len(search) > MaxSearchLen {
		c.JSON(http.StatusBadRequest, gin.H{
			"error": fmt.Sprintf("arama metni en fazla %d karakter olabilir", MaxSearchLen),
		})
		return
	}

	// Wildcard escape (%, _ ve \ karakterlerini literal yap)
	if search != "" {
		search = strings.ReplaceAll(search, "\\", "\\\\")
		search = strings.ReplaceAll(search, "%", "\\%")
		search = strings.ReplaceAll(search, "_", "\\_")
	}

	// Kategoriler: "Backend,AI" → []string{"Backend", "AI"}
	var categoryNames []string
	if catQuery := c.Query("categories"); catQuery != "" {
		for _, s := range strings.Split(catQuery, ",") {
			name := strings.TrimSpace(s)
			if name != "" {
				categoryNames = append(categoryNames, name)
			}
		}
	}

	// Kategori sayısı kontrolü (kötüye kullanım)
	if len(categoryNames) > 20 {
		c.JSON(http.StatusBadRequest, gin.H{"error": "en fazla 20 kategori seçilebilir"})
		return
	}

	// Match mode: 'or' (default) | 'and'
	matchMode := c.DefaultQuery("mode", "or")

	// Dinamik WHERE koşulları
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

	if len(categoryNames) > 0 {
		if matchMode == "and" {
			for _, catName := range categoryNames {
				conditions = append(conditions, fmt.Sprintf(`
					EXISTS (
						SELECT 1 FROM project_categories pc2
						JOIN categories c2 ON c2.id = pc2.category_id
						WHERE pc2.project_id = p.id AND c2.name = $%d
					)
				`, argIdx))
				args = append(args, catName)
				argIdx++
			}
		} else {
			conditions = append(conditions, fmt.Sprintf(`
				EXISTS (
					SELECT 1 FROM project_categories pc2
					JOIN categories c2 ON c2.id = pc2.category_id
					WHERE pc2.project_id = p.id AND c2.name = ANY($%d)
				)
			`, argIdx))
			args = append(args, categoryNames)
			argIdx++
		}
	}

	whereClause := ""
	if len(conditions) > 0 {
		whereClause = "WHERE " + strings.Join(conditions, " AND ")
	}

	// LIMIT ve OFFSET
	limitIdx := argIdx
	offsetIdx := argIdx + 1
	args = append(args, limit, offset)

	// NOT: LEFT JOIN + GROUP BY yerine scalar subquery kullanıyoruz.
	// Böylece LIMIT/OFFSET fan-out yapmaz, sayfalama doğru çalışır.
	query := fmt.Sprintf(`
		SELECT 
			p.id, p.title, p.description,
			COALESCE(p.image_url, '') AS image_url,
			(SELECT COUNT(*) FROM project_stars WHERE project_id = p.id) AS stars,
			EXISTS(SELECT 1 FROM project_stars WHERE project_id = p.id AND user_id = $1) AS starred,
			COALESCE(u.username, '') AS author,
			COALESCE(u.avatar_url, '') AS author_avatar,
			COALESCE(u.id, 0) AS author_id,
			COALESCE(
				(SELECT ARRAY_AGG(c.name ORDER BY c.name)
				 FROM project_categories pc
				 JOIN categories c ON c.id = pc.category_id
				 WHERE pc.project_id = p.id),
				ARRAY[]::varchar[]
			) AS categories
		FROM projects p
		LEFT JOIN users u ON u.id = p.author_id
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

		err := rows.Scan(
			&id, &title, &description, &imageURL,
			&stars, &starred, &author, &authorAvatar, &authorID,
			&categories,
		)
		if err != nil {
			serverError(c, err, "")
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
			"starred":      starred,
			"author":       author,
			"authorAvatar": authorAvatar,
			"authorId":     authorID,
			"categories":   categories,
		})
	}

	c.JSON(http.StatusOK, projects)
}

// Tek bir projeyi tam detaylarıyla döner (kategoriler ve yıldız bilgisi dahil).
func handleGetProject(c *gin.Context) {
	id, err := strconv.Atoi(c.Param("id"))
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "geçersiz proje id"})
		return
	}

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
		AuthorID        int       `json:"authorId"`
		Categories      []string  `json:"categories"`
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
			) AS categories
		FROM projects p
		LEFT JOIN users u ON u.id = p.author_id
		WHERE p.id = $1
	`, id, userID).Scan(
		&project.ID, &project.Title, &project.Description, &project.LongDescription,
		&project.GithubURL, &project.DemoURL, &project.ImageURL,
		&project.Stars, &project.Starred, &project.Contributors, &project.Status, &project.CreatedAt,
		&project.Author, &project.AuthorAvatar, &project.AuthorID,
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

// Kategori ID'lerinin geçerli olduğunu doğrular.
// Geçersiz varsa false döner.
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

	input.Title = sanitizeText(input.Title)
	input.Description = sanitizeText(input.Description)
	input.LongDescription = sanitizeText(input.LongDescription)

	// Uzunluk sınırları
	if len(input.Title) > MaxTitleLen {
		c.JSON(http.StatusBadRequest, gin.H{
			"error": fmt.Sprintf("title en fazla %d karakter olabilir", MaxTitleLen),
		})
		return
	}
	if len(input.Description) > MaxDescriptionLen {
		c.JSON(http.StatusBadRequest, gin.H{
			"error": fmt.Sprintf("description en fazla %d karakter olabilir", MaxDescriptionLen),
		})
		return
	}
	if len(input.LongDescription) > MaxLongDescLen {
		c.JSON(http.StatusBadRequest, gin.H{
			"error": fmt.Sprintf("longDescription en fazla %d karakter olabilir", MaxLongDescLen),
		})
		return
	}
	if len(input.GithubURL) > MaxGithubURLLen {
		c.JSON(http.StatusBadRequest, gin.H{
			"error": fmt.Sprintf("githubUrl en fazla %d karakter olabilir", MaxGithubURLLen),
		})
		return
	}
	if len(input.DemoURL) > MaxDemoURLLen {
		c.JSON(http.StatusBadRequest, gin.H{
			"error": fmt.Sprintf("demoUrl en fazla %d karakter olabilir", MaxDemoURLLen),
		})
		return
	}
	if len(input.ImageURL) > MaxImageURLLen {
		c.JSON(http.StatusBadRequest, gin.H{
			"error": fmt.Sprintf("imageUrl en fazla %d karakter olabilir", MaxImageURLLen),
		})
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

	// Kategori sayısı sınırı
	if len(input.CategoryIDs) > MaxCategories {
		c.JSON(http.StatusBadRequest, gin.H{
			"error": fmt.Sprintf("en fazla %d kategori seçilebilir", MaxCategories),
		})
		return
	}

	// Kategori ID'leri geçerli mi?
	if !validateCategoryIDs(input.CategoryIDs) {
		c.JSON(http.StatusBadRequest, gin.H{"error": "geçersiz kategori id"})
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
		serverError(c, err, "")
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
				(SELECT ARRAY_AGG(c.name ORDER BY c.name)
				 FROM project_categories pc
				 JOIN categories c ON c.id = pc.category_id
				 WHERE pc.project_id = p.id),
				ARRAY[]::varchar[]
			) AS categories
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

		if err := rows.Scan(&id, &title, &description, &imageURL, &stars, &contributors, &categories); err != nil {
			serverError(c, err, "")
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

// Kullanıcının bir projeyi yıldızlamasını sağlar (kendi projesini yıldızlayamaz).
func handleStarProject(c *gin.Context) {
	userID := c.GetInt("user_id")

	projectID, err := strconv.Atoi(c.Param("id"))
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "geçersiz proje id"})
		return
	}

	// Projenin yazarı bu kullanıcı mı?
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

// Kullanıcının yıldızını geri almasını sağlar.
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
			COALESCE(u.id, 0) AS author_id,
			COALESCE(
				(SELECT ARRAY_AGG(c.name ORDER BY c.name)
				 FROM project_categories pc
				 JOIN categories c ON c.id = pc.category_id
				 WHERE pc.project_id = p.id),
				ARRAY[]::varchar[]
			) AS categories
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

		err := rows.Scan(
			&id, &title, &description, &longDesc,
			&githubURL, &demoURL, &imageURL,
			&stars, &contributors, &status, &createdAt,
			&author, &authorAvatar, &authorID,
			&categories,
		)
		if err != nil {
			serverError(c, err, "")
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
			"authorId":        authorID,
			"categories":      categories,
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

	// Projenin yazarı bu kullanıcı mı?
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

	// Zorunlu alanlar
	if input.Title == "" || input.Description == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "title ve description zorunlu"})
		return
	}

	input.Title = sanitizeText(input.Title)
	input.Description = sanitizeText(input.Description)
	input.LongDescription = sanitizeText(input.LongDescription)

	// Uzunluk sınırları
	if len(input.Title) > MaxTitleLen {
		c.JSON(http.StatusBadRequest, gin.H{
			"error": fmt.Sprintf("title en fazla %d karakter olabilir", MaxTitleLen),
		})
		return
	}
	if len(input.Description) > MaxDescriptionLen {
		c.JSON(http.StatusBadRequest, gin.H{
			"error": fmt.Sprintf("description en fazla %d karakter olabilir", MaxDescriptionLen),
		})
		return
	}
	if len(input.LongDescription) > MaxLongDescLen {
		c.JSON(http.StatusBadRequest, gin.H{
			"error": fmt.Sprintf("longDescription en fazla %d karakter olabilir", MaxLongDescLen),
		})
		return
	}
	if len(input.GithubURL) > MaxGithubURLLen {
		c.JSON(http.StatusBadRequest, gin.H{
			"error": fmt.Sprintf("githubUrl en fazla %d karakter olabilir", MaxGithubURLLen),
		})
		return
	}
	if len(input.DemoURL) > MaxDemoURLLen {
		c.JSON(http.StatusBadRequest, gin.H{
			"error": fmt.Sprintf("demoUrl en fazla %d karakter olabilir", MaxDemoURLLen),
		})
		return
	}
	if len(input.ImageURL) > MaxImageURLLen {
		c.JSON(http.StatusBadRequest, gin.H{
			"error": fmt.Sprintf("imageUrl en fazla %d karakter olabilir", MaxImageURLLen),
		})
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

	// Kategori sayısı sınırı
	if len(input.CategoryIDs) > MaxCategories {
		c.JSON(http.StatusBadRequest, gin.H{
			"error": fmt.Sprintf("en fazla %d kategori seçilebilir", MaxCategories),
		})
		return
	}

	// Kategori ID'leri geçerli mi?
	if !validateCategoryIDs(input.CategoryIDs) {
		c.JSON(http.StatusBadRequest, gin.H{"error": "geçersiz kategori id"})
		return
	}

	// Projeyi güncelle
	_, err = db.Exec(context.Background(), `
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

	// Kategorileri güncelle: önce hepsini sil, sonra yeniden ekle
	_, err = db.Exec(context.Background(),
		`DELETE FROM project_categories WHERE project_id = $1`, projectID)
	if err != nil {
		serverError(c, err, "")
		return
	}

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

	c.JSON(http.StatusOK, gin.H{
		"id":      projectID,
		"message": "proje güncellendi",
	})
}

// Projeyi siler (sadece yazar). Cascade ile kategoriler ve yıldızlar da silinir.
func handleDeleteProject(c *gin.Context) {
	userID := c.GetInt("user_id")

	projectID, err := strconv.Atoi(c.Param("id"))
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "geçersiz proje id"})
		return
	}

	// Projenin yazarı bu kullanıcı mı?
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

	// Projeyi sil (CASCADE ile project_categories ve project_stars da silinir)
	_, err = db.Exec(context.Background(),
		`DELETE FROM projects WHERE id = $1`, projectID)
	if err != nil {
		serverError(c, err, "")
		return
	}

	c.JSON(http.StatusOK, gin.H{"message": "proje silindi"})
}
