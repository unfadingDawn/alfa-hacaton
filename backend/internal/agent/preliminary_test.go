package agent

import "testing"

func TestSemanticRelevance(t *testing.T) {
	if got := semanticRelevance(0.10); got != 0 {
		t.Fatalf("шумовая близость дала %d", got)
	}
	if low, high := semanticRelevance(0.25), semanticRelevance(0.55); low <= 0 || high <= low {
		t.Fatalf("оценка должна монотонно расти: low=%d high=%d", low, high)
	}
	if got := semanticRelevance(1); got != 85 {
		t.Fatalf("предварительная оценка должна быть ограничена 85, получено %d", got)
	}
}

func TestImportanceForHybridScore(t *testing.T) {
	for score, want := range map[int]string{0: "low", 24: "low", 25: "medium", 54: "medium", 55: "high", 85: "high"} {
		if got := importanceForScore(score); got != want {
			t.Fatalf("score=%d: получено %s, ожидалось %s", score, got, want)
		}
	}
}
