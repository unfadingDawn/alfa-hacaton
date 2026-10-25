// Package logx настраивает структурированный логгер, который вырезает
// значения полей с секретами (ключи, токены, пароли, cookie).
package logx

import (
	"io"
	"log/slog"
	"regexp"
	"strings"
)

var sensitiveKeys = []string{"key", "token", "password", "secret", "authorization", "cookie"}

// bearerRe ловит секреты, случайно попавшие в текст ошибки.
var bearerRe = regexp.MustCompile(`(?i)(bearer\s+|sk-)[A-Za-z0-9_\-\.]{6,}`)

func New(w io.Writer, level slog.Level) *slog.Logger {
	return slog.New(slog.NewJSONHandler(w, &slog.HandlerOptions{
		Level: level,
		ReplaceAttr: func(_ []string, a slog.Attr) slog.Attr {
			k := strings.ToLower(a.Key)
			for _, s := range sensitiveKeys {
				if strings.Contains(k, s) && k != "tokens" && !strings.HasSuffix(k, "_tokens") {
					return slog.String(a.Key, "[REDACTED]")
				}
			}
			if a.Value.Kind() == slog.KindString {
				return slog.String(a.Key, Scrub(a.Value.String()))
			}
			if a.Value.Kind() == slog.KindAny {
				if err, ok := a.Value.Any().(error); ok {
					return slog.String(a.Key, Scrub(err.Error()))
				}
			}
			return a
		},
	}))
}

// Scrub удаляет из строки фрагменты, похожие на API-ключи и bearer-токены.
func Scrub(s string) string {
	return bearerRe.ReplaceAllString(s, "${1}[REDACTED]")
}
