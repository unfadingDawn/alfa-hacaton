package companies

import (
	"encoding/json"
	"errors"
	"log/slog"
	"net/http"
	"strconv"
	"strings"
	"unicode/utf8"

	"github.com/go-chi/chi/v5"
	"github.com/jackc/pgx/v5"

	"github.com/alfainsight/backend/internal/auth"
	"github.com/alfainsight/backend/internal/db/dbq"
	"github.com/alfainsight/backend/internal/platform/httpx"
)

type Handler struct {
	Q   *dbq.Queries
	Svc *Service
	Log *slog.Logger
}

func (h *Handler) Routes(r chi.Router) {
	r.Get("/companies/search", h.search)
	r.Get("/companies/regions", h.regions)
	r.Get("/companies/{id}/preview", h.preview)
	r.Get("/companies/{id}", h.get)
	r.Put("/companies/{id}", h.update)
}

// RequireMember проверяет, что пользователь состоит в компании.
func RequireMember(w http.ResponseWriter, r *http.Request, q *dbq.Queries, log *slog.Logger, companyID string) bool {
	ok, err := q.IsMember(r.Context(), dbq.IsMemberParams{UserID: auth.UserID(r.Context()), CompanyID: companyID})
	if err != nil {
		httpx.Internal(w, log, err, "is member")
		return false
	}
	if !ok {
		httpx.Error(w, http.StatusForbidden, "forbidden", "Нет доступа к данным этой компании")
		return false
	}
	return true
}

func (h *Handler) search(w http.ResponseWriter, r *http.Request) {
	params := dbq.SearchCompaniesParams{
		Q:           httpx.QueryStr(r, "q"),
		Region:      httpx.QueryStr(r, "region"),
		Lim:         int32(httpx.QueryInt(r, "limit", 20, 1, 50)),
		ClientsOnly: r.URL.Query().Get("clients_only") == "true",
		Off:         int32(httpx.QueryInt(r, "offset", 0, 0, 100000)),
	}
	if v := httpx.QueryStr(r, "min_revenue"); v != nil {
		if f, err := strconv.ParseFloat(*v, 64); err == nil && f >= 0 {
			params.MinRevenue = &f
		}
	}
	rows, err := h.Q.SearchCompanies(r.Context(), params)
	if err != nil {
		httpx.Internal(w, h.Log, err, "search companies")
		return
	}
	items := make([]map[string]any, 0, len(rows))
	for _, c := range rows {
		segRev := c.GroupRevenue
		if segRev == nil {
			segRev = c.Revenue
		}
		seg, large := sizeSegment(segRev)
		items = append(items, map[string]any{
			"id": c.ID, "display_name": c.DisplayName, "okved_code": c.OkvedCode, "okved_name": c.OkvedName,
			"region": c.Region, "city": c.City, "revenue": c.Revenue, "group_revenue": c.GroupRevenue, "group_size": c.GroupSize,
			"size_segment": seg, "is_large": large,
			"client_type": c.ClientType, "eco_stage": c.EcoStage, "workers_fns": c.WorkersFns,
			"section": SectionOf(c.OkvedCode),
		})
	}
	httpx.JSON(w, http.StatusOK, map[string]any{"items": items})
}

func (h *Handler) regions(w http.ResponseWriter, r *http.Request) {
	rows, err := h.Q.ListCompanyRegions(r.Context())
	if err != nil {
		httpx.Internal(w, h.Log, err, "regions")
		return
	}
	httpx.JSON(w, http.StatusOK, map[string]any{"items": rows})
}

// preview — исходные поля обезличенного датасета для выбора компании при
// онбординге (без платёжных связей).
func (h *Handler) preview(w http.ResponseWriter, r *http.Request) {
	p, err := h.Svc.Load(r.Context(), chi.URLParam(r, "id"), false)
	if errors.Is(err, pgx.ErrNoRows) {
		httpx.Error(w, http.StatusNotFound, "not_found", "Компания не найдена в данных")
		return
	}
	if err != nil {
		httpx.Internal(w, h.Log, err, "company preview")
		return
	}
	httpx.JSON(w, http.StatusOK, p)
}

