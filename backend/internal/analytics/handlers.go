// Package analytics — персональная главная, сводные показатели и статус системы.
// Все показатели считаются по реальным данным БД; если данных нет — это
// явно сообщается в ответе (поля *_note / sufficient=false).
package analytics

import (
	"fmt"
	"log/slog"
	"net/http"
	"net/url"
	"time"

	"github.com/go-chi/chi/v5"

	"github.com/alfainsight/backend/internal/agent"
	"github.com/alfainsight/backend/internal/auth"
	"github.com/alfainsight/backend/internal/db/dbq"
	"github.com/alfainsight/backend/internal/insights"
	"github.com/alfainsight/backend/internal/llm"
	"github.com/alfainsight/backend/internal/platform/config"
	"github.com/alfainsight/backend/internal/platform/httpx"
)

type Handler struct {
	Q     *dbq.Queries
	Agent *agent.Agent
	LLM   *llm.Client
	Cfg   config.Config
	Log   *slog.Logger
}

func (h *Handler) Routes(r chi.Router) {
	r.Get("/dashboard", h.dashboard)
	r.Get("/analytics", h.analytics)
	r.Get("/analytics/quality", h.quality)
	r.Get("/system/status", h.status)
}

func (h *Handler) dashboard(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	uid := auth.UserID(ctx)
	if _, err := h.Agent.ScoreByRules(ctx, uid); err != nil {
		insights.WriteAgentError(w, h.Log, err)
		return
	}
	week := time.Now().AddDate(0, 0, -7)
	top, err := h.Q.ListFeed(ctx, dbq.ListFeedParams{UserID: uid, Sort: "relevance", Since: &week, Lim: 6})
	if err != nil {
		httpx.Internal(w, h.Log, err, "dashboard top")
		return
	}
	topItems, err := insights.BuildFeedItems(ctx, h.Q, top)
	if err != nil {
		httpx.Internal(w, h.Log, err, "dashboard items")
		return
	}
	// Связанные с компанией: совпадение тем с профилем компании/роли.
	related := []insights.FeedItem{}
	seen := map[string]bool{}
	for _, it := range topItems {
		seen[it.ID.String()] = true
	}
	more, err := h.Q.ListFeed(ctx, dbq.ListFeedParams{UserID: uid, Sort: "relevance", Since: ptr(time.Now().AddDate(0, 0, -30)), Lim: 30})
	if err == nil {
		items, _ := insights.BuildFeedItems(ctx, h.Q, more)
		for _, it := range items {
			if !seen[it.ID.String()] && it.Analysis != nil && len(it.Analysis.MatchedTopics) > 0 && len(related) < 5 {
				related = append(related, it)
			}
		}
	}
	// Рыночная ситуация: официальные сообщения Банка России и аналитика Альфа-Банка.
	research := "alfa_research"
	bank, _ := h.Q.ListFeed(ctx, dbq.ListFeedParams{UserID: uid, Sort: "date", SourceKind: &research, Lim: 3})
	bankItems, _ := insights.BuildFeedItems(ctx, h.Q, bank)
	cbr, _ := h.Q.ListMarketNews(ctx, dbq.ListMarketNewsParams{UserID: uid, Lim: 4})
	cbrItems, _ := insights.BuildFeedItems(ctx, h.Q, marketRows(cbr))

	ro, err := h.Q.UserTopRisksAndOpportunities(ctx, dbq.UserTopRisksAndOpportunitiesParams{UserID: uid, Since: time.Now().AddDate(0, 0, -30), Lim: 12})
	if err != nil {
		httpx.Internal(w, h.Log, err, "risks")
		return
	}
	risks, opps := []map[string]any{}, []map[string]any{}
	for _, x := range ro {
		item := map[string]any{"article_id": x.ArticleID, "article_title": x.Title, "title": x.ItemTitle,
			"description": x.ItemDescription, "importance_level": x.ImportanceLevel}
		if x.Kind == "risk" && len(risks) < 4 {
			risks = append(risks, item)
		} else if x.Kind == "opportunity" && len(opps) < 4 {
			opps = append(opps, item)
		}
	}
	day := startOfDay(time.Now(), h.Cfg.Timezone)
	daySum, err := h.Q.DaySummary(ctx, dbq.DaySummaryParams{UserID: uid, Since: day})
	if err != nil {
		httpx.Internal(w, h.Log, err, "day summary")
		return
	}
	topicsToday, _ := h.Q.TopicDistribution(ctx, day)
	summary := fmt.Sprintf("С начала дня получено новостей: %d. Высокая важность для вас: %d (из них оценено моделью: %d). Полных ИИ-анализов: %d.",
		daySum.NewArticles, daySum.HighImportance, daySum.HighImportanceLlm, daySum.FullAnalyses)
	if daySum.NewArticles == 0 {
		summary = "Сегодня новых новостей из подключённых источников пока нет."
	}
	httpx.JSON(w, http.StatusOK, map[string]any{
		"top_important":   topItems,
		"company_related": related,
		"market":          map[string]any{"bank_research": bankItems, "regulator": cbrItems},
		"risks":           risks,
		"opportunities":   opps,
		"risks_note":      noteIf(len(ro) == 0, "Риски и возможности появляются после полного ИИ-анализа новостей."),
		"day_summary":     map[string]any{"text": summary, "stats": daySum, "topics": topicsToday},
		"llm":             h.llmBrief(),
	})
}

