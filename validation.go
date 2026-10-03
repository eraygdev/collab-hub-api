package main

import (
	"fmt"
	"regexp"
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
