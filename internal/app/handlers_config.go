package app

import (
	"net/http"

	"github.com/gin-gonic/gin"
)

// handleConfig tüm limit değerlerini döner.
// Frontend bir kez çeker, tüm app kullanır.
func handleConfig(c *gin.Context) {
	c.JSON(http.StatusOK, gin.H{
		"limits": gin.H{
			"title": gin.H{
				"min": MinTitleLen,
				"max": MaxTitleLen,
			},
			"description": gin.H{
				"min": MinDescriptionLen,
				"max": MaxDescriptionLen,
			},
			"longDescription": gin.H{
				"min": 0,
				"max": MaxLongDescLen,
			},
			"githubUrl": gin.H{
				"max": MaxGithubURLLen,
			},
			"demoUrl": gin.H{
				"max": MaxDemoURLLen,
			},
			"imageUrl": gin.H{
				"max": MaxImageURLLen,
			},
			"username": gin.H{
				"min": MinUsernameLen,
				"max": MaxUsernameLen,
			},
			"bio": gin.H{
				"min": MinBioLen,
				"max": MaxBioLen,
			},
			"maxCategories":            MaxCategories,
			"maxProjectsPerUser":       MaxProjectsPerUser,
			"maxSearchLen":             MaxSearchLen,
			"maxProjectsPerPage":       MaxProjectsPerPage,
			"maxUsersPerSearch":        MaxUsersPerSearch,
			"allowedContributorLimits": AllowedContributorLimits,
			"defaultContributorLimit":  DefaultContributorLimit,
			"maxImageSizeBytes":        MaxImageSizeBytes,
		},
	})
}
