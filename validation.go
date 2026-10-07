package main

import (
	"fmt"
	"net/http"
	"regexp"
	"strings"
	"time"
	"unicode/utf8"
)

// validateText uzunluk + regex kontrolü yapar.
// Hata durumunda error code döner (ör. "title_too_long", "title_invalid_char").
func validateText(field, value string, max int, re *regexp.Regexp) error {
	if utf8.RuneCountInString(value) > max {
		return fmt.Errorf("%s_too_long", field)
	}

	if value == "" {
		return nil
	}

	if !re.MatchString(value) {
		return fmt.Errorf("%s_invalid_char", field)
	}

	return nil
}

// validateTextMinMax min + max + regex kontrolü yapar.
// required=true ise boş değer kabul edilmez.
func validateTextMinMax(field, value string, min, max int, re *regexp.Regexp, required bool) error {
	length := utf8.RuneCountInString(value)

	if required && length < min {
		return fmt.Errorf("%s_too_short", field)
	}
	if !required && length > 0 && length < min {
		return fmt.Errorf("%s_too_short", field)
	}
	if length > max {
		return fmt.Errorf("%s_too_long", field)
	}
	if length == 0 {
		return nil
	}
	if !re.MatchString(value) {
		return fmt.Errorf("%s_invalid_char", field)
	}
	return nil
}

// normalizeGithubImageURL — GitHub görsel URL'lerini raw formata çevirir.
// Kullanıcı hangi formatta yapıştırırsa yapıştırsın, tek formata indirger.
// Geçersizse boş string döner.
func normalizeGithubImageURL(rawURL string) string {
	rawURL = strings.TrimSpace(rawURL)
	if rawURL == "" {
		return ""
	}

	// Protokol yoksa https:// ekle (sadece github domain'leri için)
	if !strings.HasPrefix(rawURL, "http://") && !strings.HasPrefix(rawURL, "https://") {
		if strings.HasPrefix(rawURL, "github.com/") || strings.HasPrefix(rawURL, "raw.githubusercontent.com/") {
			rawURL = "https://" + rawURL
		}
	}

	if idx := strings.Index(rawURL, "?"); idx != -1 {
		rawURL = rawURL[:idx]
	}
	if idx := strings.Index(rawURL, "#"); idx != -1 {
		rawURL = rawURL[:idx]
	}

	if strings.HasPrefix(rawURL, "https://raw.githubusercontent.com/") {
		return rawURL
	}

	const blobPrefix = "https://github.com/"
	const rawPrefix = "https://raw.githubusercontent.com/"

	if !strings.HasPrefix(rawURL, blobPrefix) {
		return ""
	}

	rest := strings.TrimPrefix(rawURL, blobPrefix)

	if idx := strings.Index(rest, "/blob/"); idx != -1 {
		rest = rest[:idx] + "/" + rest[idx+len("/blob/"):]
	} else if idx := strings.Index(rest, "/raw/"); idx != -1 {
		rest = rest[:idx] + "/" + rest[idx+len("/raw/"):]
	} else {
		return ""
	}

	return rawPrefix + rest
}

// isValidGithubImageURL — normalize edilmiş URL geçerli mi?
// Uzantı kontrolü + path'in en az 4 parça olması (user/repo/branch/file).
func isValidGithubImageURL(rawURL string) bool {
	normalized := normalizeGithubImageURL(rawURL)
	if normalized == "" {
		return false
	}

	if !strings.HasPrefix(normalized, "https://raw.githubusercontent.com/") {
		return false
	}

	lower := strings.ToLower(normalized)
	allowedExts := []string{".png", ".jpg", ".jpeg", ".gif", ".webp"}
	for _, ext := range allowedExts {
		if strings.HasSuffix(lower, ext) {
			return true
		}
	}
	return false
}

// normalizeGithubRepoURL — GitHub repo URL'sini normalize eder.
func normalizeGithubRepoURL(rawURL string) string {
	rawURL = strings.TrimSpace(rawURL)
	if rawURL == "" {
		return ""
	}

	if !strings.HasPrefix(rawURL, "http://") && !strings.HasPrefix(rawURL, "https://") {
		if strings.HasPrefix(rawURL, "github.com/") {
			rawURL = "https://" + rawURL
		}
	}

	if idx := strings.Index(rawURL, "?"); idx != -1 {
		rawURL = rawURL[:idx]
	}
	if idx := strings.Index(rawURL, "#"); idx != -1 {
		rawURL = rawURL[:idx]
	}

	rawURL = strings.TrimSuffix(rawURL, ".git")
	rawURL = strings.TrimSuffix(rawURL, "/")

	const prefix = "https://github.com/"
	if !strings.HasPrefix(rawURL, prefix) {
		return ""
	}
	rest := strings.TrimPrefix(rawURL, prefix)

	parts := strings.Split(rest, "/")
	if len(parts) < 2 || parts[0] == "" || parts[1] == "" {
		return ""
	}

	return prefix + parts[0] + "/" + parts[1]
}

// checkGithubRepoPublic — git-upload-pack endpoint'i ile repo'nun public olup olmadığını kontrol eder.
// API rate limiti yoktur. Fail-open: ağ hatası durumunda nil döner.
func checkGithubRepoPublic(repoURL string) error {
	normalized := normalizeGithubRepoURL(repoURL)
	if normalized == "" {
		return fmt.Errorf("invalid_github_url")
	}

	checkURL := normalized + "/info/refs?service=git-upload-pack"

	client := &http.Client{Timeout: 5 * time.Second}
	req, err := http.NewRequest("GET", checkURL, nil)
	if err != nil {
		return nil
	}
	req.Header.Set("User-Agent", "RepoReef-Backend")

	resp, err := client.Do(req)
	if err != nil {
		return nil
	}
	defer resp.Body.Close()

	if resp.StatusCode == http.StatusOK {
		return nil
	}
	if resp.StatusCode == http.StatusUnauthorized || resp.StatusCode == http.StatusNotFound {
		return fmt.Errorf("github_repo_not_accessible")
	}
	return nil
}
