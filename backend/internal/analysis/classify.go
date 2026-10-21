// Package analysis — промпты, JSON-схемы ответов модели и их валидация.
// Пакет не зависит от БД, чтобы его можно было тестировать изолированно.
package analysis

import (
	"fmt"
	"strings"
	"time"

	"github.com/alfainsight/backend/internal/llm"
)

const PromptVersion = "2026-10-21.1"

type Topic struct {
	Code string
	Name string
}

type ArticleInput struct {
	ID          string
	Title       string
	Summary     string
	Body        string
	Source      string
	PublishedAt time.Time
}

type KeyFact struct {
	Text      string `json:"text"`
	Confirmed bool   `json:"confirmed"`
}

type Entity struct {
	Name string `json:"name"`
	Type string `json:"type"`
}

type Indicator struct {
	Name      string `json:"name"`
	Value     string `json:"value"`
	Direction string `json:"direction"`
}

type TopicScore struct {
	Code       string  `json:"code"`
	Confidence float64 `json:"confidence"`
}

// Classification — результат шага 4 конвейера: темы, факты, даты,
// организации, экономические показатели.
type Classification struct {
	Topics         []TopicScore `json:"topics"`
	EventType      string       `json:"event_type"`
	Summary        string       `json:"summary"`
	KeyFacts       []KeyFact    `json:"key_facts"`
	Dates          []string     `json:"dates"`
	Entities       []Entity     `json:"entities"`
	Indicators     []Indicator  `json:"indicators"`
	OkvedDivisions []string     `json:"okved_divisions"`
	Regions        []string     `json:"regions"`
	Countries      []string     `json:"countries"`
	Commodities    []string     `json:"commodities"`
	IsBusiness     bool         `json:"is_business_relevant"`
	Model          string       `json:"model,omitempty"`
	PromptVersion  string       `json:"prompt_version,omitempty"`
}

var eventTypes = []string{"monetary_policy_decision", "macro_statistics", "regulation", "tax_change", "market_move",
	"commodity_price", "corporate_event", "trade_policy", "sanctions", "infrastructure", "labor_market", "other"}

func ClassifyMessages(a ArticleInput, topics []Topic) []llm.Message {
	var tl strings.Builder
	for _, t := range topics {
		fmt.Fprintf(&tl, "- %s: %s\n", t.Code, t.Name)
	}
	system := `Ты — аналитик новостей для корпоративного банка. Извлекаешь из новости только то, что в ней написано.
Правила:
- Не добавляй фактов, которых нет в тексте. Если чего-то нет — оставь пустой список.
- confirmed=true только для утверждений, прямо изложенных в тексте как факт; прогнозы, оценки и мнения — confirmed=false.
- Темы выбирай только из списка кодов.
- okved_divisions — двузначные классы ОКВЭД 2 (например "46", "24"), для отраслей, которых новость прямо касается.
- Отвечай только JSON-объектом без пояснений.`
	user := fmt.Sprintf(`Темы (код: название):
%s
Типы событий: %s

Новость
Источник: %s
Дата публикации: %s
Заголовок: %s
Текст: %s

Верни JSON:
{"topics":[{"code":"...","confidence":0.0-1.0}],"event_type":"...","summary":"1–2 предложения по тексту новости",
"key_facts":[{"text":"...","confirmed":true}],"dates":["..."],"entities":[{"name":"...","type":"organization|person|country|regulator|product"}],
"indicators":[{"name":"...","value":"...","direction":"up|down|unchanged|unknown"}],
"okved_divisions":["..."],"regions":["..."],"countries":["..."],"commodities":["..."],"is_business_relevant":true}`,
		tl.String(), strings.Join(eventTypes, ", "), a.Source, a.PublishedAt.Format("2006-01-02"),
		a.Title, strings.TrimSpace(a.Summary+"\n"+a.Body))
	return []llm.Message{{Role: "system", Content: system}, {Role: "user", Content: user}}
}

// ValidateClassification нормализует ответ: убирает неизвестные темы,
// ограничивает размеры списков, проверяет обязательные поля.
func ValidateClassification(c *Classification, topics []Topic) error {
	known := map[string]bool{}
	for _, t := range topics {
		known[t.Code] = true
	}
	var ts []TopicScore
	seen := map[string]bool{}
	for _, t := range c.Topics {
		if known[t.Code] && !seen[t.Code] {
			if t.Confidence <= 0 || t.Confidence > 1 {
				t.Confidence = 0.5
			}
			seen[t.Code] = true
			ts = append(ts, t)
		}
	}
	if len(ts) > 5 {
		ts = ts[:5]
	}
	c.Topics = ts
	valid := false
	for _, e := range eventTypes {
		if c.EventType == e {
			valid = true
		}
	}
	if !valid {
		c.EventType = "other"
	}
	var divs []string
	for _, d := range c.OkvedDivisions {
		d = strings.TrimSpace(d)
		if len(d) == 2 && d[0] >= '0' && d[0] <= '9' && d[1] >= '0' && d[1] <= '9' {
			divs = append(divs, d)
		}
	}
	c.OkvedDivisions = divs
	c.KeyFacts = limit(c.KeyFacts, 8)
	c.Entities = limit(c.Entities, 12)
	c.Indicators = limit(c.Indicators, 8)
	c.Dates = limit(c.Dates, 6)
	c.Regions = limit(c.Regions, 8)
	c.Countries = limit(c.Countries, 8)
	c.Commodities = limit(c.Commodities, 8)
	if strings.TrimSpace(c.Summary) == "" {
		return fmt.Errorf("пустое поле summary")
	}
	return nil
}

func limit[T any](s []T, n int) []T {
	if s == nil {
		return []T{}
	}
	if len(s) > n {
		return s[:n]
	}
	return s
}
