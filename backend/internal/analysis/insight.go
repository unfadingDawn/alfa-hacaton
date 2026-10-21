package analysis

import (
	"encoding/json"
	"fmt"
	"sort"
	"strings"

	"github.com/alfainsight/backend/internal/llm"
)

// Evidence — элемент реестра источников. Модель ссылается на ID, а список
// источников в ответе собирает backend: модель не может «придумать» источник.
type Evidence struct {
	ID      string `json:"id"`
	Type    string `json:"type"` // article | related_news | bank_research | company_data
	Title   string `json:"title"`
	URL     string `json:"url,omitempty"`
	Date    string `json:"date,omitempty"`
	Excerpt string `json:"excerpt"`
	Page    int    `json:"page,omitempty"`
	Note    string `json:"note,omitempty"`
}

type Registry struct {
	Items []Evidence
	byID  map[string]Evidence
}

func NewRegistry() *Registry { return &Registry{byID: map[string]Evidence{}} }

func (r *Registry) Add(e Evidence) {
	if _, ok := r.byID[e.ID]; ok {
		return
	}
	r.byID[e.ID] = e
	r.Items = append(r.Items, e)
}

func (r *Registry) Get(id string) (Evidence, bool) {
	e, ok := r.byID[strings.TrimSpace(id)]
	return e, ok
}

func (r *Registry) Has(id string) bool { _, ok := r.Get(id); return ok }

// Render — текст реестра для промпта.
func (r *Registry) Render(maxExcerpt int) string {
	var b strings.Builder
	for _, e := range r.Items {
		ex := e.Excerpt
		if rr := []rune(ex); len(rr) > maxExcerpt {
			ex = string(rr[:maxExcerpt]) + "…"
		}
		date := ""
		if e.Date != "" {
			date = ", " + e.Date
		}
		note := ""
		if e.Note != "" {
			note = " [" + e.Note + "]"
		}
		fmt.Fprintf(&b, "[%s] (%s%s) %s%s\n%s\n\n", e.ID, typeLabel(e.Type), date, e.Title, note, ex)
	}
	return b.String()
}

func typeLabel(t string) string {
	switch t {
	case "article":
		return "анализируемая новость"
	case "related_news":
		return "связанная новость"
	case "bank_research":
		return "аналитика Альфа-Банка"
	case "company_data":
		return "данные о компании"
	}
	return t
}

// ---------------------------------------------------------------------------
// Шаг 1. Планирование
// ---------------------------------------------------------------------------

type Plan struct {
	FocusQuestions  []string `json:"focus_questions"`
	ResearchQueries []string `json:"research_queries"`
	NewsQueries     []string `json:"news_queries"`
	CompanyFactIDs  []string `json:"company_fact_ids"`
	Hypotheses      []string `json:"hypotheses"`
}

type Context struct {
	RoleTitle     string
	RoleFocus     []string
	WorkFocus     string
	Interests     []string
	CompanyBrief  string // краткое описание компании (факты с ID)
	UserProfile   string // что пользователь написал о компании
	Horizon       string
	Detail        string
	TopicNames    map[string]string
}

func PlanMessages(article Evidence, cls *Classification, ctx Context) []llm.Message {
	clsText := "Классификация не выполнялась."
	if cls != nil {
		b, _ := json.Marshal(cls)
		clsText = string(b)
	}
	system := `Ты — планировщик аналитического агента банка. Твоя задача — решить, какие данные нужны,
чтобы объяснить влияние новости на конкретную компанию и конкретного сотрудника. Ты не пишешь выводы.
Отвечай только JSON.`
	user := fmt.Sprintf(`Новость [%s]: %s
%s
Классификация: %s

Сотрудник: %s. Его фокус: %s. Рабочие интересы: %s. Темы интереса: %s.
Компания (факты с ID):
%s
Сведения от пользователя о компании: %s

Составь план:
- focus_questions: 2–4 вопроса, на которые должен ответить анализ для этой роли;
- research_queries: 1–3 коротких поисковых запроса (2–5 слов) по аналитике Альфа-Банка;
- news_queries: 0–2 запроса для поиска связанных новостей;
- company_fact_ids: ID фактов компании (C:...), которые важны для этой новости;
- hypotheses: 1–3 гипотезы о механизме влияния, которые нужно проверить.
JSON: {"focus_questions":[],"research_queries":[],"news_queries":[],"company_fact_ids":[],"hypotheses":[]}`,
		article.ID, article.Title, article.Excerpt, clsText, ctx.RoleTitle, strings.Join(ctx.RoleFocus, "; "),
		orDash(ctx.WorkFocus), orDash(strings.Join(ctx.Interests, ", ")), ctx.CompanyBrief, orDash(ctx.UserProfile))
	return []llm.Message{{Role: "system", Content: system}, {Role: "user", Content: user}}
}

