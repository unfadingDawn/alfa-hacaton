// Package agent — агентный конвейер персонального анализа:
// классификация → план → инструменты (профиль, аналитика банка, связанные
// новости) → черновик → самопроверка → (при необходимости) одна доработка.
// Все шаги записываются в analysis_runs.steps для трассировки.
//
// Модель вызывается только по явному действию пользователя. Фоновые задачи
// агента (разметка тем, TF-IDF + LSA, предварительная оценка) работают без LLM.
package agent

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"math"
	"regexp"
	"sort"
	"strings"
	"sync"
	"sync/atomic"
	"time"
	"unicode"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	"github.com/alfainsight/backend/internal/analysis"
	"github.com/alfainsight/backend/internal/companies"
	"github.com/alfainsight/backend/internal/db/dbq"
	"github.com/alfainsight/backend/internal/llm"
	"github.com/alfainsight/backend/internal/roles"
	"github.com/alfainsight/backend/internal/semantic"
	"github.com/alfainsight/backend/internal/users"
)

type Models struct {
	Fast      string
	Reasoning string
	Writer    string
}

type Agent struct {
	Q         *dbq.Queries
	LLM       *llm.Client
	Models    Models
	Companies *companies.Service
	Log       *slog.Logger
	// Ограничение полных анализов на пользователя в час.
	PerUserHourly int
	// Лимиты уточняющих вопросов к анализу (обращения к модели).
	QuestionsPerAnalysis  int
	QuestionsPerUserDaily int
	SemanticEnabled       bool
	SemanticConfig        semantic.Config
	SemanticRefresh       time.Duration

	mu       sync.Mutex
	inflight map[string]chan struct{}
	userHits map[uuid.UUID][]time.Time
	topics   []dbq.Topic

	semanticModel atomic.Pointer[semantic.Model]
	semanticMu    sync.Mutex
	semanticLast  time.Time
}

var (
	ErrNoCompany      = errors.New("сначала выберите компанию в профиле")
	ErrUserRateLimit  = errors.New("слишком много запусков анализа за час")
	ErrArticleMissing = errors.New("новость не найдена")
)

// ---------------------------------------------------------------------------
// Контекст пользователя
// ---------------------------------------------------------------------------

type UserContext struct {
	User      dbq.User
	Role      roles.Role
	CompanyID string
	Profile   companies.Profile
	Interests map[string]int
	Pers      users.Personalization
	AC        analysis.Context
	RC        analysis.RuleContext
	Updated   time.Time
}

func (a *Agent) topicList(ctx context.Context) ([]dbq.Topic, error) {
	a.mu.Lock()
	defer a.mu.Unlock()
	if a.topics != nil {
		return a.topics, nil
	}
	t, err := a.Q.ListTopics(ctx)
	if err != nil {
		return nil, err
	}
	a.topics = t
	return t, nil
}

func (a *Agent) knownTopics(ctx context.Context) (map[string]bool, map[string]string, []analysis.Topic) {
	ts, _ := a.topicList(ctx)
	known, names := map[string]bool{}, map[string]string{}
	var list []analysis.Topic
	for _, t := range ts {
		known[t.Code], names[t.Code] = true, t.Name
		list = append(list, analysis.Topic{Code: t.Code, Name: t.Name})
	}
	return known, names, list
}

func (a *Agent) LoadUserContext(ctx context.Context, userID uuid.UUID) (*UserContext, error) {
	u, err := a.Q.GetUserByID(ctx, userID)
	if err != nil {
		return nil, err
	}
	cid, err := a.Q.GetPrimaryCompanyID(ctx, userID)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, ErrNoCompany
	}
	if err != nil {
		return nil, err
	}
	prof, err := a.Companies.Load(ctx, cid, true)
	if err != nil {
		return nil, err
	}
	company, err := a.Q.GetCompany(ctx, cid)
	if err != nil {
		return nil, err
	}
	ints, err := a.Q.ListUserInterests(ctx, userID)
	if err != nil {
		return nil, err
	}
	_, names, _ := a.knownTopics(ctx)
	uc := &UserContext{User: u, Role: roles.Resolve(u.RoleCode), CompanyID: cid, Profile: prof,
		Interests: map[string]int{}, Pers: users.ParsePersonalization(u.Personalization), Updated: company.UpdatedAt}
	var interestNames []string
	for _, i := range ints {
		uc.Interests[i.TopicCode] = int(i.Weight)
		interestNames = append(interestNames, i.Name)
	}
	uc.AC = analysis.Context{
		RoleTitle: uc.Role.Title, RoleFocus: uc.Role.Focus, WorkFocus: u.WorkFocus, Interests: interestNames,
		CompanyBrief: CompanyBrief(prof), UserProfile: userProfileText(prof.User),
		Horizon: uc.Pers.Horizon, Detail: uc.Pers.Detail, TopicNames: names,
	}
	if u.PositionTitle != "" && u.PositionTitle != uc.Role.Title {
		uc.AC.RoleTitle = uc.Role.Title + " (роль, указанная пользователем: " + u.PositionTitle + ")"
	}
	uc.RC = analysis.RuleContext{CompanyTopics: prof.AllTopics(), RoleTopics: uc.Role.Topics, Interests: uc.Interests,
		OkvedDivision: companies.Division(prof.OkvedCode), Region: prof.Region, Countries: prof.Countries,
		ResearchBoost: uc.Pers.IncludeResearch}
	return uc, nil
}

