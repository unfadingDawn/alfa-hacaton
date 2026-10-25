package evaluation

import (
	"math"
	"sort"
	"strings"
	"time"
)

type ScoredArticle struct {
	ArticleID       string   `json:"article_id"`
	Score           int      `json:"score"`
	RuleScore       int      `json:"rule_score"`
	SemanticScore   int      `json:"semantic_score"`
	PredictedTopics []string `json:"predicted_topics"`
}

type RankingResult struct {
	QueryID   string          `json:"query_id"`
	Precision float64         `json:"precision_at_k"`
	Recall    float64         `json:"recall_at_k"`
	NDCG      float64         `json:"ndcg_at_k"`
	Top       []ScoredArticle `json:"top"`
}

type AnalysisResult struct {
	Score            int           `json:"score"`
	MatchedTopics    []string      `json:"matched_topics"`
	UsedCompanyFacts []string      `json:"used_company_facts"`
	Impact           string        `json:"impact"`
	SourceIDs        []string      `json:"source_ids"`
	Text             string        `json:"text"`
	PromptTokens     int           `json:"prompt_tokens"`
	CompletionTokens int           `json:"completion_tokens"`
	Duration         time.Duration `json:"-"`
	DurationMS       int64         `json:"duration_ms"`
	EstimatedCostUSD float64       `json:"estimated_cost_usd"`
	// Только для полного анализа (live): покрытие источниками и
	// число значений, не найденных в источниках.
	SourceCoverage   *float64 `json:"source_coverage,omitempty"`
	UnverifiedValues int      `json:"unverified_values"`
}

type CaseResult struct {
	QueryID            string   `json:"query_id"`
	ArticleID          string   `json:"article_id"`
	Mode               string   `json:"mode"`
	Success            bool     `json:"success"`
	Error              string   `json:"error,omitempty"`
	Score              int      `json:"score"`
	ScoreInRange       bool     `json:"score_in_range"`
	TopicMatch         float64  `json:"topic_match"`
	CompanyFactsRecall float64  `json:"company_facts_recall"`
	ImpactMatch        bool     `json:"impact_match"`
	ValidSourceIDs     bool     `json:"valid_source_ids"`
	ForbiddenClaims    []string `json:"forbidden_claims,omitempty"`
	PromptTokens       int      `json:"prompt_tokens"`
	CompletionTokens   int      `json:"completion_tokens"`
	DurationMS         int64    `json:"duration_ms"`
	EstimatedCostUSD   float64  `json:"estimated_cost_usd"`
	SourceCoverage     *float64 `json:"source_coverage,omitempty"`
	UnverifiedValues   int      `json:"unverified_values"`
	UsedCompanyFacts   []string `json:"used_company_facts,omitempty"`
	TextDigest         string   `json:"text_digest,omitempty"`
}

type Metrics struct {
	PrecisionAtK        float64 `json:"precision_at_k"`
	RecallAtK           float64 `json:"recall_at_k"`
	NDCGAtK             float64 `json:"ndcg_at_k"`
	TopicMatch          float64 `json:"topic_match"`
	ScoreInRangeRate    float64 `json:"score_in_range_rate"`
	PersonalizationRate float64 `json:"personalization_rate"`
	ImpactMatchRate     float64 `json:"impact_match_rate"`
	ValidSourceRate     float64 `json:"valid_source_ids_rate"`
	HallucinationRate   float64 `json:"hallucination_rate"`
	JSONSuccessRate     float64 `json:"json_success_rate"`
	AverageLatencyMS    float64 `json:"average_latency_ms"`
	AverageTokens       float64 `json:"average_tokens"`
	AverageCostUSD      float64 `json:"average_estimated_cost_usd"`
	// Доля значимых утверждений с опорой на источники (среднее по кейсам,
	// где выполнялся полный анализ) и доля кейсов с неподтверждёнными значениями.
	SourceCoverage      *float64 `json:"source_coverage,omitempty"`
	UnverifiedValueRate *float64 `json:"unverified_value_rate,omitempty"`
	// Доля пар кейсов «одна новость — разные компании/роли», в которых
	// результат различается (оценка, использованные признаки или текст).
	DifferentiationRate *float64 `json:"differentiation_rate,omitempty"`
}

type Report struct {
	SchemaVersion  string          `json:"schema_version"`
	DatasetVersion string          `json:"dataset_version"`
	PromptVersion  string          `json:"prompt_version"`
	Mode           string          `json:"mode"`
	Model          string          `json:"model"`
	GeneratedAt    time.Time       `json:"generated_at"`
	K              int             `json:"k"`
	Companies      int             `json:"companies"`
	Articles       int             `json:"articles"`
	Queries        int             `json:"queries"`
	Judgments      int             `json:"judgments"`
	Duplicates     int             `json:"duplicates_excluded"`
	Ranking        []RankingResult `json:"ranking"`
	Cases          []CaseResult    `json:"cases"`
	Metrics        Metrics         `json:"metrics"`
}