func marketRows(in []dbq.ListMarketNewsRow) []dbq.ListFeedRow {
	out := make([]dbq.ListFeedRow, 0, len(in))
	for _, r := range in {
		out = append(out, dbq.ListFeedRow(r))
	}
	return out
}

func (h *Handler) analytics(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	uid := auth.UserID(ctx)
	days := map[string]int{"7d": 7, "30d": 30, "90d": 90, "180d": 180, "365d": 365}[r.URL.Query().Get("period")]
	if days == 0 {
		days = 365
	}
	since := time.Now().AddDate(0, 0, -days)
	if _, err := h.Agent.ScoreByRules(ctx, uid); err != nil {
		insights.WriteAgentError(w, h.Log, err)
		return
	}
	dist, err := h.Q.TopicDistributionByMethod(ctx, since)
	if err != nil {
		httpx.Internal(w, h.Log, err, "distribution")
		return
	}
	daily, err := h.Q.DailyArticleCounts(ctx, dbq.DailyArticleCountsParams{UserID: uid, Since: since})
	if err != nil {
		httpx.Internal(w, h.Log, err, "daily")
		return
	}
	dailyTopics, err := h.Q.DailyTopicCounts(ctx, since)
	if err != nil {
		httpx.Internal(w, h.Log, err, "daily topics")
		return
	}
	matched, err := h.Q.UserMatchedTopics(ctx, dbq.UserMatchedTopicsParams{UserID: uid, MinScore: 40, Since: since})
	if err != nil {
		httpx.Internal(w, h.Log, err, "matched")
		return
	}
	impact, err := h.Q.UserImpactCounts(ctx, dbq.UserImpactCountsParams{UserID: uid, Since: since})
	if err != nil {
		httpx.Internal(w, h.Log, err, "impact")
		return
	}
	ro, err := h.Q.UserTopRisksAndOpportunities(ctx, dbq.UserTopRisksAndOpportunitiesParams{UserID: uid, Since: since, Lim: 20})
	if err != nil {
		httpx.Internal(w, h.Log, err, "ro")
		return
	}
	const minScore = 25 // «касается компании»: с этой оценки правила ставят среднюю важность
	monthly, err := h.Q.UserMonthlyExposure(ctx, dbq.UserMonthlyExposureParams{
		Tz: h.Cfg.Timezone.String(), MinScore: minScore, UserID: uid, Since: since})
	if err != nil {
		httpx.Internal(w, h.Log, err, "monthly")
		return
	}
	channels, err := h.Q.UserExposureChannels(ctx, dbq.UserExposureChannelsParams{UserID: uid, Since: since, MinScore: minScore})
	if err != nil {
		httpx.Internal(w, h.Log, err, "channels")
		return
	}
	total := 0
	for _, d := range daily {
		total += int(d.Total)
	}
	httpx.JSON(w, http.StatusOK, map[string]any{
		"period_days":         days,
		"articles_total":      total,
		"topic_distribution":  dist,
		"daily":               daily,
		"daily_topics":        dailyTopics,
		"company_topics":      matched,
		"impact":              impact,
		"risks_opportunities": ro,
		"monthly":             monthly,
		"channels":            channels,
		"kpi":                 exposureKPI(monthly, channels),
		"min_score":           minScore,
		"sufficient":          map[string]bool{"articles": total > 0, "llm_analyses": len(impact) > 0},
		"notes": []string{
			"Темы статей размечены моделью (llm), по словарю без ИИ (keywords) или по названию материала банка (metadata) — см. поле method.",
			"Тип влияния, баланс и накопленное влияние считаются только по оценкам модели; помесячная значимость и каналы влияния — по лучшей доступной оценке (модель или правила).",
		},
	})
}

