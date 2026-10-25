package analysis

import (
	"sort"
	"strings"
)

// Вспомогательный слой без языковой модели: предварительная разметка тем по словарю
// основ слов и предварительная оценка релевантности. Нужен, чтобы лента
// работала без массовых запросов к модели. В интерфейсе такие оценки
// помечаются как «предварительные, по правилам»; окончательную оценку
// и интерпретацию выполняет разрешённая модель по запросу пользователя.

var lexicon = map[string][]string{
	"monetary_policy":  {"ключев ставк", "банк росси", "цб рф", "денежно-кредитн", "регулятор", "набиуллин", "ставку", "смягчени", "ужесточени"},
	"inflation":        {"инфляц", "потребительских цен", "рост цен", "дефляц", "росстат зафиксировал"},
	"fx":               {"курс рубл", "курс доллар", "курс евро", "юан", "валют", "рубль укреп", "рубль ослаб"},
	"debt_market":      {"офз", "облигац", "доходност", "долгов рын", "купон", "размещени"},
	"credit":           {"кредит", "заёмщик", "заемщик", "ипотек", "лизинг", "займ", "рефинансир", "гаранти"},
	"commodities":      {"сырь", "биржев", "товарн рын", "удобрени", "уголь", "угля", "алюмин", "медь", "меди "},
	"oil_gas":          {"нефт", "газ ", "газа", "газов", "топлив", "бензин", "дизел", "opec", "опек", "brent", "нпз"},
	"metals":           {"металл", "стал", "прокат", "никел", "алюмин", "руд"},
	"gold":             {"золот", "драгметалл", "серебр"},
	"agriculture":      {"зерн", "пшениц", "урожа", "агро", "сельхоз", "продовольств", "молок", "мяс"},
	"logistics":        {"логистик", "перевоз", "грузо", "ржд", "порт", "контейнер", "транспорт", "фрахт"},
	"trade":            {"экспорт", "импорт", "пошлин", "санкц", "внешнеторг", "таможен", "параллельн импорт"},
	"regulation":       {"закон", "законопроект", "госдум", "постановлени", "требовани", "регулирован", "минфин", "минэконом", "фас"},
	"taxes_budget":     {"налог", "ндс", "ндфл", "бюджет", "дефицит бюджет", "фнс"},
	"labor":            {"рынок труда", "зарплат", "безработиц", "кадр", "занятост", "персонал"},
	"consumer_demand":  {"потребител", "спрос", "розничн продаж", "потреблени", "покупател"},
	"construction":     {"строител", "девелоп", "жиль", "недвижим", "застройщик"},
	"retail_wholesale": {"ритейл", "торгов сет", "маркетплейс", "оптов", "розниц"},
	"technology":       {"цифров", "ит-", "искусственн интеллект", "технолог", "софт", "программн"},
	"energy":           {"электроэнерг", "энергосистем", "тариф", "генерац"},
	"macro":            {"ввп", "экономик", "макроэконом", "деловой активност", "pmi", "рецесс"},
	"finance_sector":   {"банк", "финансов рын", "биржа", "мосбирж", "страхов", "депозит", "вклад"},
	"corporate":        {"сделк", "отчетност", "отчётност", "выручк", "прибыл", "дивиденд", "ipo", "слиян"},
}

// stemPhrase сопоставляет «основу» фразы с текстом: каждое слово фразы
// должно встречаться как префикс слова текста, слова — по порядку.
func stemPhrase(text, phrase string) bool {
	if !strings.Contains(phrase, " ") || strings.HasSuffix(phrase, " ") {
		return strings.Contains(text, phrase)
	}
	parts := strings.Fields(phrase)
	idx := 0
	for _, p := range parts {
		i := strings.Index(text[idx:], p)
		if i < 0 {
			return false
		}
		idx += i + len(p)
	}
	return true
}

