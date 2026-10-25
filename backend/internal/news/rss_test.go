package news

import (
	"testing"

	"golang.org/x/text/encoding/charmap"
)

// Структура повторяет реальные ленты (BOM у cbr.ru, CDATA у tass.ru).
const rssSample = "\xef\xbb\xbf<?xml version=\"1.0\" encoding=\"utf-8\"?>\n" + `<rss version="2.0"><channel><title>ТАСС</title>
<item><title>Минфин &amp; рынок</title>
<link><![CDATA[https://tass.ru/ekonomika/1]]></link>
<guid><![CDATA[https://tass.ru/ekonomika/1]]></guid>
<pubDate>Wed, 21 Oct 2026 20:17:51 +0300</pubDate>
<description>Текст &lt;b&gt;анонса&lt;/b&gt;&amp;nbsp;новости</description>
<category>Экономика и бизнес</category></item>
<item><title>Спорт</title><link>https://tass.ru/sport/2</link><description><![CDATA[<p>Матч &laquo;завершился&raquo;</p>]]></description><pubDate>Wed, 21 Oct 2026 20:00:00 +0300</pubDate><category>Спорт</category></item>
<item><title>Без даты</title><link>https://tass.ru/x/3</link></item>
</channel></rss>`

func TestParseRSS(t *testing.T) {
	items, err := Parse([]byte(rssSample))
	if err != nil {
		t.Fatal(err)
	}
	if len(items) != 2 {
		t.Fatalf("ожидалось 2 записи (без даты — отбрасывается), получено %d", len(items))
	}
	it := items[0]
	if it.Title != "Минфин & рынок" || it.Link != "https://tass.ru/ekonomika/1" || it.Description != "Текст анонса новости" {
		t.Fatalf("неверный разбор: %+v", it)
	}
	if items[1].Description != "Матч «завершился»" {
		t.Fatalf("CDATA с HTML: %q", items[1].Description)
	}
	if !MatchesCategories(it, []string{"Экономика и бизнес"}) || MatchesCategories(items[1], []string{"Экономика и бизнес"}) {
		t.Fatal("фильтр рубрик")
	}
	if !MatchesCategories(items[1], nil) {
		t.Fatal("пустой фильтр пропускает всё")
	}
}

func TestParseWindows1251(t *testing.T) {
	xml := `<?xml version="1.0" encoding="windows-1251"?><rss><channel><item><title>Курс рубля</title>` +
		`<link>https://example.org/1</link><pubDate>Mon, 19 Oct 2026 10:00:00 +0300</pubDate></item></channel></rss>`
	enc, _ := charmap.Windows1251.NewEncoder().String(xml)
	items, err := Parse([]byte(enc))
	if err != nil || len(items) != 1 || items[0].Title != "Курс рубля" {
		t.Fatalf("windows-1251: %+v %v", items, err)
	}
}

func TestNestedCategoryFilter(t *testing.T) {
	item := Item{Categories: []string{"Экономика / Налоги и сборы"}}
	if !MatchesCategories(item, []string{"Экономика"}) {
		t.Fatal("родительская рубрика должна принимать вложенную")
	}
	if MatchesCategories(item, []string{"Эконом"}) || MatchesCategories(item, []string{"Бизнес"}) {
		t.Fatal("частичное имя или другая рубрика не должны совпадать")
	}
	if !MatchesCategories(Item{Categories: []string{"  БИЗНЕС/Транспорт  "}}, []string{"бизнес"}) {
		t.Fatal("сравнение должно игнорировать регистр и пробелы вокруг разделителя")
	}
}

func TestParseAtom(t *testing.T) {
	atom := `<feed xmlns="http://www.w3.org/2005/Atom"><entry><title>Решение по ставке</title>` +
		`<link rel="alternate" href="https://example.org/a"/><updated>2026-10-21T10:00:00Z</updated><summary>Анонс</summary></entry></feed>`
	items, err := Parse([]byte(atom))
	if err != nil || len(items) != 1 || items[0].Link != "https://example.org/a" {
		t.Fatalf("atom: %+v %v", items, err)
	}
}

func TestNormalizationForDuplicates(t *testing.T) {
	a := NormTitle("ЦБ сохранил ключевую ставку — 14%!")
	b := NormTitle("цб  сохранил ключевую ставку 14 %")
	if a != b {
		t.Fatalf("нормализация: %q != %q", a, b)
	}
	if NormTitle("Ёлка") != "елка" {
		t.Fatal("ё → е")
	}
	if ContentHash("Заголовок", "текст") == ContentHash("Заголовок", "другой текст") {
		t.Fatal("разные тексты должны давать разный хеш")
	}
	if CleanText("<p>a&amp;b</p>\n\n c") != "a&b c" {
		t.Fatalf("CleanText: %q", CleanText("<p>a&amp;b</p>\n\n c"))
	}
}

func TestParseFullText(t *testing.T) {
	feed := `<?xml version="1.0" encoding="utf-8"?><rss version="2.0" xmlns:content="http://purl.org/rss/1.0/modules/content/" xmlns:yandex="http://news.yandex.ru">
<channel><item><title>Пресс-релиз</title><link>https://cbr.ru/1</link><pubDate>Wed, 21 Oct 2026 13:30:00 +0300</pubDate>
<description>Короткий анонс</description><content:encoded><![CDATA[<p>Полный <b>текст</b> пресс-релиза</p>]]></content:encoded></item>
<item><title>Новость</title><link>https://example.ru/2</link><pubDate>Wed, 21 Oct 2026 13:30:00 +0300</pubDate>
<description>Анонс</description><yandex:full-text>Полный текст для Яндекса</yandex:full-text></item>
<item><title>Без текста</title><link>https://example.ru/3</link><pubDate>Wed, 21 Oct 2026 13:30:00 +0300</pubDate><description>Анонс</description></item>
</channel></rss>`
	items, err := Parse([]byte(feed))
	if err != nil {
		t.Fatal(err)
	}
	if items[0].FullText != "Полный текст пресс-релиза" || items[1].FullText != "Полный текст для Яндекса" || items[2].FullText != "" {
		t.Fatalf("полный текст из ленты: %q / %q / %q", items[0].FullText, items[1].FullText, items[2].FullText)
	}
}
