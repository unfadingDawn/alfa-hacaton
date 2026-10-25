package evaluation

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/alfainsight/backend/internal/analysis"
	"github.com/alfainsight/backend/internal/llm"
	"github.com/alfainsight/backend/internal/roles"
)

type MockAnalyzer struct{}

func (MockAnalyzer) Mode() string  { return "mock" }
func (MockAnalyzer) Model() string { return "local-deterministic-baseline" }

func (MockAnalyzer) Analyze(_ context.Context, c Company, q Query, a Article, preliminaryScore int) (AnalysisResult, error) {
	start := time.Now()
	var topics []string
	for _, t := range analysis.KeywordTopics(a.Title, a.Summary) {
		topics = append(topics, t.Code)
	}
	var facts, sources []string
	sources = append(sources, "A1")
	for _, f := range c.Facts {
		if overlapScore(f.Topics, topics) > 0 {
			facts = append(facts, f.ID)
			sources = append(sources, f.ID)
		}
	}
	impact := "risk"
	if contains(topics, "consumer_demand") || contains(topics, "technology") {
		impact = "mixed"
	}
	text := fmt.Sprintf("%s. Для %s компании %s событие связано с темами %s.", a.Summary,
		roles.Resolve(q.RoleCode).Title, c.Name, strings.Join(topics, ", "))
	return AnalysisResult{Score: preliminaryScore, MatchedTopics: topics, UsedCompanyFacts: facts, Impact: impact,
		SourceIDs: sources, Text: text, Duration: time.Since(start), DurationMS: time.Since(start).Milliseconds()}, nil
}

type LiveAnalyzer struct {
	Client    *llm.Client
	ModelID   string
	MaxTokens int
	InputUSD  float64
	OutputUSD float64
}

func (a LiveAnalyzer) Mode() string  { return "live" }
func (a LiveAnalyzer) Model() string { return a.ModelID }

func analysisContext(c Company, q Query) analysis.Context {
	r := roles.Resolve(q.RoleCode)
	var facts []string
	for _, f := range c.Facts {
		facts = append(facts, fmt.Sprintf("[%s] %s: %s", f.ID, f.Label, f.Value))
	}
	var interests []string
	for topic := range q.Interests {
		interests = append(interests, topic)
	}
	brief := fmt.Sprintf("%s; отрасль: %s; регион: %s.\n%s", c.Name, c.Industry, c.Region, strings.Join(facts, "\n"))
	if len(c.DataGaps) > 0 {
		brief += "\nНет данных: " + strings.Join(c.DataGaps, "; ")
	}
	return analysis.Context{RoleTitle: r.Title, RoleFocus: r.Focus, Interests: interests, CompanyBrief: brief,
		Horizon: "medium", Detail: "brief"}
}

func (a LiveAnalyzer) Analyze(ctx context.Context, c Company, q Query, article Article, _ int) (AnalysisResult, error) {
	reg := analysis.NewRegistry()
	reg.Add(analysis.Evidence{ID: "A1", Type: "article", Title: article.Title, Date: article.PublishedAt.Format("2006-01-02"),
		Excerpt: article.Summary, Note: "evaluation holdout; источник: " + article.Source})
	for _, f := range c.Facts {
		reg.Add(analysis.Evidence{ID: f.ID, Type: "company_data", Title: f.Label, Excerpt: f.Value, Note: "evaluation profile"})
	}
	known := map[string]bool{}
	for _, t := range topicCatalog() {
		known[t.Code] = true
	}
	start := time.Now()
	resp, err := a.Client.Chat(ctx, llm.Request{Model: a.ModelID,
		Messages:    analysis.InsightMessages(reg.Items[0], analysisContext(c, q), nil, reg, ""),
		Temperature: 0.1, MaxTokens: a.MaxTokens, JSON: true})
	out := AnalysisResult{PromptTokens: resp.Usage.PromptTokens, CompletionTokens: resp.Usage.CompletionTokens,
		Duration: time.Since(start), DurationMS: time.Since(start).Milliseconds()}
	out.EstimatedCostUSD = float64(out.PromptTokens)*a.InputUSD/1_000_000 + float64(out.CompletionTokens)*a.OutputUSD/1_000_000
	if err != nil {
		return out, err
	}
	var insight analysis.Insight
	if err := llm.DecodeJSON(resp.Content, &insight); err != nil {
		return out, err
	}
	if err := analysis.ValidateInsight(&insight, reg, known); err != nil {
		return out, err
	}
	out.Score, out.MatchedTopics, out.Impact = insight.RelevanceScore, insight.MatchedTopics, insight.ImpactType
	for _, e := range insight.EvidenceSources {
		out.SourceIDs = append(out.SourceIDs, e.ID)
		if e.Type == "company_data" {
			out.UsedCompanyFacts = append(out.UsedCompanyFacts, e.ID)
		}
	}
	b, _ := json.Marshal(insight)
	out.Text = string(b)
	return out, nil
}

func topicCatalog() []analysis.Topic {
	return []analysis.Topic{
		{Code: "monetary_policy", Name: "Ключевая ставка и ДКП"}, {Code: "inflation", Name: "Инфляция"},
		{Code: "fx", Name: "Валютный рынок"}, {Code: "debt_market", Name: "Долговой рынок"},
		{Code: "credit", Name: "Кредитование"}, {Code: "commodities", Name: "Сырьевые рынки"},
		{Code: "oil_gas", Name: "Нефть, газ и топливо"}, {Code: "metals", Name: "Металлы"},
		{Code: "gold", Name: "Золото"}, {Code: "agriculture", Name: "Агросектор"},
		{Code: "logistics", Name: "Логистика"}, {Code: "trade", Name: "Внешняя торговля"},
		{Code: "regulation", Name: "Регулирование"}, {Code: "taxes_budget", Name: "Налоги"},
		{Code: "labor", Name: "Рынок труда"}, {Code: "consumer_demand", Name: "Потребительский спрос"},
		{Code: "retail_wholesale", Name: "Торговля"}, {Code: "technology", Name: "Технологии"},
		{Code: "energy", Name: "Энергетика"}, {Code: "macro", Name: "Макроэкономика"},
		{Code: "finance_sector", Name: "Финансовый сектор"}, {Code: "corporate", Name: "Корпоративные события"},
	}
}
