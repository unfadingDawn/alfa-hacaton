// Package insights — API ленты, карточки новости и персонального анализа.
package insights

import (
	"context"
	"encoding/json"
	"errors"
	"log/slog"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	"github.com/alfainsight/backend/internal/agent"
	"github.com/alfainsight/backend/internal/auth"
	"github.com/alfainsight/backend/internal/db/dbq"
	"github.com/alfainsight/backend/internal/llm"
	"github.com/alfainsight/backend/internal/news"
	"github.com/alfainsight/backend/internal/platform/httpx"
)

type Handler struct {
	Q        *dbq.Queries
	Agent    *agent.Agent
	Ingestor *news.Ingestor
	Log      *slog.Logger
}

func (h *Handler) Routes(r chi.Router) {
	r.Get("/feed", h.feed)
	r.Post("/feed/rank", h.rank)
	r.Post("/articles", h.submit)
	r.Get("/articles/{id}", h.article)
	r.Post("/articles/{id}/analyze", h.analyze)
	r.Post("/articles/{id}/save", h.save)
	r.Delete("/articles/{id}/save", h.unsave)
	r.Post("/articles/{id}/feedback", h.feedback)
	r.Get("/runs/{id}", h.run)
	r.Get("/research", h.researchList)
	r.Get("/research/{id}", h.researchDoc)
	r.Get("/sources", h.sources)
}

// WriteAgentError переводит ошибки агента и модели в понятные HTTP-ответы.
func WriteAgentError(w http.ResponseWriter, log *slog.Logger, err error) {
	switch {
	case errors.Is(err, agent.ErrNoCompany):
		httpx.Error(w, http.StatusConflict, "no_company", "Сначала выберите компанию в настройке профиля")
	case errors.Is(err, agent.ErrArticleMissing):
		httpx.Error(w, http.StatusNotFound, "not_found", "Новость не найдена")
	case errors.Is(err, agent.ErrUserRateLimit):
		httpx.Error(w, http.StatusTooManyRequests, "user_rate_limited", "Лимит запусков анализа на этот час исчерпан. Ранее выполненные анализы доступны.")
	case errors.Is(err, llm.ErrNotConfigured):
		httpx.Error(w, http.StatusServiceUnavailable, "llm_not_configured", "Модель не подключена: анализ недоступен. Предварительные оценки по правилам работают.")
	case errors.Is(err, llm.ErrBudgetExceeded):
		httpx.Error(w, http.StatusTooManyRequests, "llm_budget_exceeded", "Достигнут дневной лимит запросов к модели. Анализ станет доступен после сброса лимита.")
	case errors.Is(err, llm.ErrRateLimited), errors.Is(err, llm.ErrPaused):
		httpx.Error(w, http.StatusTooManyRequests, "llm_rate_limited", "Модель временно ограничила запросы: "+err.Error())
	case errors.Is(err, llm.ErrAuth):
		httpx.Error(w, http.StatusBadGateway, "llm_auth", "Ключ API модели отклонён. Проверьте LLM_API_KEY.")
	case errors.Is(err, llm.ErrModelNotAllowed):
		httpx.Error(w, http.StatusInternalServerError, "llm_model_not_allowed", "В настройках указана модель вне разрешённого списка.")
	case errors.Is(err, llm.ErrBadResponse):
		httpx.Error(w, http.StatusBadGateway, "llm_bad_response", "Модель вернула ответ в неверном формате. Попробуйте ещё раз позже.")
	case errors.Is(err, llm.ErrUnavailable), errors.Is(err, context.DeadlineExceeded):
		httpx.Error(w, http.StatusServiceUnavailable, "llm_unavailable", "Модель временно недоступна. Попробуйте позже.")
	default:
		httpx.Internal(w, log, err, "agent")
	}
}

var importanceRank = map[string]int32{"low": 1, "medium": 2, "high": 3, "critical": 4}

func periodSince(p string) *time.Time {
	d := map[string]int{"1d": 1, "7d": 7, "30d": 30, "90d": 90}[p]
	if d == 0 {
		return nil
	}
	t := time.Now().AddDate(0, 0, -d)
	return &t
}

type FeedItem struct {
	ID          uuid.UUID    `json:"id"`
	URL         string       `json:"url"`
	Title       string       `json:"title"`
	Summary     string       `json:"summary"`
	PublishedAt time.Time    `json:"published_at"`
	Source      SourceRef    `json:"source"`
	Topics      []TopicRef   `json:"topics"`
	Analysis    *AnalysisRef `json:"analysis"`
	IsSaved     bool         `json:"is_saved"`
}

