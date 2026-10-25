package analysis

import (
	"fmt"
	"math"
	"regexp"
	"strconv"
	"strings"
)

// Детерминированная проверка фактологии ответа модели: проценты, суммы,
// сроки, даты и названия организаций из текста анализа ищутся в реестре
// источников. Значение, которого нет ни в одном источнике, считается
// неподтверждённым: факты и позиции банка с таким значением удаляются,
// остальные утверждения помечаются, а агент получает замечание для доработки.

// UnverifiedValue — значение из анализа, не найденное в источниках.
type UnverifiedValue struct {
	Path   string `json:"path"`   // например, "risks[0]"
	Kind   string `json:"kind"`   // percent | money | term | date | year | entity
	Value  string `json:"value"`  // как написано в анализе
	Claim  string `json:"claim"`  // утверждение (сокращённо)
	Action string `json:"action"` // removed | marked
}

type FactCheck struct {
	Checked    int               `json:"checked"`
	Verified   int               `json:"verified"`
	Unverified []UnverifiedValue `json:"unverified"`
}

// SourceCoverage — доля значимых утверждений, подтверждённых источниками:
// утверждение подтверждено, если ссылается на источник реестра и не содержит
// неподтверждённых чисел, дат и названий.
type SourceCoverage struct {
	Confirmed int     `json:"confirmed"`
	Total     int     `json:"total"`
	Ratio     float64 `json:"ratio"`
}

// SourceCompleteness — полнота текста основной новости.
type SourceCompleteness struct {
	Level string `json:"level"` // title_only | summary | full_text
	Label string `json:"label"`
	Note  string `json:"note,omitempty"`
}

var completeness = map[string]SourceCompleteness{
	"full_text": {Level: "full_text", Label: "Полный текст"},
	"summary": {Level: "summary", Label: "Заголовок и анонс",
		Note: "Анализ построен по заголовку и анонсу новости: детали, которых нет в анонсе, не учитывались. Проверьте полный текст по ссылке на первоисточник."},
	"title_only": {Level: "title_only", Label: "Только заголовок",
		Note: "Источник передал только заголовок: выводы предварительные, факты и цифры нужно проверить в первоисточнике."},
}

// CompletenessFor возвращает описание полноты источника (по умолчанию — анонс).
func CompletenessFor(level string) SourceCompleteness {
	if c, ok := completeness[level]; ok {
		return c
	}
	return completeness["summary"]
}

// ---------------------------------------------------------------------------
// Извлечение значений
// ---------------------------------------------------------------------------

const numPat = `\d+(?:[ \x{00a0}]\d{3})*(?:[.,]\d+)?`

var (
	percentRe = regexp.MustCompile(`(` + numPat + `)\s*(?:%|процент\pL*|п\.\s?п\.?|б\.\s?п\.?|базисн\pL* пункт\pL*)`)
	moneyRe   = regexp.MustCompile(`(` + numPat + `)\s*(трлн|млрд|млн|тыс\.?)?\s*(?:руб\pL*|₽|долл\pL*|\$|евро|€|юан\pL*|usd|eur|cny|rub)`)
	termRe    = regexp.MustCompile(`(` + numPat + `)\s*(?:дн\pL*|день|недел\pL*|месяц\pL*|мес\.|лет|года?|годам|квартал\pL*)(?:[^\pL]|$)`)
	months    = []string{"январ", "феврал", "март", "апрел", "ма", "июн", "июл", "август", "сентябр", "октябр", "ноябр", "декабр"}
	dateRe    = regexp.MustCompile(`(\d{1,2})\s+(январ|феврал|март|апрел|ма[яй]|июн|июл|август|сентябр|октябр|ноябр|декабр)\pL*`)
	dotDateRe = regexp.MustCompile(`(\d{1,2})\.(\d{1,2})\.(\d{2,4})`)
	yearRe    = regexp.MustCompile(`(?:^|[^\d.,])((?:19|20)\d{2})(?:[^\d.,]|$)`)
	quotedRe  = regexp.MustCompile(`«([^»]{2,60})»`)
	anyNumRe  = regexp.MustCompile(numPat)
	scaleRe   = regexp.MustCompile(`^\s*(трлн|млрд|млн|тыс)`)
	isoDateRe = regexp.MustCompile(`(\d{4})-(\d{2})-(\d{2})`)
)

