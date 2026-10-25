package analysis

import (
	"fmt"
	"sort"
	"strings"
)

// Раскрытие трассы агента (критерий 12): какие источники найдены и как
// использованы, какие запросы ничего не дали, что изменилось после доработки.
// В трассе только структурированные результаты шагов — скрытые рассуждения
// модели не сохраняются и не показываются.

// SearchQuery — один запрос инструмента поиска.
type SearchQuery struct {
	Tool     string   `json:"tool"` // search_research | search_news
	Query    string   `json:"query"`
	Stage    string   `json:"stage"` // plan | self_check
	FoundIDs []string `json:"found_ids"`
}

// SourceUse — источник реестра и его роль в выводе.
type SourceUse struct {
	ID       string   `json:"id"`
	Type     string   `json:"type"`
	Title    string   `json:"title"`
	Page     int      `json:"page,omitempty"`
	Query    string   `json:"query,omitempty"` // по какому запросу найден
	Used     bool     `json:"used"`
	Sections []string `json:"sections"` // в каких разделах анализа на него ссылаются
}

type SourcesReport struct {
	Sources         []SourceUse   `json:"sources"`
	EmptyQueries    []SearchQuery `json:"empty_queries"`
	Queries         []SearchQuery `json:"queries"`
	CompanyFactsAll int           `json:"company_facts_total"`
}

// citedSections — в каких разделах анализа встречается каждый источник.
func citedSections(in *Insight) map[string][]string {
	out := map[string][]string{}
	add := func(section string, ids []string) {
		for _, id := range ids {
			if !contains(out[id], section) {
				out[id] = append(out[id], section)
			}
		}
	}
	for _, f := range in.WhatHappened.Facts {
		add("Что произошло", f.SourceIDs)
	}
	add("Почему важно", in.WhyItMattersIDs)
	for _, f := range in.UsedCompanyFactors {
		add("Признаки компании", []string{f.FactID})
	}
	for _, r := range in.Risks {
		add("Риски", r.SourceIDs)
	}
	for _, r := range in.Opportunities {
		add("Возможности", r.SourceIDs)
	}
	for _, a := range in.RecommendedActions {
		add("Действия", a.SourceIDs)
	}
	for _, b := range in.BankView {
		add("Позиция банка", b.SourceIDs)
	}
	return out
}

// BuildSourcesReport показывает найденные инструментами источники, их
// использование в выводах и запросы без результатов. Факты компании
// перечисляются только использованные — остальные видны в профиле.
func BuildSourcesReport(reg *Registry, in *Insight, searches []SearchQuery) SourcesReport {
	sections := citedSections(in)
	used := map[string]bool{}
	for _, e := range in.EvidenceSources {
		used[e.ID] = true
	}
	foundBy := map[string]string{}
	for _, q := range searches {
		for _, id := range q.FoundIDs {
			if _, ok := foundBy[id]; !ok {
				foundBy[id] = q.Query
			}
		}
	}
	rep := SourcesReport{Sources: []SourceUse{}, EmptyQueries: []SearchQuery{}, Queries: nonNil(searches)}
	for _, e := range reg.Items {
		if e.Type == "company_data" {
			rep.CompanyFactsAll++
			if !used[e.ID] {
				continue
			}
		}
		rep.Sources = append(rep.Sources, SourceUse{ID: e.ID, Type: e.Type, Title: e.Title, Page: e.Page, Query: foundBy[e.ID],
			Used: used[e.ID], Sections: nonNil(sections[e.ID])})
	}
	sort.SliceStable(rep.Sources, func(i, j int) bool {
		if rep.Sources[i].Used != rep.Sources[j].Used {
			return rep.Sources[i].Used
		}
		return order(rep.Sources[i].Type) < order(rep.Sources[j].Type)
	})
	for _, q := range searches {
		if len(q.FoundIDs) == 0 {
			rep.EmptyQueries = append(rep.EmptyQueries, q)
		}
	}
	return rep
}