type SourceRef struct {
	Code string `json:"code"`
	Name string `json:"name"`
	Kind string `json:"kind"`
}

type TopicRef struct {
	Code   string `json:"code"`
	Name   string `json:"name"`
	Method string `json:"method"`
}

type AnalysisRef struct {
	Method          string   `json:"method"`
	Depth           string   `json:"depth"`
	RelevanceScore  int16    `json:"relevance_score"`
	ImportanceLevel string   `json:"importance_level"`
	ImpactType      string   `json:"impact_type"`
	MatchedTopics   []string `json:"matched_topics"`
	WhyItMatters    string   `json:"why_it_matters"`
}

// BuildFeedItems добавляет темы к строкам ленты.
func BuildFeedItems(ctx context.Context, q *dbq.Queries, rows []dbq.ListFeedRow) ([]FeedItem, error) {
	ids := make([]uuid.UUID, 0, len(rows))
	for _, r := range rows {
		ids = append(ids, r.ID)
	}
	topics := map[uuid.UUID][]TopicRef{}
	if len(ids) > 0 {
		trs, err := q.ListTopicsForArticles(ctx, ids)
		if err != nil {
			return nil, err
		}
		for _, t := range trs {
			topics[t.ArticleID] = append(topics[t.ArticleID], TopicRef{Code: t.TopicCode, Name: t.Name, Method: t.Method})
		}
	}
	items := make([]FeedItem, 0, len(rows))
	for _, r := range rows {
		it := FeedItem{ID: r.ID, URL: r.Url, Title: r.Title, Summary: r.Summary, PublishedAt: r.PublishedAt,
			Source: SourceRef{r.SourceCode, r.SourceName, r.SourceKind}, Topics: topics[r.ID], IsSaved: r.IsSaved}
		if it.Topics == nil {
			it.Topics = []TopicRef{}
		}
		if r.HasAnalysis {
			it.Analysis = &AnalysisRef{Method: r.Method, Depth: r.Depth, RelevanceScore: r.RelevanceScore,
				ImportanceLevel: r.ImportanceLevel, ImpactType: r.ImpactType, MatchedTopics: r.MatchedTopics, WhyItMatters: r.WhyItMatters}
		}
		items = append(items, it)
	}
	return items, nil
}

func (h *Handler) feed(w http.ResponseWriter, r *http.Request) {
	uid := auth.UserID(r.Context())
	if _, err := h.Agent.ScoreByRules(r.Context(), uid); err != nil {
		if errors.Is(err, agent.ErrNoCompany) {
			WriteAgentError(w, h.Log, err)
			return
		}
		h.Log.Warn("rules scoring", "err", err)
	}
	p := dbq.ListFeedParams{
		UserID: uid, Q: httpx.QueryStr(r, "q"), Topic: httpx.QueryStr(r, "topic"), Impact: httpx.QueryStr(r, "impact"),
		Since: periodSince(r.URL.Query().Get("period")), SourceKind: httpx.QueryStr(r, "source_kind"),
		SavedOnly: r.URL.Query().Get("saved") == "true", Sort: "date",
		Lim: int32(httpx.QueryInt(r, "limit", 20, 1, 100)), Off: int32(httpx.QueryInt(r, "offset", 0, 0, 100000)),
	}
	if s := r.URL.Query().Get("sort"); s == "relevance" || s == "importance" {
		p.Sort = s
	}
	if v, ok := importanceRank[r.URL.Query().Get("min_importance")]; ok {
		p.MinImportance = &v
	}
	if p.Impact != nil && *p.Impact != "risk" && *p.Impact != "opportunity" && *p.Impact != "mixed" && *p.Impact != "neutral" {
		httpx.ValidationError(w, map[string]string{"impact": "Допустимо: risk, opportunity, mixed, neutral"})
		return
	}
	rows, err := h.Q.ListFeed(r.Context(), p)
	if err != nil {
		httpx.Internal(w, h.Log, err, "feed")
		return
	}
	total, err := h.Q.CountFeed(r.Context(), dbq.CountFeedParams{UserID: p.UserID, Q: p.Q, Topic: p.Topic,
		MinImportance: p.MinImportance, Impact: p.Impact, Since: p.Since, SourceKind: p.SourceKind, SavedOnly: p.SavedOnly})
	if err != nil {
		httpx.Internal(w, h.Log, err, "count feed")
		return
	}
	items, err := BuildFeedItems(r.Context(), h.Q, rows)
	if err != nil {
		httpx.Internal(w, h.Log, err, "feed topics")
		return
	}
	httpx.JSON(w, http.StatusOK, map[string]any{"items": items, "total": total, "limit": p.Lim, "offset": p.Off})
}

