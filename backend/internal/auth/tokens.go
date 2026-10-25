package auth

import (
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"errors"
	"time"

	"github.com/golang-jwt/jwt/v5"
	"github.com/google/uuid"
)

const issuer = "alfainsight"

var (
	ErrTokenExpired = errors.New("token expired")
	ErrTokenInvalid = errors.New("token invalid")
)

type Claims struct {
	jwt.RegisteredClaims
	Type string `json:"typ"`
}

type TokenIssuer struct {
	secret []byte
	ttl    time.Duration
	now    func() time.Time
}

func NewTokenIssuer(secret []byte, ttl time.Duration) *TokenIssuer {
	return &TokenIssuer{secret: secret, ttl: ttl, now: time.Now}
}

// IssueAccess выпускает короткоживущий JWT (HS256) для HttpOnly-cookie.
func (t *TokenIssuer) IssueAccess(userID uuid.UUID) (string, time.Time, error) {
	now := t.now()
	exp := now.Add(t.ttl)
	claims := Claims{
		RegisteredClaims: jwt.RegisteredClaims{
			Issuer:    issuer,
			Subject:   userID.String(),
			IssuedAt:  jwt.NewNumericDate(now),
			ExpiresAt: jwt.NewNumericDate(exp),
			ID:        uuid.NewString(),
		},
		Type: "access",
	}
	s, err := jwt.NewWithClaims(jwt.SigningMethodHS256, claims).SignedString(t.secret)
	return s, exp, err
}

// ParseAccess проверяет подпись, алгоритм, издателя, срок и тип токена.
func (t *TokenIssuer) ParseAccess(token string) (uuid.UUID, error) {
	var c Claims
	_, err := jwt.ParseWithClaims(token, &c, func(tok *jwt.Token) (any, error) {
		return t.secret, nil
	}, jwt.WithValidMethods([]string{jwt.SigningMethodHS256.Alg()}), jwt.WithIssuer(issuer),
		jwt.WithTimeFunc(t.now), jwt.WithExpirationRequired())
	if err != nil {
		if errors.Is(err, jwt.ErrTokenExpired) {
			return uuid.Nil, ErrTokenExpired
		}
		return uuid.Nil, ErrTokenInvalid
	}
	if c.Type != "access" {
		return uuid.Nil, ErrTokenInvalid
	}
	id, err := uuid.Parse(c.Subject)
	if err != nil {
		return uuid.Nil, ErrTokenInvalid
	}
	return id, nil
}

// NewRefreshToken возвращает случайный токен и его SHA-256 хеш для хранения в БД.
func NewRefreshToken() (token, hash string, err error) {
	b := make([]byte, 32)
	if _, err = rand.Read(b); err != nil {
		return "", "", err
	}
	token = base64.RawURLEncoding.EncodeToString(b)
	return token, HashToken(token), nil
}

func HashToken(token string) string {
	sum := sha256.Sum256([]byte(token))
	return hex.EncodeToString(sum[:])
}