// CompanyBrief — факты компании с ID для промпта.
func CompanyBrief(p companies.Profile) string {
	var b strings.Builder
	name := p.DisplayName
	if name == "" {
		name = "обезличенная компания " + p.ID
	}
	fmt.Fprintf(&b, "%s; %s.\n", name, p.SizeSegment)
	for _, f := range p.Facts {
		if f.Value == "нет" || f.Value == "нет данных" {
			continue
		}
		fmt.Fprintf(&b, "[%s] %s: %s\n", f.ID, f.Label, f.Value)
	}
	for _, e := range p.Exposures {
		fmt.Fprintf(&b, "Чувствительность «%s» (%s): %s\n", e.Title, strings.Join(e.FactIDs, ", "), e.Explanation)
	}
	if len(p.DataGaps) > 0 {
		fmt.Fprintf(&b, "Нет данных: %s.\n", strings.Join(p.DataGaps, "; "))
	}
	return b.String()
}

func userProfileText(u companies.UserProfile) string {
	var parts []string
	add := func(label, v string) {
		if strings.TrimSpace(v) != "" {
			parts = append(parts, label+": "+strings.TrimSpace(v))
		}
	}
	add("описание", u.Description)
	add("продукты", u.Products)
	add("рынки", u.Markets)
	add("поставщики", u.Suppliers)
	if len(u.Priorities) > 0 {
		add("приоритеты", strings.Join(u.Priorities, "; "))
	}
	add("заметки", u.Notes)
	return strings.Join(parts, ". ")
}

// ---------------------------------------------------------------------------
// Без генеративного ИИ: разметка тем и предварительная оценка
// ---------------------------------------------------------------------------

// RefreshSemanticModel перестраивает локальную TF-IDF + LSA модель по
// последним новостям. Старый snapshot остаётся доступен до успешной замены.
func (a *Agent) RefreshSemanticModel(ctx context.Context, force bool) error {
	if !a.SemanticEnabled {
		return nil
	}
	a.semanticMu.Lock()
	defer a.semanticMu.Unlock()
	if !force && !a.semanticLast.IsZero() && time.Since(a.semanticLast) < a.SemanticRefresh {
		return nil
	}
	a.semanticLast = time.Now()
	rows, err := a.Q.ListSemanticCorpus(ctx, int32(a.SemanticConfig.MaxDocuments))
	if err != nil {
		return err
	}
	docs := make([]semantic.Document, 0, len(rows))
	for _, row := range rows {
		docs = append(docs, semantic.Document{ID: row.ID.String(), Title: row.Title, Summary: row.Summary})
	}
	model, err := semantic.Fit(docs, a.SemanticConfig)
	if err != nil {
		return err
	}
	a.semanticModel.Store(model)
	a.Log.Info("semantic model ready", "version", model.Version(), "documents", model.Documents(), "dimensions", model.Dimensions())
	return nil
}

// TagKeywordTopics размечает темы статей без тем (словарь, без модели).
func (a *Agent) TagKeywordTopics(ctx context.Context) (int, error) {
	known, _, _ := a.knownTopics(ctx)
	rows, err := a.Q.ListArticlesWithoutTopics(ctx, 500)
	if err != nil {
		return 0, err
	}
	n := 0
	for _, r := range rows {
		for _, t := range analysis.KeywordTopics(r.Title, r.Summary+" "+firstRunes(r.Body, 3000)) {
			if !known[t.Code] {
				continue
			}
			if err := a.Q.AddArticleTopic(ctx, dbq.AddArticleTopicParams{ArticleID: r.ID, TopicCode: t.Code,
				Confidence: float32(t.Confidence), Method: "keywords"}); err != nil {
				return n, err
			}
		}
		n++
	}
	return n, nil
}

// ScoreByRules ставит гибридные предварительные оценки по правилам и локальной
// TF-IDF + LSA модели. Языковая модель не вызывается.
func (a *Agent) ScoreByRules(ctx context.Context, userID uuid.UUID) (int, error) {
	uc, err := a.LoadUserContext(ctx, userID)
	if err != nil {
		return 0, err
	}
	model := a.semanticModel.Load()
	if model == nil && a.SemanticEnabled {
		if err := a.RefreshSemanticModel(ctx, false); err != nil {
			a.Log.Warn("semantic model unavailable; using rules", "err", err)
		}
		model = a.semanticModel.Load()
	}
	inputHash := "preliminary:rules-v2"
	var profile semantic.Representation
	if model != nil {
		inputHash = "preliminary:" + model.Version()
		profile = model.Represent(semanticProfile(uc))
	}
	rows, err := a.Q.ListArticlesForRules(ctx, dbq.ListArticlesForRulesParams{
		Since: time.Now().AddDate(0, 0, -400), UserID: userID, InputHash: inputHash, Lim: 1000})
	if err != nil {
		return 0, err
	}
	for _, r := range rows {
		rs := analysis.ScoreByRules(r.Topics, r.Title, r.Summary, uc.RC, r.IsResearch)
		finalScore := rs.Score
		semanticScore := 0
		sim := semantic.Similarity{}
		if model != nil {
			sim = model.Similarity(profile, r.Title, r.Summary)
			semanticScore = semanticRelevance(sim.Combined)
			if semanticScore > finalScore {
				finalScore = semanticScore
				rs.Reason += " Содержание новости семантически связано с профилем пользователя (TF-IDF + LSA)."
			}
		}
		importance := importanceForScore(finalScore)
		res, _ := json.Marshal(map[string]any{"method": "hybrid", "note": "предварительная оценка без языковой модели",
			"rule_score": rs.Score, "tfidf_similarity": sim.TFIDF, "lsa_similarity": sim.LSA,
			"semantic_score": semanticScore, "final_score": finalScore, "model_version": inputHash})
		if _, err := a.Q.UpsertAnalysis(ctx, dbq.UpsertAnalysisParams{
			ArticleID: r.ID, UserID: userID, CompanyID: uc.CompanyID, RoleCode: uc.Role.Code, Depth: "brief",
			Method: "rules", InputHash: inputHash, RelevanceScore: int16(finalScore), ImportanceLevel: importance,
			ImpactType: "neutral", MatchedTopics: nonNil(rs.Matched), WhyItMatters: rs.Reason, Result: res,
		}); err != nil {
			return 0, err
		}
	}
	return len(rows), nil
}

