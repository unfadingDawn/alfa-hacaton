package analysis

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/alfainsight/backend/internal/roles"
)

func registry() *Registry {
	r := NewRegistry()
	r.Add(Evidence{ID: "A1", Type: "article", Title: "ЦБ сохранил ставку 14%", Excerpt: "Совет директоров сохранил ставку."})
	r.Add(Evidence{ID: "R10", Type: "bank_research", Title: "Ключевая ставка ЦБ РФ", Excerpt: "Мы ожидаем сохранения паузы."})
	r.Add(Evidence{ID: "C:has_loan", Type: "company_data", Title: "Кредитные продукты", Excerpt: "да"})
	return r
}

var topics = map[string]bool{"monetary_policy": true, "credit": true}

const modelAnswer = `{
 "relevance_score": 140, "importance_level": "огромная", "impact_type": "risk",
 "matched_topics": ["monetary_policy", "space_travel"],
 "what_happened": {"summary": "ЦБ сохранил ставку.", "facts": [
   {"text": "Ставка сохранена на 14%", "source_ids": ["A1"]},
   {"text": "Выдуманный факт", "source_ids": ["X9"]}]},
 "why_it_matters": "У компании есть кредиты", "why_it_matters_source_ids": ["C:has_loan", "Z1"],
 "potential_impact": "Стоимость долга не снизится",
 "risks": [
   {"title": "Дорогой долг", "description": "...", "likelihood": "high", "horizon": "short", "basis": "fact", "source_ids": ["C:has_loan"]},
   {"title": "Пауза надолго", "description": "...", "likelihood": "?", "basis": "bank_view", "source_ids": ["R10"]}],
 "opportunities": [],
 "recommended_actions": [{"action": "Проверить график рефинансирования", "source_ids": ["C:has_loan"]}],
 "bank_view": [{"text": "Банк ждёт паузы", "source_ids": ["R10"]}, {"text": "Банк ждёт снижения", "source_ids": ["A1"]}],
 "assumptions": [], "uncertainty_notes": []
}`

func TestValidateInsightGrounding(t *testing.T) {
	var in Insight
	if err := json.Unmarshal([]byte(modelAnswer), &in); err != nil {
		t.Fatal(err)
	}
	if err := ValidateInsight(&in, registry(), topics); err != nil {
		t.Fatal(err)
	}
	if in.RelevanceScore != 100 || in.ImportanceLevel != "medium" {
		t.Errorf("диапазоны/enum не исправлены: %d %s", in.RelevanceScore, in.ImportanceLevel)
	}
	if len(in.MatchedTopics) != 1 {
		t.Errorf("неизвестная тема должна удаляться: %v", in.MatchedTopics)
	}
	if len(in.WhatHappened.Facts) != 1 {
		t.Errorf("факт без источника должен удаляться: %+v", in.WhatHappened.Facts)
	}
	if strings.Join(in.WhyItMattersIDs, ",") != "C:has_loan" {
		t.Errorf("несуществующий ID должен удаляться: %v", in.WhyItMattersIDs)
	}
	if in.Risks[0].Basis != "model_inference" {
		t.Errorf("риск с basis=fact без ссылки на новость должен стать выводом модели")
	}
	if in.Risks[1].Basis != "bank_view" || in.Risks[1].Likelihood != "medium" {
		t.Errorf("риск с опорой на аналитику банка: %+v", in.Risks[1])
	}
	if len(in.BankView) != 1 || in.BankView[0].SourceIDs[0] != "R10" {
		t.Errorf("позиция банка без материала банка должна удаляться: %+v", in.BankView)
	}
	ids := []string{}
	for _, e := range in.EvidenceSources {
		ids = append(ids, e.ID)
	}
	if strings.Join(ids, ",") != "A1,R10,C:has_loan" {
		t.Errorf("источники собираются backend из использованных ID: %v", ids)
	}
	if len(in.Validation) == 0 {
		t.Error("исправления должны быть записаны в validation_notes")
	}
}

func TestValidateInsightRejectsEmpty(t *testing.T) {
	in := Insight{WhyItMatters: "x"}
	if err := ValidateInsight(&in, registry(), topics); err == nil {
		t.Fatal("пустое описание события — непригодный ответ")
	}
}

func TestRoleChangesPromptAndRules(t *testing.T) {
	cfo, _ := roles.Get("cfo")
	proc, _ := roles.Get("procurement")
	ev := Evidence{ID: "A1", Title: "Рост цен на сталь", Excerpt: "Цены на металлопрокат выросли"}
	mk := func(r roles.Role) Context {
		return Context{RoleTitle: r.Title, RoleFocus: r.Focus, CompanyBrief: "[C:okved] ОКВЭД: 25.61"}
	}
	p1 := InsightMessages(ev, mk(cfo), nil, registry(), "")[1].Content
	p2 := InsightMessages(ev, mk(proc), nil, registry(), "")[1].Content
	if !strings.Contains(p1, "маржинальность") || !strings.Contains(p2, "цены поставщиков") || p1 == p2 {
		t.Fatal("линза роли должна менять запрос к модели")
	}
	// Предварительная оценка по правилам тоже зависит от роли.
	art := []string{"commodities", "logistics"}
	base := RuleContext{CompanyTopics: []string{"metals"}}
	rc1, rc2 := base, base
	rc1.RoleTopics, rc2.RoleTopics = cfo.Topics, proc.Topics
	s1 := ScoreByRules(art, "Рост цен на сырьё", "", rc1, false)
	s2 := ScoreByRules(art, "Рост цен на сырьё", "", rc2, false)
	if s2.Score <= s1.Score {
		t.Fatalf("новость о сырье должна быть важнее закупщику: cfo=%d procurement=%d", s1.Score, s2.Score)
	}
	if !strings.Contains(s1.Reason, "без ИИ") {
		t.Fatal("оценка по правилам должна быть явно помечена")
	}
}

func TestKeywordTopics(t *testing.T) {
	ts := KeywordTopics("Банк России сохранил ключевую ставку", "Инфляция замедлилась, рубль укрепился")
	codes := map[string]bool{}
	for _, t := range ts {
		codes[t.Code] = true
		if t.Confidence > 0.6 {
			panic("словарная тема не может иметь высокую уверенность")
		}
	}
	if !codes["monetary_policy"] || !codes["inflation"] {
		t.Fatalf("темы: %+v", ts)
	}
}

func TestValidateClassification(t *testing.T) {
	c := Classification{Summary: "s", EventType: "weird", OkvedDivisions: []string{"46", "4", "abc"},
		Topics: []TopicScore{{Code: "inflation", Confidence: 3}, {Code: "nope"}}}
	if err := ValidateClassification(&c, []Topic{{Code: "inflation", Name: "Инфляция"}}); err != nil {
		t.Fatal(err)
	}
	if c.EventType != "other" || len(c.OkvedDivisions) != 1 || len(c.Topics) != 1 || c.Topics[0].Confidence != 0.5 {
		t.Fatalf("%+v", c)
	}
}

func TestValidateBrief(t *testing.T) {
	b := BriefScore{ArticleID: "a1", RelevanceScore: -5, ImportanceLevel: "x", MatchedTopics: []string{"credit", "zzz"}, WhyItMatters: "Важно"}
	if !ValidateBrief(&b, topics) || b.RelevanceScore != 0 || b.ImportanceLevel != "medium" || len(b.MatchedTopics) != 1 {
		t.Fatalf("%+v", b)
	}
	if ValidateBrief(&BriefScore{ArticleID: "a1"}, topics) {
		t.Fatal("пустое объяснение недопустимо")
	}
}
