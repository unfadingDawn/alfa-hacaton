package llm

import (
	"encoding/json"
	"fmt"
	"strings"
)

// ExtractJSON достаёт JSON-объект из ответа модели: убирает блоки
// размышлений (<think>…</think>), markdown-ограждения и текст вокруг.
func ExtractJSON(s string) (string, error) {
	if i := strings.LastIndex(s, "</think>"); i >= 0 {
		s = s[i+len("</think>"):]
	}
	s = strings.TrimSpace(s)
	s = strings.TrimPrefix(s, "```json")
	s = strings.TrimPrefix(s, "```")
	s = strings.TrimSuffix(s, "```")
	start := strings.IndexAny(s, "{[")
	if start < 0 {
		return "", fmt.Errorf("%w: JSON не найден", ErrBadResponse)
	}
	open, closeCh := s[start], byte('}')
	if open == '[' {
		closeCh = ']'
	}
	depth, inStr, esc := 0, false, false
	for i := start; i < len(s); i++ {
		ch := s[i]
		switch {
		case esc:
			esc = false
		case ch == '\\' && inStr:
			esc = true
		case ch == '"':
			inStr = !inStr
		case inStr:
		case ch == open:
			depth++
		case ch == closeCh:
			depth--
			if depth == 0 {
				return s[start : i+1], nil
			}
		}
	}
	return "", fmt.Errorf("%w: JSON обрезан (возможно, превышен max_tokens)", ErrBadResponse)
}

// DecodeJSON извлекает и разбирает JSON-ответ модели в dst.
func DecodeJSON(content string, dst any) error {
	raw, err := ExtractJSON(content)
	if err != nil {
		return err
	}
	if err := json.Unmarshal([]byte(raw), dst); err != nil {
		return fmt.Errorf("%w: %v", ErrBadResponse, err)
	}
	return nil
}
