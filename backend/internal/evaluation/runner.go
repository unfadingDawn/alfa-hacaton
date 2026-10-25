package evaluation

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"math"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/alfainsight/backend/internal/analysis"
	"github.com/alfainsight/backend/internal/roles"
	"github.com/alfainsight/backend/internal/semantic"
)

type Analyzer interface {
	Mode() string
	Model() string
	Analyze(context.Context, Company, Query, Article, int) (AnalysisResult, error)
}

type Options struct {
	K            int
	MaxLiveCases int
	InputUSD     float64
	OutputUSD    float64
}

type prepared struct {
	article Article
	topics  []string
	repr    semantic.Representation
}

func relevanceScore(similarity float64) int {
	score := int(math.Round(85 * (similarity - 0.12) / 0.5))
	return max(0, min(85, score))
}

func companyText(c Company, q Query) string {
	r := roles.Resolve(q.RoleCode)
	parts := []string{c.Name, c.Industry, c.Region, strings.Join(c.Countries, " "), strings.Join(c.Topics, " "),
		r.Title, strings.Join(r.Focus, ". ")}
	for _, f := range c.Facts {
		parts = append(parts, f.Label, f.Value)
	}
	for topic := range q.Interests {
		parts = append(parts, topic)
	}
	return strings.Join(parts, ". ")
}

func ruleContext(c Company, q Query) analysis.RuleContext {
	r := roles.Resolve(q.RoleCode)
	return analysis.RuleContext{CompanyTopics: c.Topics, RoleTopics: r.Topics, Interests: q.Interests,
		OkvedDivision: c.OkvedDivision, Region: c.Region, Countries: c.Countries}
}

func prelim(model *semantic.Model, c Company, q Query, p prepared) ScoredArticle {
	rs := analysis.ScoreByRules(p.topics, p.article.Title, p.article.Summary, ruleContext(c, q), false)
	sem := relevanceScore(semantic.Compare(model.Represent(companyText(c, q)), p.repr).Combined)
	return ScoredArticle{ArticleID: p.article.ID, Score: max(rs.Score, sem), RuleScore: rs.Score,
		SemanticScore: sem, PredictedTopics: p.topics}
}

