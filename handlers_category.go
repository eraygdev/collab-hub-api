package main

import (
	"context"
	"net/http"

	"github.com/gin-gonic/gin"
)

// Tüm kategorileri alfabetik döner (public endpoint).
func handleListCategories(c *gin.Context) {
	rows, err := db.Query(context.Background(), `
		SELECT id, name, slug
		FROM categories
		ORDER BY name ASC
	`)
	if err != nil {
		serverError(c, err, "")
		return
	}
	defer rows.Close()

	categories := []map[string]interface{}{}
	for rows.Next() {
		var id int
		var name, slug string

		if err := rows.Scan(&id, &name, &slug); err != nil {
			serverError(c, err, "")
			return
		}

		categories = append(categories, map[string]interface{}{
			"id":   id,
			"name": name,
			"slug": slug,
		})
	}

	c.JSON(http.StatusOK, categories)
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
