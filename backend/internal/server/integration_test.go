package server

// Интеграционный тест: настоящая PostgreSQL + заглушка OpenAI-совместимого API.
// Реальная модель не вызывается. Запуск:
//   TEST_DATABASE_URL=postgres://alfa:alfa@localhost:5432/alfainsight_test?sslmode=disable go test ./internal/server/
// ВНИМАНИЕ: тест пересоздаёт схему public в указанной БД.

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"net/http/cookiejar"
	"net/http/httptest"
	"os"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/alfainsight/backend/internal/db/dbq"
	"github.com/alfainsight/backend/internal/news"
	"github.com/alfainsight/backend/internal/platform/config"
	"github.com/alfainsight/backend/internal/platform/db"
	"github.com/alfainsight/backend/migrations"
)

type mockLLM struct {
	calls     atomic.Int32
	fail429   atomic.Bool
	mu        sync.Mutex
	roleSeen  []string
	modelSeen []string
	questions []string
}

func (m *mockLLM) handler(w http.ResponseWriter, r *http.Request) {
	m.calls.Add(1)
	if m.fail429.Load() {
		w.Header().Set("Retry-After", "60")
		w.WriteHeader(http.StatusTooManyRequests)
		_, _ = w.Write([]byte(`{"error":"rate limit"}`))
		return
	}
	var req struct {
		Model    string                     `json:"model"`
		Messages []struct{ Content string } `json:"messages"`
	}
	_ = json.NewDecoder(r.Body).Decode(&req)
	user := req.Messages[len(req.Messages)-1].Content
	m.mu.Lock()
	m.modelSeen = append(m.modelSeen, req.Model)
	m.mu.Unlock()
	var content string
	switch {
	case strings.Contains(user, "ВОПРОС ПОЛЬЗОВАТЕЛЯ"):
		m.mu.Lock()
		m.questions = append(m.questions, user)
		m.mu.Unlock()
		content = `{"answer":"Оценка высокая: у компании есть кредиты, а ставка не снижается.","points":[
		{"text":"У компании есть кредитные продукты","source_ids":["C:has_loan"],"basis":"company_data"},
		{"text":"Банк ожидает снижения ставки","source_ids":["R999999"],"basis":"bank_view"}],
		"not_in_sources":"","out_of_scope":false}`
	case strings.Contains(user, "Типы событий"):
		content = `{"topics":[{"code":"monetary_policy","confidence":0.9}],"event_type":"monetary_policy_decision",
		"summary":"Банк России сохранил ставку.","key_facts":[{"text":"Ставка 14%","confirmed":true}],"dates":[],"entities":[],
		"indicators":[],"okved_divisions":[],"regions":[],"countries":[],"commodities":[],"is_business_relevant":true}`
	case strings.Contains(user, "Составь план"):
		content = `{"focus_questions":["Как изменится стоимость долга?"],"research_queries":["ключевая ставка пауза"],
		"news_queries":[],"company_fact_ids":["C:has_loan"],"hypotheses":["Дорогие кредиты"]}`
	case strings.Contains(user, "ЧЕРНОВИК"):
		content = `{"passed":true,"issues":[],"additional_research_queries":[],"comment":"ok"}`
	case strings.Contains(user, "РЕЕСТР ИСТОЧНИКОВ"):
		role := strings.SplitN(strings.TrimPrefix(user, "Сотрудник: "), ".", 2)[0]
		m.mu.Lock()
		m.roleSeen = append(m.roleSeen, role)
		m.mu.Unlock()
		rid := ""
		for _, f := range strings.Fields(user) {
			if strings.HasPrefix(f, "[R") {
				rid = strings.Trim(f, "[]")
				break
			}
		}
		content = fmt.Sprintf(`{"relevance_score":82,"importance_level":"high","impact_type":"risk","matched_topics":["monetary_policy"],
		"what_happened":{"summary":"ЦБ сохранил ставку 14%%.","facts":[{"text":"Ставка сохранена","source_ids":["A1"]}]},
		"why_it_matters":"У компании есть кредитные продукты (%s)","why_it_matters_source_ids":["C:has_loan"],
		"potential_impact":"Стоимость долга останется высокой",
		"risks":[{"title":"Дорогой долг","description":"Нет снижения ставок","likelihood":"high","horizon":"short","basis":"bank_view","source_ids":["%s"]}],
		"opportunities":[],"recommended_actions":[{"action":"Пересмотреть график рефинансирования","rationale":"","owner":"CFO","source_ids":[]}],
		"bank_view":[{"text":"Аналитики ждут паузы","source_ids":["%s","R999999"]}],
		"assumptions":[],"uncertainty_notes":[]}`, role, rid, rid)
	case strings.Contains(user, "Новости:"):
		content = `{"items":[{"article_id":"a1","relevance_score":70,"importance_level":"high","impact_type":"risk","matched_topics":["monetary_policy"],"why_it_matters":"Влияет на стоимость долга"}]}`
	default:
		content = `{}`
	}
	b, _ := json.Marshal(map[string]any{
		"choices": []map[string]any{{"message": map[string]string{"content": content}, "finish_reason": "stop"}},
		"usage":   map[string]int{"prompt_tokens": 1000, "completion_tokens": 200, "total_tokens": 1200},
	})
	_, _ = w.Write(b)
}