func (h *Handler) rank(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Limit int `json:"limit"`
	}
	if r.ContentLength > 0 && !httpx.DecodeOrFail(w, r, &req) {
		return
	}
	if req.Limit <= 0 || req.Limit > 8 {
		req.Limit = 8
	}
	ctx, cancel := context.WithTimeout(r.Context(), 3*time.Minute)
	defer cancel()
	res, err := h.Agent.RankTop(ctx, auth.UserID(r.Context()), req.Limit)
	if err != nil {
		WriteAgentError(w, h.Log, err)
		return
	}
	httpx.JSON(w, http.StatusOK, res)
}

func parseID(w http.ResponseWriter, r *http.Request) (uuid.UUID, bool) {
	id, err := uuid.Parse(chi.URLParam(r, "id"))
	if err != nil {
		httpx.Error(w, http.StatusNotFound, "not_found", "Новость не найдена")
		return uuid.Nil, false
	}
	return id, true
}

func (h *Handler) article(w http.ResponseWriter, r *http.Request) {
	id, ok := parseID(w, r)
	if !ok {
		return
	}
	uid := auth.UserID(r.Context())
	a, err := h.Q.GetArticle(r.Context(), id)
	if errors.Is(err, pgx.ErrNoRows) {
		httpx.Error(w, http.StatusNotFound, "not_found", "Новость не найдена")
		return
	}
	if err != nil {
		httpx.Internal(w, h.Log, err, "article")
		return
	}
	// Новости, добавленные вручную, видит только автор.
	if a.SourceKind == "user_submitted" && (a.SubmittedBy == nil || *a.SubmittedBy != uid) {
		httpx.Error(w, http.StatusNotFound, "not_found", "Новость не найдена")
		return
	}
	trs, _ := h.Q.ListTopicsForArticles(r.Context(), []uuid.UUID{id})
	topics := []TopicRef{}
	for _, t := range trs {
		topics = append(topics, TopicRef{Code: t.TopicCode, Name: t.Name, Method: t.Method})
	}
	resp := map[string]any{
		"id": a.ID, "url": a.Url, "title": a.Title, "summary": a.Summary, "published_at": a.PublishedAt,
		"source": SourceRef{a.SourceCode, a.SourceName, a.SourceKind}, "topics": topics,
		"classification": json.RawMessage(nullJSON(a.Classification)), "duplicate_of": a.DuplicateOf,
	}
	if a.SourceKind != "rss" {
		resp["body"] = a.Body // полный текст храним только для материалов банка и добавленных вручную
	}
	if brief, err := h.Q.GetAnalysis(r.Context(), dbq.GetAnalysisParams{ArticleID: id, UserID: uid, Depth: "brief"}); err == nil {
		resp["brief"] = AnalysisRef{Method: brief.Method, Depth: brief.Depth, RelevanceScore: brief.RelevanceScore,
			ImportanceLevel: brief.ImportanceLevel, ImpactType: brief.ImpactType, MatchedTopics: brief.MatchedTopics, WhyItMatters: brief.WhyItMatters}
	}
	if full, err := h.Q.GetAnalysis(r.Context(), dbq.GetAnalysisParams{ArticleID: id, UserID: uid, Depth: "full"}); err == nil {
		resp["analysis"] = fullView(full)
		if full.RunID != nil {
			if run, err := h.Q.GetRun(r.Context(), *full.RunID); err == nil {
				resp["run"] = runView(run)
			}
		}
	}
	saved, _ := h.Q.IsArticleSaved(r.Context(), dbq.IsArticleSavedParams{UserID: uid, ArticleID: id})
	resp["is_saved"] = saved
	if fb, err := h.Q.GetFeedback(r.Context(), dbq.GetFeedbackParams{UserID: uid, ArticleID: id}); err == nil {
		resp["feedback"] = map[string]any{"rating": fb.Rating, "comment": fb.Comment}
	}
	httpx.JSON(w, http.StatusOK, resp)
}

