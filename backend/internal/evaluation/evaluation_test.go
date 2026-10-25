package evaluation

import (
	"context"
	"path/filepath"
	"runtime"
	"testing"
)

func datasetDir(t *testing.T) string {
	t.Helper()
	_, file, _, _ := runtime.Caller(0)
	return filepath.Clean(filepath.Join(filepath.Dir(file), "..", "..", "..", "data", "evaluation"))
}

func TestHoldoutDatasetAndOfflineEvaluation(t *testing.T) {
	d, err := Load(datasetDir(t))
	if err != nil {
		t.Fatal(err)
	}
	report, err := Run(context.Background(), d, MockAnalyzer{}, Options{K: 5})
	if err != nil {
		t.Fatal(err)
	}
	if report.Mode != "mock" || len(report.Ranking) != len(d.Queries) || len(report.Cases) == 0 {
		t.Fatalf("неполный report: %+v", report)
	}
	if d := report.Metrics.DifferentiationRate; d == nil || *d < 0.8 {
		t.Fatalf("одна новость для разных компаний и ролей должна давать разный результат: %v", d)
	}
	if report.Duplicates == 0 {
		t.Fatal("holdout должен проверять исключение дублей")
	}
	if report.Metrics.JSONSuccessRate != 1 || report.Metrics.ValidSourceRate != 1 {
		t.Fatalf("offline baseline должен быть структурно валиден: %+v", report.Metrics)
	}
}

func TestRankingMetricsPerfect(t *testing.T) {
	top := []ScoredArticle{{ArticleID: "a", Score: 90}, {ArticleID: "b", Score: 80}}
	p, r, n := rankingMetrics(top, map[string]bool{"a": true}, 1)
	if p != 1 || r != 1 || n != 1 {
		t.Fatalf("p=%v r=%v ndcg=%v", p, r, n)
	}
}
