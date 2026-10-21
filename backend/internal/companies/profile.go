package companies

import (
	"encoding/json"
	"fmt"
	"strings"

	"github.com/alfainsight/backend/internal/db/dbq"
)

const dataFile = "df_Uid_data_hack.csv"

// LargeBusinessRevenue — порог крупного бизнеса из материалов кейса (выручка свыше 15 млрд руб.).
const LargeBusinessRevenue = 15_000_000_000

// Fact — проверяемый признак компании с указанием поля-источника.
// ID используется агентом как ссылка в выводах (например, "C:has_loan").
type Fact struct {
	ID     string `json:"id"`
	Label  string `json:"label"`
	Value  string `json:"value"`
	Source string `json:"source"`
}

// Exposure — признак чувствительности бизнеса к внешним событиям,
// выведенный из фактических полей данных.
type Exposure struct {
	Code        string   `json:"code"`
	Title       string   `json:"title"`
	Explanation string   `json:"explanation"`
	Topics      []string `json:"topics"`
	FactIDs     []string `json:"fact_ids"`
}

// UserProfile — сведения, которые пользователь добавил или уточнил сам.
type UserProfile struct {
	Description string   `json:"description,omitempty"`
	Products    string   `json:"products,omitempty"`
	Markets     string   `json:"markets,omitempty"`
	Suppliers   string   `json:"suppliers,omitempty"`
	Priorities  []string `json:"priorities,omitempty"`
	Notes       string   `json:"notes,omitempty"`
}

type Counterparty struct {
	ID        string  `json:"id"`
	Amount    float64 `json:"amount"`
	PayCount  int64   `json:"pay_count"`
	OkvedCode string  `json:"okved_code,omitempty"`
	OkvedName string  `json:"okved_name,omitempty"`
	Region    string  `json:"region,omitempty"`
	Known     bool    `json:"known"`
}

type Industry struct {
	Direction      string  `json:"direction"`
	Division       string  `json:"division"`
	SampleName     string  `json:"sample_name"`
	Counterparties int32   `json:"counterparties"`
	Amount         float64 `json:"amount"`
	Share          float64 `json:"share"`
}

type Relations struct {
	OutgoingAmount float64        `json:"outgoing_amount"`
	OutgoingCount  int64          `json:"outgoing_count"`
	SupplierCount  int32          `json:"supplier_count"`
	IncomingAmount float64        `json:"incoming_amount"`
	IncomingCount  int64          `json:"incoming_count"`
	CustomerCount  int32          `json:"customer_count"`
	TopRecipients  []Counterparty `json:"top_recipients"`
	TopSenders     []Counterparty `json:"top_senders"`
	Industries     []Industry     `json:"industries"`
	GroupID        string         `json:"group_id,omitempty"`
	GroupSize      int32          `json:"group_size"`
	GroupMembers   []Counterparty `json:"group_members"`
	LinkedPersons  int32          `json:"linked_persons"`
	PersonRegions  []string       `json:"person_regions"`
	SharedPersons  []Counterparty `json:"companies_sharing_persons"`
}

type Profile struct {
	ID          string      `json:"id"`
	DisplayName string      `json:"display_name"`
	Section     Section     `json:"section"`
	OkvedCode   string      `json:"okved_code"`
	OkvedName   string      `json:"okved_name"`
	Region      string      `json:"region"`
	City        string      `json:"city"`
	Revenue     *float64    `json:"revenue"`
	SizeSegment string      `json:"size_segment"`
	IsLarge     bool        `json:"is_large"`
	Facts       []Fact      `json:"facts"`
	Exposures   []Exposure  `json:"exposures"`
	TopicHints  []string    `json:"topic_hints"`
	Countries   []string    `json:"fts_countries"`
	User        UserProfile `json:"user_profile"`
	Relations   *Relations  `json:"relations,omitempty"`
	DataGaps    []string    `json:"data_gaps"`
}