type client struct {
	t    *testing.T
	base string
	http *http.Client
}

func newClient(t *testing.T, base string) *client {
	jar, _ := cookiejar.New(nil)
	return &client{t: t, base: base, http: &http.Client{Jar: jar, Timeout: 60 * time.Second}}
}

func (c *client) do(method, path string, body any) (int, map[string]any) {
	c.t.Helper()
	var rd io.Reader
	if body != nil {
		b, _ := json.Marshal(body)
		rd = strings.NewReader(string(b))
	}
	req, _ := http.NewRequest(method, c.base+path, rd)
	req.Header.Set("Content-Type", "application/json")
	resp, err := c.http.Do(req)
	if err != nil {
		c.t.Fatal(err)
	}
	defer resp.Body.Close()
	var out map[string]any
	_ = json.NewDecoder(resp.Body).Decode(&out)
	return resp.StatusCode, out
}

func (c *client) must(method, path string, body any, want int) map[string]any {
	c.t.Helper()
	code, out := c.do(method, path, body)
	if code != want {
		c.t.Fatalf("%s %s: код %d, ожидался %d: %v", method, path, code, want, out)
	}
	return out
}

func onboard(c *client, email, role, company string) {
	c.must("POST", "/api/auth/register", map[string]any{"email": email, "password": "Demo12345", "full_name": "Тест " + role}, 201)
	c.must("PUT", "/api/me/profile", map[string]any{"full_name": "Тест " + role, "role_code": role}, 200)
	// Без обязательных полей компании — ошибка валидации.
	c.must("POST", "/api/me/companies", map[string]any{"company_id": company}, 422)
	c.must("POST", "/api/me/companies", map[string]any{"company_id": company, "display_name": "ООО «Тест»",
		"profile": map[string]any{"products": "напитки", "markets": "розничные сети", "suppliers": "производители",
			"business": map[string]any{"source": "alfa_business", "currencies": []string{"cny", "RUB"}, "accounts_fx": 1,
				"overdraft_available": true, "applications": []map[string]any{{"product": "leasing", "amount": 75000, "status": "на подпись"}}}}}, 200)
	c.must("PUT", "/api/me/interests", map[string]any{"topics": []map[string]any{{"code": "monetary_policy", "weight": 3}}}, 200)
	c.must("POST", "/api/me/onboarding/complete", nil, 200)
}

