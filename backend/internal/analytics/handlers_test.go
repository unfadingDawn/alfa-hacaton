package analytics

import "testing"

func TestQualityPercent(t *testing.T) {
	if got := qualityPercent(1, 0); got != nil {
		t.Fatalf("для пустой выборки ожидался nil, получено %v", got)
	}
	if got := qualityPercent(2, 3); got != 67 {
		t.Fatalf("округление: ожидалось 67, получено %v", got)
	}
}