func semanticProfile(uc *UserContext) string {
	parts := []string{uc.Role.Title, strings.Join(uc.Role.Focus, ". "), uc.User.PositionTitle, uc.User.WorkFocus,
		uc.Profile.OkvedName, uc.Profile.Section.Name, uc.Profile.Region, strings.Join(uc.Profile.Countries, " "),
		uc.Profile.User.Description, uc.Profile.User.Products, uc.Profile.User.Markets, uc.Profile.User.Suppliers,
		strings.Join(uc.Profile.User.Priorities, ". "), strings.Join(uc.AC.Interests, ". ")}
	for _, exposure := range uc.Profile.Exposures {
		parts = append(parts, exposure.Title, exposure.Explanation)
	}
	return strings.Join(parts, ". ")
}

func semanticRelevance(similarity float64) int {
	// Similarities below 0.12 are normally corpus noise. A similarity of 0.62
	// or higher reaches the maximum confidence allowed for a preliminary score.
	score := int(math.Round(85 * (similarity - 0.12) / 0.5))
	if score < 0 {
		return 0
	}
	if score > 85 {
		return 85
	}
	return score
}

func importanceForScore(score int) string {
	switch {
	case score >= 55:
		return "high"
	case score >= 25:
		return "medium"
	default:
		return "low"
	}
}

// ResetUserScores сбрасывает оценки после изменения профиля (роль, интересы,
// компания): предварительные пересчитаются, оценки модели нужно запросить заново.
func (a *Agent) ResetUserScores(ctx context.Context, userID uuid.UUID) {
	if err := a.Q.DeleteUserBriefAnalyses(ctx, userID); err != nil {
		a.Log.Warn("reset scores", "err", err)
	}
}

// ---------------------------------------------------------------------------
// С ИИ: трассировка шагов
// ---------------------------------------------------------------------------

type Step struct {
	Name       string `json:"name"`
	Title      string `json:"title"`
	Model      string `json:"model,omitempty"`
	Status     string `json:"status"`
	DurationMS int64  `json:"duration_ms"`
	Tokens     int    `json:"tokens"`
	Cached     bool   `json:"cached,omitempty"`
	Finish     string `json:"finish_reason,omitempty"`
	Summary    string `json:"summary"`
	Detail     any    `json:"detail,omitempty"`
}

type tracer struct {
	steps            []Step
	models           map[string]bool
	prompt, complete int
}

func newTracer() *tracer { return &tracer{models: map[string]bool{}} }

func (t *tracer) llmStep(name, title string, resp llm.Response, err error, start time.Time, summary string, detail any) {
	st := Step{Name: name, Title: title, Model: resp.Model, Status: "ok", DurationMS: time.Since(start).Milliseconds(),
		Tokens: resp.Usage.TotalTokens, Cached: resp.Cached, Finish: resp.Finish, Summary: summary, Detail: detail}
	if err != nil {
		st.Status, st.Summary = "error", err.Error()
	}
	if resp.Model != "" {
		t.models[resp.Model] = true
	}
	t.prompt += resp.Usage.PromptTokens
	t.complete += resp.Usage.CompletionTokens
	t.steps = append(t.steps, st)
}

func (t *tracer) toolStep(name, title string, start time.Time, summary string, detail any) {
	t.steps = append(t.steps, Step{Name: name, Title: title, Status: "ok", DurationMS: time.Since(start).Milliseconds(),
		Summary: summary, Detail: detail})
}

func (t *tracer) modelList() []string {
	var out []string
	for m := range t.models {
		out = append(out, m)
	}
	sort.Strings(out)
	return nonNil(out)
}

func (a *Agent) finishRun(ctx context.Context, runID uuid.UUID, t *tracer, runErr error) {
	steps, _ := json.Marshal(t.steps)
	status := "succeeded"
	var msg *string
	if runErr != nil {
		status = "failed"
		s := runErr.Error()
		msg = &s
	}
	if err := a.Q.FinishRun(context.WithoutCancel(ctx), dbq.FinishRunParams{ID: runID, Status: status, Models: t.modelList(),
		PromptTokens: int32(t.prompt), CompletionTokens: int32(t.complete), Steps: steps, Error: msg}); err != nil {
		a.Log.Error("finish run", "err", err)
	}
}

func (a *Agent) chatJSON(ctx context.Context, model string, msgs []llm.Message, maxTokens int, temp float64, dst any) (llm.Response, error) {
	resp, err := a.LLM.Chat(ctx, llm.Request{Model: model, Messages: msgs, Temperature: temp, MaxTokens: maxTokens, JSON: true})
	if err != nil {
		return resp, err
	}
	return resp, llm.DecodeJSON(resp.Content, dst)
}

