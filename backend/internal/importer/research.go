package importer

import (
	"regexp"
	"strconv"
	"strings"
	"time"
	"unicode"
)

// DocMeta — сведения о материале. Названия совпадают с перечнем материалов
// в презентации кейса (Приложение 2); даты взяты из шапок документов.
type DocMeta struct {
	Title     string
	Subtitle  string
	Published time.Time
	Unit      string
	Outdated  bool
	Topics    []string // темы по названию материала (метаданные, не вывод модели)
}

func d(y int, m time.Month, day int) time.Time { return time.Date(y, m, day, 9, 0, 0, 0, time.UTC) }

var KnownDocs = map[string]DocMeta{
	"Inflation_tracker_02":                            {"Инфляция 25–31 августа: основные выводы", "Еженедельный трекер инфляции", d(2026, 9, 2), "Центр макроэкономического анализа", false, []string{"inflation", "monetary_policy", "oil_gas"}},
	"Бонд_Бриф_14092026":                              {"Бонд Бриф: итоги недели", "Заседание ЦБ по ставке добавило долговому рынку негатива", d(2026, 9, 14), "Аналитика долгового рынка", false, []string{"debt_market", "monetary_policy"}},
	"Gold_digest_August26":                            {"Обзор рынка золота, август 2026", "Золото заметно выросло за август, но жёсткий сигнал ФРС охладил ралли", d(2026, 8, 31), "Аналитика сырьевых рынков", false, []string{"gold", "commodities"}},
	"FI_Strategy_3Q26_22072026":                       {"Тактический взгляд на 3-й квартал 2026 года: время собирать камни", "Стратегия на долговом рынке", d(2026, 7, 22), "Аналитика долгового рынка", false, []string{"debt_market", "monetary_policy", "credit"}},
	"ALFA_Russian_Steels_1К21PreviewUpdate_15Apr2021": {"Российская сталь: ралли цен продолжается; прогноз результатов за 1К21", "Commodities performance", d(2021, 4, 15), "Аналитика сырьевых рынков", true, []string{"metals", "commodities"}},
	"ALFA-Macro_Insights-Policy_Rate_11Sep":           {"Ключевая ставка ЦБ РФ: ставка без изменения на 14,0%, риторика стала жёстче", "Макро-инсайты", d(2026, 9, 11), "Центр макроэкономического анализа", false, []string{"monetary_policy", "inflation", "credit"}},
	"ALFA-7M26":                                       {"Макростатистика 7М26: данные по рынку труда не подтверждают охлаждения экономики", "Макростатистика", d(2026, 9, 7), "Центр макроэкономического анализа", false, []string{"labor", "macro", "monetary_policy"}},
	"ALFA-consumption":                                {"Потребление на пике", "Динамика потребительского спроса", d(2026, 8, 5), "Центр макроэкономического анализа", false, []string{"consumer_demand", "macro"}},
	"ALFA-macro_stat1H26":                             {"Макростатистика 1П26: слабый рост 1П26 смещает прогноз роста ВВП на 0,1% за год", "Макростатистика", d(2026, 7, 31), "Центр макроэкономического анализа", false, []string{"macro"}},
}

type Page struct {
	Number int
	Text   string
}

var pageMarker = regexp.MustCompile(`(?m)^=== PAGE (\d+)(?:\.txt)?\s*$`)

// SplitPages делит OCR-текст по маркерам "=== PAGE n".
func SplitPages(text string) []Page {
	idx := pageMarker.FindAllStringSubmatchIndex(text, -1)
	var pages []Page
	for i, m := range idx {
		n, _ := strconv.Atoi(text[m[2]:m[3]])
		end := len(text)
		if i+1 < len(idx) {
			end = idx[i+1][0]
		}
		pages = append(pages, Page{Number: n, Text: text[m[1]:end]})
	}
	if len(pages) == 0 && strings.TrimSpace(text) != "" {
		pages = append(pages, Page{Number: 1, Text: text})
	}
	return pages
}

// IsNoise отсекает строки OCR, состоящие из обрывков графиков и таблиц.
func IsNoise(line string) bool {
	line = strings.TrimSpace(line)
	if len([]rune(line)) < 25 {
		return true
	}
	letters, total := 0, 0
	for _, r := range line {
		if unicode.IsSpace(r) {
			continue
		}
		total++
		if unicode.IsLetter(r) {
			letters++
		}
	}
	return total == 0 || float64(letters)/float64(total) < 0.6
}

// Chunk — фрагмент для поиска с привязкой к странице.
type Chunk struct {
	Page    int
	Content string
}

// ChunkPages собирает абзацы страницы во фрагменты ~target символов.
func ChunkPages(pages []Page, target int) []Chunk {
	var out []Chunk
	for _, p := range pages {
		var buf strings.Builder
		flush := func() {
			if t := strings.TrimSpace(buf.String()); len([]rune(t)) >= 60 {
				out = append(out, Chunk{Page: p.Number, Content: t})
			}
			buf.Reset()
		}
		for _, line := range strings.Split(p.Text, "\n") {
			if IsNoise(line) {
				continue
			}
			buf.WriteString(strings.TrimSpace(line))
			buf.WriteString(" ")
			if len([]rune(buf.String())) >= target {
				flush()
			}
		}
		flush()
	}
	return out
}

// LeadText — первые содержательные строки документа (для карточки материала).
func LeadText(pages []Page, max int) string {
	var b strings.Builder
	for _, p := range pages {
		for _, line := range strings.Split(p.Text, "\n") {
			if IsNoise(line) {
				continue
			}
			b.WriteString(strings.TrimSpace(line) + " ")
			if len([]rune(b.String())) >= max {
				r := []rune(strings.TrimSpace(b.String()))
				s := string(r[:max])
				if i := strings.LastIndexAny(s, ".!?"); i > max/2 {
					s = s[:i+1]
				}
				return s
			}
		}
	}
	return strings.TrimSpace(b.String())
}
