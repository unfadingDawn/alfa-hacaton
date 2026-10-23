package companies

import (
	"fmt"
	"strings"
	"time"
	"unicode/utf8"
)

// BusinessProfile — сведения, которые видны в личном кабинете интернет-банка
// «Альфа-Бизнес» (главный экран: компании, счета и валюты, остатки, бизнес-карты,
// заявки по продуктам, тариф, индикатор риска, госзакупки).
//
// Сейчас их вводит пользователь при регистрации (Source = "user"). При входе
// из Альфа-Бизнеса те же поля будет заполнять интеграция (Source =
// "alfa_business") — см. internal/integrations/alfabusiness.
type BusinessProfile struct {
	Source    string     `json:"source"`               // user | alfa_business
	UpdatedAt *time.Time `json:"updated_at,omitempty"` // когда сведения обновлялись

	LegalName          string        `json:"legal_name,omitempty"`           // «ООО „Город Нагатино“»
	ManagerName        string        `json:"manager_name,omitempty"`         // ФИО пользователя в кабинете («Набоков И.Д.»)
	CompaniesInAccount int           `json:"companies_in_account,omitempty"` // «В 7 компаниях»: юрлица, доступные в кабинете
	AccountsRub        int           `json:"accounts_rub,omitempty"`         // «Счета: 8 в рублях»
	AccountsFx         int           `json:"accounts_fx,omitempty"`          // «… 2 в валюте»
	Currencies         []string      `json:"currencies,omitempty"`           // CNY, KZT, …
	BalanceRub         *float64      `json:"balance_rub,omitempty"`          // общий остаток, руб.
	OverdraftAvailable *bool         `json:"overdraft_available,omitempty"`  // «… с овердрафтом»
	BalanceWithOvd     *float64      `json:"balance_with_overdraft_rub,omitempty"`
	BusinessCards      *bool         `json:"business_cards,omitempty"` // «Бизнес-карты» / «Заказать бизнес-карту»
	Tariff             string        `json:"tariff,omitempty"`
	RiskIndicator      string        `json:"risk_indicator,omitempty"` // low | medium | high
	GovProcurement     *bool         `json:"gov_procurement,omitempty"`
	Applications       []Application `json:"applications,omitempty"` // «Активные заявки»
}

// Application — активная заявка по банковскому продукту.
type Application struct {
	Product string   `json:"product"` // leasing | factoring | credit | guarantee | deposit | rko | cards | other
	Amount  *float64 `json:"amount,omitempty"`
	Status  string   `json:"status,omitempty"` // «на подпись», «в работе у банка» …
}

var ProductLabels = map[string]string{
	"leasing": "лизинг", "factoring": "факторинг", "credit": "кредит", "guarantee": "банковская гарантия",
	"deposit": "депозит", "rko": "расчётно-кассовое обслуживание", "cards": "бизнес-карты", "other": "другой продукт",
}

var riskLabels = map[string]string{"low": "низкий риск", "medium": "средний риск", "high": "высокий риск"}

// KnownCurrencies — коды валют, допустимые в поле «валюты счетов».
var KnownCurrencies = map[string]bool{"USD": true, "EUR": true, "CNY": true, "KZT": true, "AED": true, "TRY": true,
	"INR": true, "BYN": true, "UZS": true, "KGS": true, "AMD": true, "HKD": true, "GBP": true, "CHF": true, "JPY": true}

// ValidateBusiness нормализует сведения и возвращает ошибки по полям.
// Все поля необязательные.
func ValidateBusiness(b *BusinessProfile, source string) map[string]string {
	errs := map[string]string{}
	if b == nil {
		return errs
	}
	b.Source = source
	now := time.Now().UTC()
	b.UpdatedAt = &now
	b.LegalName = strings.TrimSpace(b.LegalName)
	b.ManagerName = strings.TrimSpace(b.ManagerName)
	b.Tariff = strings.TrimSpace(b.Tariff)
	if utf8.RuneCountInString(b.LegalName) > 200 {
		errs["business.legal_name"] = "Не более 200 символов"
	}
	if utf8.RuneCountInString(b.ManagerName) > 120 {
		errs["business.manager_name"] = "Не более 120 символов"
	}
	if utf8.RuneCountInString(b.Tariff) > 100 {
		errs["business.tariff"] = "Не более 100 символов"
	}
	for k, v := range map[string]int{"companies_in_account": b.CompaniesInAccount, "accounts_rub": b.AccountsRub, "accounts_fx": b.AccountsFx} {
		if v < 0 || v > 1000 {
			errs["business."+k] = "Число от 0 до 1000"
		}
	}
	for k, v := range map[string]*float64{"balance_rub": b.BalanceRub, "balance_with_overdraft_rub": b.BalanceWithOvd} {
		if v != nil && (*v < 0 || *v > 1e15) {
			errs["business."+k] = "Некорректная сумма"
		}
	}
	var cur []string
	seen := map[string]bool{}
	for _, c := range b.Currencies {
		c = strings.ToUpper(strings.TrimSpace(c))
		if c == "" || c == "RUB" || seen[c] {
			continue
		}
		if !KnownCurrencies[c] {
			errs["business.currencies"] = "Неизвестный код валюты: " + c
			continue
		}
		seen[c] = true
		cur = append(cur, c)
	}
	b.Currencies = cur
	if b.RiskIndicator != "" && riskLabels[b.RiskIndicator] == "" {
		errs["business.risk_indicator"] = "Допустимо: low, medium, high"
	}
	if len(b.Applications) > 20 {
		errs["business.applications"] = "Не более 20 заявок"
	}
	var apps []Application
	for _, a := range b.Applications {
		if ProductLabels[a.Product] == "" {
			errs["business.applications"] = "Неизвестный продукт: " + a.Product
			continue
		}
		if a.Amount != nil && (*a.Amount < 0 || *a.Amount > 1e15) {
			errs["business.applications"] = "Некорректная сумма заявки"
			continue
		}
		a.Status = strings.TrimSpace(a.Status)
		if utf8.RuneCountInString(a.Status) > 60 {
			a.Status = string([]rune(a.Status)[:60])
		}
		apps = append(apps, a)
	}
	b.Applications = apps
	return errs
}