// ---------------------------------------------------------------------------
// Классификация одной статьи (по требованию)
// ---------------------------------------------------------------------------

func (a *Agent) classify(ctx context.Context, art dbq.GetArticleRow, t *tracer) (*analysis.Classification, error) {
	if len(art.Classification) > 0 {
		var c analysis.Classification
		if json.Unmarshal(art.Classification, &c) == nil {
			t.toolStep("classify", "Классификация новости", time.Now(), "взята из кэша (выполнялась ранее)", nil)
			return &c, nil
		}
	}
	known, _, list := a.knownTopics(ctx)
	_ = known
	start := time.Now()
	var c analysis.Classification
	in := analysis.ArticleInput{ID: art.ID.String(), Title: art.Title, Summary: art.Summary, Body: firstRunes(art.Body, 6000),
		Source: art.SourceName, PublishedAt: art.PublishedAt}
	resp, err := a.chatJSON(ctx, a.Models.Fast, analysis.ClassifyMessages(in, list), 900, 0.1, &c)
	if err == nil {
		err = analysis.ValidateClassification(&c, list)
	}
	if err != nil {
		t.llmStep("classify", "Классификация новости", resp, err, start, "", nil)
		return nil, err
	}
	c.Model, c.PromptVersion = a.Models.Fast, analysis.PromptVersion
	raw, _ := json.Marshal(c)
	if err := a.Q.SetArticleClassification(ctx, dbq.SetArticleClassificationParams{ID: art.ID, Classification: raw}); err != nil {
		return nil, err
	}
	_ = a.Q.DeleteArticleTopicsByMethod(ctx, dbq.DeleteArticleTopicsByMethodParams{ArticleID: art.ID, Method: "keywords"})
	var codes []string
	for _, tp := range c.Topics {
		codes = append(codes, tp.Code)
		_ = a.Q.AddArticleTopic(ctx, dbq.AddArticleTopicParams{ArticleID: art.ID, TopicCode: tp.Code,
			Confidence: float32(tp.Confidence), Method: "llm"})
	}
	t.llmStep("classify", "Классификация новости", resp, nil, start,
		fmt.Sprintf("тип события: %s; темы: %s; фактов: %d", c.EventType, strings.Join(codes, ", "), len(c.KeyFacts)), c)
	return &c, nil
}

// ---------------------------------------------------------------------------
// Полный персональный анализ (по кнопке)
// ---------------------------------------------------------------------------

type Result struct {
	Analysis dbq.PersonalAnalysis `json:"analysis"`
	Cached   bool                 `json:"cached"`
	RunID    *uuid.UUID           `json:"run_id,omitempty"`
}

func (a *Agent) inputHash(uc *UserContext, articleID uuid.UUID, contentHash string) string {
	var ints []string
	for k, v := range uc.Interests {
		ints = append(ints, fmt.Sprintf("%s:%d", k, v))
	}
	sort.Strings(ints)
	// contentHash: если источник обновил публикацию, анализ строится заново.
	h := sha256.Sum256([]byte(strings.Join([]string{analysis.PromptVersion, articleID.String(), contentHash, uc.CompanyID, uc.Role.Code,
		uc.User.PositionTitle, uc.User.WorkFocus, strings.Join(ints, ","), uc.Updated.UTC().Format(time.RFC3339),
		uc.Pers.Horizon, uc.Pers.Detail, a.Models.Writer}, "|")))
	return hex.EncodeToString(h[:])
}

func (a *Agent) allowUser(userID uuid.UUID) bool {
	if a.PerUserHourly <= 0 {
		return true
	}
	a.mu.Lock()
	defer a.mu.Unlock()
	if a.userHits == nil {
		a.userHits = map[uuid.UUID][]time.Time{}
	}
	cut := time.Now().Add(-time.Hour)
	var keep []time.Time
	for _, t := range a.userHits[userID] {
		if t.After(cut) {
			keep = append(keep, t)
		}
	}
	if len(keep) >= a.PerUserHourly {
		a.userHits[userID] = keep
		return false
	}
	a.userHits[userID] = append(keep, time.Now())
	return true
}

