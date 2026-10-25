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
			PerUserHourly: 10, JSONMode: true}}
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
	if strings.Join(names, ",") != "classify,company_profile,plan,search_research,draft,self_check" {
		t.Fatalf("трасса агента: %v", names)
	}
	runID := res["run"].(map[string]any)["id"].(string)

	// Повторный запрос — из кэша, без обращения к модели.
	if res2 := cfo.must("POST", "/api/articles/"+articleID+"/analyze", nil, 200); !res2["cached"].(bool) || mock.calls.Load() != callsAfterFirst {
		t.Fatal("повторный анализ должен браться из кэша")
	}

	// Та же новость для руководителя закупок: другая линза, классификация не повторяется.
	proc.must("POST", "/api/articles/"+articleID+"/analyze", nil, 200)
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
	before := mock.calls.Load()
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

	// Выход.
	cfo.must("POST", "/api/auth/logout", nil, 200)
	if code, _ := cfo.do("GET", "/api/me", nil); code != 401 {
		t.Fatal("после выхода доступ должен закрываться")
	}
}