var scales = map[string]float64{"трлн": 1e12, "млрд": 1e9, "млн": 1e6, "тыс": 1e3, "тыс.": 1e3}

type extracted struct {
	kind  string
	value string
	nums  []float64 // для percent, money, term, year
	day   int       // для date
	month int
}

func normText(s string) string {
	s = strings.ToLower(strings.ReplaceAll(strings.ReplaceAll(s, "ё", "е"), "Ё", "е"))
	return strings.ReplaceAll(s, " ", " ")
}

func parseNum(s string) (float64, bool) {
	s = strings.NewReplacer(" ", "", " ", "", ",", ".").Replace(s)
	v, err := strconv.ParseFloat(s, 64)
	return v, err == nil
}

func monthIndex(stem string) int {
	for i, m := range months {
		if strings.HasPrefix(stem, m) {
			return i + 1
		}
	}
	return 0
}

// extractValues находит проверяемые значения в тексте. Пересекающиеся
// совпадения не дублируются: «11 сентября 2026» — одна дата и один год.
func extractValues(text string, withTerms bool) []extracted {
	t := normText(text)
	var out []extracted
	var taken [][2]int
	overlaps := func(a, b int) bool {
		for _, r := range taken {
			if a < r[1] && b > r[0] {
				return true
			}
		}
		return false
	}
	add := func(loc []int, e extracted) {
		if overlaps(loc[0], loc[1]) {
			return
		}
		taken = append(taken, [2]int{loc[0], loc[1]})
		e.value = strings.TrimRight(strings.TrimSpace(t[loc[0]:loc[1]]), " ,.;:)!?")
		out = append(out, e)
	}
	for _, m := range moneyRe.FindAllStringSubmatchIndex(t, -1) {
		v, ok := parseNum(t[m[2]:m[3]])
		if !ok {
			continue
		}
		nums := []float64{v}
		if m[4] >= 0 {
			nums = append(nums, v*scales[t[m[4]:m[5]]])
		}
		add(m[:2], extracted{kind: "money", nums: nums})
	}
	for _, m := range percentRe.FindAllStringSubmatchIndex(t, -1) {
		if v, ok := parseNum(t[m[2]:m[3]]); ok {
			add(m[:2], extracted{kind: "percent", nums: []float64{v}})
		}
	}
	for _, m := range dateRe.FindAllStringSubmatchIndex(t, -1) {
		d, _ := strconv.Atoi(t[m[2]:m[3]])
		if d >= 1 && d <= 31 {
			add(m[:2], extracted{kind: "date", day: d, month: monthIndex(t[m[4]:m[5]])})
		}
	}
	for _, m := range dotDateRe.FindAllStringSubmatchIndex(t, -1) {
		d, _ := strconv.Atoi(t[m[2]:m[3]])
		mo, _ := strconv.Atoi(t[m[4]:m[5]])
		if d >= 1 && d <= 31 && mo >= 1 && mo <= 12 {
			add(m[:2], extracted{kind: "date", day: d, month: mo})
		}
	}
	if withTerms {
		for _, m := range termRe.FindAllStringSubmatchIndex(t, -1) {
			if v, ok := parseNum(t[m[2]:m[3]]); ok && !(v >= 1900 && v <= 2100) {
				add(m[:2], extracted{kind: "term", nums: []float64{v}})
			}
		}
	}
	for _, m := range yearRe.FindAllStringSubmatchIndex(t, -1) {
		if v, ok := parseNum(t[m[2]:m[3]]); ok {
			add(m[2:4], extracted{kind: "year", nums: []float64{v}})
		}
	}
	for _, m := range quotedRe.FindAllStringSubmatchIndex(text, -1) {
		name := strings.TrimSpace(text[m[2]:m[3]])
		// Кавычки вокруг цитаты или длинной фразы — не название.
		if len(strings.Fields(name)) > 4 {
			continue
		}
		out = append(out, extracted{kind: "entity", value: name})
	}
	return out
}

