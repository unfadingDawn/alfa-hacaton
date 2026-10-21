// Package importer загружает данные кейса в PostgreSQL. Исходные значения
// сохраняются; пустые строки становятся NULL, флаги 0.0/1.0 — boolean,
// проблемы (дубликаты, ошибки типов) пишутся в import_issues.
package importer

import (
	"encoding/csv"
	"fmt"
	"io"
	"strconv"
	"strings"
)

var (
	CompanyHeader = []string{"SYNTHETIC_U", "ORG_TYPE", "OKFS_NAME", "COMPANY_AGE_YEARS_GRADATION", "ECO_MARK_LABEL",
		"MAINOKVED_CODE", "MAINOKVED_NAME", "NEGATIVEMARK", "AMOUNT_WORKERS_FNS_GRADATION", "AMOUNT_WORKERS_SPARK_GRADATION",
		"CL_TYPE", "CL_PRODUCT_RKO_MARK", "CL_PRODUCT_GAR_MARK", "CL_PRODUCT_LOA_MARK", "CL_PRODUCT_LEA_MARK",
		"CL_PRODUCT_DEP_MARK", "CL_PRODUCT_ZPP_MARK", "FTS_FLAG", "FTS_COUNTRY_NAME", "FACTORING_MARK",
		"REVENUE_COMPANY", "GROUP_ID", "CITY", "REGION"}
	LinkHeader    = []string{"SYNTHETIC_F", "CITY_NAME", "RC_NAME", "SYNTHETIC_U"}
	PaymentHeader = []string{"BENEFICIARY", "PAYER", "PAY_AMT", "PAY_CNT"}
)

// NewReader создаёт CSV-ридер для файлов кейса (разделитель «;», кавычки
// внутри полей вида "Мск ""Юг""").
func NewReader(r io.Reader) *csv.Reader {
	cr := csv.NewReader(r)
	cr.Comma = ';'
	cr.ReuseRecord = false
	cr.FieldsPerRecord = -1
	return cr
}

// CheckHeader проверяет, что заголовок совпадает с ожидаемым (с учётом BOM).
func CheckHeader(got, want []string) error {
	if len(got) > 0 {
		got[0] = strings.TrimPrefix(got[0], "\ufeff")
	}
	if len(got) != len(want) {
		return fmt.Errorf("ожидалось %d колонок, найдено %d", len(want), len(got))
	}
	for i := range want {
		if strings.TrimSpace(got[i]) != want[i] {
			return fmt.Errorf("колонка %d: ожидалось %q, найдено %q", i+1, want[i], got[i])
		}
	}
	return nil
}

// Text: пустая строка → NULL.
func Text(s string) *string {
	s = strings.TrimSpace(s)
	if s == "" {
		return nil
	}
	return &s
}

// Flag разбирает признаки вида "0.0"/"1.0"/"0"/"1"; пустое → NULL.
func Flag(s string) (*bool, error) {
	s = strings.TrimSpace(s)
	if s == "" {
		return nil, nil
	}
	f, err := strconv.ParseFloat(s, 64)
	if err != nil {
		return nil, fmt.Errorf("не число: %q", s)
	}
	var b bool
	switch f {
	case 0:
		b = false
	case 1:
		b = true
	default:
		return nil, fmt.Errorf("ожидалось 0 или 1, найдено %q", s)
	}
	return &b, nil
}

// Number разбирает число (в том числе "1.423218135e9"); пустое → NULL.
func Number(s string) (*float64, error) {
	s = strings.TrimSpace(strings.ReplaceAll(s, ",", "."))
	if s == "" {
		return nil, nil
	}
	f, err := strconv.ParseFloat(s, 64)
	if err != nil {
		return nil, fmt.Errorf("не число: %q", s)
	}
	return &f, nil
}

// Countries разбивает "КИТАЙ, ТУРЦИЯ" в список без пустых значений и повторов.
func Countries(s string) []string {
	out := []string{}
	seen := map[string]bool{}
	for _, p := range strings.Split(s, ",") {
		p = strings.TrimSpace(p)
		if p != "" && !seen[p] {
			seen[p] = true
			out = append(out, p)
		}
	}
	return out
}

// Count разбирает количество платежей ("526", "526.0").
func Count(s string) (int32, error) {
	f, err := Number(s)
	if err != nil || f == nil {
		return 0, fmt.Errorf("пустое или нечисловое количество: %q", s)
	}
	if *f < 0 || *f != float64(int64(*f)) || *f > 2_000_000_000 {
		return 0, fmt.Errorf("некорректное количество: %q", s)
	}
	return int32(*f), nil
}

// CompanyRow — разобранная строка df_Uid_data_hack.csv.
type CompanyRow struct {
	ID, OkvedCode, OkvedName, OkvedDivision                    string
	OrgType, OkfsName, Age, EcoStage, WorkersFns, WorkersSpark *string
	ClientType, GroupID, City, Region                          *string
	Negative, Rko, Gar, Loa, Lea, Dep, Zpp, Fts, Factoring     *bool
	Countries                                                  []string
	Revenue                                                    *float64
	Row                                                        int
	Raw                                                        []string
}

// ParseCompany разбирает строку; ошибка означает, что строку нельзя импортировать.
func ParseCompany(rec []string, row int, division func(string) string) (CompanyRow, error) {
	if len(rec) != len(CompanyHeader) {
		return CompanyRow{}, fmt.Errorf("ожидалось %d полей, найдено %d", len(CompanyHeader), len(rec))
	}
	c := CompanyRow{Row: row, Raw: rec}
	c.ID = strings.TrimSpace(rec[0])
	if c.ID == "" {
		return c, fmt.Errorf("пустой SYNTHETIC_U")
	}
	c.OrgType, c.OkfsName, c.Age, c.EcoStage = Text(rec[1]), Text(rec[2]), Text(rec[3]), Text(rec[4])
	c.OkvedCode, c.OkvedName = strings.TrimSpace(rec[5]), strings.TrimSpace(rec[6])
	if c.OkvedCode == "" {
		return c, fmt.Errorf("пустой MAINOKVED_CODE")
	}
	c.OkvedDivision = division(c.OkvedCode)
	c.WorkersFns, c.WorkersSpark, c.ClientType = Text(rec[8]), Text(rec[9]), Text(rec[10])
	flags := []struct {
		idx int
		dst **bool
	}{{7, &c.Negative}, {11, &c.Rko}, {12, &c.Gar}, {13, &c.Loa}, {14, &c.Lea}, {15, &c.Dep}, {16, &c.Zpp}, {17, &c.Fts}, {19, &c.Factoring}}
	for _, f := range flags {
		v, err := Flag(rec[f.idx])
		if err != nil {
			return c, fmt.Errorf("%s: %w", CompanyHeader[f.idx], err)
		}
		*f.dst = v
	}
	c.Countries = Countries(rec[18])
	rev, err := Number(rec[20])
	if err != nil {
		return c, fmt.Errorf("REVENUE_COMPANY: %w", err)
	}
	c.Revenue = rev
	c.GroupID, c.City, c.Region = Text(rec[21]), Text(rec[22]), Text(rec[23])
	return c, nil
}

// SameContent сообщает, совпадают ли две строки полностью (полные дубликаты
// допустимо схлопнуть; противоречивые — нет).
func SameContent(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if strings.TrimSpace(a[i]) != strings.TrimSpace(b[i]) {
			return false
		}
	}
	return true
}
