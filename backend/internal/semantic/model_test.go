package semantic

import (
	"math"
	"testing"
)

func testDocuments() []Document {
	return []Document{
		{ID: "1", Title: "Банк России сохранил ставку", Summary: "Дорогое заемное финансирование влияет на кредитную нагрузку бизнеса"},
		{ID: "2", Title: "Стоимость корпоративных кредитов", Summary: "Компании пересматривают рефинансирование долга из-за денежной политики"},
		{ID: "3", Title: "Рынок облигаций", Summary: "Доходности долговых инструментов выросли вслед за ожиданиями по ставке"},
		{ID: "4", Title: "Перевозки контейнеров", Summary: "Логистические компании сообщили о сроках доставки грузов"},
		{ID: "5", Title: "Морские поставки", Summary: "Стоимость фрахта и перевозки импортных товаров снизилась"},
		{ID: "6", Title: "Урожай пшеницы", Summary: "Аграрии ожидают увеличение сбора зерновых культур"},
		{ID: "7", Title: "Розничные продажи", Summary: "Потребительский спрос вырос в крупных городах"},
		{ID: "8", Title: "Новые требования ФНС", Summary: "Изменились правила налоговой отчетности организаций"},
		{ID: "9", Title: "Курс рубля", Summary: "Валютные расчеты экспортеров повлияли на российскую валюту"},
		{ID: "10", Title: "Рынок труда", Summary: "Работодатели увеличили расходы на заработную плату"},
		{ID: "11", Title: "Кредитование предприятий", Summary: "Банковское фондирование остается дорогим для заемщиков"},
		{ID: "12", Title: "Железнодорожная логистика", Summary: "Перевозчики изменили тарифы на доставку сырья"},
	}
}

func TestNormalizeAndTerms(t *testing.T) {
	got := terms("Заёмное финансирование, КРЕДИТЫ и кредиты")
	if got["w:заемное"] == 0 || got["w:кредиты"] != 2 || got["w:и"] != 0 {
		t.Fatalf("неожиданные термы: %#v", got)
	}
}

func TestSemanticSimilarity(t *testing.T) {
	m, err := Fit(testDocuments(), Config{MaxFeatures: 500, Dimensions: 5, MinDocuments: 5})
	if err != nil {
		t.Fatal(err)
	}
	profile := m.Represent("финансовый директор стоимость долга кредитные продукты заемное финансирование")
	credit := m.Similarity(profile, "Условия банковского фондирования", "Заемщикам стало дороже рефинансировать обязательства")
	harvest := m.Similarity(profile, "Сбор зерна", "Аграрии завершили уборку пшеницы")
	if credit.Combined <= harvest.Combined {
		t.Fatalf("кредитная новость должна быть ближе профилю: credit=%+v harvest=%+v", credit, harvest)
	}
	for _, value := range []float64{credit.TFIDF, credit.LSA, credit.Combined} {
		if math.IsNaN(value) || value < 0 || value > 1 {
			t.Fatalf("некорректная близость: %v", value)
		}
	}
}

func TestFitIsDeterministic(t *testing.T) {
	a, err := Fit(testDocuments(), Config{MaxFeatures: 500, Dimensions: 5, MinDocuments: 5})
	if err != nil {
		t.Fatal(err)
	}
	b, err := Fit(testDocuments(), Config{MaxFeatures: 500, Dimensions: 5, MinDocuments: 5})
	if err != nil {
		t.Fatal(err)
	}
	query := "кредитная нагрузка и стоимость долга"
	pa, pb := a.Represent(query), b.Represent(query)
	sa := a.Similarity(pa, "Рефинансирование", "Заемное финансирование стало дороже")
	sb := b.Similarity(pb, "Рефинансирование", "Заемное финансирование стало дороже")
	if a.Version() != b.Version() || math.Abs(sa.Combined-sb.Combined) > 1e-12 {
		t.Fatal("одинаковый корпус должен давать одинаковую модель")
	}
}

func TestSmallCorpusFailsCleanly(t *testing.T) {
	if _, err := Fit(testDocuments()[:2], Config{MinDocuments: 3}); err == nil {
		t.Fatal("ожидалась ошибка для слишком маленького корпуса")
	}
}