// ---------------------------------------------------------------------------
// Корпус источников
// ---------------------------------------------------------------------------

type corpus struct {
	text  string
	nums  []float64
	dates map[[2]int]bool
}

func newCorpus(reg *Registry) *corpus {
	var b strings.Builder
	for _, e := range reg.Items {
		b.WriteString(e.Title)
		b.WriteString("\n")
		b.WriteString(e.Excerpt)
		b.WriteString("\n")
		// Дата публикации новости подтверждает дату события («11 сентября»).
		if e.Type == "article" || e.Type == "related_news" {
			b.WriteString(e.Date)
			b.WriteString("\n")
		}
	}
	c := &corpus{text: normText(b.String()), dates: map[[2]int]bool{}}
	for _, loc := range anyNumRe.FindAllStringIndex(c.text, -1) {
		v, ok := parseNum(c.text[loc[0]:loc[1]])
		if !ok {
			continue
		}
		c.nums = append(c.nums, v)
		if m := scaleRe.FindStringSubmatch(c.text[loc[1]:]); m != nil {
			c.nums = append(c.nums, v*scales[m[1]])
		}
	}
	for _, m := range dateRe.FindAllStringSubmatch(c.text, -1) {
		d, _ := strconv.Atoi(m[1])
		c.dates[[2]int{d, monthIndex(m[2])}] = true
	}
	for _, m := range dotDateRe.FindAllStringSubmatch(c.text, -1) {
		d, _ := strconv.Atoi(m[1])
		mo, _ := strconv.Atoi(m[2])
		c.dates[[2]int{d, mo}] = true
	}
	// ISO-даты публикации (2026-09-11).
	for _, m := range isoDateRe.FindAllStringSubmatch(c.text, -1) {
		mo, _ := strconv.Atoi(m[2])
		d, _ := strconv.Atoi(m[3])
		c.dates[[2]int{d, mo}] = true
		if y, ok := parseNum(m[1]); ok {
			c.nums = append(c.nums, y)
		}
	}
	return c
}

func (c *corpus) hasNumber(v float64) bool {
	for _, n := range c.nums {
		if n == v || (v != 0 && math.Abs(n-v)/math.Abs(v) < 0.005) {
			return true
		}
	}
	return false
}

func (c *corpus) verify(e extracted) bool {
	switch e.kind {
	case "date":
		return c.dates[[2]int{e.day, e.month}]
	case "entity":
		name := normText(e.value)
		return strings.Contains(c.text, name)
	default:
		for _, v := range e.nums {
			if c.hasNumber(v) {
				return true
			}
		}
		return false
	}
}

// ---------------------------------------------------------------------------
// Проверка анализа
// ---------------------------------------------------------------------------

// checkText проверяет текст и возвращает неподтверждённые значения.
func (fc *FactCheck) checkText(c *corpus, path, text string, withTerms bool) []UnverifiedValue {
	var out []UnverifiedValue
	for _, e := range extractValues(text, withTerms) {
		fc.Checked++
		if c.verify(e) {
			fc.Verified++
			continue
		}
		out = append(out, UnverifiedValue{Path: path, Kind: e.kind, Value: e.value, Claim: short2(text, 160)})
	}
	return out
}