func (b *BusinessProfile) sourceLabel() string {
	if b.Source == "alfa_business" {
		return "Личный кабинет Альфа-Бизнеса"
	}
	return "Личный кабинет Альфа-Бизнеса — введено пользователем"
}

func yesNo(v bool) string {
	if v {
		return "да"
	}
	return "нет"
}

// addBusiness дополняет профиль фактами и чувствительностями из кабинета.
func (p *Profile) addBusiness(b *BusinessProfile) {
	if b == nil {
		return
	}
	src := b.sourceLabel()
	add := func(id, label, value string) {
		p.Facts = append(p.Facts, Fact{ID: "C:ab_" + id, Label: label, Value: value, Source: src})
	}
	if b.LegalName != "" {
		add("legal_name", "Название организации", b.LegalName)
	}
	if b.CompaniesInAccount > 0 {
		add("companies", "Компаний в личном кабинете", fmt.Sprintf("%d", b.CompaniesInAccount))
	}
	if b.AccountsRub > 0 || b.AccountsFx > 0 {
		add("accounts", "Счета", fmt.Sprintf("%d в рублях, %d в валюте", b.AccountsRub, b.AccountsFx))
	}
	if len(b.Currencies) > 0 {
		add("currencies", "Валюты счетов", strings.Join(b.Currencies, ", "))
	}
	if b.BalanceRub != nil {
		add("balance", "Остаток на счетах", FormatRub(*b.BalanceRub))
	}
	if b.OverdraftAvailable != nil {
		v := yesNo(*b.OverdraftAvailable)
		if b.BalanceWithOvd != nil {
			v += ", с овердрафтом " + FormatRub(*b.BalanceWithOvd)
		}
		add("overdraft", "Овердрафт", v)
	}
	if b.BusinessCards != nil {
		add("cards", "Бизнес-карты", yesNo(*b.BusinessCards))
	}
	if b.Tariff != "" {
		add("tariff", "Тариф обслуживания", b.Tariff)
	}
	if b.RiskIndicator != "" {
		add("risk", "Индикатор риска в кабинете", riskLabels[b.RiskIndicator])
	}
	if b.GovProcurement != nil {
		add("gov", "Участие в госзакупках", yesNo(*b.GovProcurement))
	}
	var apps []string
	products := map[string]bool{}
	for _, a := range b.Applications {
		item := ProductLabels[a.Product]
		if a.Amount != nil {
			item += " " + FormatRub(*a.Amount)
		}
		if a.Status != "" {
			item += " (" + a.Status + ")"
		}
		apps = append(apps, item)
		products[a.Product] = true
	}
	if len(apps) > 0 {
		add("applications", "Активные заявки по продуктам", strings.Join(apps, "; "))
	}

	// Чувствительности: только то, что прямо следует из введённых сведений.
	if len(b.Currencies) > 0 || b.AccountsFx > 0 {
		ids := []string{}
		if len(b.Currencies) > 0 {
			ids = append(ids, "C:ab_currencies")
		}
		if b.AccountsFx > 0 {
			ids = append(ids, "C:ab_accounts")
		}
		expl := "Есть валютные счета: курсы и валютное регулирование влияют на расчёты и стоимость активов."
		if len(b.Currencies) > 0 {
			expl += " Валюты: " + strings.Join(b.Currencies, ", ") + "."
		}
		p.Exposures = append(p.Exposures, Exposure{"fx_accounts", "Валютные счета", expl, []string{"fx", "trade"}, ids})
	}
	if b.OverdraftAvailable != nil && *b.OverdraftAvailable {
		p.Exposures = append(p.Exposures, Exposure{"overdraft", "Краткосрочное финансирование",
			"Используется овердрафт: стоимость краткосрочного долга зависит от ключевой ставки.",
			[]string{"monetary_policy", "credit"}, []string{"C:ab_overdraft"}})
	}
	if products["leasing"] || products["factoring"] || products["credit"] || products["guarantee"] {
		var names []string
		for _, k := range []string{"credit", "leasing", "factoring", "guarantee"} {
			if products[k] {
				names = append(names, ProductLabels[k])
			}
		}
		p.Exposures = append(p.Exposures, Exposure{"pending_financing", "Финансирование в процессе оформления",
			"Есть активные заявки (" + strings.Join(names, ", ") + "): условия зависят от ставок и требований банков на момент сделки.",
			[]string{"credit", "monetary_policy"}, []string{"C:ab_applications"}})
	}
	if b.GovProcurement != nil && *b.GovProcurement {
		p.Exposures = append(p.Exposures, Exposure{"gov_procurement", "Госзакупки",
			"Компания участвует в госзакупках: важны изменения законодательства о закупках, бюджетных расходов и требований к гарантиям.",
			[]string{"regulation", "taxes_budget"}, []string{"C:ab_gov"}})
	}
}
