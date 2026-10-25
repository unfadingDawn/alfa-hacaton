// Package users — профиль пользователя, интересы, персонализация и онбординг.
package users

import (
	"encoding/base64"
	"encoding/json"
	"errors"
	"log/slog"
	"net/http"
	"strings"
	"unicode/utf8"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/alfainsight/backend/internal/auth"
	"github.com/alfainsight/backend/internal/companies"
	"github.com/alfainsight/backend/internal/db/dbq"
	"github.com/alfainsight/backend/internal/platform/httpx"
	"github.com/alfainsight/backend/internal/roles"
)

// Personalization — настройки ленты, которые пользователь меняет сам.
type Personalization struct {
	MinRelevance    int    `json:"min_relevance"`
	IncludeResearch bool   `json:"include_research"`
	Horizon         string `json:"horizon"` // short | medium | long
	Detail          string `json:"detail"`  // brief | detailed
}

func DefaultPersonalization() Personalization {
	return Personalization{MinRelevance: 0, IncludeResearch: true, Horizon: "medium", Detail: "detailed"}
}

func ParsePersonalization(raw []byte) Personalization {
	p := DefaultPersonalization()
	if len(raw) > 0 {
		_ = json.Unmarshal(raw, &p)
	}
	return p
}

type Handler struct {
	Pool *pgxpool.Pool
	Q    *dbq.Queries
	Svc  *companies.Service
	Log  *slog.Logger
	// OnProfileChanged вызывается после изменения роли/интересов/компании,
	// чтобы пересчитать персональные оценки.
	OnProfileChanged func(userID uuid.UUID)
}

func (h *Handler) Routes(r chi.Router) {
	r.Get("/me", h.me)
	r.Put("/me/profile", h.updateProfile)
	r.Put("/me/interests", h.updateInterests)
	r.Put("/me/personalization", h.updatePersonalization)
	r.Put("/me/avatar", h.updateAvatar)
	r.Delete("/me/avatar", h.deleteAvatar)
	r.Post("/me/companies", h.attachCompany)
	r.Post("/me/onboarding/complete", h.completeOnboarding)
	r.Get("/topics", h.topics)
	r.Get("/roles", h.roles)
}

func (h *Handler) me(w http.ResponseWriter, r *http.Request) {
	uid := auth.UserID(r.Context())
	u, err := h.Q.GetUserByID(r.Context(), uid)
	if errors.Is(err, pgx.ErrNoRows) {
		httpx.Error(w, http.StatusUnauthorized, "unauthorized", "Пользователь не найден")
		return
	}
	if err != nil {
		httpx.Internal(w, h.Log, err, "get me")
		return
	}
	interests, err := h.Q.ListUserInterests(r.Context(), uid)
	if err != nil {
		httpx.Internal(w, h.Log, err, "interests")
		return
	}
	comps, err := h.Q.ListUserCompanies(r.Context(), uid)
	if err != nil {
		httpx.Internal(w, h.Log, err, "user companies")
		return
	}
	role, _ := roles.Get(u.RoleCode)
	httpx.JSON(w, http.StatusOK, map[string]any{
		"id": u.ID, "email": u.Email, "full_name": u.FullName, "role_code": u.RoleCode, "role": role,
		"position_title": u.PositionTitle, "work_focus": u.WorkFocus,
		"personalization": ParsePersonalization(u.Personalization), "onboarding_completed": u.OnboardingCompleted,
		"interests": interests, "companies": comps, "created_at": u.CreatedAt, "avatar": u.Avatar,
	})
}

type profileReq struct {
	FullName      string `json:"full_name"`
	RoleCode      string `json:"role_code"`
	PositionTitle string `json:"position_title"`
	WorkFocus     string `json:"work_focus"`
}

