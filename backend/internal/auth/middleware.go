package auth

import (
	"context"
	"errors"
	"net"
	"net/http"
	"net/url"
	"strings"
	"sync"
	"time"

	"github.com/google/uuid"

	"github.com/alfainsight/backend/internal/platform/httpx"
)

const (
	AccessCookie  = "ai_access"
	RefreshCookie = "ai_refresh"
)

type ctxKey struct{}

func WithUserID(ctx context.Context, id uuid.UUID) context.Context {
	return context.WithValue(ctx, ctxKey{}, id)
}

// UserID возвращает ID пользователя из контекста запроса (после RequireAuth).
func UserID(ctx context.Context) uuid.UUID {
	id, _ := ctx.Value(ctxKey{}).(uuid.UUID)
	return id
}

// RequireAuth проверяет access-токен из HttpOnly cookie.
func RequireAuth(tokens *TokenIssuer) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			c, err := r.Cookie(AccessCookie)
			if err != nil || c.Value == "" {
				httpx.Error(w, http.StatusUnauthorized, "unauthorized", "Требуется вход в систему")
				return
			}
			id, err := tokens.ParseAccess(c.Value)
			if err != nil {
				code := "unauthorized"
				if errors.Is(err, ErrTokenExpired) {
					code = "token_expired"
				}
				httpx.Error(w, http.StatusUnauthorized, code, "Сессия истекла, войдите снова")
				return
			}
			next.ServeHTTP(w, r.WithContext(WithUserID(r.Context(), id)))
		})
	}
}

// OriginGuard — защита от CSRF для изменяющих запросов: помимо SameSite=Lax
// проверяем, что Origin (если браузер его прислал) входит в список разрешённых.
func OriginGuard(allowed []string) func(http.Handler) http.Handler {
	set := map[string]bool{}
	for _, a := range allowed {
		set[strings.TrimRight(a, "/")] = true
	}
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			switch r.Method {
			case http.MethodGet, http.MethodHead, http.MethodOptions:
				next.ServeHTTP(w, r)
				return
			}
			origin := r.Header.Get("Origin")
			if origin != "" && !set[origin] {
				// Запрос пришёл через прокси Next.js на тот же хост — тоже допустимо.
				if u, err := url.Parse(origin); err != nil || u.Host != r.Host {
					httpx.Error(w, http.StatusForbidden, "forbidden_origin", "Запрос с недопустимого источника")
					return
				}
			}
			next.ServeHTTP(w, r)
		})
	}
}

// RateLimiter — простой ограничитель попыток входа по IP (in-memory).
type RateLimiter struct {
	mu     sync.Mutex
	hits   map[string][]time.Time
	limit  int
	window time.Duration
}

func NewRateLimiter(limit int, window time.Duration) *RateLimiter {
	return &RateLimiter{hits: map[string][]time.Time{}, limit: limit, window: window}
}

func (rl *RateLimiter) Allow(key string) bool {
	rl.mu.Lock()
	defer rl.mu.Unlock()
	now := time.Now()
	cut := now.Add(-rl.window)
	h := rl.hits[key][:0]
	for _, t := range rl.hits[key] {
		if t.After(cut) {
			h = append(h, t)
		}
	}
	if len(h) >= rl.limit {
		rl.hits[key] = h
		return false
	}
	rl.hits[key] = append(h, now)
	return true
}

func ClientIP(r *http.Request) string {
	if xf := r.Header.Get("X-Forwarded-For"); xf != "" {
		return strings.TrimSpace(strings.Split(xf, ",")[0])
	}
	host, _, err := net.SplitHostPort(r.RemoteAddr)
	if err != nil {
		return r.RemoteAddr
	}
	return host
}