func ValidatePlan(p *Plan) {
	p.FocusQuestions = limitStr(p.FocusQuestions, 4, 300)
	p.ResearchQueries = limitStr(p.ResearchQueries, 3, 80)
	p.NewsQueries = limitStr(p.NewsQueries, 2, 80)
	p.CompanyFactIDs = limitStr(p.CompanyFactIDs, 12, 40)
	p.Hypotheses = limitStr(p.Hypotheses, 3, 300)
}

// ---------------------------------------------------------------------------
// Шаг 2. Персональный анализ (структурированный ответ)
// ---------------------------------------------------------------------------

type Claim struct {
	Text      string   `json:"text"`
	SourceIDs []string `json:"source_ids"`
}

type RiskOpp struct {
	Title       string   `json:"title"`
	Description string   `json:"description"`
	Likelihood  string   `json:"likelihood"` // low | medium | high
	Horizon     string   `json:"horizon"`    // short | medium | long
	Basis       string   `json:"basis"`      // fact | bank_view | model_inference
	SourceIDs   []string `json:"source_ids"`
}

type Action struct {
	Action    string   `json:"action"`
	Rationale string   `json:"rationale"`
	Owner     string   `json:"owner"`
	SourceIDs []string `json:"source_ids"`
}

type WhatHappened struct {
	Summary string  `json:"summary"`
	Facts   []Claim `json:"facts"`
}

// Insight — ответ модели. Поля relevance_score … uncertainty_notes — требуемая
// структура; evidence_sources заполняет backend по реестру.
type Insight struct {
	RelevanceScore     int          `json:"relevance_score"`
	ImportanceLevel    string       `json:"importance_level"`
	ImpactType         string       `json:"impact_type"`
	MatchedTopics      []string     `json:"matched_topics"`
	WhatHappened       WhatHappened `json:"what_happened"`
	WhyItMatters       string       `json:"why_it_matters"`
	WhyItMattersIDs    []string     `json:"why_it_matters_source_ids"`
	PotentialImpact    string       `json:"potential_impact"`
	Risks              []RiskOpp    `json:"risks"`
	Opportunities      []RiskOpp    `json:"opportunities"`
	RecommendedActions []Action     `json:"recommended_actions"`
	BankView           []Claim      `json:"bank_view"`
	Assumptions        []string     `json:"assumptions"`
	UncertaintyNotes   []string     `json:"uncertainty_notes"`
	EvidenceSources    []Evidence   `json:"evidence_sources"`
	Validation         []string     `json:"validation_notes,omitempty"`
}

const insightSchema = `{
 "relevance_score": 0-100,
 "importance_level": "low|medium|high|critical",
 "impact_type": "risk|opportunity|mixed|neutral",
 "matched_topics": ["коды тем"],
 "what_happened": {"summary": "что произошло, 2–3 предложения, только по источникам",
                   "facts": [{"text": "подтверждённый факт", "source_ids": ["A1"]}]},
 "why_it_matters": "почему это важно ИМЕННО этой компании и этой роли — со ссылкой на признаки компании",
 "why_it_matters_source_ids": ["C:...", "A1"],
 "potential_impact": "механизм и масштаб возможного влияния на бизнес",
 "risks": [{"title": "", "description": "", "likelihood": "low|medium|high", "horizon": "short|medium|long",
            "basis": "fact|bank_view|model_inference", "source_ids": []}],
 "opportunities": [ ...как risks... ],
 "recommended_actions": [{"action": "конкретное действие или вопрос для проверки", "rationale": "", "owner": "кто", "source_ids": []}],
 "bank_view": [{"text": "позиция/прогноз аналитиков Альфа-Банка своими словами", "source_ids": ["R..."]}],
 "assumptions": ["что предполагается, но не подтверждено данными"],
 "uncertainty_notes": ["ограничения анализа, чего не хватает"]
}`

