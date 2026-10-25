package analysis

import (
	"strings"
	"testing"
)

func TestBuildSourcesReport(t *testing.T) {
	reg := registry()
	reg.Add(Evidence{ID: "R11", Type: "bank_research", Title: "Инфляция", Excerpt: "Инфляция 6,8%."})
	reg.Add(Evidence{ID: "C:okved", Type: "company_data", Title: "ОКВЭД", Excerpt: "46.34"})
	in := &Insight{
		WhatHappened:       WhatHappened{Facts: []Claim{{Text: "Ставка 14%", SourceIDs: []string{"A1"}}}},
		WhyItMattersIDs:    []string{"C:has_loan"},
		UsedCompanyFactors: []CompanyFactor{{FactID: "C:has_loan", Mechanism: "стоимость долга"}},
		BankView:           []Claim{{Text: "Пауза", SourceIDs: []string{"R10"}}},
		Risks:              []RiskOpp{{Title: "Долг", SourceIDs: []string{"R10", "C:has_loan"}}},
	}
	in.EvidenceSources = usedEvidence(in, reg)
	searches := []SearchQuery{
		{Tool: "search_research", Query: "ключевая ставка пауза", Stage: "plan", FoundIDs: []string{"R10", "R11"}},
		{Tool: "search_research", Query: "ковенанты", Stage: "self_check", FoundIDs: []string{}},
	}
	rep := BuildSourcesReport(reg, in, searches)
	got := map[string]SourceUse{}
	for _, s := range rep.Sources {
		got[s.ID] = s
	}
	if _, ok := got["C:okved"]; ok {
		t.Fatal("неиспользованные факты компании в отчёт не попадают")
	}
	if r := got["R10"]; !r.Used || r.Query != "ключевая ставка пауза" || strings.Join(r.Sections, ",") != "Риски,Позиция банка" {
		t.Fatalf("использованный материал банка: %+v", r)
	}
	if r := got["R11"]; r.Used || len(r.Sections) != 0 {
		t.Fatalf("найденный, но неиспользованный материал: %+v", r)
	}
	if c := got["C:has_loan"]; strings.Join(c.Sections, ",") != "Почему важно,Признаки компании,Риски" {
		t.Fatalf("разделы признака компании: %+v", c)
	}
	if rep.Sources[len(rep.Sources)-1].ID != "R11" {
		t.Fatal("неиспользованные источники идут в конце")
	}
	if len(rep.EmptyQueries) != 1 || rep.EmptyQueries[0].Query != "ковенанты" || rep.CompanyFactsAll != 2 {
		t.Fatalf("запросы без результатов: %+v", rep)
	}
}

func TestDiffInsights(t *testing.T) {
	before := &Insight{RelevanceScore: 82, ImportanceLevel: "high", Risks: []RiskOpp{{Title: "Дорогой долг"}, {Title: "Рост расходов на 3 млрд"}},
		RecommendedActions: []Action{{Action: "Пересмотреть график"}}, FactCheck: &FactCheck{Unverified: []UnverifiedValue{{Value: "3 млрд"}}}}
	after := &Insight{RelevanceScore: 75, ImportanceLevel: "high", Risks: []RiskOpp{{Title: "Дорогой  долг"}},
		RecommendedActions: []Action{{Action: "Пересмотреть график"}, {Action: "Запросить фиксацию ставки"}},
		UsedCompanyFactors: []CompanyFactor{{FactID: "C:has_loan"}}, FactCheck: &FactCheck{}}
	got := strings.Join(DiffInsights(before, after), "\n")
	for _, want := range []string{"Релевантность: 82 → 75", "Риски — убрано: Рост расходов на 3 млрд", "Действия — добавлено: Запросить фиксацию ставки",
		"Признаки компании — добавлено: C:has_loan", "не найденных в источниках: 1 → 0"} {
		if !strings.Contains(got, want) {
			t.Errorf("нет изменения %q в:\n%s", want, got)
		}
	}
	if strings.Contains(got, "Риски — добавлено") {
		t.Error("регистр и пробелы в названии риска не считаются изменением")
	}
	if d := DiffInsights(after, after); len(d) != 1 || !strings.Contains(d[0], "Существенных изменений") {
		t.Fatalf("без изменений: %v", d)
	}
}