func s(p *string) string {
	if p == nil {
		return ""
	}
	return *p
}

func flag(p *bool) string {
	switch {
	case p == nil:
		return "нет данных"
	case *p:
		return "да"
	default:
		return "нет"
	}
}

func isTrue(p *bool) bool { return p != nil && *p }

var clientTypeLabels = map[string]string{
	"CLIENT":   "действующий клиент",
	"PROSPECT": "потенциальный клиент",
}

// FormatRub форматирует сумму в рублях в «млн/млрд руб.».
func FormatRub(v float64) string {
	switch {
	case v >= 1e9:
		return strings.Replace(fmt.Sprintf("%.1f млрд руб.", v/1e9), ".", ",", 1)
	case v >= 1e6:
		return strings.Replace(fmt.Sprintf("%.1f млн руб.", v/1e6), ".", ",", 1)
	default:
		return fmt.Sprintf("%.0f руб.", v)
	}
}

func sizeSegment(rev *float64) (string, bool) {
	if rev == nil {
		return "выручка неизвестна", false
	}
	switch {
	case *rev >= LargeBusinessRevenue:
		return "крупный бизнес (выручка от 15 млрд руб.)", true
	case *rev >= 2e9:
		return "средний и крупный-средний бизнес (2–15 млрд руб.)", false
	case *rev >= 800e6:
		return "средний бизнес (0,8–2 млрд руб.)", false
	default:
		return "малый бизнес (до 800 млн руб.)", false
	}
}

