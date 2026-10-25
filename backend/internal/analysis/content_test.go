package analysis

import (
	"strings"
	"testing"
)

func TestIsGenericAction(t *testing.T) {
	generic := []Action{
		{Action: "Следить за развитием ситуации", Object: "кредитный портфель"},
		{Action: "Оценить потенциальные риски для бизнеса", Object: "бизнес"},
		{Action: "Проанализировать ситуацию на рынке", Object: "рынок"},
		{Action: "Пересмотреть график рефинансирования", Object: ""},
		{Action: "Пересмотреть бюджет", Object: "риски"},
	}
	for _, a := range generic {
		if !IsGenericAction(a) {
			t.Errorf("общее действие не распознано: %+v", a)
		}
	}
	concrete := Action{Action: "Запросить у банка условия фиксации ставки", Object: "кредитная линия на 2 млрд руб."}
	if IsGenericAction(concrete) {
		t.Error("конкретное действие помечено как общее")
	}
}

func TestValidateActionsAndFactors(t *testing.T) {
	in := &Insight{
		RelevanceScore: 70, ImportanceLevel: "high", ImpactType: "risk",
		WhatHappened: WhatHappened{Summary: "ЦБ сохранил ставку 14%."}, WhyItMatters: "У компании есть кредиты",
		RecommendedActions: []Action{
			{Action: "Следить за развитием ситуации", Object: "ставка", Owner: "CFO", Horizon: "short", Rationale: "ставка высокая"},
			{Action: "Пересмотреть график рефинансирования", Object: "кредитная линия", Owner: "Финансовый директор",
				Horizon: "когда-нибудь", Rationale: "у компании есть кредиты [C:has_loan]", SourceIDs: []string{"C:has_loan", "X1"}},
			{Action: "Обновить бюджет процентных расходов", Object: "бюджет на 2027 год", Owner: "", Horizon: "medium"},
			{Action: "Запросить котировки поставщиков", Object: "закупка упаковки", Owner: "Закупки", Horizon: "short", Rationale: ""},
		},
		UsedCompanyFactors: []CompanyFactor{
			{FactID: "C:has_loan", Mechanism: "сохранение ставки удерживает стоимость обслуживания долга"},
			{FactID: "C:has_loan", Mechanism: "дубль"},
			{FactID: "C:unknown", Mechanism: "признака нет в профиле"},
			{FactID: "A1", Mechanism: "это не признак компании"},
			{FactID: "C:okved", Mechanism: ""},
		},
	}
	reg := registry()
	reg.Add(Evidence{ID: "C:okved", Type: "company_data", Title: "ОКВЭД", Excerpt: "46.34"})
	if err := ValidateInsight(in, reg, topics); err != nil {
		t.Fatal(err)
	}
	if len(in.RecommendedActions) != 1 {
		t.Fatalf("остаётся только конкретное обоснованное действие: %+v", in.RecommendedActions)
	}
	a := in.RecommendedActions[0]
	if a.Horizon != "medium" || strings.Join(a.SourceIDs, ",") != "C:has_loan" {
		t.Fatalf("горизонт нормализуется, неизвестные источники удаляются: %+v", a)
	}
	if len(in.UsedCompanyFactors) != 1 || in.UsedCompanyFactors[0].FactID != "C:has_loan" {
		t.Fatalf("признаки компании: %+v", in.UsedCompanyFactors)
	}
	notes := strings.Join(in.Validation, "\n")
	if !strings.Contains(notes, "общее действие") || !strings.Contains(notes, "без причины") || !strings.Contains(notes, "признаков компании") {
		t.Fatalf("заметки валидатора: %s", notes)
	}
	found := false
	for _, e := range in.EvidenceSources {
		if e.ID == "C:has_loan" {
			found = true
		}
	}
	if !found {
		t.Fatal("признак компании из used_company_factors попадает в источники")
	}
	if issues := ContentIssues(in); len(issues) != 0 {
		t.Fatalf("анализ с механизмом и действием не требует доработки: %+v", issues)
	}
}

func TestContentIssues(t *testing.T) {
	in := &Insight{RelevanceScore: 75, RecommendedActions: []Action{{Action: "x", Object: "y", Rationale: "z"}}}
	paths := []string{}
	for _, i := range ContentIssues(in) {
		paths = append(paths, i.Path)
	}
	if strings.Join(paths, ",") != "used_company_factors,recommended_actions[0]" {
		t.Fatalf("замечания: %v", paths)
	}
	in.RecommendedActions = nil
	if got := len(ContentIssues(in)); got != 2 {
		t.Fatalf("без действий и признаков — два замечания, получено %d", got)
	}
	if ContentIssues(&Insight{RelevanceScore: 20}) != nil {
		t.Fatal("слабо связанная новость не требует действий")
	}
}