// Revision — итог самопроверки и доработки.
type Revision struct {
	Applied bool         `json:"applied"`
	Issues  []CheckIssue `json:"issues"`
	Changes []string     `json:"changes"`
	Note    string       `json:"note,omitempty"`
}

// DiffInsights описывает, что изменилось после доработки, человеческим языком.
func DiffInsights(before, after *Insight) []string {
	var out []string
	if before.RelevanceScore != after.RelevanceScore {
		out = append(out, fmt.Sprintf("Релевантность: %d → %d", before.RelevanceScore, after.RelevanceScore))
	}
	if before.ImportanceLevel != after.ImportanceLevel {
		out = append(out, fmt.Sprintf("Важность: %s → %s", before.ImportanceLevel, after.ImportanceLevel))
	}
	if before.ImpactType != after.ImpactType {
		out = append(out, fmt.Sprintf("Тип влияния: %s → %s", before.ImpactType, after.ImpactType))
	}
	setDiff := func(label string, a, b []string) {
		added, removed := diffLists(a, b)
		if len(added) > 0 {
			out = append(out, label+" — добавлено: "+strings.Join(added, "; "))
		}
		if len(removed) > 0 {
			out = append(out, label+" — убрано: "+strings.Join(removed, "; "))
		}
	}
	titles := func(items []RiskOpp) []string {
		var r []string
		for _, x := range items {
			r = append(r, x.Title)
		}
		return r
	}
	setDiff("Риски", titles(before.Risks), titles(after.Risks))
	setDiff("Возможности", titles(before.Opportunities), titles(after.Opportunities))
	acts := func(items []Action) []string {
		var r []string
		for _, x := range items {
			r = append(r, short2(x.Action, 80))
		}
		return r
	}
	setDiff("Действия", acts(before.RecommendedActions), acts(after.RecommendedActions))
	factors := func(items []CompanyFactor) []string {
		var r []string
		for _, x := range items {
			r = append(r, x.FactID)
		}
		return r
	}
	setDiff("Признаки компании", factors(before.UsedCompanyFactors), factors(after.UsedCompanyFactors))
	if len(before.WhatHappened.Facts) != len(after.WhatHappened.Facts) {
		out = append(out, fmt.Sprintf("Подтверждённых фактов: %d → %d", len(before.WhatHappened.Facts), len(after.WhatHappened.Facts)))
	}
	if len(before.BankView) != len(after.BankView) {
		out = append(out, fmt.Sprintf("Позиций Альфа-Банка: %d → %d", len(before.BankView), len(after.BankView)))
	}
	unv := func(in *Insight) int {
		if in.FactCheck == nil {
			return 0
		}
		return len(in.FactCheck.Unverified)
	}
	if unv(before) != unv(after) {
		out = append(out, fmt.Sprintf("Значений, не найденных в источниках: %d → %d", unv(before), unv(after)))
	}
	if before.SourceCoverage != nil && after.SourceCoverage != nil && before.SourceCoverage.Ratio != after.SourceCoverage.Ratio {
		out = append(out, fmt.Sprintf("Покрытие источниками: %.0f%% → %.0f%%", before.SourceCoverage.Ratio*100, after.SourceCoverage.Ratio*100))
	}
	if len(out) == 0 {
		out = append(out, "Существенных изменений в структуре анализа нет: уточнены формулировки")
	}
	return out
}

func diffLists(a, b []string) (added, removed []string) {
	in := func(list []string, s string) bool {
		n := normPhrase(s)
		for _, x := range list {
			if normPhrase(x) == n {
				return true
			}
		}
		return false
	}
	for _, x := range b {
		if !in(a, x) {
			added = append(added, x)
		}
	}
	for _, x := range a {
		if !in(b, x) {
			removed = append(removed, x)
		}
	}
	return
}
