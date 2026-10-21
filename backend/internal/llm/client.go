// Package llm — HTTP-клиент OpenAI-совместимого API для разрешённых моделей.
// Ключ передаётся только в заголовке Authorization и нигде не логируется.
package llm

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/alfainsight/backend/internal/platform/logx"
)

type Message struct {
	Role    string `json:"role"`
	Content string `json:"content"`
}

type Request struct {
	Model       string
	Messages    []Message
	Temperature float64
	MaxTokens   int
	JSON        bool
}

type Usage struct {
	PromptTokens     int `json:"prompt_tokens"`
	CompletionTokens int `json:"completion_tokens"`
	TotalTokens      int `json:"total_tokens"`
}

type Response struct {
	Model    string        `json:"model"`
	Content  string        `json:"content"`
	Usage    Usage         `json:"usage"`
	Cached   bool          `json:"cached"`
	Duration time.Duration `json:"duration"`
}

type Client struct {
	baseURL  string
	apiKey   string
	http     *http.Client
	guard    *Guard
	log      *slog.Logger
	jsonMode bool
	maxOut   int
	maxIn    int

	cacheMu sync.Mutex
	cache   map[string]cacheEntry
	flight  map[string]*call
}

type cacheEntry struct {
	resp Response
	at   time.Time
}

type call struct {
	done chan struct{}
	resp Response
	err  error
}

const cacheTTL = 6 * time.Hour
const cacheMax = 500

func NewClient(baseURL, apiKey string, timeout time.Duration, jsonMode bool, guard *Guard, maxOut, maxIn int, log *slog.Logger) *Client {
	return &Client{
		baseURL: strings.TrimRight(baseURL, "/"), apiKey: apiKey,
		http:  &http.Client{Timeout: timeout},
		guard: guard, log: log, jsonMode: jsonMode, maxOut: maxOut, maxIn: maxIn,
		cache: map[string]cacheEntry{}, flight: map[string]*call{},
	}
}

func (c *Client) Configured() bool { return c != nil && c.baseURL != "" && c.apiKey != "" }

func (c *Client) Guard() *Guard { return c.guard }

func cacheKey(r Request) string {
	b, _ := json.Marshal(r)
	sum := sha256.Sum256(b)
	return hex.EncodeToString(sum[:])
}

// estimateTokens — грубая оценка для резервирования бюджета (≈3 символа на токен
// для русского текста) плюс максимальный размер ответа.
func estimateTokens(r Request) int64 {
	n := 0
	for _, m := range r.Messages {
		n += len([]rune(m.Content))
	}
	return int64(n/3 + r.MaxTokens)
}

// Chat выполняет один запрос к модели с проверками:
// разрешённая модель → кэш → объединение одинаковых запросов → лимиты → HTTP.
func (c *Client) Chat(ctx context.Context, req Request) (Response, error) {
	if !c.Configured() {
		return Response{}, ErrNotConfigured
	}
	if _, ok := MatchAllowed(req.Model); !ok || req.Model == "" {
		return Response{}, fmt.Errorf("%w: %q", ErrModelNotAllowed, req.Model)
	}
	if req.MaxTokens <= 0 || req.MaxTokens > c.maxOut {
		req.MaxTokens = c.maxOut
	}
	for i := range req.Messages {
		if r := []rune(req.Messages[i].Content); c.maxIn > 0 && len(r) > c.maxIn {
			req.Messages[i].Content = string(r[:c.maxIn]) + "\n[текст сокращён]"
		}
	}
	key := cacheKey(req)

	c.cacheMu.Lock()
	if e, ok := c.cache[key]; ok && time.Since(e.at) < cacheTTL {
		c.cacheMu.Unlock()
		r := e.resp
		r.Cached, r.Usage = true, Usage{}
		return r, nil
	}
	if f, ok := c.flight[key]; ok {
		c.cacheMu.Unlock()
		select {
		case <-f.done:
			r := f.resp
			r.Cached, r.Usage = true, Usage{}
			return r, f.err
		case <-ctx.Done():
			return Response{}, ctx.Err()
		}
	}
	f := &call{done: make(chan struct{})}
	c.flight[key] = f
	c.cacheMu.Unlock()

	f.resp, f.err = c.do(ctx, req)

	c.cacheMu.Lock()
	delete(c.flight, key)
	if f.err == nil {
		if len(c.cache) >= cacheMax {
			for k := range c.cache {
				delete(c.cache, k)
				break
			}
		}
		c.cache[key] = cacheEntry{resp: f.resp, at: time.Now()}
	}
	c.cacheMu.Unlock()
	close(f.done)
	return f.resp, f.err
}

