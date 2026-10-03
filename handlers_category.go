package main

import (
	"context"
	"net/http"

	"github.com/gin-gonic/gin"
)

// handleListCategories tüm kategorileri alfabetik döner.
func handleListCategories(c *gin.Context) {
	rows, err := db.Query(c.Request.Context(), `
		SELECT id, name, slug FROM categories ORDER BY name ASC
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

// validateCategoryIDs verilen ID'lerin hepsinin geçerli olduğunu doğrular.
func validateCategoryIDs(ctx context.Context, categoryIDs []int) bool {
	if len(categoryIDs) == 0 {
		return true
	}
	var count int
	err := db.QueryRow(ctx,
		`SELECT COUNT(*) FROM categories WHERE id = ANY($1)`,
		categoryIDs,
	).Scan(&count)
	if err != nil {
		return false
	}
	return count == len(categoryIDs)
}