// Analyze возвращает сохранённый анализ, если входные данные не менялись
// (кэш), иначе выполняет агентный конвейер. force=true — пересчитать.
func (a *Agent) Analyze(ctx context.Context, userID, articleID uuid.UUID, force bool) (Result, error) {
	uc, err := a.LoadUserContext(ctx, userID)
	if err != nil {
		return Result{}, err
	}
	art, err := a.Q.GetArticle(ctx, articleID)
	if errors.Is(err, pgx.ErrNoRows) {
		return Result{}, ErrArticleMissing
	}
	if err != nil {
		return Result{}, err
	}
	hash := a.inputHash(uc, articleID, art.ContentHash)
	if !force {
		if ex, err := a.Q.GetAnalysis(ctx, dbq.GetAnalysisParams{ArticleID: articleID, UserID: userID, Depth: "full"}); err == nil && ex.InputHash == hash {
			return Result{Analysis: ex, Cached: true, RunID: ex.RunID}, nil
		}
		// Тот же анализ для той же компании и роли у коллеги — переиспользуем.
		if ex, err := a.Q.FindCachedFullAnalysis(ctx, dbq.FindCachedFullAnalysisParams{ArticleID: articleID, InputHash: hash}); err == nil {
			saved, err := a.save(ctx, uc, articleID, hash, ex.RunID, ex.Result, int(ex.RelevanceScore), ex.ImportanceLevel,
				ex.ImpactType, ex.MatchedTopics, ex.WhyItMatters)
			return Result{Analysis: saved, Cached: true, RunID: ex.RunID}, err
		}
	}
	// Защита от двойного клика: один конвейер на пару пользователь+статья.
	key := userID.String() + articleID.String()
	a.mu.Lock()
	if a.inflight == nil {
		a.inflight = map[string]chan struct{}{}
	}
	if ch, ok := a.inflight[key]; ok {
		a.mu.Unlock()
		select {
		case <-ch:
		case <-ctx.Done():
			return Result{}, ctx.Err()
		}
		ex, err := a.Q.GetAnalysis(ctx, dbq.GetAnalysisParams{ArticleID: articleID, UserID: userID, Depth: "full"})
		return Result{Analysis: ex, Cached: true, RunID: ex.RunID}, err
	}
	ch := make(chan struct{})
	a.inflight[key] = ch
	a.mu.Unlock()
	defer func() {
		a.mu.Lock()
		delete(a.inflight, key)
		a.mu.Unlock()
		close(ch)
	}()
	if !a.LLM.Configured() {
		return Result{}, llm.ErrNotConfigured
	}
	if !a.allowUser(userID) {
		return Result{}, ErrUserRateLimit
	}
	return a.run(ctx, uc, art, hash)
}

func contentLevel(art dbq.GetArticleRow) string {
	if art.ContentLevel != nil {
		return *art.ContentLevel
	}
	return "summary"
}

