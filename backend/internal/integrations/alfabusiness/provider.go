// Package alfabusiness — контракт будущей интеграции «Вход из Альфа-Бизнеса».
//
// По ответам организаторов подключение к закрытым системам банка в рамках
// хакатона не требуется, а пользователь в интернет-банке уже авторизован и
// его полномочия проверены. Поэтому здесь описан только контракт: что сервис
// ожидает получить от банка и как это ложится на модель данных AlfaInsight.
// Реальной реализации нет — NotConnected честно сообщает об этом, ничего не
// подставляя.
//
// Поток при внедрении:
//  1. Пользователь нажимает «Вход из Альфа-Бизнеса» (или открывает AlfaInsight
//     из меню интернет-банка) — банк передаёт одноразовый код/токен.
//  2. Provider.Exchange обменивает код на сведения о пользователе и компаниях.
//  3. Пользователь находится или создаётся по ExternalUserID; компании
//     сопоставляются со справочником по ClientID; сведения кабинета
//     сохраняются в companies.profile->'business' с Source = "alfa_business".
package alfabusiness

import (
	"context"
	"errors"

	"github.com/alfainsight/backend/internal/companies"
)

var ErrNotConnected = errors.New("вход из Альфа-Бизнеса появится после подключения к интернет-банку; сейчас войдите по email")

// User — пользователь интернет-банка (физическое лицо).
type User struct {
	ExternalUserID string `json:"external_user_id"`
	FullName       string `json:"full_name"` // «Набоков И.Д.»
	Email          string `json:"email"`
	Phone          string `json:"phone"`
	AvatarURL      string `json:"avatar_url"`
}

// Company — юрлицо, доступное пользователю в кабинете («Мои компании»).
type Company struct {
	ClientID  string                    `json:"client_id"` // идентификатор клиента банка (в данных кейса — SYNTHETIC_U)
	IsCurrent bool                      `json:"is_current"`
	Business  companies.BusinessProfile `json:"business"`
}

type Session struct {
	User      User      `json:"user"`
	Companies []Company `json:"companies"`
}

// Provider — источник сведений из интернет-банка.
type Provider interface {
	// Exchange обменивает одноразовый код банка на сведения о пользователе и компаниях.
	Exchange(ctx context.Context, code string) (Session, error)
	Connected() bool
}

// NotConnected — используется, пока интеграции нет.
type NotConnected struct{}

func (NotConnected) Exchange(context.Context, string) (Session, error) {
	return Session{}, ErrNotConnected
}
func (NotConnected) Connected() bool { return false }

// ApplyToProfile переносит сведения кабинета в профиль компании AlfaInsight,
// сохраняя то, что пользователь ввёл сам (продукты, рынки, приоритеты и т. д.).
func ApplyToProfile(dst *companies.UserProfile, c Company) {
	b := c.Business
	b.Source = "alfa_business"
	dst.Business = &b
}