type chatBody struct {
	Model          string            `json:"model"`
	Messages       []Message         `json:"messages"`
	Temperature    float64           `json:"temperature"`
	MaxTokens      int               `json:"max_tokens"`
	ResponseFormat map[string]string `json:"response_format,omitempty"`
}

type chatResp struct {
	Model   string `json:"model"`
	Choices []struct {
		Message struct {
			Content string `json:"content"`
		} `json:"message"`
		FinishReason string `json:"finish_reason"`
	} `json:"choices"`
	Usage Usage `json:"usage"`
}

func (c *Client) do(ctx context.Context, req Request) (Response, error) {
	release, err := c.guard.Acquire(ctx, estimateTokens(req))
	if err != nil {
		return Response{}, err
	}
	var used int64
	defer func() { release(used) }()

	body := chatBody{Model: req.Model, Messages: req.Messages, Temperature: req.Temperature, MaxTokens: req.MaxTokens}
	if req.JSON && c.jsonMode {
		body.ResponseFormat = map[string]string{"type": "json_object"}
	}
	start := time.Now()
	// Не более одной повторной попытки и только для сетевых ошибок/5xx.
	for attempt := 0; attempt < 2; attempt++ {
		resp, status, retryAfter, raw, err := c.post(ctx, "/chat/completions", body)
		if err == nil && status == http.StatusOK {
			var cr chatResp
			if err := json.Unmarshal(raw, &cr); err != nil || len(cr.Choices) == 0 {
				return Response{}, fmt.Errorf("%w: не удалось разобрать ответ", ErrBadResponse)
			}
			used = int64(cr.Usage.TotalTokens)
			if used == 0 {
				used = int64(cr.Usage.PromptTokens + cr.Usage.CompletionTokens)
			}
			c.log.Info("llm call", "model", req.Model, "prompt_tokens", cr.Usage.PromptTokens,
				"completion_tokens", cr.Usage.CompletionTokens, "ms", time.Since(start).Milliseconds(),
				"finish", cr.Choices[0].FinishReason)
			return Response{Model: req.Model, Content: cr.Choices[0].Message.Content, Usage: cr.Usage,
				Duration: time.Since(start)}, nil
		}
		_ = resp
		if err != nil {
			if ctx.Err() != nil {
				return Response{}, fmt.Errorf("%w: превышено время ожидания ответа", ErrUnavailable)
			}
			c.log.Warn("llm network error", "model", req.Model, "attempt", attempt, "err", err)
			if attempt == 0 {
				time.Sleep(2 * time.Second)
				continue
			}
			return Response{}, fmt.Errorf("%w: %s", ErrUnavailable, logx.Scrub(err.Error()))
		}
		snippet := logx.Scrub(string(raw))
		if len(snippet) > 300 {
			snippet = snippet[:300]
		}
		c.log.Warn("llm http error", "model", req.Model, "status", status, "body", snippet)
		lower := strings.ToLower(snippet)
		switch {
		case status == http.StatusUnauthorized:
			c.guard.Pause(15*time.Minute, "ключ отклонён")
			return Response{}, ErrAuth
		case status == http.StatusTooManyRequests:
			c.guard.Pause(retryAfter, "HTTP 429 от провайдера")
			return Response{}, ErrRateLimited
		case strings.Contains(lower, "budget") || strings.Contains(lower, "quota") ||
			strings.Contains(lower, "limit") || status == http.StatusPaymentRequired:
			c.guard.Pause(untilMidnight(c.guard.cfg.Location), "исчерпан лимит провайдера")
			return Response{}, fmt.Errorf("%w (ответ провайдера: HTTP %d)", ErrBudgetExceeded, status)
		case status == http.StatusBadRequest && req.JSON && body.ResponseFormat != nil && strings.Contains(lower, "response_format"):
			// Сервер не поддерживает JSON-режим — повторяем без него (JSON всё равно требуется промптом).
			body.ResponseFormat = nil
			continue
		case status >= 500 && attempt == 0:
			time.Sleep(2 * time.Second)
			continue
		case status == http.StatusNotFound:
			return Response{}, fmt.Errorf("%w: модель %q не найдена у провайдера", ErrUnavailable, req.Model)
		default:
			return Response{}, fmt.Errorf("%w: HTTP %d", ErrUnavailable, status)
		}
	}
	return Response{}, ErrUnavailable
}

func untilMidnight(loc *time.Location) time.Duration {
	if loc == nil {
		loc = time.UTC
	}
	now := time.Now().In(loc)
	next := time.Date(now.Year(), now.Month(), now.Day()+1, 0, 5, 0, 0, loc)
	return next.Sub(now)
}