func (a *Agent) run(ctx context.Context, uc *UserContext, art dbq.GetArticleRow, hash string) (Result, error) {
	articleID := art.ID
	level := contentLevel(art)
	uc.AC.ContentLevel = level
	completeness := analysis.CompletenessFor(level)
	uid := uc.User.ID
	runID, err := a.Q.CreateRun(ctx, dbq.CreateRunParams{Kind: "analyze", UserID: &uid, CompanyID: &uc.CompanyID, ArticleID: &articleID})
	if err != nil {
		return Result{}, err
	}
	t := newTracer()
	var runErr error
	defer func() { a.finishRun(ctx, runID, t, runErr) }()

	// 1. Классификация (1 запрос к быстрой модели; повторно не выполняется).
	cls, err := a.classify(ctx, art, t)
	if err != nil {
		runErr = err
		return Result{}, err
	}

	// Реестр источников: новость и факты компании.
	reg := analysis.NewRegistry()
	body := art.Summary
	if strings.TrimSpace(art.Body) != "" {
		body = art.Summary + "\n" + firstRunes(art.Body, 5000)
	}
	main := analysis.Evidence{ID: "A1", Type: "article", Title: art.Title, URL: art.Url, Date: art.PublishedAt.Format("2006-01-02"),
		Excerpt: body, Note: "источник: " + art.SourceName + "; полнота: " + strings.ToLower(completeness.Label)}
	if art.SourceKind == "user_submitted" {
		main.Note = "добавлено пользователем вручную; достоверность не проверена; полнота: " + strings.ToLower(completeness.Label)
	}
	reg.Add(main)
	start := time.Now()
	for _, f := range uc.Profile.Facts {
		reg.Add(analysis.Evidence{ID: f.ID, Type: "company_data", Title: f.Label, Excerpt: f.Value, Note: f.Source})
	}
	t.toolStep("company_profile", "Профиль компании", start,
		fmt.Sprintf("%d фактов, %d чувствительностей, пробелы: %d", len(uc.Profile.Facts), len(uc.Profile.Exposures), len(uc.Profile.DataGaps)),
		map[string]any{"company_id": uc.CompanyID, "exposures": uc.Profile.Exposures, "data_gaps": uc.Profile.DataGaps})

	// 2. План (модель рассуждений).
	start = time.Now()
	var plan analysis.Plan
	resp, err := a.chatJSON(ctx, a.Models.Reasoning, analysis.PlanMessages(main, cls, uc.AC), 900, 0.1, &plan)
	if err != nil {
		t.llmStep("plan", "Планирование анализа", resp, err, start, "", nil)
		if !errors.Is(err, llm.ErrBadResponse) {
			runErr = err
			return Result{}, err
		}
		// План не разобран — продолжаем с запросами из классификации.
		plan = analysis.Plan{}
	} else {
		analysis.ValidatePlan(&plan)
		t.llmStep("plan", "Планирование анализа", resp, nil, start,
			fmt.Sprintf("вопросов: %d, запросов к аналитике: %d, к новостям: %d", len(plan.FocusQuestions), len(plan.ResearchQueries), len(plan.NewsQueries)), plan)
	}
	if len(plan.ResearchQueries) == 0 {
		plan.ResearchQueries = fallbackQueries(cls, uc.AC.TopicNames)
	}

	// 3. Инструменты: аналитика Альфа-Банка и связанные новости (БД, без модели).
	searches := a.searchResearch(ctx, reg, plan.ResearchQueries, "plan", t)
	searches = append(searches, a.searchNews(ctx, reg, art.ID, plan.NewsQueries, t)...)

	known, _, _ := a.knownTopics(ctx)

	// 4. Черновик (модель-«писатель»).
	draft, err := a.draft(ctx, main, uc, &plan, reg, known, "", t, "draft", "Персональный анализ (черновик)")
	if err != nil {
		runErr = err
		return Result{}, err
	}

	// 5. Самопроверка; при серьёзных проблемах — дополнительный поиск и одна доработка.
	start = time.Now()
	var chk analysis.Check
	resp, err = a.chatJSON(ctx, a.Models.Reasoning, analysis.CheckMessages(draft, reg, uc.AC), 800, 0.1, &chk)
	final := draft
	revision := analysis.Revision{Issues: []analysis.CheckIssue{}, Changes: []string{}}
	if err != nil {
		revision.Note = "Самопроверка не выполнена: " + err.Error()
		t.llmStep("self_check", "Самопроверка", resp, err, start, "", nil)
		draft.UncertaintyNotes = append(draft.UncertaintyNotes, "Автоматическая самопроверка не выполнена: "+err.Error())
	} else {
		analysis.ValidateCheck(&chk)
		// Детерминированная проверка чисел и названий дополняет LLM-контролёра:
		// неподтверждённое значение — серьёзное замечание и повод для доработки.
		// То же для содержательности: механизм через признаки компании и конкретные действия.
		if issues := append(analysis.FactCheckIssues(draft), analysis.ContentIssues(draft)...); len(issues) > 0 {
			chk.Issues = append(chk.Issues, issues...)
			chk.Passed = false
		}
		t.llmStep("self_check", "Самопроверка", resp, nil, start,
			fmt.Sprintf("пройдена: %s, замечаний: %d", yesNo(chk.Passed), len(chk.Issues)), chk)
		revision.Issues = chk.Issues
		if chk.Passed {
			revision.Note = "Самопроверка пройдена, доработка не потребовалась"
		}
		if !chk.Passed {
			if len(chk.AdditionalQueries) > 0 {
				searches = append(searches, a.searchResearch(ctx, reg, chk.AdditionalQueries, "self_check", t)...)
			}
			revised, err := a.draft(ctx, main, uc, &plan, reg, known, analysis.RevisionNote(&chk), t, "revise", "Доработка по замечаниям")
			if err == nil {
				final = revised
				final.Validation = append(final.Validation, "Анализ доработан после самопроверки")
				revision.Applied = true
				revision.Changes = analysis.DiffInsights(draft, revised)
				revision.Note = "Анализ доработан по замечаниям самопроверки"
			} else {
				revision.Note = "Доработка не удалась, показан черновик"
				final.UncertaintyNotes = append(final.UncertaintyNotes, "Доработка после самопроверки не удалась; показан черновик с замечаниями контролёра.")
				for _, i := range chk.Issues {
					final.Validation = append(final.Validation, "Замечание контролёра: "+i.Problem)
				}
			}
		}
	}

	final.SourceCompleteness = &completeness
	if completeness.Note != "" {
		final.UncertaintyNotes = append([]string{completeness.Note}, final.UncertaintyNotes...)
	}
	result := map[string]any{"insight": final, "plan": plan, "classification": cls, "role": uc.Role,
		"revision": revision, "sources_report": analysis.BuildSourcesReport(reg, final, searches),
		"models": t.modelList(), "prompt_version": analysis.PromptVersion, "generated_at": time.Now().UTC(),
		"company_id": uc.CompanyID}
	raw, _ := json.Marshal(result)
	saved, err := a.save(ctx, uc, articleID, hash, &runID, raw, final.RelevanceScore, final.ImportanceLevel, final.ImpactType,
		final.MatchedTopics, final.WhyItMatters)
	if err != nil {
		runErr = err
		return Result{}, err
	}
	return Result{Analysis: saved, RunID: &runID}, nil
}

func (a *Agent) draft(ctx context.Context, main analysis.Evidence, uc *UserContext, plan *analysis.Plan, reg *analysis.Registry,
	known map[string]bool, revision string, t *tracer, name, title string) (*analysis.Insight, error) {
	start := time.Now()
	var in analysis.Insight
	resp, err := a.chatJSON(ctx, a.Models.Writer, analysis.InsightMessages(main, uc.AC, plan, reg, revision), 3500, 0.1, &in)
	if err == nil {
		err = analysis.ValidateInsight(&in, reg, known)
	}
	if err != nil {
		t.llmStep(name, title, resp, err, start, "", nil)
		return nil, err
	}
	t.llmStep(name, title, resp, nil, start, fmt.Sprintf("релевантность %d, важность %s, рисков %d, возможностей %d, действий %d, признаков компании %d, источников %d, исправлений валидатора %d",
		in.RelevanceScore, in.ImportanceLevel, len(in.Risks), len(in.Opportunities), len(in.RecommendedActions), len(in.UsedCompanyFactors),
		len(in.EvidenceSources), len(in.Validation)),
		map[string]any{"validation_notes": in.Validation})
	if in.FactCheck != nil {
		t.toolStep("fact_check_"+name, "Проверка чисел, дат и названий", time.Now(),
			fmt.Sprintf("проверено значений: %d, не найдено в источниках: %d; покрытие источниками %d из %d утверждений",
				in.FactCheck.Checked, len(in.FactCheck.Unverified), in.SourceCoverage.Confirmed, in.SourceCoverage.Total),
			map[string]any{"fact_check": in.FactCheck, "source_coverage": in.SourceCoverage})
	}
	return &in, nil
}