func TestEndToEnd(t *testing.T) {
	dsn := os.Getenv("TEST_DATABASE_URL")
	if dsn == "" {
		t.Skip("TEST_DATABASE_URL не задан — интеграционный тест пропущен")
	}
	ctx := context.Background()
	pool, err := db.Connect(ctx, dsn)
	if err != nil {
		t.Fatal(err)
	}
	defer pool.Close()
	if _, err := pool.Exec(ctx, `DROP SCHEMA public CASCADE; CREATE SCHEMA public;`); err != nil {
		t.Fatal(err)
	}
	if err := db.Migrate(pool, migrations.FS); err != nil {
		t.Fatal(err)
	}
	// Тестовые записи (не данные кейса): две компании, материал банка, новость.
	fixture := `
	INSERT INTO companies (id, okved_code, okved_name, okved_division, has_loan, revenue, region, city, source_row)
	VALUES ('T_1','46.34','Торговля оптовая напитками','46',true,44401187115,'Москва','г. Москва',1),
	       ('T_2','25.61','Обработка металлов','25',false,1423218135,'Свердловская область','г. Екатеринбург',2);
	INSERT INTO payments (payer, beneficiary, amount, pay_count, is_internal, source_row) VALUES ('T_1','T_2',1000000,3,false,1);
	INSERT INTO research_documents (file_name,title,published_on) VALUES ('test.pdf','Ключевая ставка ЦБ РФ','2026-09-11');
	INSERT INTO research_chunks (document_id,chunk_no,page,content)
	  SELECT id,0,1,'Мы ожидаем сохранения паузы по ключевой ставке на уровне 14% до конца года.' FROM research_documents;
	INSERT INTO articles (source_id,url,title,summary,published_at,title_norm,content_hash)
	  SELECT id,'https://example.test/1','Банк России сохранил ключевую ставку','Совет директоров сохранил ставку 14%.',now(),'t1','h1'
	  FROM sources WHERE code='cbr_press';`
	if _, err := pool.Exec(ctx, fixture); err != nil {
		t.Fatal(err)
	}

	mock := &mockLLM{}
	llmSrv := httptest.NewServer(http.HandlerFunc(mock.handler))
	defer llmSrv.Close()
	cfg := config.Config{JWTSecret: []byte(strings.Repeat("s", 40)), AccessTTL: 15 * time.Minute, RefreshTTL: time.Hour,
		AllowedOrigins: []string{"http://localhost:3000"}, Timezone: time.UTC, NewsInitialDays: 7,
		LLM: config.LLM{BaseURL: llmSrv.URL, APIKey: "sk-test", ModelFast: "google/gemma-4-26b-a4b-it",
			ModelReasoning: "z-ai/glm-5.3-flash", ModelWriter: "qwen/qwen3.6-35b-a3b", Timeout: 10 * time.Second,
			DailyTokenBudget: 1_000_000, DailyCallLimit: 100, PerMinuteLimit: 100, MaxOutputTokens: 2200, MaxInputChars: 24000,
			PerUserHourly: 10, QuestionsPerAnalysis: 2, QuestionsPerUserDaily: 30, JSONMode: true}}
	app := New(cfg, pool, slog.New(slog.NewTextHandler(io.Discard, nil)))
	if _, err := app.Agent.TagKeywordTopics(ctx); err != nil {
		t.Fatal(err)
	}
	srv := httptest.NewServer(app.Router)
	defer srv.Close()

	cfo, proc, other := newClient(t, srv.URL), newClient(t, srv.URL), newClient(t, srv.URL)
	if code, _ := cfo.do("GET", "/api/me", nil); code != 401 {
		t.Fatalf("без входа /me должен давать 401, получено %d", code)
	}
	onboard(cfo, "cfo@test.ru", "cfo", "T_1")
	onboard(proc, "proc@test.ru", "procurement", "T_1")
	onboard(other, "other@test.ru", "ceo", "T_2")

	// Повторная регистрация того же email.
	if code, out := newClient(t, srv.URL).do("POST", "/api/auth/register",
		map[string]any{"email": "CFO@test.ru", "password": "Demo12345", "full_name": "Дубль"}); code != 422 {
		t.Fatalf("дубль email: %d %v", code, out)
	}

	// Лента без LLM: локальная предварительная оценка.
	feed := cfo.must("GET", "/api/feed?sort=relevance", nil, 200)
	items := feed["items"].([]any)
	var articleID string
	for _, it := range items {
		m := it.(map[string]any)
		if strings.Contains(m["title"].(string), "ключевую ставку") {
			articleID = m["id"].(string)
			if m["analysis"].(map[string]any)["method"] != "rules" {
				t.Fatal("до анализа моделью оценка должна быть помечена как rules")
			}
		}
	}
	if articleID == "" {
		t.Fatalf("тестовая новость не найдена в ленте: %v", items)
	}
	if mock.calls.Load() != 0 {
		t.Fatal("лента не должна вызывать модель")
	}

	// Фильтры ленты.
	if f := cfo.must("GET", "/api/feed?topic=monetary_policy", nil, 200); f["total"].(float64) < 1 {
		t.Fatal("фильтр по теме")
	}
	if f := cfo.must("GET", "/api/feed?q=металлургия", nil, 200); f["total"].(float64) != 0 {
		t.Fatal("поиск должен отфильтровать нерелевантное")
	}

	// Полный анализ агентом.
	res := cfo.must("POST", "/api/articles/"+articleID+"/analyze", nil, 200)
	if res["cached"].(bool) {
		t.Fatal("первый анализ не может быть из кэша")
	}
	callsAfterFirst := mock.calls.Load()
	if callsAfterFirst != 4 {
		t.Fatalf("ожидалось 4 запроса (классификация, план, черновик, проверка), было %d", callsAfterFirst)
	}
	insight := res["analysis"].(map[string]any)["result"].(map[string]any)["insight"].(map[string]any)
	ev := insight["evidence_sources"].([]any)
	var ids []string
	for _, e := range ev {
		ids = append(ids, e.(map[string]any)["id"].(string))
	}
	joined := strings.Join(ids, ",")
	if !strings.Contains(joined, "A1") || !strings.Contains(joined, "C:has_loan") || strings.Contains(joined, "R999999") ||
		!strings.Contains(joined, "R") {
		t.Fatalf("источники: %v", ids)
	}
	if len(insight["bank_view"].([]any)) != 1 {
		t.Fatal("позиция банка должна опираться на материал банка")
	}
	steps := res["run"].(map[string]any)["steps"].([]any)
	var names []string
	for _, s := range steps {
		names = append(names, s.(map[string]any)["name"].(string))
	}
	if strings.Join(names, ",") != "classify,company_profile,plan,search_research,draft,fact_check_draft,self_check" {
		t.Fatalf("трасса агента: %v", names)
	}
	runID := res["run"].(map[string]any)["id"].(string)

	// Повторный запрос — из кэша, без обращения к модели.
	if res2 := cfo.must("POST", "/api/articles/"+articleID+"/analyze", nil, 200); !res2["cached"].(bool) || mock.calls.Load() != callsAfterFirst {
		t.Fatal("повторный анализ должен браться из кэша")
	}

	// Та же новость для руководителя закупок: другая линза, классификация не повторяется.
	proc.must("POST", "/api/articles/"+articleID+"/analyze", nil, 200)
	var before int32
	if got := mock.calls.Load() - callsAfterFirst; got != 3 {
		t.Fatalf("для второй роли ожидалось 3 запроса (без повторной классификации), было %d", got)
	}
	mock.mu.Lock()
	roles := strings.Join(mock.roleSeen, "|")
	models := strings.Join(mock.modelSeen, "|")
	mock.mu.Unlock()
	if !strings.Contains(roles, "Финансовый директор") || !strings.Contains(roles, "Руководитель закупок") {
		t.Fatalf("промпты должны отличаться ролью: %s", roles)
	}
	for _, m := range strings.Split(models, "|") {
		if m != "google/gemma-4-26b-a4b-it" && m != "z-ai/glm-5.3-flash" && m != "qwen/qwen3.6-35b-a3b" {
			t.Fatalf("использована модель вне настроек: %s", m)
		}
	}

	// Уточняющие вопросы к готовому анализу.
	qpath := "/api/articles/" + articleID + "/analysis/questions"
	if code, out := other.do("POST", qpath, map[string]any{"question": "Почему такая оценка?"}); code != 409 ||
		out["error"].(map[string]any)["code"] != "no_analysis" {
		t.Fatalf("вопрос без своего анализа: %d %v", code, out)
	}
	other.must("GET", qpath, nil, 409)
	hist := cfo.must("GET", qpath, nil, 200)
	if len(hist["items"].([]any)) != 0 || len(hist["quick_questions"].([]any)) != 4 {
		t.Fatalf("пустая история и быстрые вопросы: %v", hist)
	}
	analysisID := hist["analysis_id"].(string)
	before = mock.calls.Load()
	det := cfo.must("POST", qpath, map[string]any{"preset": "company_factors", "analysis_id": analysisID}, 200)["item"].(map[string]any)
	if det["method"] != "deterministic" || mock.calls.Load() != before ||
		!strings.Contains(fmt.Sprint(det["source_ids"]), "C:has_loan") {
		t.Fatalf("признаки компании отвечаются без модели и со ссылкой на факт: %v", det)
	}
	ask := cfo.must("POST", qpath, map[string]any{"question": "Почему такая оценка?"}, 200)
	item := ask["item"].(map[string]any)
	if ask["cached"].(bool) || item["method"] != "llm" || mock.calls.Load() != before+1 || item["preset"] != "why_score" {
		t.Fatalf("вопрос к модели: %v", ask)
	}
	ans := item["answer"].(map[string]any)
	pts := ans["points"].([]any)
	if src := fmt.Sprint(item["source_ids"]); strings.Contains(src, "R999999") || !strings.Contains(src, "C:has_loan") ||
		pts[1].(map[string]any)["basis"] != "model_inference" || len(ans["sources"].([]any)) != 1 {
		t.Fatalf("ответ должен опираться только на реестр анализа: %v", ans)
	}
	mock.mu.Lock()
	qprompt := mock.questions[len(mock.questions)-1]
	mock.mu.Unlock()
	if !strings.Contains(qprompt, "Финансовый директор") || !strings.Contains(qprompt, "[C:has_loan]") {
		t.Fatal("в промпт вопроса должны попасть роль и реестр источников анализа")
	}
	if c := cfo.must("POST", qpath, map[string]any{"question": "  почему ТАКАЯ оценка "}, 200); !c["cached"].(bool) || mock.calls.Load() != before+1 {
		t.Fatalf("одинаковый вопрос должен браться из кэша: %v", c)
	}
	cfo.must("POST", qpath, map[string]any{"question": "аб"}, 422)
	cfo.must("POST", qpath, map[string]any{"preset": "unknown"}, 422)
	if code, out := cfo.do("POST", qpath, map[string]any{"question": "Что дальше?", "analysis_id": "00000000-0000-0000-0000-000000000001"}); code != 409 ||
		out["error"].(map[string]any)["code"] != "stale_analysis" {
		t.Fatalf("вопрос к устаревшему анализу: %d %v", code, out)
	}
	cfo.must("POST", qpath, map[string]any{"preset": "check_first"}, 200)
	if code, out := cfo.do("POST", qpath, map[string]any{"question": "Как это повлияет на ковенанты?"}); code != 429 ||
		out["error"].(map[string]any)["code"] != "question_limit" || mock.calls.Load() != before+2 {
		t.Fatalf("лимит вопросов на анализ: %d %v", code, out)
	}
	hist = cfo.must("GET", qpath, nil, 200)
	lim := hist["limits"].(map[string]any)
	if len(hist["items"].([]any)) != 3 || lim["used_for_analysis"].(float64) != 2 || lim["per_analysis"].(float64) != 2 {
		t.Fatalf("история вопросов и лимиты: %v", hist)
	}
	qall := cfo.must("GET", "/api/analytics/quality?period=30d", nil, 200)
	if acc := qall["accuracy"].(map[string]any); acc["analyses_checked"].(float64) != 1 || acc["avg_source_coverage"] == nil ||
		acc["content_levels"].(map[string]any)["title_only"].(float64) != 1 {
		t.Fatalf("метрики фактологической точности: %v", acc)
	}
	qq := qall["questions"].(map[string]any)
	if qq["total"].(float64) != 3 || qq["llm_calls"].(float64) != 2 || qq["deterministic"].(float64) != 1 ||
		qq["cache_hits"].(float64) != 1 || qq["grounded_rate"].(float64) != 100 {
		t.Fatalf("метрики вопросов: %v", qq)
	}

	// Сохранение и отзыв.
	cfo.must("POST", "/api/articles/"+articleID+"/save", nil, 200)
	if f := cfo.must("GET", "/api/feed?saved=true", nil, 200); f["total"].(float64) != 1 {
		t.Fatal("сохранённые")
	}
	cfo.must("POST", "/api/articles/"+articleID+"/feedback", map[string]any{"rating": 1}, 200)
	quality := cfo.must("GET", "/api/analytics/quality?period=30d", nil, 200)
	if quality["runs"].(map[string]any)["total"].(float64) < 1 ||
		quality["feedback"].(map[string]any)["helpfulness_rate"].(float64) != 100 {
		t.Fatalf("метрики качества: %v", quality)
	}
	cfo.must("DELETE", "/api/articles/"+articleID+"/save", nil, 200)

	// Разграничение доступа.
	other.must("GET", "/api/companies/T_1", nil, 403)
	other.must("PUT", "/api/companies/T_1", map[string]any{"display_name": "Взлом"}, 403)
	other.must("GET", "/api/runs/"+runID, nil, 404)
	prof := cfo.must("GET", "/api/companies/T_1", nil, 200)
	b := prof["user_profile"].(map[string]any)["business"].(map[string]any)
	if b["source"] != "user" || len(b["currencies"].([]any)) != 1 {
		t.Fatalf("сведения кабинета: источник задаёт сервер, RUB отбрасывается: %v", b)
	}
	var codes []string
	for _, e := range prof["exposures"].([]any) {
		codes = append(codes, e.(map[string]any)["code"].(string))
	}
	if !strings.Contains(strings.Join(codes, ","), "fx_accounts") || !strings.Contains(strings.Join(codes, ","), "pending_financing") {
		t.Fatalf("сведения кабинета должны давать чувствительности: %v", codes)
	}
	// Анализ коллеги не виден другому пользователю: у other нет полного анализа.
	if a := other.must("GET", "/api/articles/"+articleID, nil, 200); a["analysis"] != nil {
		t.Fatal("анализ другого пользователя не должен быть виден")
	}
	// Новость, добавленная вручную, видна только автору.
	sub := cfo.must("POST", "/api/articles", map[string]any{"title": "Моя тестовая новость",
		"text": "Текст тестовой новости, добавленной пользователем вручную для проверки анализа."}, 201)
	other.must("GET", "/api/articles/"+sub["id"].(string), nil, 404)

	// Ошибка лимита провайдера: понятный ответ и пауза без повторов.
	mock.fail429.Store(true)
	before = mock.calls.Load()
	if code, out := other.do("POST", "/api/articles/"+articleID+"/analyze", nil); code != 429 ||
		out["error"].(map[string]any)["code"] != "llm_rate_limited" {
		t.Fatalf("429 от провайдера: %d %v", code, out)
	}
	if code, _ := other.do("POST", "/api/articles/"+articleID+"/analyze", nil); code != 429 || mock.calls.Load() != before+1 {
		t.Fatalf("после 429 запросы к модели должны быть приостановлены: calls=%d", mock.calls.Load()-before)
	}

	// Аналитика и главная работают на реальных записях.
	an := cfo.must("GET", "/api/analytics?period=30d", nil, 200)
	if an["sufficient"].(map[string]any)["llm_analyses"] != true {
		t.Fatal("после анализа должны быть данные для аналитики")
	}
	kpi := an["kpi"].(map[string]any)
	if len(an["monthly"].([]any)) == 0 || kpi["balance"].(map[string]any)["rated"].(float64) == 0 {
		t.Fatalf("помесячный срез и KPI должны учитывать оценки модели: %v", kpi)
	}

	// Лендинг: реальный пример без авторизации, без имени пользователя и названия компании.
	sc := newClient(t, srv.URL).must("GET", "/api/public/showcase", nil, 200)
	ex, ok := sc["example"].(map[string]any)
	if !ok || len(ex["lenses"].([]any)) == 0 {
		t.Fatalf("на лендинге должен быть реальный разбор модели: %v", sc)
	}
	raw, _ := json.Marshal(sc)
	if strings.Contains(string(raw), "@") {
		t.Fatal("в публичной витрине не должно быть email пользователей")
	}

	dash := cfo.must("GET", "/api/dashboard", nil, 200)
	if len(dash["risks"].([]any)) == 0 {
		t.Fatal("риски из полного анализа должны попасть на главную")
	}

	// Полнота источника и покрытие источниками в полном анализе.
	if insight["source_completeness"].(map[string]any)["level"] != "title_only" ||
		insight["source_coverage"].(map[string]any)["total"].(float64) == 0 || insight["fact_check"] == nil {
		t.Fatalf("полнота источника, покрытие и проверка фактов: %v", insight)
	}

	testIngestion(t, ctx, pool, app)

	// Выход.
	cfo.must("POST", "/api/auth/logout", nil, 200)
	if code, _ := cfo.do("GET", "/api/me", nil); code != 401 {
		t.Fatal("после выхода доступ должен закрываться")
	}
}