func (c *Client) post(ctx context.Context, path string, body any) (*http.Response, int, time.Duration, []byte, error) {
	b, err := json.Marshal(body)
	if err != nil {
		return nil, 0, 0, nil, err
	}
	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, c.baseURL+path, bytes.NewReader(b))
	if err != nil {
		return nil, 0, 0, nil, err
	}
	httpReq.Header.Set("Authorization", "Bearer "+c.apiKey)
	httpReq.Header.Set("Content-Type", "application/json")
	resp, err := c.http.Do(httpReq)
	if err != nil {
		return nil, 0, 0, nil, err
	}
	defer resp.Body.Close()
	raw, _ := io.ReadAll(io.LimitReader(resp.Body, 4<<20))
	var ra time.Duration
	if s, err := strconv.Atoi(resp.Header.Get("Retry-After")); err == nil {
		ra = time.Duration(s) * time.Second
	}
	return resp, resp.StatusCode, ra, raw, nil
}

func (c *Client) get(ctx context.Context, path string) (int, []byte, error) {
	httpReq, err := http.NewRequestWithContext(ctx, http.MethodGet, c.baseURL+path, nil)
	if err != nil {
		return 0, nil, err
	}
	httpReq.Header.Set("Authorization", "Bearer "+c.apiKey)
	resp, err := c.http.Do(httpReq)
	if err != nil {
		return 0, nil, errors.New(logx.Scrub(err.Error()))
	}
	defer resp.Body.Close()
	raw, _ := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	return resp.StatusCode, raw, nil
}

type ModelInfo struct {
	ID      string `json:"id"`
	Allowed bool   `json:"allowed"`
	Title   string `json:"title,omitempty"`
}

// ListModels — GET /models (не расходует токены). Помечает разрешённые модели.
func (c *Client) ListModels(ctx context.Context) ([]ModelInfo, error) {
	if !c.Configured() {
		return nil, ErrNotConfigured
	}
	status, raw, err := c.get(ctx, "/models")
	if err != nil {
		return nil, fmt.Errorf("%w: %v", ErrUnavailable, err)
	}
	if status == http.StatusUnauthorized || status == http.StatusForbidden {
		return nil, ErrAuth
	}
	if status != http.StatusOK {
		return nil, fmt.Errorf("%w: HTTP %d", ErrUnavailable, status)
	}
	var body struct {
		Data []struct {
			ID string `json:"id"`
		} `json:"data"`
	}
	if err := json.Unmarshal(raw, &body); err != nil {
		return nil, fmt.Errorf("%w: список моделей", ErrBadResponse)
	}
	out := make([]ModelInfo, 0, len(body.Data))
	for _, m := range body.Data {
		a, ok := MatchAllowed(m.ID)
		out = append(out, ModelInfo{ID: m.ID, Allowed: ok, Title: a.Title})
	}
	return out, nil
}

// KeyInfo пытается получить сведения о лимитах ключа. Многие OpenAI-совместимые
// шлюзы (например, LiteLLM) отдают их на GET /key/info; если эндпоинта нет,
// возвращается (nil, nil). Запрос не расходует токены.
func (c *Client) KeyInfo(ctx context.Context) (map[string]any, error) {
	if !c.Configured() {
		return nil, ErrNotConfigured
	}
	base := strings.TrimSuffix(c.baseURL, "/v1")
	for _, url := range []string{base + "/key/info", c.baseURL + "/key/info"} {
		httpReq, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
		if err != nil {
			continue
		}
		httpReq.Header.Set("Authorization", "Bearer "+c.apiKey)
		resp, err := c.http.Do(httpReq)
		if err != nil {
			continue
		}
		raw, _ := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
		resp.Body.Close()
		if resp.StatusCode != http.StatusOK {
			continue
		}
		var info map[string]any
		if json.Unmarshal(raw, &info) == nil {
			redactKeys(info)
			return info, nil
		}
	}
	return nil, nil
}

// redactKeys удаляет из ответа поля, похожие на сам ключ.
func redactKeys(m map[string]any) {
	for k, v := range m {
		lk := strings.ToLower(k)
		if lk == "key" || lk == "token" || lk == "api_key" || strings.HasSuffix(lk, "_key") && lk != "key_alias" {
			m[k] = "[REDACTED]"
			continue
		}
		if s, ok := v.(string); ok && strings.HasPrefix(s, "sk-") {
			m[k] = "[REDACTED]"
		}
		if sub, ok := v.(map[string]any); ok {
			redactKeys(sub)
		}
	}
}
