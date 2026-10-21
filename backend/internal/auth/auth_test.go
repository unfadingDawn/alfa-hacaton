package auth

import (
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
)

func TestPasswordHash(t *testing.T) {
	h, err := HashPassword("Secret123")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.HasPrefix(h, "$argon2id$") || strings.Contains(h, "Secret123") {
		t.Fatalf("неожиданный формат хеша: %s", h)
	}
	if ok, _ := VerifyPassword("Secret123", h); !ok {
		t.Fatal("верный пароль не принят")
	}
	if ok, _ := VerifyPassword("secret123", h); ok {
		t.Fatal("неверный пароль принят")
	}
	h2, _ := HashPassword("Secret123")
	if h == h2 {
		t.Fatal("соль должна быть случайной")
	}
	if _, err := VerifyPassword("x", "plain"); err == nil {
		t.Fatal("некорректный хеш должен давать ошибку")
	}
}

func TestAccessToken(t *testing.T) {
	ti := NewTokenIssuer([]byte(strings.Repeat("k", 32)), time.Minute)
	id := uuid.New()
	tok, _, err := ti.IssueAccess(id)
	if err != nil {
		t.Fatal(err)
	}
	got, err := ti.ParseAccess(tok)
	if err != nil || got != id {
		t.Fatalf("parse: %v %v", got, err)
	}
	other := NewTokenIssuer([]byte(strings.Repeat("x", 32)), time.Minute)
	if _, err := other.ParseAccess(tok); err != ErrTokenInvalid {
		t.Fatal("токен с чужой подписью должен отклоняться")
	}
	ti.now = func() time.Time { return time.Now().Add(2 * time.Minute) }
	if _, err := ti.ParseAccess(tok); err != ErrTokenExpired {
		t.Fatalf("просроченный токен: %v", err)
	}
}

func TestRefreshTokenHash(t *testing.T) {
	tok, hash, err := NewRefreshToken()
	if err != nil || HashToken(tok) != hash || strings.Contains(hash, tok) {
		t.Fatal("в БД хранится только хеш refresh-токена")
	}
}

func TestValidateRegistration(t *testing.T) {
	if errs := ValidateRegistration(registerReq{Email: "a@b.ru", Password: "abcdefg1", FullName: "Иван"}); len(errs) != 0 {
		t.Fatalf("валидные данные отклонены: %v", errs)
	}
	errs := ValidateRegistration(registerReq{Email: "not-an-email", Password: "onlyletters", FullName: "И"})
	for _, f := range []string{"email", "password", "full_name"} {
		if errs[f] == "" {
			t.Errorf("нет ошибки для %s", f)
		}
	}
}

func TestRateLimiter(t *testing.T) {
	rl := NewRateLimiter(2, time.Minute)
	if !rl.Allow("ip") || !rl.Allow("ip") || rl.Allow("ip") {
		t.Fatal("третья попытка должна блокироваться")
	}
	if !rl.Allow("other") {
		t.Fatal("лимит считается по ключу")
	}
}