func nilIfZero(v, n int32) any {
	if n == 0 {
		return nil
	}
	return v
}

func qualityPercent(a, b int32) any {
	if b == 0 {
		return nil
	}
	return int(float64(a)/float64(b)*100 + 0.5)
}

// quality возвращает мониторинг только для текущего пользователя. Endpoint
// читает уже сохранённые трассы и отзывы и никогда не вызывает LLM.
func (h *Handler) quality(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	uid := auth.UserID(ctx)
	days := map[string]int{"7d": 7, "30d": 30, "90d": 90}[r.URL.Query().Get("period")]
	if days == 0 {
		days = 30
	}
	since := time.Now().AddDate(0, 0, -days)
	runs, err := h.Q.UserQualityRunStats(ctx, dbq.UserQualityRunStatsParams{UserID: &uid, Since: since})
	if err != nil {
		httpx.Internal(w, h.Log, err, "quality runs")
		return
	}
	steps, err := h.Q.UserQualityStepStats(ctx, dbq.UserQualityStepStatsParams{UserID: &uid, Since: since})
	if err != nil {
		httpx.Internal(w, h.Log, err, "quality steps")
		return
	}
	analyses, err := h.Q.UserQualityAnalysisStats(ctx, dbq.UserQualityAnalysisStatsParams{UserID: uid, Since: since})
	if err != nil {
		httpx.Internal(w, h.Log, err, "quality analyses")
		return
	}
	feedback, err := h.Q.UserQualityFeedbackStats(ctx, dbq.UserQualityFeedbackStatsParams{UserID: uid, Since: since})
	if err != nil {
		httpx.Internal(w, h.Log, err, "quality feedback")
		return
	}
	daily, err := h.Q.UserQualityDaily(ctx, dbq.UserQualityDailyParams{UserID: &uid, Since: since})
	if err != nil {
		httpx.Internal(w, h.Log, err, "quality daily")
		return
	}
	models, err := h.Q.UserQualityModels(ctx, dbq.UserQualityModelsParams{UserID: &uid, Since: since})
	if err != nil {
		httpx.Internal(w, h.Log, err, "quality models")
		return
	}
	versions, err := h.Q.UserQualityPromptVersions(ctx, dbq.UserQualityPromptVersionsParams{UserID: uid, Since: since})
	if err != nil {
		httpx.Internal(w, h.Log, err, "quality prompt versions")
		return
	}
	questions, err := h.Q.UserQualityQuestionStats(ctx, dbq.UserQualityQuestionStatsParams{UserID: uid, Since: since})
	if err != nil {
		httpx.Internal(w, h.Log, err, "quality questions")
		return
	}
	presets, err := h.Q.UserQualityQuestionPresets(ctx, dbq.UserQualityQuestionPresetsParams{UserID: uid, Since: since})
	if err != nil {
		httpx.Internal(w, h.Log, err, "quality question presets")
		return
	}
	httpx.JSON(w, http.StatusOK, map[string]any{
		"period_days": days,
		"runs": map[string]any{
			"total": runs.Total, "succeeded": runs.Succeeded, "failed": runs.Failed,
			"success_rate": qualityPercent(runs.Succeeded, runs.Total), "json_errors": runs.JsonErrors,
			"truncated": runs.Truncated, "timeouts": runs.Timeouts, "auth_errors": runs.AuthErrors,
			"rate_limited": runs.RateLimited,
		},
		"performance": map[string]any{
			"avg_latency_ms": runs.AvgLatencyMs, "p95_latency_ms": runs.P95LatencyMs,
			"avg_tokens": runs.AvgTokens, "total_tokens": runs.TotalTokens, "avg_steps": runs.AvgSteps,
			"llm_steps": steps.LlmSteps, "cached_steps": steps.CachedSteps,
			"cache_rate": qualityPercent(steps.CachedSteps, steps.LlmSteps),
		},
		"quality": map[string]any{
			"full_analyses": analyses.Total, "with_bank_view": analyses.WithBankView,
			"bank_view_rate":         qualityPercent(analyses.WithBankView, analyses.Total),
			"with_company_facts":     analyses.WithCompanyFacts,
			"company_grounding_rate": qualityPercent(analyses.WithCompanyFacts, analyses.Total),
			"avg_sources":            analyses.AvgSources, "avg_risks": analyses.AvgRisks,
			"avg_opportunities": analyses.AvgOpportunities, "avg_actions": analyses.AvgActions,
			"avg_validation_notes": analyses.AvgValidationNotes,
			"self_checks":          steps.SelfChecks, "self_check_passed": steps.SelfCheckPassed,
			"self_check_pass_rate": qualityPercent(steps.SelfCheckPassed, steps.SelfChecks),
			"revisions":            steps.Revisions, "revised_analyses": analyses.Revised,
			"validator_corrections": steps.ValidatorCorrections,
		},
		"accuracy": map[string]any{
			"analyses_checked": analyses.WithCoverage, "avg_source_coverage": nilIfZero(analyses.AvgSourceCoverage, analyses.WithCoverage),
			"checked_values": analyses.CheckedValues, "unverified_values": analyses.UnverifiedValues,
			"unverified_value_rate": qualityPercent(analyses.UnverifiedValues, analyses.CheckedValues),
			"with_unverified":       analyses.WithUnverified,
			"content_levels": map[string]int32{"full_text": analyses.LevelFullText, "summary": analyses.LevelSummary,
				"title_only": analyses.LevelTitleOnly},
		},
		"feedback": map[string]any{
			"total": feedback.Total, "positive": feedback.Positive, "negative": feedback.Negative,
			"with_comment":     feedback.WithComment,
			"helpfulness_rate": qualityPercent(feedback.Positive, feedback.Total),
		},
		"questions": map[string]any{
			"total": questions.Total, "succeeded": questions.Succeeded, "failed": questions.Failed,
			"success_rate": qualityPercent(questions.Succeeded, questions.Total), "analyses": questions.Analyses,
			"llm_calls": questions.LlmCalls, "deterministic": questions.Deterministic, "cache_hits": questions.CacheHits,
			"quick": questions.Quick, "with_sources": questions.WithSources,
			"grounded_rate": qualityPercent(questions.WithSources, questions.Succeeded), "avg_sources": questions.AvgSources,
			"out_of_scope": questions.OutOfScope, "not_in_sources": questions.NotInSources,
			"validator_corrections": questions.ValidatorCorrections, "avg_llm_latency_ms": questions.AvgLlmLatencyMs,
			"tokens": questions.Tokens, "by_preset": presets,
		},
		"daily": daily, "models": models, "prompt_versions": versions,
		"sufficient": map[string]bool{"runs": runs.Total > 0, "feedback": feedback.Total > 0, "questions": questions.Total > 0},
		"notes": []string{
			"Метрики рассчитаны только по данным текущего пользователя и не вызывают языковую модель.",
			"Пустое процентное значение означает, что для расчёта ещё недостаточно данных.",
		},
	})
}

