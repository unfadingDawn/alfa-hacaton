package auth

import (
	"errors"
	"log/slog"
	"net/http"
	"net/mail"
	"strings"
	"time"
	"unicode"
	"unicode/utf8"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"

	"github.com/alfainsight/backend/internal/db/dbq"
	"github.com/alfainsight/backend/internal/platform/httpx"
)

type Handler struct {
	Q            *dbq.Queries
	Tokens       *TokenIssuer
	RefreshTTL   time.Duration
	CookieSecure bool
	Log          *slog.Logger
	Limiter      *RateLimiter
}

func (h *Handler) Routes(r chi.Router) {
	r.Post("/auth/register", h.register)
	r.Post("/auth/login", h.login)
	r.Post("/auth/logout", h.logout)
	r.Post("/auth/refresh", h.refresh)
}

type registerReq struct {
	Email    string `json:"email"`
	Password string `json:"password"`
	FullName string `json:"full_name"`
}

type loginReq struct {
	Email    string `json:"email"`
	Password string `json:"password"`
}

// ValidateRegistration возвращает ошибки по полям (пустая карта — всё верно).
func ValidateRegistration(req registerReq) map[string]string {
	errs := map[string]string{}
	email := strings.TrimSpace(req.Email)
	if a, err := mail.ParseAddress(email); err != nil || a.Address != email || !strings.Contains(email, ".") {
		errs["email"] = "Введите корректный email"
	}
	if utf8.RuneCountInString(req.Password) < 8 {
		errs["password"] = "Пароль должен содержать не менее 8 символов"
	} else if utf8.RuneCountInString(req.Password) > 128 {
		errs["password"] = "Пароль слишком длинный"
	} else {
		var letter, digit bool
		for _, c := range req.Password {
			letter = letter || unicode.IsLetter(c)
			digit = digit || unicode.IsDigit(c)
		}
		if !letter || !digit {
			errs["password"] = "Пароль должен содержать буквы и цифры"
		}
	}
	name := strings.TrimSpace(req.FullName)
	if utf8.RuneCountInString(name) < 2 || utf8.RuneCountInString(name) > 120 {
		errs["full_name"] = "Укажите имя (от 2 до 120 символов)"
	}
	return errs
}

func (h *Handler) register(w http.ResponseWriter, r *http.Request) {
	if !h.Limiter.Allow("reg:" + ClientIP(r)) {
		httpx.Error(w, http.StatusTooManyRequests, "rate_limited", "Слишком много попыток. Подождите минуту.")
		return
	}
	var req registerReq
	if !httpx.DecodeOrFail(w, r, &req) {
		return
	}
	if errs := ValidateRegistration(req); len(errs) > 0 {
		httpx.ValidationError(w, errs)
		return
	}
	hash, err := HashPassword(req.Password)
	if err != nil {
		httpx.Internal(w, h.Log, err, "hash password")
		return
	}
	u, err := h.Q.CreateUser(r.Context(), dbq.CreateUserParams{
		Email: strings.TrimSpace(req.Email), PasswordHash: hash, FullName: strings.TrimSpace(req.FullName),
	})
	if err != nil {
		var pgErr *pgconn.PgError
		if errors.As(err, &pgErr) && pgErr.Code == "23505" {
			httpx.ValidationError(w, map[string]string{"email": "Пользователь с таким email уже зарегистрирован"})
			return
		}
		httpx.Internal(w, h.Log, err, "create user")
		return
	}
	if err := h.startSession(w, r, u.ID); err != nil {
		httpx.Internal(w, h.Log, err, "start session")
		return
	}
	httpx.JSON(w, http.StatusCreated, map[string]any{"id": u.ID, "email": u.Email, "full_name": u.FullName})
}