func fullView(a dbq.PersonalAnalysis) map[string]any {
	return map[string]any{"id": a.ID, "relevance_score": a.RelevanceScore, "importance_level": a.ImportanceLevel,
		"impact_type": a.ImpactType, "matched_topics": a.MatchedTopics, "why_it_matters": a.WhyItMatters,
		"role_code": a.RoleCode, "company_id": a.CompanyID, "created_at": a.CreatedAt, "result": json.RawMessage(nullJSON(a.Result)),
		"run_id": a.RunID}
}

func runView(run dbq.AnalysisRun) map[string]any {
	return map[string]any{"id": run.ID, "kind": run.Kind, "status": run.Status, "models": run.Models,
		"prompt_tokens": run.PromptTokens, "completion_tokens": run.CompletionTokens, "steps": json.RawMessage(nullJSON(run.Steps)),
		"error": run.Error, "started_at": run.StartedAt, "finished_at": run.FinishedAt}
}

func nullJSON(b []byte) []byte {
	if len(b) == 0 {
		return []byte("null")
	}
	return b
}

func (h *Handler) analyze(w http.ResponseWriter, r *http.Request) {
	id, ok := parseID(w, r)
	if !ok {
		return
	}
	uid := auth.UserID(r.Context())
	a, err := h.Q.GetArticle(r.Context(), id)
	if err != nil || (a.SourceKind == "user_submitted" && (a.SubmittedBy == nil || *a.SubmittedBy != uid)) {
		httpx.Error(w, http.StatusNotFound, "not_found", "Новость не найдена")
		return
	}
	force := r.URL.Query().Get("force") == "true"
	ctx, cancel := context.WithTimeout(r.Context(), 5*time.Minute)
	defer cancel()
	res, err := h.Agent.Analyze(ctx, uid, id, force)
	if err != nil {
		WriteAgentError(w, h.Log, err)
		return
	}
	out := map[string]any{"cached": res.Cached, "analysis": fullView(res.Analysis)}
	if res.RunID != nil {
		if run, err := h.Q.GetRun(r.Context(), *res.RunID); err == nil {
			out["run"] = runView(run)
		}
	}
	httpx.JSON(w, http.StatusOK, out)
}

type submitReq struct {
	Title       string `json:"title"`
	Text        string `json:"text"`
	URL         string `json:"url"`
	PublishedAt string `json:"published_at"`
}

func (h *Handler) submit(w http.ResponseWriter, r *http.Request) {
	var req submitReq
	if !httpx.DecodeOrFail(w, r, &req) {
		return
	}
	errs := map[string]string{}
	req.Title, req.Text, req.URL = strings.TrimSpace(req.Title), strings.TrimSpace(req.Text), strings.TrimSpace(req.URL)
	if n := utf8.RuneCountInString(req.Title); n < 5 || n > 300 {
		errs["title"] = "Заголовок от 5 до 300 символов"
	}
	if n := utf8.RuneCountInString(req.Text); n < 50 || n > 20000 {
		errs["text"] = "Текст новости от 50 до 20 000 символов"
	}
	if req.URL != "" {
		u, err := url.Parse(req.URL)
		if err != nil || (u.Scheme != "http" && u.Scheme != "https") || u.Host == "" {
			errs["url"] = "Ссылка должна начинаться с http:// или https://"
		}
	}
	pub := time.Now()
	if req.PublishedAt != "" {
		t, err := time.Parse("2006-01-02", req.PublishedAt)
		if err != nil || t.After(time.Now().Add(24*time.Hour)) {
			errs["published_at"] = "Дата в формате ГГГГ-ММ-ДД, не в будущем"
		} else {
			pub = t.Add(9 * time.Hour)
		}
	}
	if len(errs) > 0 {
		httpx.ValidationError(w, errs)
		return
	}
	id, err := h.Ingestor.Submit(r.Context(), auth.UserID(r.Context()), req.Title, req.Text, req.URL, pub)
	if errors.Is(err, pgx.ErrNoRows) {
		httpx.ValidationError(w, map[string]string{"url": "Новость с такой ссылкой уже есть в системе"})
		return
	}
	if err != nil {
		httpx.Internal(w, h.Log, err, "submit")
		return
	}
	_, _ = h.Agent.TagKeywordTopics(r.Context())
	httpx.JSON(w, http.StatusCreated, map[string]any{"id": id})
}

func (h *Handler) save(w http.ResponseWriter, r *http.Request) {
	id, ok := parseID(w, r)
	if !ok {
		return
	}
	if _, err := h.Q.GetArticle(r.Context(), id); err != nil {
		httpx.Error(w, http.StatusNotFound, "not_found", "Новость не найдена")
		return
	}
	if err := h.Q.SaveArticle(r.Context(), dbq.SaveArticleParams{UserID: auth.UserID(r.Context()), ArticleID: id}); err != nil {
		httpx.Internal(w, h.Log, err, "save")
		return
	}
	httpx.JSON(w, http.StatusOK, map[string]any{"saved": true})
}

