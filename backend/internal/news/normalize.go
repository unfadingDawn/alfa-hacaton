// Package news — получение новостей из разрешённых источников,
// нормализация, удаление дубликатов и API ленты.
package news

import (
	"crypto/sha256"
	"encoding/hex"
	"html"
	"regexp"
	"strings"
	"unicode"
)

var (
	tagRe   = regexp.MustCompile(`(?s)<[^>]*>`)
	spaceRe = regexp.MustCompile(`\s+`)
)

// CleanText убирает HTML-разметку и сущности, схлопывает пробелы.
func CleanText(s string) string {
	s = tagRe.ReplaceAllString(s, " ")
	s = html.UnescapeString(s)
	s = strings.ReplaceAll(s, " ", " ")
	return strings.TrimSpace(spaceRe.ReplaceAllString(s, " "))
}

// NormTitle — ключ для поиска дубликатов: нижний регистр, только буквы и цифры, ё→е.
func NormTitle(s string) string {
	var b strings.Builder
	space := false
	for _, r := range strings.ToLower(s) {
		if r == 'ё' {
			r = 'е'
		}
		if unicode.IsLetter(r) || unicode.IsDigit(r) {
			b.WriteRune(r)
			space = false
		} else if !space && b.Len() > 0 {
			b.WriteRune(' ')
			space = true
		}
	}
	return strings.TrimSpace(b.String())
}

// ContentHash — хеш нормализованного заголовка и начала текста.
func ContentHash(title, text string) string {
	t := NormTitle(text)
	if r := []rune(t); len(r) > 300 {
		t = string(r[:300])
	}
	sum := sha256.Sum256([]byte(NormTitle(title) + "|" + t))
	return hex.EncodeToString(sum[:])
}

// Truncate обрезает строку по границе слова.
func Truncate(s string, max int) string {
	r := []rune(s)
	if len(r) <= max {
		return s
	}
	cut := string(r[:max])
	if i := strings.LastIndex(cut, " "); i > max/2 {
		cut = cut[:i]
	}
	return cut + "…"
}
