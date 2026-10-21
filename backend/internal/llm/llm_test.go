package llm

import (
	"context"
	"errors"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"
)

func TestMatchAllowed(t *testing.T) {
	ok := []string{"gemma-4-26b-a4b", "google/gemma-4-26B-A4B", "GLM-5.3-Flash", "zai/glm-5.3-flash",
		"Qwen3.6-35B-A3B", "qwen/qwen3.6-35b-a3b-fp8", "deepseek-v4.1-flash", "Qwen3-VL-30B-A3B-Instruct", "kimi-k2.6"}
	for _, id := range ok {
		if _, allowed := MatchAllowed(id); !allowed {
			t.Errorf("%q должна быть разрешена", id)
		}
	}
	bad := []string{"gpt-4o", "qwen3.6-235b-a22b", "glm-5.3", "gemma-4-12b", "deepseek-v4.1", "kimi-k2", "llama-3"}
	for _, id := range bad {
		if _, allowed := MatchAllowed(id); allowed {
			t.Errorf("%q не должна быть разрешена", id)
		}
	}
}

func TestExtractJSON(t *testing.T) {
	cases := map[string]string{
		"```json\n{\"a\":1}\n```":                       `{"a":1}`,
		"<think>рассуждение {x}</think>\n{\"b\":\"}\"}": `{"b":"}"}`,
		"Ответ: {\"c\":{\"d\":[1,2]}} конец":            `{"c":{"d":[1,2]}}`,
	}
	for in, want := range cases {
		got, err := ExtractJSON(in)
		if err != nil || got != want {
			t.Errorf("ExtractJSON(%q) = %q, %v; want %q", in, got, err, want)
		}
	}
	if _, err := ExtractJSON(`{"a": [1, 2`); !errors.Is(err, ErrBadResponse) {
		t.Errorf("обрезанный JSON должен давать ErrBadResponse, got %v", err)
	}
}

type fakeStore struct{ used int64 }

func (f fakeStore) TokensUsedSince(context.Context, time.Time) (int64, error) { return f.used, nil }

func TestGuardBudgetAndRate(t *testing.T) {
	g := NewGuard(GuardConfig{DailyTokenBudget: 1000, DailyCallLimit: 3, PerMinuteLimit: 2}, fakeStore{used: 900})
	if _, err := g.Acquire(context.Background(), 200); !errors.Is(err, ErrBudgetExceeded) {
		t.Fatalf("ожидался ErrBudgetExceeded, got %v", err)
	}
	g2 := NewGuard(GuardConfig{DailyTokenBudget: 100000, DailyCallLimit: 10, PerMinuteLimit: 2}, fakeStore{})
	for i := 0; i < 2; i++ {
		rel, err := g2.Acquire(context.Background(), 10)
		if err != nil {
			t.Fatal(err)
		}
		rel(10)
	}
	if _, err := g2.Acquire(context.Background(), 10); !errors.Is(err, ErrRateLimited) {
		t.Fatalf("ожидался ErrRateLimited, got %v", err)
	}
	g2.Pause(time.Minute, "test")
	if _, err := g2.Acquire(context.Background(), 10); !errors.Is(err, ErrPaused) {
		t.Fatalf("ожидался ErrPaused, got %v", err)
	}
}

func newTestClient(url string, g *Guard) *Client {
	return NewClient(url, "sk-test-secret-value", 5*time.Second, true, g, 500, 10000,
		slog.New(slog.NewTextHandler(io.Discard, nil)))
}

func TestChatCachesAndSendsKeyOnlyInHeader(t *testing.T) {
	var calls atomic.Int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls.Add(1)
		if r.Header.Get("Authorization") != "Bearer sk-test-secret-value" {
			t.Errorf("неверный заголовок авторизации")
		}
		b, _ := io.ReadAll(r.Body)
		if strings.Contains(string(b), "sk-test") {
			t.Errorf("ключ не должен попадать в тело запроса")
		}
		w.Write([]byte(`{"choices":[{"message":{"content":"{\"ok\":true}"},"finish_reason":"stop"}],"usage":{"prompt_tokens":10,"completion_tokens":5,"total_tokens":15}}`))
	}))
	defer srv.Close()
	c := newTestClient(srv.URL, NewGuard(GuardConfig{DailyTokenBudget: 100000, DailyCallLimit: 10, PerMinuteLimit: 10}, fakeStore{}))
	req := Request{Model: "gemma-4-26b-a4b", Messages: []Message{{Role: "user", Content: "test"}}, JSON: true}
	r1, err := c.Chat(context.Background(), req)
	if err != nil || r1.Cached {
		t.Fatalf("первый вызов: %v cached=%v", err, r1.Cached)
	}
	r2, err := c.Chat(context.Background(), req)
	if err != nil || !r2.Cached || r2.Usage.TotalTokens != 0 {
		t.Fatalf("второй вызов должен идти из кэша: %v %+v", err, r2)
	}
	if calls.Load() != 1 {
		t.Fatalf("ожидался 1 HTTP-запрос, было %d", calls.Load())
	}
}

func TestChatRejectsDisallowedModel(t *testing.T) {
	c := newTestClient("http://127.0.0.1:1", NewGuard(GuardConfig{}, nil))
	_, err := c.Chat(context.Background(), Request{Model: "gpt-4o", Messages: []Message{{Role: "user", Content: "x"}}})
	if !errors.Is(err, ErrModelNotAllowed) {
		t.Fatalf("ожидался ErrModelNotAllowed, got %v", err)
	}
}

func TestChat429PausesGuardWithoutRetry(t *testing.T) {
	var calls atomic.Int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls.Add(1)
		w.Header().Set("Retry-After", "120")
		w.WriteHeader(http.StatusTooManyRequests)
		w.Write([]byte(`{"error":"rate limit"}`))
	}))
	defer srv.Close()
	g := NewGuard(GuardConfig{DailyTokenBudget: 100000, DailyCallLimit: 10, PerMinuteLimit: 10}, fakeStore{})
	c := newTestClient(srv.URL, g)
	req := Request{Model: "glm-5.3-flash", Messages: []Message{{Role: "user", Content: "x"}}}
	if _, err := c.Chat(context.Background(), req); !errors.Is(err, ErrRateLimited) {
		t.Fatalf("ожидался ErrRateLimited, got %v", err)
	}
	req.Messages[0].Content = "y"
	if _, err := c.Chat(context.Background(), req); !errors.Is(err, ErrPaused) {
		t.Fatalf("после 429 запросы должны быть приостановлены, got %v", err)
	}
	if calls.Load() != 1 {
		t.Fatalf("после 429 не должно быть повторов, было %d запросов", calls.Load())
	}
}

func TestChatBudgetErrorPausesUntilMidnight(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusBadRequest)
		w.Write([]byte(`{"error":{"message":"Budget has been exceeded! Current cost: 500"}}`))
	}))
	defer srv.Close()
	g := NewGuard(GuardConfig{DailyTokenBudget: 100000, DailyCallLimit: 10, PerMinuteLimit: 10}, fakeStore{})
	c := newTestClient(srv.URL, g)
	_, err := c.Chat(context.Background(), Request{Model: "kimi-k2.6", Messages: []Message{{Role: "user", Content: "x"}}})
	if !errors.Is(err, ErrBudgetExceeded) {
		t.Fatalf("ожидался ErrBudgetExceeded, got %v", err)
	}
	if st := g.Status(); st.PausedUntil == nil {
		t.Fatal("после ошибки бюджета клиент должен быть на паузе")
	}
}
