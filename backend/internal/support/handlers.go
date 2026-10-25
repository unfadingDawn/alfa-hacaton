package support

import (
	"context"
	"encoding/json"
	"errors"
	"log/slog"
	"net/http"
	"time"

	"github.com/go-chi/chi/v5"

	"github.com/alfainsight/backend/internal/auth"
	"github.com/alfainsight/backend/internal/db/dbq"
	"github.com/alfainsight/backend/internal/platform/httpx"
)

type Handler struct {
	A          *Assistant
	Log        *slog.Logger
	OnAgentErr func(w http.ResponseWriter, err error)
}

func (h *Handler) Routes(r chi.Router) {
	r.Get("/support/assistant", h.history)
	r.Post("/support/assistant", h.ask)
}

func messageView(m dbq.SupportMessage) map[string]any {
	return map[string]any{"id": m.ID, "question": m.Question, "preset": m.Preset, "method": m.Method,
		"answer": json.RawMessage(m.Answer), "model": m.Model, "tokens": m.Tokens, "duration_ms": m.DurationMs,
		"cached": m.Cached, "created_at": m.CreatedAt}
}

func (h *Handler) history(w http.ResponseWriter, r *http.Request) {
	items, left, err := h.A.History(r.Context(), auth.UserID(r.Context()))
	if err != nil {
		httpx.Internal(w, h.Log, err, "support history")
		return
	}
	out := make([]map[string]any, 0, len(items))
	for _, m := range items {
		out = append(out, messageView(m))
	}
	httpx.JSON(w, http.StatusOK, map[string]any{"items": out, "quick_questions": QuickQuestions, "left_today": left,
		"llm_available": h.A.LLM.Configured()})
}

func (h *Handler) ask(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Question string `json:"question"`
		Preset   string `json:"preset"`
	}
	if !httpx.DecodeOrFail(w, r, &req) {
		return
	}
	ctx, cancel := context.WithTimeout(r.Context(), 90*time.Second)
	defer cancel()
	res, err := h.A.Ask(ctx, auth.UserID(r.Context()), req.Question, req.Preset)
	switch {
	case errors.Is(err, ErrQuestionLength):
		httpx.ValidationError(w, map[string]string{"question": "Вопрос от 3 до 500 символов"})
		return
	case errors.Is(err, ErrUnknownPreset):
		httpx.ValidationError(w, map[string]string{"preset": "Неизвестный быстрый вопрос"})
		return
	case errors.Is(err, ErrLimit):
		httpx.Error(w, http.StatusTooManyRequests, "support_limit", "Лимит вопросов помощнику на сегодня исчерпан. Частые вопросы по-прежнему доступны.")
		return
	case err != nil:
		h.OnAgentErr(w, err)
		return
	}
	httpx.JSON(w, http.StatusOK, map[string]any{"item": messageView(res.Message), "cached": res.Cached, "left_today": res.Left})
}