func (a *Agent) save(ctx context.Context, uc *UserContext, articleID uuid.UUID, hash string, runID *uuid.UUID, raw []byte,
	score int, importance, impact string, topics []string, why string) (dbq.PersonalAnalysis, error) {
	p := dbq.UpsertAnalysisParams{ArticleID: articleID, UserID: uc.User.ID, CompanyID: uc.CompanyID, RoleCode: uc.Role.Code,
		Depth: "full", Method: "llm", InputHash: hash, RelevanceScore: int16(score), ImportanceLevel: importance,
		ImpactType: impact, MatchedTopics: nonNil(topics), WhyItMatters: why, Result: raw, RunID: runID}
	saved, err := a.Q.UpsertAnalysis(ctx, p)
	if err != nil {
		return saved, err
	}
	// Краткая оценка для ленты берётся из полного анализа.
	p.Depth = "brief"
	p.Result, _ = json.Marshal(map[string]any{"method": "llm", "from": "full_analysis"})
	_, err = a.Q.UpsertAnalysis(ctx, p)
	return saved, err
}

// ---------------------------------------------------------------------------
// Инструменты агента (чтение БД, только в пределах данных пользователя)
// ---------------------------------------------------------------------------

var wordRe = regexp.MustCompile(`[\p{L}\p{N}]+`)

// TSQuery строит безопасный запрос "слово:* | слово:*" из свободного текста.
func TSQuery(q string) string {
	var parts []string
	seen := map[string]bool{}
	for _, w := range wordRe.FindAllString(strings.ToLower(q), -1) {
		if len([]rune(w)) < 3 || stopWords[w] || seen[w] {
			continue
		}
		seen[w] = true
		parts = append(parts, w+":*")
		if len(parts) == 6 {
			break
		}
	}
	return strings.Join(parts, " | ")
}

var stopWords = map[string]bool{"для": true, "это": true, "как": true, "что": true, "при": true, "или": true, "над": true,
	"под": true, "его": true, "она": true, "они": true, "the": true, "and": true, "влияние": true, "компании": true}

// foundRef — найденный источник в трассе шага.
type foundRef struct {
	ID    string `json:"id"`
	Title string `json:"title"`
}

type searchTrace struct {
	Query string     `json:"query"`
	Stage string     `json:"stage"`
	Found []foundRef `json:"found"`
}

func (a *Agent) searchResearch(ctx context.Context, reg *analysis.Registry, queries []string, stage string, t *tracer) []analysis.SearchQuery {
	start := time.Now()
	var hits []searchTrace
	var out []analysis.SearchQuery
	added := 0
	for _, q := range queries {
		tsq := TSQuery(q)
		if tsq == "" {
			continue
		}
		rows, err := a.Q.SearchResearchChunks(ctx, dbq.SearchResearchChunksParams{Tsquery: tsq, Lim: 3})
		if err != nil {
			a.Log.Warn("research search", "q", q, "err", err)
			continue
		}
		h := searchTrace{Query: q, Stage: stage, Found: []foundRef{}}
		sq := analysis.SearchQuery{Tool: "search_research", Query: q, Stage: stage, FoundIDs: []string{}}
		for _, r := range rows {
			id := fmt.Sprintf("R%d", r.ID)
			if reg.Has(id) {
				// Уже найден другим запросом — тоже подтверждает этот запрос.
				sq.FoundIDs = append(sq.FoundIDs, id)
				h.Found = append(h.Found, foundRef{ID: id, Title: r.Title})
				continue
			}
			if added >= 5 {
				break
			}
			note := fmt.Sprintf("стр. %d", r.Page)
			if r.IsOutdated {
				note += "; материал 2021 года — может быть неактуален"
			}
			date := ""
			if r.PublishedOn != nil {
				date = r.PublishedOn.Format("2006-01-02")
			}
			reg.Add(analysis.Evidence{ID: id, Type: "bank_research", Title: r.Title, URL: fmt.Sprintf("/research/%d", r.DocumentID),
				Date: date, Excerpt: r.Content, Page: int(r.Page), Note: note})
			sq.FoundIDs = append(sq.FoundIDs, id)
			h.Found = append(h.Found, foundRef{ID: id, Title: r.Title})
			added++
		}
		hits = append(hits, h)
		out = append(out, sq)
	}
	t.toolStep("search_research", "Поиск в аналитике Альфа-Банка", start,
		fmt.Sprintf("запросов: %d, новых фрагментов: %d", len(hits), added), hits)
	return out
}

func (a *Agent) searchNews(ctx context.Context, reg *analysis.Registry, exclude uuid.UUID, queries []string, t *tracer) []analysis.SearchQuery {
	if len(queries) == 0 {
		return nil
	}
	start := time.Now()
	n := 0
	var hits []searchTrace
	var out []analysis.SearchQuery
	for _, q := range queries {
		tsq := TSQuery(q)
		if tsq == "" {
			continue
		}
		rows, err := a.Q.SearchRelatedArticles(ctx, dbq.SearchRelatedArticlesParams{Tsquery: tsq, ExcludeID: exclude, Lim: 2})
		if err != nil {
			continue
		}
		h := searchTrace{Query: q, Stage: "plan", Found: []foundRef{}}
		sq := analysis.SearchQuery{Tool: "search_news", Query: q, Stage: "plan", FoundIDs: []string{}}
		for _, r := range rows {
			if n >= 3 {
				break
			}
			n++
			id := fmt.Sprintf("N%d", n)
			reg.Add(analysis.Evidence{ID: id, Type: "related_news", Title: r.Title, URL: r.Url,
				Date: r.PublishedAt.Format("2006-01-02"), Excerpt: r.Summary, Note: "источник: " + r.SourceName})
			sq.FoundIDs = append(sq.FoundIDs, id)
			h.Found = append(h.Found, foundRef{ID: id, Title: r.Title})
		}
		hits = append(hits, h)
		out = append(out, sq)
	}
	t.toolStep("search_news", "Поиск связанных новостей", start, fmt.Sprintf("запросов: %d, найдено: %d", len(hits), n), hits)
	return out
}