func InsightMessages(article Evidence, ctx Context, plan *Plan, reg *Registry, revision string) []llm.Message {
	planText := ""
	if plan != nil {
		planText = "План анализа:\n- " + strings.Join(append(plan.FocusQuestions, plan.Hypotheses...), "\n- ")
	}
	detail := "подробно"
	if ctx.Detail == "brief" {
		detail = "кратко (по 1–2 пункта в списках)"
	}
	horizon := map[string]string{"short": "до 3 месяцев", "medium": "3–12 месяцев", "long": "больше года"}[ctx.Horizon]
	system := `Ты — аналитик корпоративного банка. Объясняешь, как событие может повлиять на конкретную компанию
и конкретного сотрудника. Это не пересказ новости: нужны механизм влияния, последствия, риски, возможности и действия.
Жёсткие правила:
1. Используй только источники из реестра. Ссылайся на них по ID в source_ids. Не придумывай источники, цитаты, цифры и факты.
2. what_happened.facts — только то, что прямо есть в новости [A1] или связанных новостях [N...].
3. bank_view — только позиции из материалов Альфа-Банка [R...]; если подходящих материалов нет — пустой список.
4. basis: "fact" — следует из новости; "bank_view" — из аналитики банка; "model_inference" — твой вывод.
5. Внутренних данных компании, которых нет в реестре (C:...), не существует: не выдумывай объёмы, контракты, поставщиков. Если вывод
   зависит от неизвестных данных — запиши это в assumptions/uncertainty_notes.
6. Учитывай роль: каждое последствие сформулируй через то, что важно этому сотруднику.
7. Если новость слабо связана с компанией — честно поставь низкую релевантность.
Отвечай только JSON.`
	user := fmt.Sprintf(`Сотрудник: %s.
Что для него важно: %s.
Его рабочие интересы: %s. Темы интереса: %s.
Горизонт планирования: %s. Детализация: %s.

Компания:
%s
Сведения от пользователя: %s

%s

РЕЕСТР ИСТОЧНИКОВ (ссылайся только на эти ID):
%s
%s
Верни JSON по схеме:
%s`, ctx.RoleTitle, strings.Join(ctx.RoleFocus, "; "), orDash(ctx.WorkFocus), orDash(strings.Join(ctx.Interests, ", ")),
		orDash(horizon), detail, ctx.CompanyBrief, orDash(ctx.UserProfile), planText, reg.Render(1400), revision, insightSchema)
	return []llm.Message{{Role: "system", Content: system}, {Role: "user", Content: user}}
}

var (
	importanceLevels = map[string]bool{"low": true, "medium": true, "high": true, "critical": true}
	impactTypes      = map[string]bool{"risk": true, "opportunity": true, "mixed": true, "neutral": true}
	likelihoods      = map[string]bool{"low": true, "medium": true, "high": true}
	horizons         = map[string]bool{"short": true, "medium": true, "long": true}
)

