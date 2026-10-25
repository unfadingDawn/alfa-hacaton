package companies

import (
	"strconv"
	"strings"
)

// Section — раздел ОКВЭД 2 (ОК 029-2014) по двузначному коду класса.
type Section struct {
	Letter string `json:"letter"`
	Name   string `json:"name"`
}

var sections = []struct {
	from, to int
	s        Section
}{
	{1, 3, Section{"A", "Сельское, лесное хозяйство, охота, рыболовство и рыбоводство"}},
	{5, 9, Section{"B", "Добыча полезных ископаемых"}},
	{10, 33, Section{"C", "Обрабатывающие производства"}},
	{35, 35, Section{"D", "Обеспечение электрической энергией, газом и паром"}},
	{36, 39, Section{"E", "Водоснабжение, водоотведение, утилизация отходов"}},
	{41, 43, Section{"F", "Строительство"}},
	{45, 47, Section{"G", "Торговля оптовая и розничная; ремонт автотранспортных средств"}},
	{49, 53, Section{"H", "Транспортировка и хранение"}},
	{55, 56, Section{"I", "Гостиницы и предприятия общественного питания"}},
	{58, 63, Section{"J", "Информация и связь"}},
	{64, 66, Section{"K", "Финансовая и страховая деятельность"}},
	{68, 68, Section{"L", "Операции с недвижимым имуществом"}},
	{69, 75, Section{"M", "Профессиональная, научная и техническая деятельность"}},
	{77, 82, Section{"N", "Административная деятельность и сопутствующие услуги"}},
	{84, 84, Section{"O", "Государственное управление и обеспечение военной безопасности"}},
	{85, 85, Section{"P", "Образование"}},
	{86, 88, Section{"Q", "Здравоохранение и социальные услуги"}},
	{90, 93, Section{"R", "Культура, спорт, досуг и развлечения"}},
	{94, 96, Section{"S", "Предоставление прочих видов услуг"}},
	{97, 98, Section{"T", "Деятельность домашних хозяйств"}},
	{99, 99, Section{"U", "Деятельность экстерриториальных организаций"}},
}

// Division возвращает двузначный класс ОКВЭД ("46.71.2" -> "46").
func Division(code string) string {
	code = strings.TrimSpace(code)
	if i := strings.Index(code, "."); i >= 0 {
		code = code[:i]
	}
	if len(code) == 1 {
		code = "0" + code
	}
	return code
}

func SectionOf(code string) Section {
	n, err := strconv.Atoi(Division(code))
	if err != nil {
		return Section{"?", "Не определено"}
	}
	for _, s := range sections {
		if n >= s.from && n <= s.to {
			return s.s
		}
	}
	return Section{"?", "Не определено"}
}

// topicHints — какие темы новостей обычно затрагивают компании класса ОКВЭД.
// Это вспомогательная эвристика для предварительного отбора кандидатов;
// окончательную оценку релевантности делает языковая модель.
var divisionTopics = map[string][]string{
	"01": {"agriculture", "commodities", "trade"}, "02": {"agriculture", "commodities"}, "03": {"agriculture", "trade"},
	"05": {"commodities", "energy"}, "06": {"oil_gas", "commodities", "trade"}, "07": {"metals", "commodities"},
	"08": {"commodities", "construction"}, "09": {"oil_gas"},
	"10": {"agriculture", "consumer_demand", "inflation"}, "11": {"agriculture", "consumer_demand", "regulation"},
	"12": {"regulation", "consumer_demand"}, "13": {"consumer_demand", "trade"}, "14": {"consumer_demand", "trade"},
	"15": {"consumer_demand", "trade"}, "16": {"commodities", "construction"}, "17": {"commodities"},
	"19": {"oil_gas", "commodities"}, "20": {"commodities", "agriculture"}, "21": {"regulation", "trade"},
	"22": {"commodities", "oil_gas"}, "23": {"construction", "commodities"}, "24": {"metals", "commodities", "trade"},
	"25": {"metals"}, "26": {"technology", "trade"}, "27": {"technology", "energy"}, "28": {"technology", "credit"},
	"29": {"consumer_demand", "trade", "credit"}, "30": {"logistics", "trade"}, "31": {"consumer_demand"},
	"32": {"consumer_demand"}, "33": {"technology"},
	"35": {"energy", "regulation"}, "36": {"regulation"}, "37": {"regulation"}, "38": {"regulation"}, "39": {"regulation"},
	"41": {"construction", "credit", "monetary_policy"}, "42": {"construction", "taxes_budget"}, "43": {"construction"},
	"45": {"consumer_demand", "trade", "credit"}, "46": {"retail_wholesale", "trade", "fx"},
	"47": {"retail_wholesale", "consumer_demand", "inflation"},
	"49": {"logistics", "oil_gas"}, "50": {"logistics", "trade"}, "51": {"logistics"}, "52": {"logistics", "trade"},
	"53": {"logistics"}, "55": {"consumer_demand"}, "56": {"consumer_demand", "inflation"},
	"58": {"technology"}, "59": {"technology"}, "60": {"technology"}, "61": {"technology", "regulation"},
	"62": {"technology", "labor"}, "63": {"technology"},
	"64": {"finance_sector", "monetary_policy", "debt_market"}, "65": {"finance_sector", "regulation"},
	"66": {"finance_sector", "debt_market"}, "68": {"construction", "credit", "monetary_policy"},
	"69": {"regulation", "taxes_budget"}, "70": {"corporate", "macro"}, "71": {"construction"},
	"72": {"technology"}, "73": {"consumer_demand"}, "74": {"corporate"}, "75": {"agriculture"},
	"77": {"credit", "monetary_policy"}, "78": {"labor"}, "79": {"consumer_demand"}, "80": {"regulation"},
	"81": {"construction"}, "82": {"corporate"}, "85": {"regulation", "labor"}, "86": {"regulation"},
	"87": {"regulation"}, "88": {"regulation"}, "90": {"consumer_demand"}, "93": {"consumer_demand"},
}

// Уточнения по подклассам, где тема явно следует из названия вида деятельности.
var codeTopics = map[string][]string{
	"46.71": {"oil_gas", "commodities"}, "46.72": {"metals", "commodities"}, "46.73": {"construction"},
	"46.75": {"commodities"}, "46.21": {"agriculture", "commodities"}, "46.3": {"agriculture", "consumer_demand"},
	"46.46": {"regulation"}, "46.69": {"technology"}, "47.30": {"oil_gas"}, "47.11": {"consumer_demand"},
	"64.91": {"credit", "monetary_policy"},
}

func TopicHints(okvedCode string) []string {
	seen := map[string]bool{}
	var out []string
	add := func(ts []string) {
		for _, t := range ts {
			if !seen[t] {
				seen[t] = true
				out = append(out, t)
			}
		}
	}
	for prefix, ts := range codeTopics {
		if strings.HasPrefix(okvedCode, prefix) {
			add(ts)
		}
	}
	add(divisionTopics[Division(okvedCode)])
	return out
}