func Run(ctx context.Context, d Dataset, analyzer Analyzer, opts Options) (Report, error) {
	if opts.K <= 0 {
		opts.K = 5
	}
	companies := map[string]Company{}
	for _, c := range d.Companies {
		companies[c.ID] = c
	}
	articles := map[string]Article{}
	var docs []semantic.Document
	duplicates := 0
	for _, a := range d.Articles {
		articles[a.ID] = a
		if a.DuplicateOf == "" {
			docs = append(docs, semantic.Document{ID: a.ID, Title: a.Title, Summary: a.Summary})
		} else {
			duplicates++
		}
	}
	model, err := semantic.Fit(docs, semantic.Config{MaxDocuments: len(docs), MaxFeatures: 1500, Dimensions: 12, MinDocuments: 5})
	if err != nil {
		return Report{}, fmt.Errorf("fit semantic model: %w", err)
	}
	prep := map[string]prepared{}
	for _, a := range d.Articles {
		if a.DuplicateOf != "" {
			continue
		}
		var topics []string
		for _, t := range analysis.KeywordTopics(a.Title, a.Summary) {
			topics = append(topics, t.Code)
		}
		prep[a.ID] = prepared{article: a, topics: topics, repr: model.RepresentDocument(a.Title, a.Summary)}
	}
	judgments := map[string][]Judgment{}
	for _, j := range d.Judgments {
		judgments[j.QueryID] = append(judgments[j.QueryID], j)
	}
	report := Report{SchemaVersion: "1", DatasetVersion: d.Version, PromptVersion: analysis.PromptVersion,
		Mode: analyzer.Mode(), Model: analyzer.Model(), GeneratedAt: time.Now().UTC(), K: opts.K,
		Companies: len(d.Companies), Articles: len(d.Articles), Queries: len(d.Queries), Judgments: len(d.Judgments), Duplicates: duplicates}
	prelimByQuery := map[string]map[string]ScoredArticle{}
	for _, q := range d.Queries {
		c := companies[q.CompanyID]
		var ranked []ScoredArticle
		prelimByQuery[q.ID] = map[string]ScoredArticle{}
		for id, p := range prep {
			s := prelim(model, c, q, p)
			ranked = append(ranked, s)
			prelimByQuery[q.ID][id] = s
		}
		sort.Slice(ranked, func(i, j int) bool {
			if ranked[i].Score != ranked[j].Score {
				return ranked[i].Score > ranked[j].Score
			}
			return ranked[i].ArticleID < ranked[j].ArticleID
		})
		relevant := map[string]bool{}
		for _, j := range judgments[q.ID] {
			if j.ExpectedRelevant {
				relevant[j.ArticleID] = true
			}
		}
		precision, recall, ndcg := rankingMetrics(ranked, relevant, opts.K)
		top := ranked[:min(opts.K, len(ranked))]
		report.Ranking = append(report.Ranking, RankingResult{QueryID: q.ID, Precision: precision, Recall: recall, NDCG: ndcg, Top: top})
	}

	liveCount := 0
	for _, q := range d.Queries {
		c := companies[q.CompanyID]
		for _, j := range judgments[q.ID] {
			if !j.ExpectedRelevant {
				continue
			}
			if analyzer.Mode() == "live" && opts.MaxLiveCases > 0 && liveCount >= opts.MaxLiveCases {
				continue
			}
			liveCount++
			p := prelimByQuery[q.ID][j.ArticleID]
			out, analyzeErr := analyzer.Analyze(ctx, c, q, articles[j.ArticleID], p.Score)
			cr := CaseResult{QueryID: q.ID, ArticleID: j.ArticleID, Mode: analyzer.Mode(), Success: analyzeErr == nil,
				Score: out.Score, PromptTokens: out.PromptTokens, CompletionTokens: out.CompletionTokens,
				DurationMS: out.DurationMS, EstimatedCostUSD: out.EstimatedCostUSD,
				SourceCoverage: out.SourceCoverage, UnverifiedValues: out.UnverifiedValues,
				UsedCompanyFacts: out.UsedCompanyFacts, TextDigest: digest(out.Text)}
			if analyzeErr != nil {
				cr.Error = analyzeErr.Error()
			} else {
				cr.ScoreInRange = out.Score >= j.ExpectedScoreRange[0] && out.Score <= j.ExpectedScoreRange[1]
				cr.TopicMatch = overlapScore(j.ExpectedTopics, out.MatchedTopics)
				cr.CompanyFactsRecall = overlapScore(j.ExpectedCompanyFacts, out.UsedCompanyFacts)
				cr.ImpactMatch = contains(j.ExpectedImpact, out.Impact)
				valid := map[string]bool{"A1": true}
				for _, f := range c.Facts {
					valid[f.ID] = true
				}
				cr.ValidSourceIDs = true
				for _, id := range out.SourceIDs {
					if !valid[id] {
						cr.ValidSourceIDs = false
					}
				}
				cr.ForbiddenClaims = forbiddenClaims(out.Text, j.MustNotClaim)
			}
			report.Cases = append(report.Cases, cr)
		}
	}
	aggregate(&report)
	return report, nil
}

func Save(report Report, path string) error {
	if path == "" {
		path = filepath.Join("evaluation-results", fmt.Sprintf("%s-%s.json", report.Mode, report.GeneratedAt.Format("20060102T150405Z")))
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	b, err := json.MarshalIndent(report, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(path, append(b, '\n'), 0o644)
}

type Delta struct {
	PrecisionAtK        float64 `json:"precision_at_k"`
	RecallAtK           float64 `json:"recall_at_k"`
	NDCGAtK             float64 `json:"ndcg_at_k"`
	TopicMatch          float64 `json:"topic_match"`
	ScoreInRangeRate    float64 `json:"score_in_range_rate"`
	PersonalizationRate float64 `json:"personalization_rate"`
	HallucinationRate   float64 `json:"hallucination_rate"`
	JSONSuccessRate     float64 `json:"json_success_rate"`
}

func Compare(report Report, baselinePath string) (Delta, error) {
	var baseline Report
	if err := readJSON(baselinePath, &baseline); err != nil {
		return Delta{}, err
	}
	return Delta{
		PrecisionAtK:        report.Metrics.PrecisionAtK - baseline.Metrics.PrecisionAtK,
		RecallAtK:           report.Metrics.RecallAtK - baseline.Metrics.RecallAtK,
		NDCGAtK:             report.Metrics.NDCGAtK - baseline.Metrics.NDCGAtK,
		TopicMatch:          report.Metrics.TopicMatch - baseline.Metrics.TopicMatch,
		ScoreInRangeRate:    report.Metrics.ScoreInRangeRate - baseline.Metrics.ScoreInRangeRate,
		PersonalizationRate: report.Metrics.PersonalizationRate - baseline.Metrics.PersonalizationRate,
		HallucinationRate:   report.Metrics.HallucinationRate - baseline.Metrics.HallucinationRate,
		JSONSuccessRate:     report.Metrics.JSONSuccessRate - baseline.Metrics.JSONSuccessRate,
	}, nil
}

func digest(text string) string {
	if text == "" {
		return ""
	}
	sum := sha256.Sum256([]byte(text))
	return hex.EncodeToString(sum[:8])
}