// exposureKPI сводит помесячный срез в показатели периода. Если для показателя
// нет данных (например, модель ещё ничего не оценивала), значение — nil.
func exposureKPI(rows []dbq.UserMonthlyExposureRow, channels []dbq.UserExposureChannelsRow) map[string]any {
	var total, relevant, acute, risks, opps, mixed, rated, net, full, bank int
	peak := map[string]any(nil)
	for _, m := range rows {
		total += int(m.Total)
		relevant += int(m.Relevant)
		acute += int(m.Critical + m.High)
		risks += int(m.Risks)
		opps += int(m.Opportunities)
		mixed += int(m.Mixed)
		rated += int(m.LlmRated)
		net += int(m.NetImpact)
		full += int(m.FullAnalyses)
		bank += int(m.WithBankView)
		if m.Relevant > 0 && (peak == nil || int(m.Relevant) > peak["relevant"].(int)) {
			peak = map[string]any{"month": m.Month, "relevant": int(m.Relevant)}
		}
	}
	pct := func(a, b int) any {
		if b == 0 {
			return nil
		}
		return int(float64(a)/float64(b)*100 + 0.5)
	}
	var balance any
	if d := risks + opps + mixed; d > 0 {
		balance = int(float64(opps-risks)/float64(d)*100 + 0.5*sign(opps-risks))
	}
	var top any
	if len(channels) > 0 {
		top = map[string]any{"code": channels[0].Code, "name": channels[0].Name, "articles": channels[0].Articles, "share": pct(int(channels[0].Articles), relevant)}
	}
	return map[string]any{
		"relevant_share": map[string]any{"value": pct(relevant, total), "relevant": relevant, "total": total},
		"acuteness":      map[string]any{"value": pct(acute, relevant), "acute": acute, "relevant": relevant},
		"balance":        map[string]any{"value": balance, "risks": risks, "opportunities": opps, "mixed": mixed, "rated": rated, "net_impact": net},
		"bank_backed":    map[string]any{"value": pct(bank, full), "with_bank_view": bank, "full": full},
		"channels":       map[string]any{"count": len(channels), "top": top},
		"peak_month":     peak,
	}
}