func ValidateProfile(req *profileReq) map[string]string {
	errs := map[string]string{}
	req.FullName = strings.TrimSpace(req.FullName)
	req.PositionTitle = strings.TrimSpace(req.PositionTitle)
	req.WorkFocus = strings.TrimSpace(req.WorkFocus)
	if n := utf8.RuneCountInString(req.FullName); n < 2 || n > 120 {
		errs["full_name"] = "Укажите имя (от 2 до 120 символов)"
	}
	if _, ok := roles.Get(req.RoleCode); !ok {
		errs["role_code"] = "Выберите роль из списка"
	}
	// Должность отдельно не запрашивается: для «Иного сотрудника» пользователь
	// сам описывает свою роль, для остальных используется название роли.
	if req.RoleCode == "other" {
		if n := utf8.RuneCountInString(req.PositionTitle); n < 2 {
			errs["position_title"] = "Опишите вашу роль"
		}
	} else if r, ok := roles.Get(req.RoleCode); ok {
		req.PositionTitle = r.Title
	}
	if utf8.RuneCountInString(req.PositionTitle) > 120 {
		errs["position_title"] = "Не более 120 символов"
	}
	if utf8.RuneCountInString(req.WorkFocus) > 1000 {
		errs["work_focus"] = "Не более 1000 символов"
	}
	return errs
}

func (h *Handler) updateProfile(w http.ResponseWriter, r *http.Request) {
	var req profileReq
	if !httpx.DecodeOrFail(w, r, &req) {
		return
	}
	if errs := ValidateProfile(&req); len(errs) > 0 {
		httpx.ValidationError(w, errs)
		return
	}
	uid := auth.UserID(r.Context())
	prev, err := h.Q.GetUserByID(r.Context(), uid)
	if err != nil {
		httpx.Internal(w, h.Log, err, "get user")
		return
	}
	if req.PositionTitle == "" {
		req.PositionTitle = roles.Resolve(req.RoleCode).Title
	}
	u, err := h.Q.UpdateUserProfile(r.Context(), dbq.UpdateUserProfileParams{
		ID: uid, FullName: req.FullName, RoleCode: req.RoleCode, PositionTitle: req.PositionTitle, WorkFocus: req.WorkFocus,
	})
	if err != nil {
		httpx.Internal(w, h.Log, err, "update profile")
		return
	}
	if prev.RoleCode != u.RoleCode || prev.WorkFocus != u.WorkFocus {
		h.changed(uid)
	}
	httpx.JSON(w, http.StatusOK, map[string]any{"ok": true})
}

type interestReq struct {
	Topics []struct {
		Code   string `json:"code"`
		Weight int    `json:"weight"`
	} `json:"topics"`
}

func (h *Handler) updateInterests(w http.ResponseWriter, r *http.Request) {
	var req interestReq
	if !httpx.DecodeOrFail(w, r, &req) {
		return
	}
	if len(req.Topics) > 23 {
		httpx.ValidationError(w, map[string]string{"topics": "Слишком много тем"})
		return
	}
	known, err := h.Q.ListTopics(r.Context())
	if err != nil {
		httpx.Internal(w, h.Log, err, "topics")
		return
	}
	valid := map[string]bool{}
	for _, t := range known {
		valid[t.Code] = true
	}
	for _, t := range req.Topics {
		if !valid[t.Code] {
			httpx.ValidationError(w, map[string]string{"topics": "Неизвестная тема: " + t.Code})
			return
		}
	}
	uid := auth.UserID(r.Context())
	tx, err := h.Pool.Begin(r.Context())
	if err != nil {
		httpx.Internal(w, h.Log, err, "begin")
		return
	}
	defer func() { _ = tx.Rollback(r.Context()) }()
	q := h.Q.WithTx(tx)
	if err := q.DeleteUserInterests(r.Context(), uid); err != nil {
		httpx.Internal(w, h.Log, err, "delete interests")
		return
	}
	for _, t := range req.Topics {
		wt := t.Weight
		if wt < 1 || wt > 3 {
			wt = 1
		}
		if err := q.AddUserInterest(r.Context(), dbq.AddUserInterestParams{UserID: uid, TopicCode: t.Code, Weight: int16(wt)}); err != nil {
			httpx.Internal(w, h.Log, err, "add interest")
			return
		}
	}
	if err := tx.Commit(r.Context()); err != nil {
		httpx.Internal(w, h.Log, err, "commit")
		return
	}
	h.changed(uid)
	httpx.JSON(w, http.StatusOK, map[string]any{"ok": true})
}

