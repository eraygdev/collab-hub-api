package main

import (
	"fmt"
	"regexp"
	"unicode/utf8"
)

// validateText: uzunluk + regex kontrolü yapar.
// Hata varsa error döner, yoksa nil.
//
// field: hata mesajında görünecek alan adı (örn: "title")
// value: kontrol edilecek değer
// max:   maksimum karakter sayısı (rune bazlı, Türkçe karakter 1 sayılır)
// re:    uygulanacak regex (constants.go'dan)
func validateText(field, value string, max int, re *regexp.Regexp) error {
	// 1) Uzunluk kontrolü (rune bazlı — Türkçe karakterler 1 sayılsın)
	if utf8.RuneCountInString(value) > max {
		return fmt.Errorf("%s en fazla %d karakter olabilir", field, max)
	}

	// 2) Boş değer → regex kontrolüne gerek yok (opsiyonel alanlar)
	if value == "" {
		return nil
	}

	// 3) Regex kontrolü
	if !re.MatchString(value) {
		if bad := findInvalidChar(value, re); bad != "" {
			return fmt.Errorf("%s geçersiz karakter içeriyor: '%s'", field, bad)
		}
		return fmt.Errorf("%s geçersiz karakter içeriyor", field)
	}

	return nil
}

// findInvalidChar: value içindeki regex'e uymayan İLK karakteri döner.
// Frontend'deki findInvalidChar ile aynı mantık.
// Bulunamazsa "" döner.
func findInvalidChar(value string, re *regexp.Regexp) string {
	for _, ch := range value {
		if !re.MatchString(string(ch)) {
			return string(ch)
		}
	}
	return ""
}