// testIngestion — дедупликация, обновление публикаций и полнота текста на настоящей БД.
func testIngestion(t *testing.T, ctx context.Context, pool *pgxpool.Pool, app *App) {
	q := dbq.New(pool)
	src := func(code string) dbq.Source {
		s, err := q.GetSourceByCode(ctx, code)
		if err != nil {
			t.Fatal(err)
		}
		return s
	}
	tass, interfax, cbr := src("tass"), src("interfax"), src("cbr_press")
	now := time.Now()
	store := func(s dbq.Source, title, desc, link, full string) news.StoreResult {
		t.Helper()
		r, err := app.Ingestor.Store(ctx, s, news.Item{Title: title, Description: desc, Link: link, FullText: full, Published: now})
		if err != nil {
			t.Fatal(err)
		}
		return r
	}
	level := func(id any) (string, string) {
		var lvl, body string
		if err := pool.QueryRow(ctx, `SELECT content_level, body FROM articles WHERE id = $1`, id).Scan(&lvl, &body); err != nil {
			t.Fatal(err)
		}
		return lvl, body
	}

	// Одна новость из нескольких источников: близкий заголовок другого издания.
	r := store(tass, "Банк России сохранил ключевую ставку на уровне 14% годовых",
		"Совет директоров Банка России принял решение сохранить ключевую ставку на уровне 14% годовых.", "https://tass.test/1", "")
	if !r.Inserted || r.DuplicateOf == nil {
		t.Fatalf("тот же факт из другого источника должен стать дубликатом: %+v", r)
	}
	// Похожая, но другая новость: другое направление и число.
	if r := store(interfax, "ЦБ повысил ключевую ставку до 15%", "Банк России неожиданно повысил ставку до 15% годовых.", "https://ifx.test/1", ""); r.DuplicateOf != nil {
		t.Fatal("повышение ставки — другое событие, не дубликат сохранения")
	}
	// Разный регистр написания.
	a := store(tass, "МИНФИН РАЗМЕСТИЛ ОФЗ НА 50 МЛРД РУБЛЕЙ", "", "https://tass.test/2", "")
	if r := store(interfax, "Минфин разместил ОФЗ на 50 млрд рублей", "Аукцион прошёл успешно.", "https://ifx.test/2", ""); r.DuplicateOf == nil || *r.DuplicateOf != a.ID {
		t.Fatalf("регистр не должен мешать дедупликации: %+v", r)
	}
	if r := store(interfax, "Минфин разместил ОФЗ на 30 млрд рублей", "", "https://ifx.test/3", ""); r.DuplicateOf != nil {
		t.Fatal("другая сумма размещения — другое событие")
	}
	if lvl, _ := level(a.ID); lvl != "title_only" {
		t.Fatalf("без анонса — только заголовок, получено %s", lvl)
	}

	// Обновление старой публикации: та же ссылка, новый текст.
	orig := store(interfax, "Сбербанк изменил ставки по вкладам", "Банк изменил ставки по ряду вкладов с понедельника.", "https://ifx.test/4", "")
	if _, err := pool.Exec(ctx, `UPDATE articles SET classification = '{"x":1}', classified_at = now() WHERE id = $1`, orig.ID); err != nil {
		t.Fatal(err)
	}
	upd := store(interfax, "Сбербанк повысил ставки по вкладам до 18%", "Банк повысил ставки по вкладам до 18% годовых с понедельника.", "https://ifx.test/4", "")
	if !upd.Updated || upd.ID != orig.ID {
		t.Fatalf("та же ссылка с новым текстом — обновление, а не новая новость: %+v", upd)
	}
	var title string
	var classified, updatedAt *time.Time
	if err := pool.QueryRow(ctx, `SELECT title, classified_at, updated_at FROM articles WHERE id = $1`, orig.ID).Scan(&title, &classified, &updatedAt); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(title, "18%") || classified != nil || updatedAt == nil {
		t.Fatalf("обновлённая публикация: заголовок %q, классификация сброшена %v, updated_at %v", title, classified == nil, updatedAt)
	}
	if again := store(interfax, "Сбербанк повысил ставки по вкладам до 18%", "Банк повысил ставки по вкладам до 18% годовых с понедельника.", "https://ifx.test/4", ""); again.Inserted || again.Updated {
		t.Fatalf("повтор без изменений ничего не меняет: %+v", again)
	}

	// Полный текст: только если источник передаёт его в ленте и это разрешено.
	full := strings.Repeat("Банк России опубликовал решение по ключевой ставке и сопроводительный комментарий. ", 6)
	if r := store(cbr, "Банк России опубликовал резюме обсуждения ключевой ставки", "Резюме обсуждения ключевой ставки за сентябрь опубликовано на сайте.", "https://cbr.test/9", full); true {
		if lvl, body := level(r.ID); lvl != "full_text" || body == "" {
			t.Fatalf("официальная лента с разрешённым полным текстом: %s", lvl)
		}
	}
	if r := store(tass, "Росстат опубликовал данные о промышленном производстве", "Промышленное производство в сентябре выросло на 2,1% в годовом выражении.", "https://tass.test/9", full); true {
		if lvl, body := level(r.ID); lvl != "summary" || body != "" {
			t.Fatalf("полный текст СМИ не сохраняется без разрешения: %s %q", lvl, body)
		}
	}
}