// KeywordTopics — предварительные темы статьи по словарю (не более 4).
func KeywordTopics(title, text string) []TopicScore {
	t := " " + strings.ToLower(strings.ReplaceAll(title+" "+title+" "+text, "ё", "е")) + " "
	type hit struct {
		code  string
		score int
	}
	var hits []hit
	for code, stems := range lexicon {
		n := 0
		for _, s := range stems {
			if stemPhrase(t, strings.ReplaceAll(s, "ё", "е")) {
				n++
			}
		}
		if n > 0 {
			hits = append(hits, hit{code, n})
		}
	}
	sort.Slice(hits, func(i, j int) bool {
		if hits[i].score != hits[j].score {
			return hits[i].score > hits[j].score
		}
		return hits[i].code < hits[j].code
	})
	var out []TopicScore
	for i, h := range hits {
		if i >= 4 {
			break
		}
		conf := 0.3 + 0.1*float64(h.score)
		if conf > 0.6 {
			conf = 0.6
		}
		out = append(out, TopicScore{Code: h.code, Confidence: conf})
	}
	return out
}

// RuleContext — то, что известно о пользователе и компании без модели.
type RuleContext struct {
	CompanyTopics []string       // темы по ОКВЭД и продуктам компании
	RoleTopics    []string       // темы роли пользователя
	Interests     map[string]int // темы интересов пользователя с весом 1..3
	OkvedDivision string
	Region        string
	Countries     []string
	ResearchBoost bool
}

type RuleScore struct {
	Score      int
	Importance string
	Matched    []string
	Reason     string
}

// ScoreByRules — прозрачная предварительная оценка 0..100.
func ScoreByRules(articleTopics []string, title, text string, rc RuleContext, isResearch bool) RuleScore {
	comp := toSet(rc.CompanyTopics)
	role := toSet(rc.RoleTopics)
	var matched []string
	var reasons []string
	var per []int
	for _, t := range articleTopics {
		v := 0
		if comp[t] {
			v += 12
		}
		if role[t] {
			v += 10
		}
		if w := rc.Interests[t]; w > 0 {
			v += 5 * w
		}
		if v > 0 {
			matched = append(matched, t)
			per = append(per, v)
		}
	}
	// Учитываются две самые сильные темы — чтобы длинный список тем не завышал оценку.
	sort.Sort(sort.Reverse(sort.IntSlice(per)))
	score := 0
	for i, v := range per {
		if i == 2 {
			break
		}
		score += v
	}
	if len(matched) > 0 {
		reasons = append(reasons, "темы совпадают с профилем компании, ролью или интересами")
	}
	low := strings.ToLower(title + " " + text)
	if rc.Region != "" && strings.Contains(low, strings.ToLower(stemRegion(rc.Region))) {
		score += 8
		reasons = append(reasons, "упоминается регион компании")
	}
	for _, c := range rc.Countries {
		if c != "" && strings.Contains(low, strings.ToLower(stemRegion(c))) {
			score += 8
			reasons = append(reasons, "упоминается страна из данных ВЭД компании")
			break
		}
	}
	if isResearch && rc.ResearchBoost && len(matched) > 0 {
		score += 5
		reasons = append(reasons, "материал аналитиков Альфа-Банка")
	}
	if score > 85 {
		score = 85 // правила не дают максимальной уверенности — это делает модель
	}
	imp := "low"
	switch {
	case score >= 55:
		imp = "high"
	case score >= 25:
		imp = "medium"
	}
	reason := "Предварительная оценка по правилам (без языковой модели): совпадений с профилем не найдено."
	if len(reasons) > 0 {
		reason = "Предварительная оценка по правилам (без языковой модели): " + strings.Join(reasons, "; ") + "."
	}
	return RuleScore{Score: score, Importance: imp, Matched: matched, Reason: reason}
}

// stemRegion — грубая основа названия ("Свердловская область" → "свердловск").
func stemRegion(s string) string {
	s = strings.Fields(s + " x")[0]
	r := []rune(s)
	switch {
	case len(r) > 6:
		r = r[:len(r)-2]
	case len(r) > 4:
		r = r[:len(r)-1]
	}
	return string(r)
}

func toSet(in []string) map[string]bool {
	m := map[string]bool{}
	for _, s := range in {
		m[s] = true
	}
	return m
}