func fallbackQueries(cls *analysis.Classification, names map[string]string) []string {
	var out []string
	if cls == nil {
		return out
	}
	for _, tp := range cls.Topics {
		if n, ok := names[tp.Code]; ok {
			out = append(out, n)
		}
		if len(out) == 2 {
			break
		}
	}
	return out
}

// ---------------------------------------------------------------------------
// Оценка ленты моделью (по кнопке, один запрос на N новостей)
// ---------------------------------------------------------------------------

type RankResult struct {
	Scored  int        `json:"scored"`
	Skipped int        `json:"skipped"`
	RunID   *uuid.UUID `json:"run_id,omitempty"`
}

// RankTop оценивает моделью до n новостей с лучшей предварительной оценкой,
// у которых ещё нет оценки модели. Ровно один запрос к модели.
func (a *Agent) RankTop(ctx context.Context, userID uuid.UUID, n int) (RankResult, error) {
	if !a.LLM.Configured() {
		return RankResult{}, llm.ErrNotConfigured
	}
	if !a.allowUser(userID) {
		return RankResult{}, ErrUserRateLimit
	}
	uc, err := a.LoadUserContext(ctx, userID)
	if err != nil {
		return RankResult{}, err
	}
	if _, err := a.ScoreByRules(ctx, userID); err != nil {
		return RankResult{}, err
	}
	rows, err := a.Q.ListFeed(ctx, dbq.ListFeedParams{UserID: userID, Sort: "relevance", Lim: 60, Since: ptrTime(time.Now().AddDate(0, 0, -14))})
	if err != nil {
		return RankResult{}, err
	}
	var batch []analysis.Evidence
	ids := map[string]uuid.UUID{}
	for _, r := range rows {
		if r.Method == "llm" {
			continue
		}
		key := fmt.Sprintf("a%d", len(batch)+1)
		ids[key] = r.ID
		batch = append(batch, analysis.Evidence{ID: key, Title: r.Title, Date: r.PublishedAt.Format("2006-01-02"),
			Excerpt: firstRunes(r.Summary, 500)})
		if len(batch) == n {
			break
		}
	}
	if len(batch) == 0 {
		return RankResult{}, nil
	}
	uid := uc.User.ID
	runID, err := a.Q.CreateRun(ctx, dbq.CreateRunParams{Kind: "rank", UserID: &uid, CompanyID: &uc.CompanyID})
	if err != nil {
		return RankResult{}, err
	}
	t := newTracer()
	start := time.Now()
	var out analysis.BriefBatch
	resp, err := a.chatJSON(ctx, a.Models.Fast, analysis.BriefMessages(batch, uc.AC), 300*len(batch)+200, 0.2, &out)
	t.llmStep("rank", "Оценка релевантности ленты", resp, err, start, fmt.Sprintf("новостей в запросе: %d", len(batch)), nil)
	if err != nil {
		a.finishRun(ctx, runID, t, err)
		return RankResult{}, err
	}
	known, _, _ := a.knownTopics(ctx)
	res := RankResult{RunID: &runID}
	for _, bs := range out.Items {
		aid, ok := ids[strings.Trim(bs.ArticleID, "[] ")]
		if !ok || !analysis.ValidateBrief(&bs, known) {
			res.Skipped++
			continue
		}
		raw, _ := json.Marshal(map[string]any{"method": "llm", "from": "feed_ranking", "model": a.Models.Fast})
		if _, err := a.Q.UpsertAnalysis(ctx, dbq.UpsertAnalysisParams{ArticleID: aid, UserID: userID, CompanyID: uc.CompanyID,
			RoleCode: uc.Role.Code, Depth: "brief", Method: "llm", InputHash: "rank:" + a.Models.Fast,
			RelevanceScore: int16(bs.RelevanceScore), ImportanceLevel: bs.ImportanceLevel, ImpactType: bs.ImpactType,
			MatchedTopics: bs.MatchedTopics, WhyItMatters: bs.WhyItMatters, Result: raw, RunID: &runID}); err != nil {
			a.finishRun(ctx, runID, t, err)
			return res, err
		}
		res.Scored++
	}
	a.finishRun(ctx, runID, t, nil)
	return res, nil
}

// ---------------------------------------------------------------------------

func yesNo(v bool) string {
	if v {
		return "да"
	}
	return "нет"
}

func firstRunes(s string, n int) string {
	r := []rune(s)
	if len(r) <= n {
		return s
	}
	cut := string(r[:n])
	if i := strings.LastIndexFunc(cut, unicode.IsSpace); i > n/2 {
		cut = cut[:i]
	}
	return cut + "…"
}

func nonNil(s []string) []string {
	if s == nil {
		return []string{}
	}
	return s
}

func ptrTime(t time.Time) *time.Time { return &t }
