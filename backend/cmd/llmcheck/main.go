// llmcheck — безопасная проверка подключения к модели.
//
//	go run ./cmd/llmcheck              — список моделей и лимиты ключа (0 токенов)
//	go run ./cmd/llmcheck -test        — + ОДИН запрос: классификация одной новости
//
// Тестовая новость берётся из реального материала кейса (data/research),
// ничего не выдумывается. Ключ читается из окружения и не печатается.
package main

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"log/slog"
	"os"
	"regexp"
	"strings"
	"time"

	"github.com/alfainsight/backend/internal/analysis"
	"github.com/alfainsight/backend/internal/llm"
	"github.com/alfainsight/backend/internal/platform/logx"
)

func main() {
	test := flag.Bool("test", false, "выполнить один тестовый запрос классификации")
	file := flag.String("file", "../data/research/ALFA-Macro_Insights-Policy_Rate_11Sep.txt", "файл материала для тестовой новости")
	maxTokens := flag.Int("max-tokens", 700, "max_tokens для тестового запроса")
	flag.Parse()

	base := strings.TrimRight(os.Getenv("LLM_BASE_URL"), "/")
	key := os.Getenv("LLM_API_KEY")
	model := os.Getenv("LLM_MODEL_FAST")
	fmt.Printf("LLM_BASE_URL: %s\nLLM_API_KEY: %s\nLLM_MODEL_FAST: %s\n\n", orNone(base), mask(key), orNone(model))
	if base == "" || key == "" {
		fmt.Println("Модель не настроена. Укажите LLM_BASE_URL и LLM_API_KEY в .env (ключ — только в .env, не в коде).")
		os.Exit(2)
	}
	log := logx.New(os.Stderr, slog.LevelWarn)
	guard := llm.NewGuard(llm.GuardConfig{DailyTokenBudget: 20000, DailyCallLimit: 2, PerMinuteLimit: 2}, nil)
	c := llm.NewClient(base, key, 60*time.Second, true, guard, *maxTokens, 6000, log)
	ctx, cancel := context.WithTimeout(context.Background(), 90*time.Second)
	defer cancel()

	fmt.Println("1) Список моделей (GET /models, токены не расходуются)")
	models, err := c.ListModels(ctx)
	if err != nil {
		fmt.Println("   ошибка:", err)
	} else {
		for _, m := range models {
			mark := "  "
			if m.Allowed {
				mark = "✔ "
			}
			fmt.Printf("   %s%s %s\n", mark, m.ID, m.Title)
		}
		fmt.Println("   ✔ — модель из утверждённого списка кейса")
	}

	fmt.Println("\n2) Сведения о лимитах ключа (GET /key/info, если шлюз его поддерживает)")
	if info, err := c.KeyInfo(ctx); err != nil {
		fmt.Println("   ошибка:", err)
	} else if info == nil {
		fmt.Println("   эндпоинт недоступен — лимиты уточняйте у организаторов")
	} else {
		pick := map[string]any{}
		src := info
		if inner, ok := info["info"].(map[string]any); ok {
			src = inner
		}
		for _, k := range []string{"spend", "max_budget", "budget_duration", "budget_reset_at", "tpm_limit", "rpm_limit", "models", "max_parallel_requests"} {
			if v, ok := src[k]; ok {
				pick[k] = v
			}
		}
		b, _ := json.MarshalIndent(pick, "   ", "  ")
		fmt.Println("  ", string(b))
	}

	if !*test {
		fmt.Println("\nТестовый запрос не выполнялся. Запустите с флагом -test, чтобы отправить ОДИН запрос.")
		return
	}
	if _, ok := llm.MatchAllowed(model); !ok {
		fmt.Printf("\nLLM_MODEL_FAST=%q не входит в список разрешённых моделей — запрос не отправлен.\n", model)
		os.Exit(2)
	}
	art, err := testArticle(*file)
	if err != nil {
		fmt.Println("Не удалось прочитать тестовый материал:", err)
		os.Exit(1)
	}
	fmt.Printf("\n3) Один запрос: классификация новости «%s» моделью %s (max_tokens=%d)\n", art.Title, model, *maxTokens)
	topics := []analysis.Topic{{Code: "monetary_policy", Name: "Ключевая ставка и ДКП"}, {Code: "inflation", Name: "Инфляция"},
		{Code: "credit", Name: "Кредитование и финансирование"}, {Code: "debt_market", Name: "Долговой рынок"}, {Code: "macro", Name: "Макроэкономика"},
		{Code: "taxes_budget", Name: "Налоги и бюджет"}, {Code: "fx", Name: "Курс рубля и валютный рынок"}}
	resp, err := c.Chat(ctx, llm.Request{Model: model, Messages: analysis.ClassifyMessages(art, topics),
		Temperature: 0.1, MaxTokens: *maxTokens, JSON: true})
	if err != nil {
		fmt.Println("   ошибка:", err)
		os.Exit(1)
	}
	fmt.Printf("   токены: вход %d, выход %d, всего %d; время %s\n", resp.Usage.PromptTokens,
		resp.Usage.CompletionTokens, resp.Usage.TotalTokens, resp.Duration.Round(time.Millisecond))
	var cl analysis.Classification
	if err := llm.DecodeJSON(resp.Content, &cl); err != nil {
		fmt.Println("   ответ не прошёл разбор:", err)
		fmt.Println(resp.Content)
		os.Exit(1)
	}
	if err := analysis.ValidateClassification(&cl, topics); err != nil {
		fmt.Println("   ответ не прошёл валидацию:", err)
		os.Exit(1)
	}
	b, _ := json.MarshalIndent(cl, "   ", "  ")
	fmt.Println("   результат (после валидации):\n  ", string(b))
}

var pageRe = regexp.MustCompile(`(?m)^=== PAGE.*$`)

// testArticle берёт первую страницу реального материала Альфа-Банка.
func testArticle(path string) (analysis.ArticleInput, error) {
	raw, err := os.ReadFile(path)
	if err != nil {
		return analysis.ArticleInput{}, err
	}
	pages := pageRe.Split(string(raw), -1)
	text := ""
	for _, p := range pages {
		if strings.TrimSpace(p) != "" {
			text = p
			break
		}
	}
	var lines []string
	for _, l := range strings.Split(text, "\n") {
		if l = strings.TrimSpace(l); l != "" {
			lines = append(lines, l)
		}
	}
	title := "Материал Альфа-Банка"
	if len(lines) > 3 {
		// строки: подразделение, дата, рубрика, заголовок
		title = lines[2] + ": " + lines[3]
	}
	body := strings.Join(lines, " ")
	if r := []rune(body); len(r) > 2500 {
		body = string(r[:2500])
	}
	return analysis.ArticleInput{ID: "test", Title: title, Body: body,
		Source:      "Центр макроэкономического анализа Альфа-Банка (материал кейса)",
		PublishedAt: time.Date(2026, 9, 11, 0, 0, 0, 0, time.UTC)}, nil
}

func mask(k string) string {
	if k == "" {
		return "(не задан)"
	}
	return fmt.Sprintf("задан (%d символов, значение скрыто)", len(k))
}

func orNone(s string) string {
	if s == "" {
		return "(не задан)"
	}
	return s
}
