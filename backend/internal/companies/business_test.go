package companies

import (
	"testing"

	"github.com/alfainsight/backend/internal/db/dbq"
)

func f(v float64) *float64 { return &v }
func b(v bool) *bool       { return &v }

func TestValidateUpdateRequired(t *testing.T) {
	req := UpdateReq{}
	errs := ValidateUpdate(&req)
	for _, k := range []string{"display_name", "products", "markets", "suppliers"} {
		if errs[k] == "" {
			t.Errorf("поле %s должно быть обязательным", k)
		}
	}
	if errs["priorities"] != "" || errs["description"] != "" {
		t.Error("приоритеты и описание — необязательные")
	}
	req = UpdateReq{DisplayName: "ООО", Profile: UserProfile{Products: "x", Markets: "y", Suppliers: "z"}}
	if errs := ValidateUpdate(&req); len(errs) != 0 {
		t.Fatalf("заполненная форма отклонена: %v", errs)
	}
}

func TestValidateBusiness(t *testing.T) {
	bp := &BusinessProfile{Source: "alfa_business", Currencies: []string{"cny", "RUB", "CNY", "XXX"},
		RiskIndicator: "low", Applications: []Application{{Product: "leasing", Amount: f(75000)}, {Product: "magic"}}}
	errs := ValidateBusiness(bp, "user")
	if bp.Source != "user" {
		t.Fatal("источник задаёт сервер")
	}
	if len(bp.Currencies) != 1 || bp.Currencies[0] != "CNY" {
		t.Fatalf("валюты: %v", bp.Currencies)
	}
	if errs["business.currencies"] == "" || errs["business.applications"] == "" {
		t.Fatalf("неизвестные коды должны давать ошибку: %v", errs)
	}
	if len(bp.Applications) != 1 {
		t.Fatal("неизвестный продукт отбрасывается")
	}
}

func TestBusinessFactsAndExposures(t *testing.T) {
	c := dbq.Company{ID: "U_1", OkvedCode: "46.34", OkvedName: "Торговля оптовая напитками"}
	p := Build(c)
	before := len(p.Exposures)
	p.addBusiness(&BusinessProfile{Source: "user", Currencies: []string{"CNY"}, OverdraftAvailable: b(true),
		GovProcurement: b(true), Applications: []Application{{Product: "factoring"}}})
	if len(p.Exposures)-before != 4 {
		t.Fatalf("ожидалось 4 чувствительности из кабинета, получено %d", len(p.Exposures)-before)
	}
	found := false
	for _, fc := range p.Facts {
		if fc.ID == "C:ab_currencies" && fc.Source == "Личный кабинет Альфа-Бизнеса — введено пользователем" {
			found = true
		}
	}
	if !found {
		t.Fatal("факт должен ссылаться на источник «введено пользователем»")
	}
}

func TestGroupRevenueSegment(t *testing.T) {
	rev, grp, n := 5e9, 20e9, int32(3)
	p := Build(dbq.Company{ID: "U_2", OkvedCode: "41.20", OkvedName: "Строительство", Revenue: &rev, GroupRevenue: &grp, GroupSize: &n})
	if !p.IsLarge {
		t.Fatal("крупный бизнес определяется по выручке группы")
	}
	ct := "CLOSE_CLIENT"
	p = Build(dbq.Company{ID: "U_3", OkvedCode: "41.20", OkvedName: "x", ClientType: &ct})
	for _, fc := range p.Facts {
		if fc.ID == "C:client_type" && fc.Value[:6] != "бывший"[:6] {
			t.Fatalf("CLOSE_CLIENT: %s", fc.Value)
		}
	}
}
