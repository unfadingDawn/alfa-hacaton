package importer

import (
	"strings"
	"testing"

	"github.com/alfainsight/backend/internal/companies"
)

// Строки взяты из реальных файлов кейса (первые записи файлов).
const companiesSample = `SYNTHETIC_U;ORG_TYPE;OKFS_NAME;COMPANY_AGE_YEARS_GRADATION;ECO_MARK_LABEL;MAINOKVED_CODE;MAINOKVED_NAME;NEGATIVEMARK;AMOUNT_WORKERS_FNS_GRADATION;AMOUNT_WORKERS_SPARK_GRADATION;CL_TYPE;CL_PRODUCT_RKO_MARK;CL_PRODUCT_GAR_MARK;CL_PRODUCT_LOA_MARK;CL_PRODUCT_LEA_MARK;CL_PRODUCT_DEP_MARK;CL_PRODUCT_ZPP_MARK;FTS_FLAG;FTS_COUNTRY_NAME;FACTORING_MARK;REVENUE_COMPANY;GROUP_ID;CITY;REGION
U_67;КОММ_ОРГ;ЧАСТНАЯ СОБСТВЕННОСТЬ;16-25;Рождение;46.51;Торговля оптовая компьютерами, периферийными устройствами к компьютерам и программным обеспечением;0.0;Менее 15;;PROSPECT;0.0;0.0;0.0;0.0;0.0;0.0;0.0;;0.0;136493.0;group_134463_sb;г. Новосибирск;Новосибирская область
U_80;КОММ_ОРГ;ЧАСТНАЯ СОБСТВЕННОСТЬ;16-25;Оптимизация;25.61;Обработка металлов и нанесение покрытий на металлы;0.0;16-100;;CLOSE_CLIENT;0.0;0.0;0.0;1.0;0.0;0.0;0.0;;0.0;1423218135.0;group_104611_sb;г. Екатеринбург;Свердловская область
U_1465;КОММ_ОРГ;ЧАСТНАЯ СОБСТВЕННОСТЬ;8-15;Оптимизация;46.34;Торговля оптовая напитками;0.0;Более 251;;CLIENT;1.0;0.0;1.0;1.0;0.0;0.0;1.0;ГРУЗИЯ, АЗЕРБАЙДЖАН, ТУРКМЕНИСТАН;0.0;44401187115.0;group_134460_sb;г. Москва;Москва
`

func TestParseCompaniesFromRealRows(t *testing.T) {
	r := NewReader(strings.NewReader(companiesSample))
	h, err := r.Read()
	if err != nil {
		t.Fatal(err)
	}
	if err := CheckHeader(h, CompanyHeader); err != nil {
		t.Fatal(err)
	}
	var rows []CompanyRow
	for i := 0; i < 3; i++ {
		rec, err := r.Read()
		if err != nil {
			t.Fatal(err)
		}
		c, err := ParseCompany(rec, i+2, companies.Division)
		if err != nil {
			t.Fatalf("строка %d: %v", i+2, err)
		}
		rows = append(rows, c)
	}
	c := rows[2]
	if c.ID != "U_1465" || c.OkvedDivision != "46" || c.Revenue == nil || *c.Revenue != 44401187115 {
		t.Fatalf("неверный разбор: %+v", c)
	}
	if c.Loa == nil || !*c.Loa || c.Dep == nil || *c.Dep {
		t.Fatalf("флаги продуктов разобраны неверно")
	}
	if strings.Join(c.Countries, "|") != "ГРУЗИЯ|АЗЕРБАЙДЖАН|ТУРКМЕНИСТАН" {
		t.Fatalf("страны: %v", c.Countries)
	}
	if rows[0].WorkersSpark != nil || len(rows[0].Countries) != 0 {
		t.Fatalf("пустые значения должны становиться NULL/пустым списком")
	}
	if rows[0].EcoStage == nil || *rows[0].EcoStage != "Рождение" {
		t.Fatalf("eco stage")
	}
}

func TestParseRejectsBadValues(t *testing.T) {
	rec := strings.Split(strings.Split(companiesSample, "\n")[1], ";")
	bad := append([]string{}, rec...)
	bad[13] = "2.0" // флаг кредита должен быть 0/1
	if _, err := ParseCompany(bad, 2, companies.Division); err == nil {
		t.Fatal("ожидалась ошибка для флага 2.0")
	}
	bad = append([]string{}, rec...)
	bad[20] = "много"
	if _, err := ParseCompany(bad, 2, companies.Division); err == nil {
		t.Fatal("ожидалась ошибка для нечисловой выручки")
	}
	if _, err := ParseCompany(rec[:10], 2, companies.Division); err == nil {
		t.Fatal("ожидалась ошибка для неполной строки")
	}
}

func TestCheckHeaderWithBOM(t *testing.T) {
	h := []string{"\ufeffBENEFICIARY", "PAYER", "PAY_AMT", "PAY_CNT"}
	if err := CheckHeader(h, PaymentHeader); err != nil {
		t.Fatal(err)
	}
	if err := CheckHeader([]string{"PAYER", "BENEFICIARY", "PAY_AMT", "PAY_CNT"}, PaymentHeader); err == nil {
		t.Fatal("перепутанные колонки должны давать ошибку")
	}
}

func TestQuotedCityInLinks(t *testing.T) {
	r := NewReader(strings.NewReader("SYNTHETIC_F;CITY_NAME;RC_NAME;SYNTHETIC_U\nF_64255;\"Мск \"\"Юг\"\"\";МОСКОВСКИЙ;U_146717\n"))
	_, _ = r.Read()
	rec, err := r.Read()
	if err != nil || rec[1] != `Мск "Юг"` {
		t.Fatalf("кавычки в CSV: %q %v", rec, err)
	}
}

func TestCountAndSameContent(t *testing.T) {
	if n, err := Count("526.0"); err != nil || n != 526 {
		t.Fatalf("Count: %d %v", n, err)
	}
	if _, err := Count("1.5"); err == nil {
		t.Fatal("дробное количество платежей недопустимо")
	}
	if !SameContent([]string{"a", "b "}, []string{"a", "b"}) || SameContent([]string{"a"}, []string{"b"}) {
		t.Fatal("SameContent")
	}
}

func TestResearchChunking(t *testing.T) {
	text := "=== PAGE 1.txt\nЦентр макроэкономического анализа\n11 сентября 2026 г.\n" +
		"ЦБ РФ принял решение оставить ставку неизменной на уровне 14,0%: ЦБ РФ на сегодняшнем заседании принял решение взять паузу.\n" +
		"F 0,35\nF 0,30 | ~ :: 12.01 09.02\n" +
		"=== PAGE 2.txt\nМы ожидаем сохранения паузы на заседании 23 октября: с учётом риторики ЦБ РФ мы меняем наш взгляд на динамику ставки.\n"
	pages := SplitPages(text)
	if len(pages) != 2 || pages[1].Number != 2 {
		t.Fatalf("страницы: %+v", pages)
	}
	chunks := ChunkPages(pages, 900)
	for _, c := range chunks {
		if strings.Contains(c.Content, "F 0,35") {
			t.Fatal("шум графиков должен отбрасываться")
		}
	}
	if len(chunks) == 0 || chunks[len(chunks)-1].Page != 2 {
		t.Fatalf("фрагменты должны сохранять номер страницы: %+v", chunks)
	}
}