func (h *Handler) get(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	if !RequireMember(w, r, h.Q, h.Log, id) {
		return
	}
	p, err := h.Svc.Load(r.Context(), id, true)
	if errors.Is(err, pgx.ErrNoRows) {
		httpx.Error(w, http.StatusNotFound, "not_found", "Компания не найдена")
		return
	}
	if err != nil {
		httpx.Internal(w, h.Log, err, "company profile")
		return
	}
	httpx.JSON(w, http.StatusOK, p)
}

type UpdateReq struct {
	DisplayName string      `json:"display_name"`
	Profile     UserProfile `json:"profile"`
}

// ValidateUpdate проверяет сведения о компании от пользователя. Обязательны
// название, продукты, рынки и поставщики; приоритеты, описание, заметки и
// сведения из кабинета Альфа-Бизнеса — необязательные.
func ValidateUpdate(req *UpdateReq) map[string]string {
	errs := map[string]string{}
	req.DisplayName = strings.TrimSpace(req.DisplayName)
	pr := &req.Profile
	pr.Description, pr.Products = strings.TrimSpace(pr.Description), strings.TrimSpace(pr.Products)
	pr.Markets, pr.Suppliers, pr.Notes = strings.TrimSpace(pr.Markets), strings.TrimSpace(pr.Suppliers), strings.TrimSpace(pr.Notes)
	required := map[string]string{"display_name": req.DisplayName, "products": pr.Products, "markets": pr.Markets, "suppliers": pr.Suppliers}
	for k, v := range required {
		if v == "" {
			errs[k] = "Обязательное поле"
		}
	}
	if n := utf8.RuneCountInString(req.DisplayName); n > 200 {
		errs["display_name"] = "Название не длиннее 200 символов"
	}
	for k, v := range map[string]string{"description": pr.Description, "products": pr.Products,
		"markets": pr.Markets, "suppliers": pr.Suppliers, "notes": pr.Notes} {
		if utf8.RuneCountInString(v) > 2000 {
			errs[k] = "Не более 2000 символов"
		}
	}
	if len(pr.Priorities) > 10 {
		errs["priorities"] = "Не более 10 приоритетов"
	}
	clean := []string{}
	for _, p := range pr.Priorities {
		p = strings.TrimSpace(p)
		if p != "" && utf8.RuneCountInString(p) <= 200 {
			clean = append(clean, p)
		}
	}
	pr.Priorities = clean
	if pr.Business != nil {
		// Через API пользователь всегда вводит сведения сам; источник
		// "alfa_business" выставляет только интеграция с банком.
		for k, v := range ValidateBusiness(pr.Business, "user") {
			errs[k] = v
		}
	}
	return errs
}

func (h *Handler) update(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	if !RequireMember(w, r, h.Q, h.Log, id) {
		return
	}
	var req UpdateReq
	if !httpx.DecodeOrFail(w, r, &req) {
		return
	}
	if errs := ValidateUpdate(&req); len(errs) > 0 {
		httpx.ValidationError(w, errs)
		return
	}
	if err := h.Save(r, id, req); err != nil {
		httpx.Internal(w, h.Log, err, "update company")
		return
	}
	p, err := h.Svc.Load(r.Context(), id, true)
	if err != nil {
		httpx.Internal(w, h.Log, err, "reload company")
		return
	}
	httpx.JSON(w, http.StatusOK, p)
}

// Save записывает пользовательские уточнения (исходные поля не меняются).
func (h *Handler) Save(r *http.Request, id string, req UpdateReq) error {
	raw, _ := json.Marshal(req.Profile)
	var name *string
	if req.DisplayName != "" {
		name = &req.DisplayName
	}
	_, err := h.Q.UpdateCompanyProfile(r.Context(), dbq.UpdateCompanyProfileParams{ID: id, DisplayName: name, Profile: raw})
	return err
}
