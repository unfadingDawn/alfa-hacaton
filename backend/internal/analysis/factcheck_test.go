package analysis

import (
	"strings"
	"testing"
)

func factRegistry() *Registry {
	r := NewRegistry()
	r.Add(Evidence{ID: "A1", Type: "article", Title: "Банк России сохранил ключевую ставку на уровне 14% годовых", Date: "2026-09-11",
		Excerpt: "Совет директоров 11 сентября 2026 года сохранил ставку 14,00%. Следующее заседание — 23 октября."})
	r.Add(Evidence{ID: "R10", Type: "bank_research", Title: "Ключевая ставка ЦБ РФ", Date: "2026-09-12",
		Excerpt: "Мы ожидаем снижения ставки до 13% к концу года; инфляция 6,8% г/г."})
	r.Add(Evidence{ID: "C:revenue", Type: "company_data", Title: "Выручка компании", Excerpt: "44 401 187 115 руб."})
	r.Add(Evidence{ID: "C:has_loan", Type: "company_data", Title: "Кредитные продукты", Excerpt: "да"})
	return r
}

func TestExtractValues(t *testing.T) {
	got := extractValues("ЦБ 11 сентября 2026 сохранил ставку 14,5%; выручка 44,4 млрд ₽, срок 6 месяцев, «Газпром нефть».", true)
	kinds := map[string]string{}
	for _, e := range got {
		kinds[e.kind] += e.value + ";"
	}
	if kinds["date"] != "11 сентября;" || kinds["year"] != "2026;" || kinds["percent"] != "14,5%;" ||
		kinds["money"] != "44,4 млрд ₽;" || kinds["term"] != "6 месяцев;" || kinds["entity"] != "Газпром нефть;" {
		t.Fatalf("извлечение значений: %#v", kinds)
	}
}

func TestFactCheckInsight(t *testing.T) {
	in := &Insight{
		WhatHappened: WhatHappened{Summary: "11 сентября ЦБ сохранил ставку 14%.", Facts: []Claim{
			{Text: "Ставка сохранена на уровне 14%", SourceIDs: []string{"A1"}},
			{Text: "Ставка повышена до 15%", SourceIDs: []string{"A1"}},
			{Text: "Решение принято 12 сентября", SourceIDs: []string{"A1"}},
		}},
		WhyItMatters:    "Выручка компании 44,4 млрд ₽, есть кредиты",
		WhyItMattersIDs: []string{"C:revenue"},
		BankView: []Claim{
			{Text: "Банк ждёт снижения до 13% к концу года", SourceIDs: []string{"R10"}},
			{Text: "Банк ждёт ставку 12%", SourceIDs: []string{"R10"}},
		},
		Risks: []RiskOpp{
			{Title: "Рост расходов", Description: "Проценты вырастут на 3 млрд руб.", Basis: "fact", SourceIDs: []string{"A1"}},
			{Title: "Дорогой долг", Description: "Ставка 14% сохранится", Basis: "fact", SourceIDs: []string{"A1"}},
		},
		RecommendedActions: []Action{{Action: "В течение 2 недель пересмотреть график рефинансирования", SourceIDs: []string{"C:has_loan"}}},
	}
	notes := FactCheckInsight(in, factRegistry())
	if len(in.WhatHappened.Facts) != 1 || !strings.Contains(in.WhatHappened.Facts[0].Text, "14%") {
		t.Fatalf("факты с неподтверждёнными 15%% и 12 сентября должны удаляться: %+v", in.WhatHappened.Facts)
	}
	if len(in.BankView) != 1 || !strings.Contains(in.BankView[0].Text, "13%") {
		t.Fatalf("позиция банка с 12%% удаляется: %+v", in.BankView)
	}
	if in.Risks[0].Basis != "model_inference" || in.Risks[1].Basis != "fact" {
		t.Fatalf("риск с неподтверждённой суммой понижается до вывода модели: %+v", in.Risks)
	}
	fc := in.FactCheck
	var marked, removed []string
	for _, u := range fc.Unverified {
		if u.Action == "marked" {
			marked = append(marked, u.Path+"="+u.Value)
		} else {
			removed = append(removed, u.Path+"="+u.Value)
		}
	}
	if strings.Join(marked, ",") != "risks[0]=3 млрд руб" {
		t.Fatalf("помечено: %v (сроки в рекомендациях и подтверждённая выручка не помечаются)", marked)
	}
	if len(removed) != 3 || len(notes) < 4 {
		t.Fatalf("удалено: %v, заметки: %v", removed, notes)
	}
	if fc.Verified == 0 || fc.Checked != fc.Verified+len(fc.Unverified) {
		t.Fatalf("счётчики проверки: %+v", fc)
	}
	if len(in.UncertaintyNotes) != 1 || !strings.Contains(in.UncertaintyNotes[0], "3 млрд руб") {
		t.Fatalf("помеченное значение попадает в ограничения анализа: %v", in.UncertaintyNotes)
	}

	issues := FactCheckIssues(in)
	if len(issues) != 1 || issues[0].Severity != "high" || issues[0].Path != "risks[0]" {
		t.Fatalf("замечание для доработки: %+v", issues)
	}

	cov := ComputeSourceCoverage(in)
	// Значимые: 1 факт, why_it_matters, 2 риска, 1 действие, 1 позиция банка = 6; не подтверждён risks[0].
	if cov.Total != 6 || cov.Confirmed != 5 || cov.Ratio != 0.83 {
		t.Fatalf("покрытие источниками: %+v", cov)
	}
}

func TestFactCheckEntities(t *testing.T) {
	in := &Insight{WhatHappened: WhatHappened{Summary: "ЦБ сохранил ставку.", Facts: []Claim{
		{Text: "Решение принял «Банк России»", SourceIDs: []string{"A1"}},
		{Text: "Решение поддержал «Газпромбанк»", SourceIDs: []string{"A1"}},
	}}}
	FactCheckInsight(in, factRegistry())
	if len(in.WhatHappened.Facts) != 1 || !strings.Contains(in.WhatHappened.Facts[0].Text, "Банк России") {
		t.Fatalf("название организации, которого нет в источниках, удаляется вместе с фактом: %+v", in.WhatHappened.Facts)
	}
}

func TestCompletenessFor(t *testing.T) {
	if CompletenessFor("full_text").Note != "" || CompletenessFor("title_only").Note == "" || CompletenessFor("").Level != "summary" {
		t.Fatal("описание полноты источника")
	}
	if !strings.Contains(completenessRule("title_only"), "только заголовок") {
		t.Fatal("правило промпта для неполного источника")
	}
}
