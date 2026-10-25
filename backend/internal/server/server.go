// Package server собирает зависимости и HTTP-роутер (используется в main и тестах).
package server

import (
	"context"
	"log/slog"
	"net/http"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/go-chi/chi/v5/middleware"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/alfainsight/backend/api"
	"github.com/alfainsight/backend/internal/agent"
	"github.com/alfainsight/backend/internal/analytics"
	"github.com/alfainsight/backend/internal/auth"
	"github.com/alfainsight/backend/internal/companies"
	"github.com/alfainsight/backend/internal/db/dbq"
	"github.com/alfainsight/backend/internal/insights"
	"github.com/alfainsight/backend/internal/integrations/alfabusiness"
	"github.com/alfainsight/backend/internal/llm"
	"github.com/alfainsight/backend/internal/news"
	"github.com/alfainsight/backend/internal/platform/config"
	"github.com/alfainsight/backend/internal/platform/httpx"
	"github.com/alfainsight/backend/internal/semantic"
	"github.com/alfainsight/backend/internal/users"
)

type App struct {
	Router   http.Handler
	Agent    *agent.Agent
	Ingestor *news.Ingestor
	LLM      *llm.Client
}

func New(cfg config.Config, pool *pgxpool.Pool, log *slog.Logger) *App {
	q := dbq.New(pool)
	guard := llm.NewGuard(llm.GuardConfig{
		DailyTokenBudget: cfg.LLM.DailyTokenBudget, DailyCallLimit: cfg.LLM.DailyCallLimit,
		PerMinuteLimit: cfg.LLM.PerMinuteLimit, MaxOutputTokens: cfg.LLM.MaxOutputTokens,
		MaxInputChars: cfg.LLM.MaxInputChars, PauseOnLimit: 10 * time.Minute, Location: cfg.Timezone,
	}, q)
	client := llm.NewClient(cfg.LLM.BaseURL, cfg.LLM.APIKey, cfg.LLM.Timeout, cfg.LLM.JSONMode, guard,
		cfg.LLM.MaxOutputTokens, cfg.LLM.MaxInputChars, log)
	for _, m := range []string{cfg.LLM.ModelFast, cfg.LLM.ModelReasoning, cfg.LLM.ModelWriter} {
		if _, ok := llm.MatchAllowed(m); m != "" && !ok {
			log.Error("model is not in the allowed list — LLM calls will be rejected", "model", m)
		}
	}

	compSvc := &companies.Service{Q: q}
	ag := &agent.Agent{Q: q, LLM: client, Companies: compSvc, Log: log, PerUserHourly: cfg.LLM.PerUserHourly,
		Models:          agent.Models{Fast: cfg.LLM.ModelFast, Reasoning: cfg.LLM.ModelReasoning, Writer: cfg.LLM.ModelWriter},
		SemanticEnabled: cfg.Semantic.Enabled, SemanticRefresh: cfg.Semantic.RefreshInterval,
		SemanticConfig: semantic.Config{MaxDocuments: cfg.Semantic.MaxDocuments, MaxFeatures: cfg.Semantic.MaxFeatures,
			Dimensions: cfg.Semantic.Dimensions, MinDocuments: cfg.Semantic.MinDocuments}}
	ingestor := &news.Ingestor{Q: q, Log: log, HTTP: news.NewHTTPClient(), InitialDays: cfg.NewsInitialDays}
	tokens := auth.NewTokenIssuer(cfg.JWTSecret, cfg.AccessTTL)

	authH := &auth.Handler{Q: q, Tokens: tokens, RefreshTTL: cfg.RefreshTTL, CookieSecure: cfg.CookieSecure, Log: log,
		Limiter: auth.NewRateLimiter(10, time.Minute), AlfaBusiness: alfabusiness.NotConnected{}}
	usersH := &users.Handler{Pool: pool, Q: q, Svc: compSvc, Log: log,
		OnProfileChanged: func(id uuid.UUID) { ag.ResetUserScores(context.Background(), id) }}
	compH := &companies.Handler{Q: q, Svc: compSvc, Log: log}
	insH := &insights.Handler{Q: q, Agent: ag, Ingestor: ingestor, Log: log}
	anH := &analytics.Handler{Q: q, Agent: ag, LLM: client, Cfg: cfg, Log: log}

	r := chi.NewRouter()
	r.Use(middleware.RequestID, middleware.RealIP, middleware.Recoverer, requestLogger(log))
	r.Use(auth.OriginGuard(cfg.AllowedOrigins))
	r.Route("/api", func(r chi.Router) {
		r.Get("/health", func(w http.ResponseWriter, r *http.Request) {
			if err := pool.Ping(r.Context()); err != nil {
				httpx.Error(w, http.StatusServiceUnavailable, "db_unavailable", "База данных недоступна")
				return
			}
			httpx.JSON(w, http.StatusOK, map[string]any{"status": "ok"})
		})
		r.Get("/openapi.yaml", func(w http.ResponseWriter, _ *http.Request) {
			w.Header().Set("Content-Type", "application/yaml; charset=utf-8")
			_, _ = w.Write(api.OpenAPI)
		})
		authH.Routes(r)
		anH.PublicRoutes(r)
		r.Group(func(r chi.Router) {
			r.Use(auth.RequireAuth(tokens))
			usersH.Routes(r)
			compH.Routes(r)
			insH.Routes(r)
			anH.Routes(r)
		})
	})

	return &App{Router: r, Agent: ag, Ingestor: ingestor, LLM: client}
}

func requestLogger(log *slog.Logger) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			ww := middleware.NewWrapResponseWriter(w, r.ProtoMajor)
			start := time.Now()
			next.ServeHTTP(ww, r)
			// Логируются только метод, путь и статус — без тел, cookie и query.
			log.Info("http", "method", r.Method, "path", r.URL.Path, "status", ww.Status(),
				"ms", time.Since(start).Milliseconds())
		})
	}
}