func contains(in []string, want string) bool {
	for _, v := range in {
		if v == want {
			return true
		}
	}
	return false
}

func overlapScore(expected, actual []string) float64 {
	if len(expected) == 0 {
		return 1
	}
	hits := 0
	for _, x := range expected {
		if contains(actual, x) {
			hits++
		}
	}
	return float64(hits) / float64(len(expected))
}

func rankingMetrics(top []ScoredArticle, relevant map[string]bool, k int) (precision, recall, ndcg float64) {
	if k > len(top) {
		k = len(top)
	}
	if k == 0 {
		return 0, 0, 0
	}
	hits, dcg := 0, 0.0
	for i := 0; i < k; i++ {
		if relevant[top[i].ArticleID] {
			hits++
			dcg += 1 / math.Log2(float64(i)+2)
		}
	}
	precision = float64(hits) / float64(k)
	if len(relevant) > 0 {
		recall = float64(hits) / float64(len(relevant))
	}
	ideal := 0.0
	for i := 0; i < min(k, len(relevant)); i++ {
		ideal += 1 / math.Log2(float64(i)+2)
	}
	if ideal > 0 {
		ndcg = dcg / ideal
	}
	return
}

func aggregate(report *Report) {
	for _, r := range report.Ranking {
		report.Metrics.PrecisionAtK += r.Precision
		report.Metrics.RecallAtK += r.Recall
		report.Metrics.NDCGAtK += r.NDCG
	}
	if n := float64(len(report.Ranking)); n > 0 {
		report.Metrics.PrecisionAtK /= n
		report.Metrics.RecallAtK /= n
		report.Metrics.NDCGAtK /= n
	}
	for _, c := range report.Cases {
		if c.Success {
			report.Metrics.JSONSuccessRate++
		}
		if c.ScoreInRange {
			report.Metrics.ScoreInRangeRate++
		}
		report.Metrics.TopicMatch += c.TopicMatch
		report.Metrics.PersonalizationRate += c.CompanyFactsRecall
		if c.ImpactMatch {
			report.Metrics.ImpactMatchRate++
		}
		if c.ValidSourceIDs {
			report.Metrics.ValidSourceRate++
		}
		if len(c.ForbiddenClaims) > 0 {
			report.Metrics.HallucinationRate++
		}
		report.Metrics.AverageLatencyMS += float64(c.DurationMS)
		report.Metrics.AverageTokens += float64(c.PromptTokens + c.CompletionTokens)
		report.Metrics.AverageCostUSD += c.EstimatedCostUSD
	}
	covSum, covN, unverified := 0.0, 0, 0
	for _, c := range report.Cases {
		if c.SourceCoverage != nil {
			covSum += *c.SourceCoverage
			covN++
			if c.UnverifiedValues > 0 {
				unverified++
			}
		}
	}
	byArticle := map[string][]CaseResult{}
	for _, c := range report.Cases {
		if c.Success {
			byArticle[c.ArticleID] = append(byArticle[c.ArticleID], c)
		}
	}
	pairs, differ := 0, 0
	for _, cs := range byArticle {
		for i := range cs {
			for j := i + 1; j < len(cs); j++ {
				pairs++
				a, b := cs[i], cs[j]
				if a.Score != b.Score || strings.Join(a.UsedCompanyFacts, ",") != strings.Join(b.UsedCompanyFacts, ",") || a.TextDigest != b.TextDigest {
					differ++
				}
			}
		}
	}
	if pairs > 0 {
		rate := float64(differ) / float64(pairs)
		report.Metrics.DifferentiationRate = &rate
	}
	if covN > 0 {
		cov, rate := covSum/float64(covN), float64(unverified)/float64(covN)
		report.Metrics.SourceCoverage, report.Metrics.UnverifiedValueRate = &cov, &rate
	}
	if n := float64(len(report.Cases)); n > 0 {
		report.Metrics.JSONSuccessRate /= n
		report.Metrics.ScoreInRangeRate /= n
		report.Metrics.TopicMatch /= n
		report.Metrics.PersonalizationRate /= n
		report.Metrics.ImpactMatchRate /= n
		report.Metrics.ValidSourceRate /= n
		report.Metrics.HallucinationRate /= n
		report.Metrics.AverageLatencyMS /= n
		report.Metrics.AverageTokens /= n
		report.Metrics.AverageCostUSD /= n
	}
}

func forbiddenClaims(text string, forbidden []string) []string {
	text = strings.ToLower(text)
	var found []string
	for _, claim := range forbidden {
		if claim != "" && strings.Contains(text, strings.ToLower(claim)) {
			found = append(found, claim)
		}
	}
	sort.Strings(found)
	return found
}
