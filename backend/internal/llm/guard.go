package llm

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"time"
)

// Ошибки, которые API и интерфейс показывают пользователю понятным текстом.
var (
	ErrNotConfigured   = errors.New("модель не подключена: задайте LLM_BASE_URL, LLM_API_KEY и модели в .env")
	ErrModelNotAllowed = errors.New("модель не входит в список разрешённых организаторами")
	ErrBudgetExceeded  = errors.New("достигнут дневной лимит запросов к модели")
	ErrRateLimited     = errors.New("провайдер модели ограничил частоту запросов, повторите позже")
	ErrPaused          = errors.New("запросы к модели временно приостановлены после ошибки лимита")
	ErrAuth            = errors.New("ключ API отклонён провайдером")
	ErrUnavailable     = errors.New("модель временно недоступна")
	ErrBadResponse     = errors.New("модель вернула ответ в неверном формате")
	// ErrOutputTruncated также является ErrBadResponse, чтобы необязательные
	// этапы агента могли использовать прежний fallback.
	ErrOutputTruncated = fmt.Errorf("%w: ответ обрезан по лимиту токенов", ErrBadResponse)
)

// UsageStore возвращает число токенов, израсходованных с начала суток
// (по журналу запусков в БД), чтобы лимит переживал перезапуск сервиса.
type UsageStore interface {
	TokensUsedSince(ctx context.Context, since time.Time) (int64, error)
}

// GuardConfig — все пределы задаются в .env. Значения по умолчанию
// консервативны: организаторы дают общий ключ с дневным лимитом (500 руб.),
// точная цена токена неизвестна, поэтому оставляем большой запас.
type GuardConfig struct {
	DailyTokenBudget int64         // жёсткий предел токенов в сутки (вход + выход)
	DailyCallLimit   int           // предел числа запросов в сутки
	PerMinuteLimit   int           // предел запросов в минуту
	MaxOutputTokens  int           // max_tokens для одного ответа
	MaxInputChars    int           // обрезка входного текста
	PauseOnLimit     time.Duration // пауза после 429/ошибки бюджета провайдера
	Location         *time.Location
}

// Guard — единая точка контроля расхода: бюджет, частота, «предохранитель»
// после ошибок лимита. Каждый вызов модели проходит через Acquire/Release.
type Guard struct {
	cfg   GuardConfig
	store UsageStore

	mu          sync.Mutex
	day         string
	calls       int
	minute      []time.Time
	reserved    int64 // токены «в полёте» (оценка), ещё не записанные в БД
	usedCache   int64
	usedAt      time.Time
	pausedUntil time.Time
	pauseReason string
}

func NewGuard(cfg GuardConfig, store UsageStore) *Guard {
	if cfg.Location == nil {
		cfg.Location = time.UTC
	}
	return &Guard{cfg: cfg, store: store}
}

func (g *Guard) startOfDay(now time.Time) time.Time {
	t := now.In(g.cfg.Location)
	return time.Date(t.Year(), t.Month(), t.Day(), 0, 0, 0, 0, g.cfg.Location)
}

// Acquire резервирует оценку токенов на один запрос или возвращает ошибку.
func (g *Guard) Acquire(ctx context.Context, estimate int64) (func(actual int64), error) {
	now := time.Now()
	g.mu.Lock()
	if now.Before(g.pausedUntil) {
		until := g.pausedUntil.In(g.cfg.Location).Format("15:04")
		reason := g.pauseReason
		g.mu.Unlock()
		return nil, fmt.Errorf("%w до %s (%s)", ErrPaused, until, reason)
	}
	day := g.startOfDay(now).Format("2006-01-02")
	if day != g.day {
		g.day, g.calls, g.usedCache, g.usedAt = day, 0, 0, time.Time{}
	}
	if g.cfg.DailyCallLimit > 0 && g.calls >= g.cfg.DailyCallLimit {
		g.mu.Unlock()
		return nil, fmt.Errorf("%w: %d запросов за сутки", ErrBudgetExceeded, g.cfg.DailyCallLimit)
	}
	cut := now.Add(-time.Minute)
	recent := g.minute[:0]
	for _, t := range g.minute {
		if t.After(cut) {
			recent = append(recent, t)
		}
	}
	g.minute = recent
	if g.cfg.PerMinuteLimit > 0 && len(g.minute) >= g.cfg.PerMinuteLimit {
		g.mu.Unlock()
		return nil, fmt.Errorf("%w: не более %d запросов в минуту", ErrRateLimited, g.cfg.PerMinuteLimit)
	}
	needRefresh := now.Sub(g.usedAt) > 30*time.Second
	g.mu.Unlock()

	if needRefresh && g.store != nil {
		used, err := g.store.TokensUsedSince(ctx, g.startOfDay(now))
		if err == nil {
			g.mu.Lock()
			g.usedCache, g.usedAt = used, now
			g.mu.Unlock()
		}
	}

	g.mu.Lock()
	defer g.mu.Unlock()
	if g.cfg.DailyTokenBudget > 0 && g.usedCache+g.reserved+estimate > g.cfg.DailyTokenBudget {
		return nil, fmt.Errorf("%w: израсходовано ~%d из %d токенов", ErrBudgetExceeded, g.usedCache+g.reserved, g.cfg.DailyTokenBudget)
	}
	g.calls++
	g.minute = append(g.minute, now)
	g.reserved += estimate
	var once sync.Once
	return func(actual int64) {
		once.Do(func() {
			g.mu.Lock()
			g.reserved -= estimate
			g.usedCache += actual
			g.mu.Unlock()
		})
	}, nil
}

// Pause останавливает все запросы к модели (после 429 или ошибки бюджета).
func (g *Guard) Pause(d time.Duration, reason string) {
	if d <= 0 {
		d = g.cfg.PauseOnLimit
	}
	g.mu.Lock()
	defer g.mu.Unlock()
	if until := time.Now().Add(d); until.After(g.pausedUntil) {
		g.pausedUntil, g.pauseReason = until, reason
	}
}

type GuardStatus struct {
	Day              string     `json:"day"`
	Calls            int        `json:"calls"`
	DailyCallLimit   int        `json:"daily_call_limit"`
	TokensUsed       int64      `json:"tokens_used"`
	DailyTokenBudget int64      `json:"daily_token_budget"`
	PausedUntil      *time.Time `json:"paused_until,omitempty"`
	PauseReason      string     `json:"pause_reason,omitempty"`
}

func (g *Guard) Status() GuardStatus {
	g.mu.Lock()
	defer g.mu.Unlock()
	st := GuardStatus{Day: g.day, Calls: g.calls, DailyCallLimit: g.cfg.DailyCallLimit,
		TokensUsed: g.usedCache + g.reserved, DailyTokenBudget: g.cfg.DailyTokenBudget}
	if time.Now().Before(g.pausedUntil) {
		t := g.pausedUntil
		st.PausedUntil, st.PauseReason = &t, g.pauseReason
	}
	return st
}
