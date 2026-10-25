package llm

import (
	"regexp"
	"strings"
)

// AllowedModel — модель из утверждённого списка кейса (Приложение 3).
type AllowedModel struct {
	Slug    string `json:"slug"`
	Title   string `json:"title"`
	Purpose string `json:"purpose"`
}

// Allowed — единственные модели, которые разрешено использовать.
// «Использование других LLM не допускается.»
var Allowed = []AllowedModel{
	{"gemma-4-26b-a4b", "Gemma 4 26B A4B", "классификация, извлечение информации, простые преобразования текста"},
	{"glm-5.3-flash", "GLM-5.3-Flash", "задачи повышенной сложности, глубокий анализ и рассуждения"},
	{"qwen3.6-35b-a3b", "Qwen3.6 35B A3B", "универсальная: анализ текста, генерация, обработка материалов"},
	{"deepseek-v4.1-flash", "DeepSeek-V4.1-Flash", "исследовательские задачи, поиск взаимосвязей, большие объёмы"},
	{"qwen3-vl-30b-a3b-instruct", "Qwen3-VL-30B-A3B-Instruct", "изображения и мультимодальные данные"},
	{"kimi-k2.6", "Kimi-K2.6", "обработка и анализ текстовой информации"},
}

var nonAlnum = regexp.MustCompile(`[^a-z0-9.]+`)

// normalize приводит идентификатор к виду "qwen3.6-35b-a3b":
// убирает префикс провайдера ("qwen/..."), регистр и разделители.
func normalize(id string) string {
	id = strings.ToLower(strings.TrimSpace(id))
	if i := strings.LastIndex(id, "/"); i >= 0 {
		id = id[i+1:]
	}
	id = nonAlnum.ReplaceAllString(id, "-")
	return strings.Trim(id, "-")
}

// MatchAllowed возвращает разрешённую модель, которой соответствует
// идентификатор провайдера. Допускаются суффиксы квантования/даты
// ("...-fp8", "...-2026-05"), но не другие семейства и размеры.
func MatchAllowed(id string) (AllowedModel, bool) {
	n := normalize(id)
	for _, m := range Allowed {
		if n == m.Slug || strings.HasPrefix(n, m.Slug+"-") {
			return m, true
		}
	}
	return AllowedModel{}, false
}