func (h *Handler) updatePersonalization(w http.ResponseWriter, r *http.Request) {
	var p Personalization
	if !httpx.DecodeOrFail(w, r, &p) {
		return
	}
	errs := map[string]string{}
	if p.MinRelevance < 0 || p.MinRelevance > 100 {
		errs["min_relevance"] = "Значение от 0 до 100"
	}
	if p.Horizon != "short" && p.Horizon != "medium" && p.Horizon != "long" {
		errs["horizon"] = "Допустимо: short, medium, long"
	}
	if p.Detail != "brief" && p.Detail != "detailed" {
		errs["detail"] = "Допустимо: brief, detailed"
	}
	if len(errs) > 0 {
		httpx.ValidationError(w, errs)
		return
	}
	raw, _ := json.Marshal(p)
	if _, err := h.Q.UpdateUserPersonalization(r.Context(), dbq.UpdateUserPersonalizationParams{
		ID: auth.UserID(r.Context()), Personalization: raw}); err != nil {
		httpx.Internal(w, h.Log, err, "personalization")
		return
	}
	httpx.JSON(w, http.StatusOK, p)
}

type attachReq struct {
	CompanyID   string                `json:"company_id"`
	DisplayName string                `json:"display_name"`
	Profile     companies.UserProfile `json:"profile"`
}

// attachCompany привязывает пользователя к компании из датасета и сохраняет уточнения.
func (h *Handler) attachCompany(w http.ResponseWriter, r *http.Request) {
	var req attachReq
	if !httpx.DecodeOrFail(w, r, &req) {
		return
	}
	req.CompanyID = strings.TrimSpace(req.CompanyID)
	if req.CompanyID == "" {
		httpx.ValidationError(w, map[string]string{"company_id": "Выберите компанию"})
		return
	}
	upd := companies.UpdateReq{DisplayName: req.DisplayName, Profile: req.Profile}
	if errs := companies.ValidateUpdate(&upd); len(errs) > 0 {
		httpx.ValidationError(w, errs)
		return
	}
	if _, err := h.Q.GetCompany(r.Context(), req.CompanyID); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			httpx.ValidationError(w, map[string]string{"company_id": "Компания не найдена в данных"})
			return
		}
		httpx.Internal(w, h.Log, err, "get company")
		return
	}
	uid := auth.UserID(r.Context())
	tx, err := h.Pool.Begin(r.Context())
	if err != nil {
		httpx.Internal(w, h.Log, err, "begin")
		return
	}
	defer func() { _ = tx.Rollback(r.Context()) }()
	q := h.Q.WithTx(tx)
	if err := q.ClearPrimaryMembership(r.Context(), uid); err != nil {
		httpx.Internal(w, h.Log, err, "clear primary")
		return
	}
	if err := q.AddMembership(r.Context(), dbq.AddMembershipParams{UserID: uid, CompanyID: req.CompanyID, IsPrimary: true}); err != nil {
		httpx.Internal(w, h.Log, err, "add membership")
		return
	}
	raw, _ := json.Marshal(upd.Profile)
	var name *string
	if upd.DisplayName != "" {
		name = &upd.DisplayName
	}
	if _, err := q.UpdateCompanyProfile(r.Context(), dbq.UpdateCompanyProfileParams{ID: req.CompanyID, DisplayName: name, Profile: raw}); err != nil {
		httpx.Internal(w, h.Log, err, "update company")
		return
	}
	if err := tx.Commit(r.Context()); err != nil {
		httpx.Internal(w, h.Log, err, "commit")
		return
	}
	h.changed(uid)
	p, err := h.Svc.Load(r.Context(), req.CompanyID, true)
	if err != nil {
		httpx.Internal(w, h.Log, err, "load company")
		return
	}
	httpx.JSON(w, http.StatusOK, p)
}