// Build собирает профиль компании из исходных полей. Каждый вывод
// сопровождается ссылкой на поле CSV, из которого он получен.
func Build(c dbq.Company) Profile {
	seg, large := sizeSegment(c.Revenue)
	p := Profile{
		ID: c.ID, DisplayName: s(c.DisplayName), Section: SectionOf(c.OkvedCode),
		OkvedCode: c.OkvedCode, OkvedName: c.OkvedName, Region: s(c.Region), City: s(c.City),
		Revenue: c.Revenue, SizeSegment: seg, IsLarge: large, Countries: c.FtsCountries,
		TopicHints: TopicHints(c.OkvedCode), Facts: []Fact{}, Exposures: []Exposure{}, DataGaps: []string{},
	}
	if len(c.Profile) > 0 {
		_ = json.Unmarshal(c.Profile, &p.User)
	}
	add := func(id, label, value, column string) {
		p.Facts = append(p.Facts, Fact{ID: "C:" + id, Label: label, Value: value, Source: dataFile + ": " + column})
	}
	add("okved", "Основной вид деятельности (ОКВЭД)", c.OkvedCode+" — "+c.OkvedName, "MAINOKVED_CODE, MAINOKVED_NAME")
	add("section", "Раздел ОКВЭД", p.Section.Letter+" — "+p.Section.Name, "MAINOKVED_CODE (производное)")
	if c.Revenue != nil {
		add("revenue", "Годовая выручка", FormatRub(*c.Revenue)+" ("+seg+")", "REVENUE_COMPANY")
	} else {
		p.DataGaps = append(p.DataGaps, "Нет данных о выручке")
	}
	if c.Region != nil {
		add("region", "Регион регистрации", s(c.Region)+", "+s(c.City), "REGION, CITY")
	}
	if c.CompanyAge != nil {
		add("age", "Возраст компании, лет", s(c.CompanyAge), "COMPANY_AGE_YEARS_GRADATION")
	}
	if c.EcoStage != nil {
		add("eco_stage", "Стадия развития бизнеса", s(c.EcoStage), "ECO_MARK_LABEL")
	} else {
		p.DataGaps = append(p.DataGaps, "Не указана стадия развития бизнеса")
	}
	if c.WorkersFns != nil {
		add("workers", "Численность работников (ФНС)", s(c.WorkersFns), "AMOUNT_WORKERS_FNS_GRADATION")
	}
	if c.OkfsName != nil {
		add("okfs", "Форма собственности", strings.ToLower(s(c.OkfsName)), "OKFS_NAME")
	}
	if c.ClientType != nil {
		label, ok := clientTypeLabels[s(c.ClientType)]
		if !ok {
			label = s(c.ClientType) + " (значение не расшифровано в описании данных)"
		}
		add("client_type", "Тип клиента банка", label, "CL_TYPE")
	}
	products := []struct {
		id, label, col string
		v              *bool
	}{
		{"has_rko", "Расчётно-кассовое обслуживание", "CL_PRODUCT_RKO_MARK", c.HasRko},
		{"has_loan", "Кредитные продукты", "CL_PRODUCT_LOA_MARK", c.HasLoan},
		{"has_leasing", "Лизинговые продукты", "CL_PRODUCT_LEA_MARK", c.HasLeasing},
		{"has_guarantee", "Банковские гарантии", "CL_PRODUCT_GAR_MARK", c.HasGuarantee},
		{"has_deposit", "Депозитные продукты", "CL_PRODUCT_DEP_MARK", c.HasDeposit},
		{"has_payroll", "Зарплатный проект", "CL_PRODUCT_ZPP_MARK", c.HasPayroll},
		{"has_factoring", "Факторинг", "FACTORING_MARK", c.HasFactoring},
		{"fts", "Сведения в базе ФТС (внешнеэкономическая деятельность)", "FTS_FLAG", c.FtsFlag},
	}
	for _, pr := range products {
		add(pr.id, pr.label, flag(pr.v), pr.col)
	}
	if len(c.FtsCountries) > 0 {
		add("fts_countries", "Страны в данных ФТС", strings.Join(c.FtsCountries, ", "), "FTS_COUNTRY_NAME")
	}
	if c.NegativeMark != nil {
		add("negative_mark", "Признак ключевого собственника (NEGATIVEMARK)", flag(c.NegativeMark), "NEGATIVEMARK")
	}
	if c.GroupID != nil {
		add("group", "Идентификатор группы компаний", s(c.GroupID), "GROUP_ID")
	}

	// Чувствительности, выводимые из продуктов и ВЭД.
	if isTrue(c.HasLoan) {
		p.Exposures = append(p.Exposures, Exposure{"debt_cost", "Стоимость заёмного финансирования",
			"У компании есть кредитные продукты: изменения ключевой ставки и условий кредитования влияют на стоимость долга.",
			[]string{"monetary_policy", "credit", "debt_market"}, []string{"C:has_loan"}})
	}
	if isTrue(c.HasLeasing) {
		p.Exposures = append(p.Exposures, Exposure{"leasing", "Лизинговые платежи",
			"Используются лизинговые продукты: условия зависят от процентных ставок и стоимости техники/оборудования.",
			[]string{"credit", "monetary_policy"}, []string{"C:has_leasing"}})
	}
	if isTrue(c.HasDeposit) {
		p.Exposures = append(p.Exposures, Exposure{"deposits", "Доходность размещения ликвидности",
			"Есть депозитные продукты: изменение ставок влияет на доход от размещения свободных средств.",
			[]string{"monetary_policy", "finance_sector"}, []string{"C:has_deposit"}})
	}
	if isTrue(c.HasGuarantee) {
		p.Exposures = append(p.Exposures, Exposure{"guarantees", "Банковские гарантии",
			"Используются гарантии: важны требования заказчиков, госзакупок и условия банков.",
			[]string{"credit", "regulation"}, []string{"C:has_guarantee"}})
	}
	if isTrue(c.HasFactoring) {
		p.Exposures = append(p.Exposures, Exposure{"receivables", "Финансирование дебиторской задолженности",
			"Используется факторинг: важны платёжная дисциплина покупателей и стоимость фондирования.",
			[]string{"credit"}, []string{"C:has_factoring"}})
	}
	if isTrue(c.HasPayroll) {
		p.Exposures = append(p.Exposures, Exposure{"payroll", "Фонд оплаты труда",
			"Есть зарплатный проект: изменения на рынке труда и в зарплатах затрагивают расходы на персонал.",
			[]string{"labor"}, []string{"C:has_payroll"}})
	}
	if isTrue(c.FtsFlag) {
		ex := Exposure{"foreign_trade", "Внешнеэкономическая деятельность",
			"Компания присутствует в данных ФТС: курс рубля, пошлины, санкции и логистика влияют на импорт/экспорт.",
			[]string{"trade", "fx", "logistics"}, []string{"C:fts"}}
		if len(c.FtsCountries) > 0 {
			ex.Explanation += " Страны: " + strings.Join(c.FtsCountries, ", ") + "."
			ex.FactIDs = append(ex.FactIDs, "C:fts_countries")
		}
		p.Exposures = append(p.Exposures, ex)
	}
	if strings.TrimSpace(p.User.Description) == "" && strings.TrimSpace(p.User.Products) == "" {
		p.DataGaps = append(p.DataGaps, "Нет описания продуктов и рынков компании (можно добавить в профиле)")
	}
	return p
}