func (h *Handler) unsave(w http.ResponseWriter, r *http.Request) {
	id, ok := parseID(w, r)
	if !ok {
		return
	}
	if err := h.Q.UnsaveArticle(r.Context(), dbq.UnsaveArticleParams{UserID: auth.UserID(r.Context()), ArticleID: id}); err != nil {
		httpx.Internal(w, h.Log, err, "unsave")
		return
	}
	httpx.JSON(w, http.StatusOK, map[string]any{"saved": false})
}

func (h *Handler) feedback(w http.ResponseWriter, r *http.Request) {
	id, ok := parseID(w, r)
	if !ok {
		return
	}
	var req struct {
		Rating  int    `json:"rating"`
		Comment string `json:"comment"`
	}
	if !httpx.DecodeOrFail(w, r, &req) {
		return
	}
	if req.Rating != 1 && req.Rating != -1 {
		httpx.ValidationError(w, map[string]string{"rating": "Оценка: 1 или -1"})
		return
	}
	if utf8.RuneCountInString(req.Comment) > 1000 {
		httpx.ValidationError(w, map[string]string{"comment": "Не более 1000 символов"})
		return
	}
	uid := auth.UserID(r.Context())
	var aid *uuid.UUID
	if a, err := h.Q.GetAnalysis(r.Context(), dbq.GetAnalysisParams{ArticleID: id, UserID: uid, Depth: "full"}); err == nil {
		aid = &a.ID
	}
	if err := h.Q.UpsertFeedback(r.Context(), dbq.UpsertFeedbackParams{UserID: uid, ArticleID: id, AnalysisID: aid,
		Rating: int16(req.Rating), Comment: strings.TrimSpace(req.Comment)}); err != nil {
		var pgErr interface{ SQLState() string }
		if errors.As(err, &pgErr) && pgErr.SQLState() == "23503" {
			httpx.Error(w, http.StatusNotFound, "not_found", "Новость не найдена")
			return
		}
		httpx.Internal(w, h.Log, err, "feedback")
		return
	}
	httpx.JSON(w, http.StatusOK, map[string]any{"ok": true})
}

func (h *Handler) run(w http.ResponseWriter, r *http.Request) {
	id, ok := parseID(w, r)
	if !ok {
		return
	}
	run, err := h.Q.GetRun(r.Context(), id)
	if err != nil || run.UserID == nil || *run.UserID != auth.UserID(r.Context()) {
		httpx.Error(w, http.StatusNotFound, "not_found", "Запуск не найден")
		return
	}
	httpx.JSON(w, http.StatusOK, runView(run))
}

func (h *Handler) researchList(w http.ResponseWriter, r *http.Request) {
	docs, err := h.Q.ListResearchDocuments(r.Context())
	if err != nil {
		httpx.Internal(w, h.Log, err, "research")
		return
	}
	httpx.JSON(w, http.StatusOK, map[string]any{"items": docs})
}

func (h *Handler) researchDoc(w http.ResponseWriter, r *http.Request) {
	id, err := strconv.Atoi(chi.URLParam(r, "id"))
	if err != nil {
		httpx.Error(w, http.StatusNotFound, "not_found", "Материал не найден")
		return
	}
	doc, err := h.Q.GetResearchDocument(r.Context(), int32(id))
	if err != nil {
		httpx.Error(w, http.StatusNotFound, "not_found", "Материал не найден")
		return
	}
	chunks, err := h.Q.ListResearchChunksByDoc(r.Context(), int32(id))
	if err != nil {
		httpx.Internal(w, h.Log, err, "research chunks")
		return
	}
	var articleID *uuid.UUID
	if aid, err := h.Q.GetResearchArticleID(r.Context(), strings.TrimSuffix(doc.FileName, ".pdf")); err == nil {
		articleID = &aid
	}
	httpx.JSON(w, http.StatusOK, map[string]any{"document": doc, "chunks": chunks, "article_id": articleID})
}

func (h *Handler) sources(w http.ResponseWriter, r *http.Request) {
	s, err := h.Q.ListSources(r.Context())
	if err != nil {
		httpx.Internal(w, h.Log, err, "sources")
		return
	}
	httpx.JSON(w, http.StatusOK, map[string]any{"items": s})
}
