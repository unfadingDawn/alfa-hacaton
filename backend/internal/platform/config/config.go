// Package config читает конфигурацию только из переменных окружения.
// Секреты (JWT_SECRET, LLM_API_KEY) никогда не логируются: String() их маскирует.
package config

import (
	"errors"
	"fmt"
	"os"
	"strconv"
	"strings"
	"time"
)

type LLM struct {
	BaseURL          string
	APIKey           string
	ModelFast        string // классификация, извлечение, ранжирование
	ModelReasoning   string // планирование и самопроверка агента
	ModelWriter      string // формирование персонального анализа
	Timeout          time.Duration
	DailyTokenBudget int64
	DailyCallLimit   int
	PerMinuteLimit   int
	MaxOutputTokens  int
	MaxInputChars    int
	PerUserHourly    int
	JSONMode         bool
}

// Configured сообщает, заданы ли адрес и ключ. Без них анализ моделью недоступен.
func (l LLM) Configured() bool { return l.BaseURL != "" && l.APIKey != "" }

type Config struct {
	DatabaseURL     string
	HTTPAddr        string
	JWTSecret       []byte
	AccessTTL       time.Duration
	RefreshTTL      time.Duration
	CookieSecure    bool
	AllowedOrigins  []string
	LLM             LLM
	NewsFetchEnable bool
	NewsInterval    time.Duration
	NewsInitialDays int
	Timezone        *time.Location
}

func Load() (Config, error) {
	var errs []string
	c := Config{
		DatabaseURL:     env("DATABASE_URL", ""),
		HTTPAddr:        env("HTTP_ADDR", ":8080"),
		JWTSecret:       []byte(env("JWT_SECRET", "")),
		AccessTTL:       envDuration("ACCESS_TOKEN_TTL", 15*time.Minute),
		RefreshTTL:      envDuration("REFRESH_TOKEN_TTL", 30*24*time.Hour),
		CookieSecure:    envBool("COOKIE_SECURE", false),
		AllowedOrigins:  splitList(env("ALLOWED_ORIGINS", "http://localhost:3000")),
		NewsFetchEnable: envBool("NEWS_FETCH_ENABLED", true),
		NewsInterval:    envDuration("NEWS_FETCH_INTERVAL", 60*time.Minute),
		NewsInitialDays: envInt("NEWS_INITIAL_DAYS", 7),
		LLM: LLM{
			BaseURL:          strings.TrimRight(env("LLM_BASE_URL", ""), "/"),
			APIKey:           env("LLM_API_KEY", ""),
			ModelFast:        env("LLM_MODEL_FAST", ""),
			ModelReasoning:   env("LLM_MODEL_REASONING", ""),
			ModelWriter:      env("LLM_MODEL_WRITER", ""),
			Timeout:          envDuration("LLM_TIMEOUT", 90*time.Second),
			DailyTokenBudget: int64(envInt("LLM_DAILY_TOKEN_BUDGET", 300_000)),
			DailyCallLimit:   envInt("LLM_DAILY_CALL_LIMIT", 80),
			PerMinuteLimit:   envInt("LLM_PER_MINUTE_LIMIT", 6),
			MaxOutputTokens:  envInt("LLM_MAX_OUTPUT_TOKENS", 2200),
			MaxInputChars:    envInt("LLM_MAX_INPUT_CHARS", 24000),
			PerUserHourly:    envInt("ANALYZE_PER_USER_HOURLY", 10),
			JSONMode:         envBool("LLM_JSON_MODE", true),
		},
	}
	tz, err := time.LoadLocation(env("APP_TIMEZONE", "Europe/Moscow"))
	if err != nil {
		tz = time.FixedZone("MSK", 3*3600)
	}
	c.Timezone = tz

	if c.DatabaseURL == "" {
		errs = append(errs, "DATABASE_URL не задан")
	}
	if len(c.JWTSecret) < 32 {
		errs = append(errs, "JWT_SECRET должен содержать не менее 32 символов")
	}
	// Если модели не заданы явно, используется одна writer-модель для всех шагов.
	if c.LLM.ModelWriter == "" {
		c.LLM.ModelWriter = c.LLM.ModelReasoning
	}
	if c.LLM.ModelFast == "" {
		c.LLM.ModelFast = c.LLM.ModelWriter
	}
	if c.LLM.ModelReasoning == "" {
		c.LLM.ModelReasoning = c.LLM.ModelWriter
	}
	if len(errs) > 0 {
		return c, errors.New(strings.Join(errs, "; "))
	}
	return c, nil
}

// String возвращает безопасное для логов описание конфигурации.
func (c Config) String() string {
	return fmt.Sprintf("http=%s llm_configured=%t llm_base=%s models=[fast=%s reasoning=%s writer=%s] news_fetch=%t interval=%s",
		c.HTTPAddr, c.LLM.Configured(), redactURL(c.LLM.BaseURL), c.LLM.ModelFast, c.LLM.ModelReasoning, c.LLM.ModelWriter,
		c.NewsFetchEnable, c.NewsInterval)
}

func redactURL(u string) string {
	if u == "" {
		return "(не задан)"
	}
	// Отбрасываем возможные креды в URL вида https://user:pass@host.
	if i := strings.Index(u, "@"); i > 0 {
		if j := strings.Index(u, "://"); j > 0 && j < i {
			return u[:j+3] + "***@" + u[i+1:]
		}
	}
	return u
}

func env(k, def string) string {
	if v, ok := os.LookupEnv(k); ok && strings.TrimSpace(v) != "" {
		return strings.TrimSpace(v)
	}
	return def
}

func envBool(k string, def bool) bool {
	v, err := strconv.ParseBool(env(k, strconv.FormatBool(def)))
	if err != nil {
		return def
	}
	return v
}

func envInt(k string, def int) int {
	v, err := strconv.Atoi(env(k, strconv.Itoa(def)))
	if err != nil {
		return def
	}
	return v
}

func envDuration(k string, def time.Duration) time.Duration {
	v, err := time.ParseDuration(env(k, def.String()))
	if err != nil {
		return def
	}
	return v
}

func splitList(s string) []string {
	var out []string
	for _, p := range strings.Split(s, ",") {
		if p = strings.TrimSpace(p); p != "" {
			out = append(out, p)
		}
	}
	return out
}