// ValidateInsight проверяет формат и связь выводов с реестром источников.
// Неизвестные ID удаляются; утверждения, потерявшие опору, понижаются до
// вывода модели или удаляются. Все исправления записываются в Validation.
// Возвращает ошибку, только если ответ непригоден в целом.
func ValidateInsight(in *Insight, reg *Registry, knownTopics map[string]bool) error {
	var notes []string
	if in.RelevanceScore < 0 || in.RelevanceScore > 100 {
		notes = append(notes, fmt.Sprintf("relevance_score=%d вне диапазона — ограничен", in.RelevanceScore))
		in.RelevanceScore = clamp(in.RelevanceScore, 0, 100)
	}
	if !importanceLevels[in.ImportanceLevel] {
		notes = append(notes, "importance_level некорректен — установлен medium")
		in.ImportanceLevel = "medium"
	}
	if !impactTypes[in.ImpactType] {
		in.ImpactType = deriveImpact(in)
	}
	var topics []string
	for _, t := range in.MatchedTopics {
		if knownTopics[t] {
			topics = append(topics, t)
		}
	}
	in.MatchedTopics = nonNil(topics)

	filter := func(ids []string, allowPrefix ...string) ([]string, int) {
		var out []string
		dropped := 0
		seen := map[string]bool{}
		for _, id := range ids {
			id = strings.Trim(strings.TrimSpace(id), "[]")
			if !reg.Has(id) {
				dropped++
				continue
			}
			if len(allowPrefix) > 0 && !hasAnyPrefix(id, allowPrefix) {
				dropped++
				continue
			}
			if !seen[id] {
				seen[id] = true
				out = append(out, id)
			}
		}
		return nonNil(out), dropped
	}
	unknown := 0

	var facts []Claim
	for _, f := range in.WhatHappened.Facts {
		ids, d := filter(f.SourceIDs, "A", "N")
		unknown += d
		if strings.TrimSpace(f.Text) == "" {
			continue
		}
		if len(ids) == 0 {
			notes = append(notes, "Факт без ссылки на новость удалён: «"+short(f.Text)+"»")
			continue
		}
		facts = append(facts, Claim{Text: f.Text, SourceIDs: ids})
	}
	in.WhatHappened.Facts = nonNil(facts)
	if strings.TrimSpace(in.WhatHappened.Summary) == "" {
		return fmt.Errorf("пустое описание события (what_happened.summary)")
	}
	if strings.TrimSpace(in.WhyItMatters) == "" {
		return fmt.Errorf("пустое поле why_it_matters")
	}
	in.WhyItMattersIDs, _ = filter(in.WhyItMattersIDs)

	var bank []Claim
	for _, b := range in.BankView {
		ids, d := filter(b.SourceIDs, "R")
		unknown += d
		if len(ids) == 0 {
			notes = append(notes, "Утверждение о позиции банка без ссылки на материал Альфа-Банка удалено")
			continue
		}
		bank = append(bank, Claim{Text: b.Text, SourceIDs: ids})
	}
	in.BankView = nonNil(bank)

	fixRO := func(items []RiskOpp, kind string) []RiskOpp {
		var out []RiskOpp
		for _, r := range items {
			if strings.TrimSpace(r.Title) == "" && strings.TrimSpace(r.Description) == "" {
				continue
			}
			ids, d := filter(r.SourceIDs)
			unknown += d
			r.SourceIDs = ids
			if !likelihoods[r.Likelihood] {
				r.Likelihood = "medium"
			}
			if !horizons[r.Horizon] {
				r.Horizon = "medium"
			}
			switch r.Basis {
			case "fact":
				if !anyPrefix(ids, "A", "N") {
					r.Basis = "model_inference"
					notes = append(notes, kind+" «"+short(r.Title)+"»: нет ссылки на новость — помечено как вывод модели")
				}
			case "bank_view":
				if !anyPrefix(ids, "R") {
					r.Basis = "model_inference"
					notes = append(notes, kind+" «"+short(r.Title)+"»: нет ссылки на аналитику банка — помечено как вывод модели")
				}
			default:
				r.Basis = "model_inference"
			}
			out = append(out, r)
		}
		if len(out) > 6 {
			out = out[:6]
		}
		return nonNil(out)
	}
	in.Risks = fixRO(in.Risks, "Риск")
	in.Opportunities = fixRO(in.Opportunities, "Возможность")

	var acts []Action
	for _, a := range in.RecommendedActions {
		if strings.TrimSpace(a.Action) == "" {
			continue
		}
		a.SourceIDs, _ = filter(a.SourceIDs)
		acts = append(acts, a)
	}
	if len(acts) > 6 {
		acts = acts[:6]
	}
	in.RecommendedActions = nonNil(acts)
	in.Assumptions = nonNil(limitStr(in.Assumptions, 8, 500))
	in.UncertaintyNotes = nonNil(limitStr(in.UncertaintyNotes, 8, 500))
	if unknown > 0 {
		notes = append(notes, fmt.Sprintf("Удалено ссылок на несуществующие или неподходящие источники: %d", unknown))
	}
	if len(in.BankView) == 0 {
		in.UncertaintyNotes = append(in.UncertaintyNotes, "В предоставленных материалах Альфа-Банка не найдено позиции, напрямую относящейся к этому событию.")
	}
	in.Validation = append(in.Validation, notes...)
	in.EvidenceSources = usedEvidence(in, reg)
	return nil
}

