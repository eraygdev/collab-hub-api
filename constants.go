package main

import "regexp"

// Profil limitleri
const (
	MaxUsernameLen = 30
	MaxBioLen      = 150
)

// Proje limitleri
const (
	MaxTitleLen       = 80
	MaxDescriptionLen = 150
	MaxLongDescLen    = 500
	MaxGithubURLLen   = 200
	MaxDemoURLLen     = 200
	MaxImageURLLen    = 300
	MaxCategories     = 5
)

// Arama limitleri
const (
	MaxSearchLen = 100
)

// Sayfalama limitleri
const (
	MaxProjectsPerPage = 20
	MaxUsersPerSearch  = 5
)

// ─────────────────────────────────────────────────────────
// KARAKTER FİLTRELERİ (frontend validators.js ile birebir uyumlu)
// ─────────────────────────────────────────────────────────
//
// Neden var? Çünkü frontend zaten engelliyor ama backend de
// engellemeli (defense in depth). Curl/Postman ile doğrudan
// istek atılırsa burada yakalanır.
//
// Neden const değil? regexp.MustCompile bir fonksiyon çağrısıdır,
// derleme zamanında hesaplanamaz. Bu yüzden global var kullanıyoruz.
// Global olmasının sebebi: regex derleme pahalıdır, bir kere derle.

var (
	// Başlık / Kullanıcı Adı: harf, rakam, _ . ve boşluk (Türkçe dahil)
	TitleRegex    = regexp.MustCompile(`^[a-zA-Z0-9çÇğĞıİöÖşŞüÜ_. ]*$`)
	UsernameRegex = regexp.MustCompile(`^[a-zA-Z0-9çÇğĞıİöÖşŞüÜ_. ]*$`)

	// Açıklamalar / Bio / Arama: harf, rakam, noktalama, boşluk
	TextRegex = regexp.MustCompile(`^[a-zA-Z0-9çÇğĞıİöÖşŞüÜ.,!?;:'"()\[\]{}\-_/|@#$%&*+=~\s]*$`)
	BioRegex  = regexp.MustCompile(`^[a-zA-Z0-9çÇğĞıİöÖşŞüÜ.,!?;:'"()\[\]{}\-_/|@#$%&*+=~\s]*$`)
)