func (h *Handler) login(w http.ResponseWriter, r *http.Request) {
	if !h.Limiter.Allow("login:" + ClientIP(r)) {
		httpx.Error(w, http.StatusTooManyRequests, "rate_limited", "Слишком много попыток входа. Подождите минуту.")
		return
	}
	var req loginReq
	if !httpx.DecodeOrFail(w, r, &req) {
		return
	}
	if strings.TrimSpace(req.Email) == "" || req.Password == "" {
		httpx.ValidationError(w, map[string]string{"email": "Введите email и пароль"})
		return
	}
	u, err := h.Q.GetUserByEmail(r.Context(), strings.TrimSpace(req.Email))
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			// Выполняем хеширование, чтобы время ответа не выдавало наличие email.
			_, _ = VerifyPassword(req.Password, dummyHash)
			httpx.Error(w, http.StatusUnauthorized, "invalid_credentials", "Неверный email или пароль")
			return
		}
		httpx.Internal(w, h.Log, err, "get user")
		return
	}
	ok, err := VerifyPassword(req.Password, u.PasswordHash)
	if err != nil || !ok {
		httpx.Error(w, http.StatusUnauthorized, "invalid_credentials", "Неверный email или пароль")
		return
	}
	if err := h.startSession(w, r, u.ID); err != nil {
		httpx.Internal(w, h.Log, err, "start session")
		return
	}
	httpx.JSON(w, http.StatusOK, map[string]any{"id": u.ID, "email": u.Email, "full_name": u.FullName,
		"onboarding_completed": u.OnboardingCompleted})
}

func (h *Handler) refresh(w http.ResponseWriter, r *http.Request) {
	c, err := r.Cookie(RefreshCookie)
	if err != nil || c.Value == "" {
		httpx.Error(w, http.StatusUnauthorized, "unauthorized", "Сессия не найдена")
		return
	}
	s, err := h.Q.GetActiveSessionByHash(r.Context(), HashToken(c.Value))
	if err != nil {
		h.clearCookies(w)
		httpx.Error(w, http.StatusUnauthorized, "unauthorized", "Сессия истекла, войдите снова")
		return
	}
	// Ротация: старый refresh-токен отзывается, выдаётся новый.
	if err := h.Q.RevokeSession(r.Context(), s.ID); err != nil {
		httpx.Internal(w, h.Log, err, "revoke session")
		return
	}
	if err := h.startSession(w, r, s.UserID); err != nil {
		httpx.Internal(w, h.Log, err, "start session")
		return
	}
	httpx.JSON(w, http.StatusOK, map[string]any{"ok": true})
}

func (h *Handler) logout(w http.ResponseWriter, r *http.Request) {
	if c, err := r.Cookie(RefreshCookie); err == nil && c.Value != "" {
		_ = h.Q.RevokeSessionByHash(r.Context(), HashToken(c.Value))
	}
	h.clearCookies(w)
	httpx.JSON(w, http.StatusOK, map[string]any{"ok": true})
}

func (h *Handler) startSession(w http.ResponseWriter, r *http.Request, userID uuid.UUID) error {
	access, accessExp, err := h.Tokens.IssueAccess(userID)
	if err != nil {
		return err
	}
	refresh, refreshHash, err := NewRefreshToken()
	if err != nil {
		return err
	}
	ua := r.UserAgent()
	if len(ua) > 300 {
		ua = ua[:300]
	}
	refreshExp := time.Now().Add(h.RefreshTTL)
	if _, err := h.Q.CreateSession(r.Context(), dbq.CreateSessionParams{
		UserID: userID, TokenHash: refreshHash, UserAgent: ua, ExpiresAt: refreshExp,
	}); err != nil {
		return err
	}
	http.SetCookie(w, &http.Cookie{Name: AccessCookie, Value: access, Path: "/", HttpOnly: true,
		Secure: h.CookieSecure, SameSite: http.SameSiteLaxMode, Expires: accessExp})
	http.SetCookie(w, &http.Cookie{Name: RefreshCookie, Value: refresh, Path: "/api/auth", HttpOnly: true,
		Secure: h.CookieSecure, SameSite: http.SameSiteLaxMode, Expires: refreshExp})
	return nil
}

func (h *Handler) clearCookies(w http.ResponseWriter) {
	for _, c := range []struct{ name, path string }{{AccessCookie, "/"}, {RefreshCookie, "/api/auth"}} {
		http.SetCookie(w, &http.Cookie{Name: c.name, Value: "", Path: c.path, HttpOnly: true,
			Secure: h.CookieSecure, SameSite: http.SameSiteLaxMode, MaxAge: -1})
	}
}

// dummyHash — хеш случайной строки для выравнивания времени ответа.
var dummyHash = func() string {
	h, _ := HashPassword("timing-equalizer-not-a-real-password-1")
	return h
}()
