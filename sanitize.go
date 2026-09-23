package main

import (
	"regexp"
	"strings"
)

var htmlTagRe = regexp.MustCompile(`<[^>]*>`)

// Basit HTML tag temizleme (script/style içerikleri dahil).
// Tam bir HTML parser değil ama basit saldırıları engeller.
func sanitizeText(s string) string {
	// HTML tag'lerini sil
	s = htmlTagRe.ReplaceAllString(s, "")
	// Script/style içeriklerini sil
	s = strings.ReplaceAll(s, "javascript:", "")
	s = strings.ReplaceAll(s, "data:", "")
	s = strings.ReplaceAll(s, "onerror=", "")
	s = strings.ReplaceAll(s, "onload=", "")
	s = strings.ReplaceAll(s, "<script", "")
	s = strings.ReplaceAll(s, "</script", "")
	return strings.TrimSpace(s)
}