// FactCheckInsight проверяет числа, даты и названия в анализе по реестру,
// удаляет неподтверждённые факты и позиции банка, понижает основание
// рисков и возможностей и возвращает заметки валидатора.
func FactCheckInsight(in *Insight, reg *Registry) []string {
	c := newCorpus(reg)
	fc := &FactCheck{}
	var notes []string
	mark := func(vals []UnverifiedValue, action string) {
		for i := range vals {
			vals[i].Action = action
		}
		fc.Unverified = append(fc.Unverified, vals...)
	}

	var facts []Claim
	for i, f := range in.WhatHappened.Facts {
		if bad := fc.checkText(c, fmt.Sprintf("what_happened.facts[%d]", i), f.Text, true); len(bad) > 0 {
			mark(bad, "removed")
			notes = append(notes, "Факт удалён: значение «"+bad[0].Value+"» не найдено в источниках")
			continue
		}
		facts = append(facts, f)
	}
	in.WhatHappened.Facts = nonNil(facts)

	var bank []Claim
	for i, b := range in.BankView {
		if bad := fc.checkText(c, fmt.Sprintf("bank_view[%d]", i), b.Text, true); len(bad) > 0 {
			mark(bad, "removed")
			notes = append(notes, "Позиция банка удалена: значение «"+bad[0].Value+"» не найдено в материалах")
			continue
		}
		bank = append(bank, b)
	}
	in.BankView = nonNil(bank)

	mark(fc.checkText(c, "what_happened.summary", in.WhatHappened.Summary, true), "marked")
	mark(fc.checkText(c, "why_it_matters", in.WhyItMatters, false), "marked")
	mark(fc.checkText(c, "potential_impact", in.PotentialImpact, false), "marked")
	ro := func(items []RiskOpp, name, kind string) {
		for i := range items {
			r := &items[i]
			bad := fc.checkText(c, fmt.Sprintf("%s[%d]", name, i), r.Title+". "+r.Description, false)
			if len(bad) == 0 {
				continue
			}
			mark(bad, "marked")
			if r.Basis == "fact" || r.Basis == "bank_view" {
				r.Basis = "model_inference"
				notes = append(notes, kind+" «"+short(r.Title)+"»: значение «"+bad[0].Value+"» не найдено в источниках — помечено как вывод модели")
			}
		}
	}
	ro(in.Risks, "risks", "Риск")
	ro(in.Opportunities, "opportunities", "Возможность")
	for i, a := range in.RecommendedActions {
		mark(fc.checkText(c, fmt.Sprintf("recommended_actions[%d]", i), a.Action+". "+a.Rationale, false), "marked")
	}
	if len(fc.Unverified) > 0 {
		marked := 0
		for _, u := range fc.Unverified {
			if u.Action == "marked" && marked < 3 {
				in.UncertaintyNotes = append(in.UncertaintyNotes, "Значение «"+u.Value+"» не найдено в источниках — проверьте его перед использованием.")
				marked++
			}
		}
	}
	fc.Unverified = nonNil(fc.Unverified)
	in.FactCheck = fc
	return notes
}

// ComputeSourceCoverage считает долю значимых утверждений с опорой на источники.
func ComputeSourceCoverage(in *Insight) *SourceCoverage {
	bad := map[string]bool{}
	if in.FactCheck != nil {
		for _, u := range in.FactCheck.Unverified {
			if u.Action == "marked" {
				bad[u.Path] = true
			}
		}
	}
	cov := &SourceCoverage{}
	count := func(path string, ids []string) {
		cov.Total++
		if len(ids) > 0 && !bad[path] {
			cov.Confirmed++
		}
	}
	for i, f := range in.WhatHappened.Facts {
		count(fmt.Sprintf("what_happened.facts[%d]", i), f.SourceIDs)
	}
	count("why_it_matters", in.WhyItMattersIDs)
	for i, r := range in.Risks {
		count(fmt.Sprintf("risks[%d]", i), r.SourceIDs)
	}
	for i, r := range in.Opportunities {
		count(fmt.Sprintf("opportunities[%d]", i), r.SourceIDs)
	}
	for i, a := range in.RecommendedActions {
		count(fmt.Sprintf("recommended_actions[%d]", i), a.SourceIDs)
	}
	for i, b := range in.BankView {
		count(fmt.Sprintf("bank_view[%d]", i), b.SourceIDs)
	}
	if cov.Total > 0 {
		cov.Ratio = math.Round(float64(cov.Confirmed)/float64(cov.Total)*100) / 100
	}
	return cov
}

// FactCheckIssues — замечания для доработки анализа агентом.
func FactCheckIssues(in *Insight) []CheckIssue {
	if in.FactCheck == nil {
		return nil
	}
	var out []CheckIssue
	for _, u := range in.FactCheck.Unverified {
		if u.Action != "marked" {
			continue
		}
		out = append(out, CheckIssue{Path: u.Path, Severity: "high",
			Problem: "значение «" + u.Value + "» не найдено ни в одном источнике реестра: убери его или замени значением из источника"})
		if len(out) == 5 {
			break
		}
	}
	return out
}
