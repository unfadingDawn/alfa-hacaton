package agent

// Тесты различий персонализации: одна и та же новость должна по-разному
// оцениваться и объясняться для разных ролей, отраслей и признаков компании.
// Языковая модель не вызывается: проверяются вход модели (промпт) и
// детерминированные слои — правила ранжирования и профиль компании.

import (
	"strings"
	"testing"

	"github.com/alfainsight/backend/internal/analysis"
	"github.com/alfainsight/backend/internal/companies"
	"github.com/alfainsight/backend/internal/db/dbq"
	"github.com/alfainsight/backend/internal/roles"
)

func ptr[T any](v T) *T { return &v }

type newsCase struct{ name, title, summary string }

var (
	rateNews      = newsCase{"ставка", "Банк России сохранил ключевую ставку на уровне 14%", "Совет директоров сохранил ставку; денежно-кредитная политика остаётся жёсткой."}
	commodityNews = newsCase{"сырьё", "Биржевые цены на сырьё и алюминий выросли на 12%", "Поставщики предупреждают о повышении отпускных цен на металл."}
	taxNews       = newsCase{"налоги", "Минфин предложил повысить НДС и изменить сроки уплаты налогов", "Законопроект внесён в Госдуму."}
	demandNews    = newsCase{"спрос", "Потребительский спрос замедлился: покупатели сокращают траты", "Потребление в сентябре снизилось."}
	laborNews     = newsCase{"кадры", "Дефицит ИТ-специалистов усилился, зарплаты программистов выросли", "Рынок труда в цифровой отрасли перегрет."}
)

func scoreFor(p companies.Profile, roleCode string, n newsCase) int {
	var topics []string
	for _, t := range analysis.KeywordTopics(n.title, n.summary) {
		topics = append(topics, t.Code)
	}
	r := roles.Resolve(roleCode)
	rc := analysis.RuleContext{CompanyTopics: p.AllTopics(), RoleTopics: r.Topics, Interests: map[string]int{},
		OkvedDivision: companies.Division(p.OkvedCode), Region: p.Region, Countries: p.Countries}
	return analysis.ScoreByRules(topics, n.title, n.summary, rc, false).Score
}

func wholesaler() companies.Profile {
	return companies.Build(dbq.Company{ID: "T_W", OkvedCode: "46.34", OkvedName: "Торговля оптовая напитками", OkvedDivision: "46",
		HasLoan: ptr(true), HasRko: ptr(true), Revenue: ptr(44_401_187_115.0), Region: ptr("Москва"), City: ptr("г. Москва")})
}

// Одна компания, одна новость — разные роли (CFO, закупки, CEO, главный бухгалтер).
func TestRoleDifferences(t *testing.T) {
	p := wholesaler()
	cases := []struct {
		news newsCase
		top  string
	}{{rateNews, "cfo"}, {commodityNews, "procurement"}, {demandNews, "ceo"}, {taxNews, "chief_accountant"}}
	roleCodes := []string{"cfo", "procurement", "ceo", "chief_accountant"}
	for _, c := range cases {
		scores := map[string]int{}
		for _, r := range roleCodes {
			scores[r] = scoreFor(p, r, c.news)
		}
		for _, r := range roleCodes {
			if r != c.top && scores[r] >= scores[c.top] {
				t.Errorf("новость «%s»: роль %s должна быть выше остальных, оценки %v", c.news.name, c.top, scores)
			}
		}
	}

	// Вход модели отличается линзой роли: фокус роли попадает в промпт.
	main := analysis.Evidence{ID: "A1", Type: "article", Title: rateNews.title, Excerpt: rateNews.summary}
	reg := analysis.NewRegistry()
	reg.Add(main)
	prompts := map[string]string{}
	for _, code := range roleCodes {
		r := roles.Resolve(code)
		ctx := analysis.Context{RoleTitle: r.Title, RoleFocus: r.Focus, CompanyBrief: CompanyBrief(p)}
		msgs := analysis.InsightMessages(main, ctx, nil, reg, "")
		prompts[code] = msgs[1].Content
		for _, f := range r.Focus {
			if !strings.Contains(msgs[1].Content, f) {
				t.Errorf("в промпт роли %s не попал фокус «%s»", code, f)
			}
		}
	}
	for i, a := range roleCodes {
		for _, b := range roleCodes[i+1:] {
			if prompts[a] == prompts[b] {
				t.Errorf("промпты ролей %s и %s совпадают", a, b)
			}
		}
	}
}

// Одна роль, одна новость — разные отрасли и признаки компании.
func TestIndustryAndFactDifferences(t *testing.T) {
	metal := companies.Build(dbq.Company{ID: "T_M", OkvedCode: "24.10", OkvedName: "Производство чугуна, стали и ферросплавов",
		OkvedDivision: "24", HasLoan: ptr(true), FtsFlag: ptr(true), FtsCountries: []string{"Китай"},
		Revenue: ptr(80_000_000_000.0), Region: ptr("Свердловская область"), City: ptr("г. Екатеринбург")})
	it := companies.Build(dbq.Company{ID: "T_I", OkvedCode: "62.01", OkvedName: "Разработка компьютерного программного обеспечения",
		OkvedDivision: "62", HasLoan: ptr(false), HasPayroll: ptr(true), Revenue: ptr(20_000_000_000.0),
		Region: ptr("Москва"), City: ptr("г. Москва")})

	cases := []struct {
		news   newsCase
		higher companies.Profile
		lower  companies.Profile
	}{
		{rateNews, metal, it},      // кредиты → стоимость долга
		{commodityNews, metal, it}, // металлургия → цены сырья и металлов
		{laborNews, it, metal},     // ИТ и зарплатный проект → рынок труда
	}
	for _, c := range cases {
		h, l := scoreFor(c.higher, "cfo", c.news), scoreFor(c.lower, "cfo", c.news)
		if h <= l {
			t.Errorf("новость «%s»: для %s (%d) оценка должна быть выше, чем для %s (%d)", c.news.name, c.higher.OkvedName, h, c.lower.OkvedName, l)
		}
	}

	exposureCodes := func(p companies.Profile) string {
		var out []string
		for _, e := range p.Exposures {
			out = append(out, e.Code)
		}
		return strings.Join(out, ",")
	}
	if exposureCodes(metal) == exposureCodes(it) {
		t.Fatalf("чувствительности должны зависеть от признаков компании: %s", exposureCodes(metal))
	}
	mb, ib := CompanyBrief(metal), CompanyBrief(it)
	if !strings.Contains(mb, "Стоимость заёмного финансирования") || strings.Contains(ib, "Стоимость заёмного финансирования") {
		t.Fatal("кредитная чувствительность есть только у компании с кредитами")
	}
	if !strings.Contains(mb, "Китай") || strings.Contains(ib, "Китай") {
		t.Fatal("страны ВЭД попадают в промпт только у компании с ВЭД")
	}
	if !strings.Contains(ib, "Фонд оплаты труда") {
		t.Fatal("зарплатный проект даёт чувствительность к рынку труда")
	}
}
