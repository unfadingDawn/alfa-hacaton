package analytics

import (
	"net/http"
	"strings"

	"github.com/go-chi/chi/v5"

	"github.com/alfainsight/backend/internal/db/dbq"
	"github.com/alfainsight/backend/internal/platform/httpx"
	"github.com/alfainsight/backend/internal/roles"
)

// PublicRoutes — маршруты без авторизации (лендинг).
func (h *Handler) PublicRoutes(r chi.Router) {
	r.Get("/public/showcase", h.showcase)
}

type showcaseLens struct {
	Role            string `json:"role"`
	Industry        string `json:"industry"`
	Depth           string `json:"depth"`
	RelevanceScore  int16  `json:"relevance_score"`
	ImportanceLevel string `json:"importance_level"`
	ImpactType      string `json:"impact_type"`
	WhyItMatters    string `json:"why_it_matters"`
	BankView        string `json:"bank_view,omitempty"`
	Risk            string `json:"risk,omitempty"`
	Opportunity     string `json:"opportunity,omitempty"`
}

// showcase отдаёт реальный пример работы сервиса: последний разбор новости моделью
// (по возможности — для нескольких ролей) и свежие поступления из источников.
// Ничего не генерирует и к модели не обращается. Имена пользователей и названия
// компаний, введённые пользователями, не раскрываются: компания описывается отраслью.
func (h *Handler) showcase(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	out := map[string]any{"example": nil}

	if picks, err := h.Q.ShowcaseArticles(ctx); err == nil && len(picks) > 0 {
		art, err1 := h.Q.ShowcaseArticle(ctx, picks[0].ArticleID)
		rows, err2 := h.Q.ShowcaseAnalyses(ctx, picks[0].ArticleID)
		if err1 == nil && err2 == nil && len(rows) > 0 {
			lenses := make([]showcaseLens, 0, len(rows))
			for _, x := range rows {
				scrub := func(s string) string { return scrubName(s, x.CompanyName) }
				lenses = append(lenses, showcaseLens{
					Role: roles.Resolve(x.RoleCode).Title, Industry: x.Industry, Depth: x.Depth,
					RelevanceScore: x.RelevanceScore, ImportanceLevel: x.ImportanceLevel, ImpactType: x.ImpactType,
					WhyItMatters: scrub(x.WhyItMatters), BankView: scrub(x.BankView),
					Risk: scrub(x.Risk), Opportunity: scrub(x.Opportunity),
				})
			}
			out["example"] = map[string]any{
				"article": map[string]any{"title": art.Title, "summary": art.Summary, "url": art.Url,
					"published_at": art.PublishedAt, "source": art.SourceName, "source_kind": art.SourceKind},
				"lenses": lenses,
			}
		}
	}
	news, err := h.Q.ShowcaseLatest(ctx, dbq.ShowcaseLatestParams{Kind: "rss", Lim: 4})
	if err != nil {
		httpx.Internal(w, h.Log, err, "showcase latest")
		return
	}
	research, _ := h.Q.ShowcaseLatest(ctx, dbq.ShowcaseLatestParams{Kind: "alfa_research", Lim: 1})
	out["latest_news"] = news
	out["latest_research"] = research
	w.Header().Set("Cache-Control", "public, max-age=60")
	httpx.JSON(w, http.StatusOK, out)
}

// scrubName заменяет название компании, которое ввёл пользователь, нейтральным словом.
func scrubName(s, name string) string {
	name = strings.TrimSpace(name)
	if s == "" || len([]rune(name)) < 3 {
		return s
	}
	return strings.ReplaceAll(s, name, "компания")
}