func (h *Handler) completeOnboarding(w http.ResponseWriter, r *http.Request) {
	uid := auth.UserID(r.Context())
	u, err := h.Q.GetUserByID(r.Context(), uid)
	if err != nil {
		httpx.Internal(w, h.Log, err, "get user")
		return
	}
	errs := map[string]string{}
	if u.RoleCode == "" {
		errs["role_code"] = "Заполните шаг «Пользователь»: выберите роль"
	}
	if _, err := h.Q.GetPrimaryCompanyID(r.Context(), uid); err != nil {
		errs["company_id"] = "Заполните шаг «Компания»: выберите компанию"
	}
	if len(errs) > 0 {
		httpx.ValidationError(w, errs)
		return
	}
	if err := h.Q.SetOnboardingCompleted(r.Context(), dbq.SetOnboardingCompletedParams{ID: uid, Completed: true}); err != nil {
		httpx.Internal(w, h.Log, err, "complete onboarding")
		return
	}
	h.changed(uid)
	httpx.JSON(w, http.StatusOK, map[string]any{"ok": true})
}

func (h *Handler) topics(w http.ResponseWriter, r *http.Request) {
	t, err := h.Q.ListTopics(r.Context())
	if err != nil {
		httpx.Internal(w, h.Log, err, "topics")
		return
	}
	httpx.JSON(w, http.StatusOK, map[string]any{"items": t})
}

func (h *Handler) roles(w http.ResponseWriter, _ *http.Request) {
	httpx.JSON(w, http.StatusOK, map[string]any{"items": roles.All})
}

func (h *Handler) changed(uid uuid.UUID) {
	if h.OnProfileChanged != nil {
		h.OnProfileChanged(uid)
	}
}

// Фото профиля: data URL изображения (JPEG/PNG/WebP), уменьшенного на клиенте.
const maxAvatarBytes = 400 * 1024

var avatarPrefixes = []string{"data:image/jpeg;base64,", "data:image/png;base64,", "data:image/webp;base64,"}

func ValidateAvatar(dataURL string) string {
	if len(dataURL) > maxAvatarBytes {
		return "Изображение слишком большое (до 300 КБ)"
	}
	for _, p := range avatarPrefixes {
		if strings.HasPrefix(dataURL, p) {
			b64 := dataURL[len(p):]
			if _, err := base64.StdEncoding.DecodeString(b64); err != nil || len(b64) < 100 {
				return "Не удалось прочитать изображение"
			}
			return ""
		}
	}
	return "Поддерживаются JPEG, PNG и WebP"
}

func (h *Handler) updateAvatar(w http.ResponseWriter, r *http.Request) {
	var req struct {
		DataURL string `json:"data_url"`
	}
	if !httpx.DecodeOrFail(w, r, &req) {
		return
	}
	if msg := ValidateAvatar(req.DataURL); msg != "" {
		httpx.ValidationError(w, map[string]string{"avatar": msg})
		return
	}
	if err := h.Q.UpdateUserAvatar(r.Context(), dbq.UpdateUserAvatarParams{ID: auth.UserID(r.Context()), Avatar: &req.DataURL}); err != nil {
		httpx.Internal(w, h.Log, err, "avatar")
		return
	}
	httpx.JSON(w, http.StatusOK, map[string]any{"ok": true})
}

func (h *Handler) deleteAvatar(w http.ResponseWriter, r *http.Request) {
	if err := h.Q.UpdateUserAvatar(r.Context(), dbq.UpdateUserAvatarParams{ID: auth.UserID(r.Context())}); err != nil {
		httpx.Internal(w, h.Log, err, "avatar")
		return
	}
	httpx.JSON(w, http.StatusOK, map[string]any{"ok": true})
}