// usedEvidence собирает список источников, на которые реально сослались выводы.
func usedEvidence(in *Insight, reg *Registry) []Evidence {
	used := map[string]bool{"A1": true}
	mark := func(ids []string) {
		for _, id := range ids {
			used[id] = true
		}
	}
	for _, f := range in.WhatHappened.Facts {
		mark(f.SourceIDs)
	}
	mark(in.WhyItMattersIDs)
	for _, b := range in.BankView {
		mark(b.SourceIDs)
	}
	for _, r := range append(append([]RiskOpp{}, in.Risks...), in.Opportunities...) {
		mark(r.SourceIDs)
	}
	for _, a := range in.RecommendedActions {
		mark(a.SourceIDs)
	}
	var out []Evidence
	for _, e := range reg.Items {
		if used[e.ID] {
			out = append(out, e)
		}
	}
	sort.SliceStable(out, func(i, j int) bool { return order(out[i].Type) < order(out[j].Type) })
	return out
}

func order(t string) int {
	switch t {
	case "article":
		return 0
	case "related_news":
		return 1
	case "bank_research":
		return 2
	}
	return 3
}

func deriveImpact(in *Insight) string {
	switch {
	case len(in.Risks) > 0 && len(in.Opportunities) > 0:
		return "mixed"
	case len(in.Risks) > 0:
		return "risk"
	case len(in.Opportunities) > 0:
		return "opportunity"
	}
	return "neutral"
}

// ---------------------------------------------------------------------------
// Шаг 3. Самопроверка
// ---------------------------------------------------------------------------

type CheckIssue struct {
	Path     string `json:"path"`
	Problem  string `json:"problem"`
	Severity string `json:"severity"` // low | high
}

type Check struct {
	Passed            bool         `json:"passed"`
	Issues            []CheckIssue `json:"issues"`
	AdditionalQueries []string     `json:"additional_research_queries"`
	Comment           string       `json:"comment"`
}

func CheckMessages(draft *Insight, reg *Registry, ctx Context) []llm.Message {
	d := *draft
	d.EvidenceSources = nil
	b, _ := json.MarshalIndent(d, "", " ")
	system := `Ты — контролёр качества аналитики банка. Проверяешь черновик анализа по реестру источников.
Ищи: факты, которых нет в источниках; цифры, отличающиеся от источников; позицию банка, не подтверждённую материалами [R...];
противоречия между пунктами; выдуманные данные о компании; пересказ вместо объяснения влияния; несоответствие роли сотрудника.
Отвечай только JSON.`
	user := fmt.Sprintf(`Роль сотрудника: %s.
РЕЕСТР ИСТОЧНИКОВ:
%s
ЧЕРНОВИК:
%s

Верни JSON: {"passed": true|false, "issues":[{"path":"risks[0]","problem":"...","severity":"low|high"}],
"additional_research_queries":["до 2 запросов, если не хватает аналитики банка"], "comment":"кратко"}
passed=false только при проблемах severity=high.`, ctx.RoleTitle, reg.Render(900), string(b))
	return []llm.Message{{Role: "system", Content: system}, {Role: "user", Content: user}}
}

func ValidateCheck(c *Check) {
	var out []CheckIssue
	for _, i := range c.Issues {
		if i.Severity != "high" {
			i.Severity = "low"
		}
		if strings.TrimSpace(i.Problem) != "" {
			out = append(out, i)
		}
	}
	if len(out) > 10 {
		out = out[:10]
	}
	c.Issues = nonNil(out)
	c.AdditionalQueries = limitStr(c.AdditionalQueries, 2, 80)
	high := false
	for _, i := range c.Issues {
		if i.Severity == "high" {
			high = true
		}
	}
	c.Passed = !high
}

func RevisionNote(c *Check) string {
	var b strings.Builder
	b.WriteString("\nКОНТРОЛЁР НАШЁЛ ПРОБЛЕМЫ В ПРЕДЫДУЩЕЙ ВЕРСИИ — исправь их:\n")
	for _, i := range c.Issues {
		fmt.Fprintf(&b, "- [%s] %s: %s\n", i.Severity, i.Path, i.Problem)
	}
	return b.String()
}

