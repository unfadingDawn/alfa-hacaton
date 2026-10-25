package analysis

import (
	"fmt"
	"strings"
)

// Содержательность анализа (критерий 02): вывод должен объяснять механизм
// влияния через конкретные признаки компании, а действия — быть конкретными:
// объект, ответственный, горизонт и причина. Общие советы вроде «следить за
// развитием ситуации» удаляются валидатором.

// CompanyFactor — признак компании, повлиявший на вывод, и механизм влияния.
type CompanyFactor struct {
	FactID    string `json:"fact_id"`
	Mechanism string `json:"mechanism"`
}

// Формулировки, которые не являются действием: их можно написать про любую новость.
var genericActionPhrases = []string{
	"следить за развитием", "следить за ситуацией", "следить за новостями", "отслеживать ситуацию", "отслеживать развитие",
	"мониторить ситуацию", "держать руку на пульсе", "держать в поле зрения", "оценить потенциальные риски",
	"оценить возможные риски", "оценить риски", "оценить последствия", "проанализировать ситуацию", "изучить ситуацию",
	"быть готовым к изменениям", "быть готовыми к изменениям", "принять необходимые меры", "принять меры",
	"учитывать при планировании", "принять во внимание", "иметь в виду",
}

// Объекты, которые ничего не конкретизируют.
var genericObjects = map[string]bool{
	"ситуация": true, "развитие ситуации": true, "риски": true, "потенциальные риски": true, "возможные риски": true,
	"рынок": true, "изменения": true, "новости": true, "последствия": true, "бизнес": true, "компания": true,
}

func normPhrase(s string) string {
	return strings.Join(strings.Fields(strings.ToLower(strings.ReplaceAll(s, "ё", "е"))), " ")
}

// IsGenericAction — действие сформулировано общими словами.
func IsGenericAction(a Action) bool {
	text := normPhrase(a.Action)
	for _, p := range genericActionPhrases {
		if strings.Contains(text, p) {
			return true
		}
	}
	obj := strings.Trim(normPhrase(a.Object), " .,«»\"")
	return obj == "" || genericObjects[obj]
}

// validateFactors оставляет только признаки компании из реестра с описанным механизмом.
func validateFactors(in *Insight, reg *Registry) []string {
	var notes []string
	var out []CompanyFactor
	seen := map[string]bool{}
	dropped := 0
	for _, f := range in.UsedCompanyFactors {
		id := strings.Trim(strings.TrimSpace(f.FactID), "[]")
		f.Mechanism = short2(f.Mechanism, 300)
		e, ok := reg.Get(id)
		if !ok || e.Type != "company_data" || f.Mechanism == "" {
			dropped++
			continue
		}
		if seen[id] {
			continue
		}
		seen[id] = true
		out = append(out, CompanyFactor{FactID: id, Mechanism: f.Mechanism})
		if len(out) == 5 {
			break
		}
	}
	if dropped > 0 {
		notes = append(notes, fmt.Sprintf("Удалено признаков компании без механизма или вне профиля: %d", dropped))
	}
	in.UsedCompanyFactors = nonNil(out)
	return notes
}

// validateActions удаляет общие и необоснованные действия и нормализует поля.
func validateActions(in *Insight, reg *Registry) []string {
	var notes []string
	var acts []Action
	for _, a := range in.RecommendedActions {
		a.Action, a.Object = short2(a.Action, 300), short2(a.Object, 150)
		a.Owner, a.Rationale = short2(a.Owner, 100), short2(a.Rationale, 400)
		if a.Action == "" {
			continue
		}
		if IsGenericAction(a) {
			notes = append(notes, "Удалено общее действие без конкретного объекта: «"+short(a.Action)+"»")
			continue
		}
		if a.Rationale == "" {
			notes = append(notes, "Удалено действие без причины: «"+short(a.Action)+"»")
			continue
		}
		if !horizons[a.Horizon] {
			a.Horizon = "medium"
		}
		var ids []string
		for _, id := range a.SourceIDs {
			id = strings.Trim(strings.TrimSpace(id), "[]")
			if reg.Has(id) && !contains(ids, id) {
				ids = append(ids, id)
			}
		}
		a.SourceIDs = nonNil(ids)
		acts = append(acts, a)
		if len(acts) == 4 {
			break
		}
	}
	in.RecommendedActions = nonNil(acts)
	return notes
}

func contains(in []string, s string) bool {
	for _, x := range in {
		if x == s {
			return true
		}
	}
	return false
}

// ContentIssues — замечания о содержательности для доработки агентом:
// значимая новость без механизма через признаки компании, без конкретных
// действий или с действиями без ответственного.
func ContentIssues(in *Insight) []CheckIssue {
	if in.RelevanceScore < 40 {
		return nil
	}
	var out []CheckIssue
	if len(in.UsedCompanyFactors) == 0 {
		out = append(out, CheckIssue{Path: "used_company_factors", Severity: "high",
			Problem: "не указано, через какие признаки компании (C:…) событие влияет на бизнес и каков механизм влияния"})
	}
	if len(in.RecommendedActions) == 0 {
		out = append(out, CheckIssue{Path: "recommended_actions", Severity: "high",
			Problem: "нет конкретных действий: укажи объект, ответственного, горизонт и причину; общие советы запрещены"})
	}
	for i, a := range in.RecommendedActions {
		if strings.TrimSpace(a.Owner) == "" {
			out = append(out, CheckIssue{Path: fmt.Sprintf("recommended_actions[%d]", i), Severity: "high",
				Problem: "у действия не указан ответственный (роль или подразделение)"})
		}
	}
	return out
}