// AllTopics объединяет темы из ОКВЭД и чувствительностей.
func (p Profile) AllTopics() []string {
	seen := map[string]bool{}
	var out []string
	for _, t := range p.TopicHints {
		if !seen[t] {
			seen[t] = true
			out = append(out, t)
		}
	}
	for _, e := range p.Exposures {
		for _, t := range e.Topics {
			if !seen[t] {
				seen[t] = true
				out = append(out, t)
			}
		}
	}
	return out
}

// RelationFacts превращает сведения о контрагентах в проверяемые факты.
func (p *Profile) RelationFacts() []Fact {
	if p.Relations == nil {
		return nil
	}
	r := p.Relations
	var out []Fact
	if r.SupplierCount == 0 && r.CustomerCount == 0 {
		p.DataGaps = append(p.DataGaps, "Нет данных о платежах с контрагентами")
		return nil
	}
	out = append(out, Fact{ID: "C:payments", Label: "Платёжные связи",
		Value: fmt.Sprintf("исходящие платежи %d получателям на %s; входящие от %d плательщиков на %s",
			r.SupplierCount, FormatRub(r.OutgoingAmount), r.CustomerCount, FormatRub(r.IncomingAmount)),
		Source: "df_Uid_payment_hack.csv: PAYER, BENEFICIARY, PAY_AMT, PAY_CNT"})
	var outInd, inInd []string
	for _, in := range r.Industries {
		if in.Share < 0.05 {
			continue
		}
		item := fmt.Sprintf("%s (класс ОКВЭД %s, %.0f%%)", in.SampleName, in.Division, in.Share*100)
		if in.Direction == "outgoing" && len(outInd) < 4 {
			outInd = append(outInd, item)
		} else if in.Direction == "incoming" && len(inInd) < 4 {
			inInd = append(inInd, item)
		}
	}
	if len(outInd) > 0 {
		out = append(out, Fact{ID: "C:supplier_industries", Label: "Отрасли получателей платежей (поставщики/подрядчики)",
			Value: strings.Join(outInd, "; "), Source: "df_Uid_payment_hack.csv + df_Uid_data_hack.csv: MAINOKVED"})
	}
	if len(inInd) > 0 {
		out = append(out, Fact{ID: "C:customer_industries", Label: "Отрасли плательщиков (покупатели/заказчики)",
			Value: strings.Join(inInd, "; "), Source: "df_Uid_payment_hack.csv + df_Uid_data_hack.csv: MAINOKVED"})
	}
	if r.GroupSize > 1 {
		out = append(out, Fact{ID: "C:group_size", Label: "Компаний в группе",
			Value: fmt.Sprintf("%d", r.GroupSize), Source: "df_Uid_data_hack.csv: GROUP_ID"})
	}
	return out
}