// ---------------------------------------------------------------------------
// Пакетная оценка релевантности (по кнопке, не массово)
// ---------------------------------------------------------------------------

type BriefScore struct {
	ArticleID       string   `json:"article_id"`
	RelevanceScore  int      `json:"relevance_score"`
	ImportanceLevel string   `json:"importance_level"`
	ImpactType      string   `json:"impact_type"`
	MatchedTopics   []string `json:"matched_topics"`
	WhyItMatters    string   `json:"why_it_matters"`
}

type BriefBatch struct {
	Items []BriefScore `json:"items"`
}

func BriefMessages(items []Evidence, ctx Context) []llm.Message {
	var b strings.Builder
	for _, e := range items {
		fmt.Fprintf(&b, "[%s] %s (%s)\n%s\n\n", e.ID, e.Title, e.Date, e.Excerpt)
	}
	system := `Ты оцениваешь, насколько новости важны конкретному сотруднику конкретной компании.
Оценивай по связи с бизнесом компании и задачами роли, а не по громкости новости. Не выдумывай данные о компании.
Отвечай только JSON.`
	user := fmt.Sprintf(`Сотрудник: %s. Важно: %s. Интересы: %s.
Компания:
%s

Новости:
%s
Для каждой новости верни оценку. why_it_matters — 1 предложение про эту компанию и роль (или «слабо связана с бизнесом компании»).
JSON: {"items":[{"article_id":"ID из квадратных скобок","relevance_score":0-100,"importance_level":"low|medium|high|critical",
"impact_type":"risk|opportunity|mixed|neutral","matched_topics":["коды тем"],"why_it_matters":"..."}]}`,
		ctx.RoleTitle, strings.Join(ctx.RoleFocus, "; "), orDash(strings.Join(ctx.Interests, ", ")), ctx.CompanyBrief, b.String())
	return []llm.Message{{Role: "system", Content: system}, {Role: "user", Content: user}}
}

func ValidateBrief(bs *BriefScore, knownTopics map[string]bool) bool {
	if strings.TrimSpace(bs.ArticleID) == "" || strings.TrimSpace(bs.WhyItMatters) == "" {
		return false
	}
	bs.RelevanceScore = clamp(bs.RelevanceScore, 0, 100)
	if !importanceLevels[bs.ImportanceLevel] {
		bs.ImportanceLevel = "medium"
	}
	if !impactTypes[bs.ImpactType] {
		bs.ImpactType = "neutral"
	}
	var t []string
	for _, x := range bs.MatchedTopics {
		if knownTopics[x] {
			t = append(t, x)
		}
	}
	bs.MatchedTopics = nonNil(t)
	bs.WhyItMatters = short2(bs.WhyItMatters, 400)
	return true
}

// ---------------------------------------------------------------------------

func orDash(s string) string {
	if strings.TrimSpace(s) == "" {
		return "не указано"
	}
	return s
}

func limitStr(in []string, n, maxLen int) []string {
	var out []string
	for _, s := range in {
		s = strings.TrimSpace(s)
		if s == "" {
			continue
		}
		out = append(out, short2(s, maxLen))
		if len(out) == n {
			break
		}
	}
	return nonNil(out)
}

func short(s string) string { return short2(s, 60) }

func short2(s string, n int) string {
	r := []rune(strings.TrimSpace(s))
	if len(r) <= n {
		return string(r)
	}
	return string(r[:n]) + "…"
}

func clamp(v, lo, hi int) int {
	if v < lo {
		return lo
	}
	if v > hi {
		return hi
	}
	return v
}

func hasAnyPrefix(id string, prefixes []string) bool {
	for _, p := range prefixes {
		if strings.HasPrefix(id, p) {
			return true
		}
	}
	return false
}

func anyPrefix(ids []string, prefixes ...string) bool {
	for _, id := range ids {
		if hasAnyPrefix(id, prefixes) {
			return true
		}
	}
	return false
}

func nonNil[T any](s []T) []T {
	if s == nil {
		return []T{}
	}
	return s
}