func sign(x int) float64 {
	switch {
	case x > 0:
		return 1
	case x < 0:
		return -1
	}
	return 0
}

func (h *Handler) llmBrief() map[string]any {
	host := ""
	if u, err := url.Parse(h.Cfg.LLM.BaseURL); err == nil {
		host = u.Host
	}
	return map[string]any{"configured": h.LLM.Configured(), "host": host, "guard": h.LLM.Guard().Status()}
}

func (h *Handler) status(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	stats, err := h.Q.DatasetStats(ctx)
	if err != nil {
		httpx.Internal(w, h.Log, err, "stats")
		return
	}
	issues, _ := h.Q.CountImportIssues(ctx)
	sources, _ := h.Q.ListSources(ctx)
	arts, _ := h.Q.ArticleStats(ctx)
	models := []map[string]any{}
	for role, m := range map[string]string{"fast": h.Cfg.LLM.ModelFast, "reasoning": h.Cfg.LLM.ModelReasoning, "writer": h.Cfg.LLM.ModelWriter} {
		a, ok := llm.MatchAllowed(m)
		models = append(models, map[string]any{"role": role, "id": m, "allowed": ok, "title": a.Title})
	}
	httpx.JSON(w, http.StatusOK, map[string]any{
		"llm":            h.llmBrief(),
		"models":         models,
		"allowed_models": llm.Allowed,
		"dataset":        stats,
		"import_issues":  issues,
		"sources":        sources,
		"articles":       arts,
		"background_llm": false,
		"notes": []string{
			"Фоновые запросы к модели отключены: анализ выполняется только по кнопке пользователя.",
			"Предварительные оценки рассчитываются локально по правилам и TF-IDF + LSA без LLM; темы по словарю помечены в интерфейсе.",
		},
	})
}

func startOfDay(t time.Time, loc *time.Location) time.Time {
	t = t.In(loc)
	return time.Date(t.Year(), t.Month(), t.Day(), 0, 0, 0, 0, loc)
}

func noteIf(cond bool, s string) string {
	if cond {
		return s
	}
	return ""
}

func ptr[T any](v T) *T { return &v }
