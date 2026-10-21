// Package httpx — общие помощники для JSON API: ответы, ошибки, разбор тела.
package httpx

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"strconv"
	"strings"
)

type ErrorBody struct {
	Error APIError `json:"error"`
}

type APIError struct {
	Code    string            `json:"code"`
	Message string            `json:"message"`
	Fields  map[string]string `json:"fields,omitempty"`
}

func (e *APIError) Error() string { return e.Code + ": " + e.Message }

func JSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.Header().Set("Cache-Control", "no-store")
	w.WriteHeader(status)
	if v != nil {
		_ = json.NewEncoder(w).Encode(v)
	}
}

func Error(w http.ResponseWriter, status int, code, msg string) {
	JSON(w, status, ErrorBody{Error: APIError{Code: code, Message: msg}})
}

func ValidationError(w http.ResponseWriter, fields map[string]string) {
	JSON(w, http.StatusUnprocessableEntity, ErrorBody{Error: APIError{
		Code: "validation_error", Message: "Проверьте правильность заполнения полей", Fields: fields,
	}})
}

// Internal логирует ошибку и возвращает клиенту обезличенное сообщение.
func Internal(w http.ResponseWriter, log *slog.Logger, err error, where string) {
	log.Error("internal error", "where", where, "err", err)
	Error(w, http.StatusInternalServerError, "internal_error", "Внутренняя ошибка сервера. Попробуйте позже.")
}

const maxBody = 1 << 20

// Decode разбирает JSON-тело с ограничением размера и запретом неизвестных полей.
func Decode(r *http.Request, dst any) error {
	dec := json.NewDecoder(io.LimitReader(r.Body, maxBody))
	dec.DisallowUnknownFields()
	if err := dec.Decode(dst); err != nil {
		var syn *json.SyntaxError
		var typ *json.UnmarshalTypeError
		switch {
		case errors.Is(err, io.EOF):
			return &APIError{Code: "bad_request", Message: "Пустое тело запроса"}
		case errors.As(err, &syn):
			return &APIError{Code: "bad_request", Message: "Некорректный JSON"}
		case errors.As(err, &typ):
			return &APIError{Code: "bad_request", Message: fmt.Sprintf("Неверный тип поля %q", typ.Field)}
		case strings.HasPrefix(err.Error(), "json: unknown field"):
			return &APIError{Code: "bad_request", Message: "Неизвестное поле: " + strings.TrimPrefix(err.Error(), "json: unknown field ")}
		default:
			return &APIError{Code: "bad_request", Message: "Не удалось разобрать запрос"}
		}
	}
	return nil
}

// DecodeOrFail пишет ответ 400 при ошибке разбора и возвращает false.
func DecodeOrFail(w http.ResponseWriter, r *http.Request, dst any) bool {
	if err := Decode(r, dst); err != nil {
		var ae *APIError
		if errors.As(err, &ae) {
			JSON(w, http.StatusBadRequest, ErrorBody{Error: *ae})
		} else {
			Error(w, http.StatusBadRequest, "bad_request", "Некорректный запрос")
		}
		return false
	}
	return true
}

func QueryInt(r *http.Request, key string, def, min, max int) int {
	v, err := strconv.Atoi(r.URL.Query().Get(key))
	if err != nil {
		return def
	}
	if v < min {
		return min
	}
	if v > max {
		return max
	}
	return v
}

// QueryStr возвращает nil для пустого параметра (удобно для sqlc.narg).
func QueryStr(r *http.Request, key string) *string {
	v := strings.TrimSpace(r.URL.Query().Get(key))
	if v == "" {
		return nil
	}
	return &v
}
