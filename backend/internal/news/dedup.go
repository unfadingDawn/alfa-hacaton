package news

import (
	"regexp"
	"strings"
)

// Поиск одного события из разных источников. Точные дубликаты (тот же хеш
// или нормализованный заголовок) отсекаются запросом к БД; здесь — близкие
// заголовки разных изданий: «Банк России сохранил ключевую ставку на уровне
// 14%» и «ЦБ сохранил ключевую ставку 14% годовых».
//
// Похожие, но разные события не склеиваются: противоположное направление
// («повысил»/«снизил»/«сохранил»), разные числа («50 млрд»/«30 млрд») или
// разные участники («Сбербанк»/«ВТБ») дают несовпадение.

// Синонимы приводятся к одному токену до разбиения на слова.
// В RE2 \b работает только для ASCII, поэтому границы слов — пробелы.
var dedupSynonyms = []struct {
	re *regexp.Regexp
	to string
}{
	{synRe(`банк россии|центробанк\pL*|центральн\pL* банк\pL*|цб рф|цбр`), "цб"},
	{synRe(`министерство финансов|минфин\pL*`), "минфин"},
	{synRe(`правительство рф|кабмин\pL*|правительств\pL*`), "правительство"},
	{synRe(`мосбирж\pL*|московск\pL* бирж\pL*`), "мосбиржа"},
}

func synRe(alt string) *regexp.Regexp { return regexp.MustCompile(`(^| )(?:` + alt + `)( |$)`) }

var (
	decimalCommaRe = regexp.MustCompile(`(\d),(\d)`)
	nonTokenRe     = regexp.MustCompile(`[^\p{L}\p{N}.]+`)
)

var dedupStop = map[string]bool{
	"на": true, "в": true, "во": true, "и": true, "по": true, "до": true, "с": true, "со": true, "за": true, "от": true,
	"о": true, "об": true, "к": true, "у": true, "из": true, "не": true, "что": true, "это": true, "для": true, "при": true,
	"года": true, "году": true, "год": true, "г": true, "рф": true, "россии": true, "годовых": true, "уровне": true,
	"млрд": true, "млн": true, "трлн": true, "руб": true, "рублей": true, "рубля": true, "процента": true, "процентов": true,
	"как": true, "его": true, "ее": true, "их": true, "а": true, "но": true, "же": true,
}

// Основы глаголов направления (первые 5 букв слова).
var dedupDirection = map[string]string{
	"повыс": "up", "вырос": "up", "рост": "up", "увели": "up", "подор": "up", "подня": "up", "ускор": "up",
	"снизи": "down", "сниже": "down", "упал": "down", "упали": "down", "сокра": "down", "подеш": "down", "пониз": "down",
	"обвал": "down", "замед": "down", "сохра": "hold", "остав": "hold", "неизм": "hold",
}

var numberTokenRe = regexp.MustCompile(`^\d+(\.\d+)?$`)

func dedupTokens(title string) (words map[string]bool, numbers map[string]bool, dirs map[string]bool) {
	s := strings.ReplaceAll(strings.ToLower(title), "ё", "е")
	// Десятичная запятая не должна разрывать число: «14,5%» → «14.5».
	s = decimalCommaRe.ReplaceAllString(s, "$1.$2")
	s = " " + nonTokenRe.ReplaceAllString(s, " ") + " "
	for _, syn := range dedupSynonyms {
		// Дважды: соседние совпадения делят общий пробел.
		for i := 0; i < 2; i++ {
			s = syn.re.ReplaceAllString(s, "${1}"+syn.to+"${2}")
		}
	}
	words, numbers, dirs = map[string]bool{}, map[string]bool{}, map[string]bool{}
	for _, w := range strings.Fields(s) {
		w = strings.Trim(w, ".")
		if w == "" || dedupStop[w] {
			continue
		}
		if numberTokenRe.MatchString(w) {
			if strings.Contains(w, ".") {
				w = strings.TrimSuffix(strings.TrimRight(w, "0"), ".") // «14.00» и «14» — одно число
			}
			numbers[w] = true
			words["#"+w] = true
			continue
		}
		stem := w
		if r := []rune(w); len(r) > 5 {
			stem = string(r[:5])
		}
		if d, ok := dedupDirection[stem]; ok {
			dirs[d] = true
		}
		words[stem] = true
	}
	return words, numbers, dirs
}

// SameEvent сообщает, описывают ли два заголовка одно событие.
func SameEvent(titleA, titleB string) bool {
	if NormTitle(titleA) == NormTitle(titleB) {
		return true
	}
	a, na, da := dedupTokens(titleA)
	b, nb, db := dedupTokens(titleB)
	if len(a) < 3 || len(b) < 3 {
		return false
	}
	// Противоположное направление события: «повысил» и «снизил».
	if len(da) > 0 && len(db) > 0 {
		shared := false
		for d := range da {
			if db[d] {
				shared = true
			}
		}
		if !shared {
			return false
		}
	}
	// Числа меньшего заголовка должны встречаться в большем.
	if len(na) > 0 && len(nb) > 0 {
		small, big := na, nb
		if len(nb) < len(na) {
			small, big = nb, na
		}
		for n := range small {
			if !big[n] {
				return false
			}
		}
	}
	inter := 0
	for w := range a {
		if b[w] {
			inter++
		}
	}
	minLen, union := len(a), len(a)+len(b)-inter
	if len(b) < minLen {
		minLen = len(b)
	}
	overlap := float64(inter) / float64(minLen)
	jaccard := float64(inter) / float64(union)
	return overlap >= 0.8 && jaccard >= 0.5
}
