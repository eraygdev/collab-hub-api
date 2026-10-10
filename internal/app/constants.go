package app

import "regexp"

// ─────────────────────────────────────────────────────────
// API VERSION
// ─────────────────────────────────────────────────────────
const APIVersion = "v1"

// Profil limitleri
const (
	MinUsernameLen = 3
	MaxUsernameLen = 30
	MinBioLen      = 0
	MaxBioLen      = 150
)

// Proje limitleri
const (
	MinTitleLen        = 3
	MaxTitleLen        = 80
	MinDescriptionLen  = 20
	MaxDescriptionLen  = 150
	MaxLongDescLen     = 500
	MaxGithubURLLen    = 200
	MaxDemoURLLen      = 200
	MaxImageURLLen     = 300
	MaxImageSizeBytes  = 2 * 1024 * 1024 // 2 MB
	MaxCategories      = 5
	MaxProjectsPerUser = 10
)

// [YENİ] Katkıcı limitleri
const (
	DefaultContributorLimit = 10
)

// [YENİ] AllowedContributorLimits — create'te seçilebilecek değerler
var AllowedContributorLimits = []int{5, 10, 20, 50}

// Arama limitleri
const (
	MaxSearchLen = 100
)

// Sayfalama limitleri
const (
	MaxProjectsPerPage = 20
	MaxUsersPerSearch  = 5
)

// Karakter filtreleri (frontend validators.js ile uyumlu)
var (
	TitleRegex    = regexp.MustCompile(`^[a-zA-Z0-9çÇğĞıİöÖşŞüÜ_. ]*$`)
	UsernameRegex = regexp.MustCompile(`^[a-zA-Z0-9çÇğĞıİöÖşŞüÜ_.\- ]*$`)
	TextRegex     = regexp.MustCompile(`^[a-zA-Z0-9çÇğĞıİöÖşŞüÜ.,!?;:'"()\[\]{}\-_/|@#$%&*+=~\s]*$`)
	BioRegex      = regexp.MustCompile(`^[a-zA-Z0-9çÇğĞıİöÖşŞüÜ.,!?;:'"()\[\]{}\-_/|@#$%&*+=~\s]*$`)
)

// [YENİ] isAllowedContributorLimit — değer izinli listede mi?
func isAllowedContributorLimit(limit int) bool {
	for _, allowed := range AllowedContributorLimits {
		if limit == allowed {
			return true
		}
	}
	return false
}
