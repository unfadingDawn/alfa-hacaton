// Package db открывает пул соединений и применяет миграции goose.
package db

import (
	"context"
	"database/sql"
	"fmt"
	"io/fs"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/jackc/pgx/v5/stdlib"
	"github.com/pressly/goose/v3"
)

func Connect(ctx context.Context, url string) (*pgxpool.Pool, error) {
	cfg, err := pgxpool.ParseConfig(url)
	if err != nil {
		return nil, fmt.Errorf("parse DATABASE_URL: %w", err)
	}
	cfg.MaxConns = 15
	cfg.MaxConnIdleTime = 5 * time.Minute
	var pool *pgxpool.Pool
	// База в docker compose может подниматься дольше API — ждём до ~30 с.
	for attempt := 0; attempt < 15; attempt++ {
		pool, err = pgxpool.NewWithConfig(ctx, cfg)
		if err == nil {
			if err = pool.Ping(ctx); err == nil {
				return pool, nil
			}
			pool.Close()
		}
		select {
		case <-ctx.Done():
			return nil, ctx.Err()
		case <-time.After(2 * time.Second):
		}
	}
	return nil, fmt.Errorf("database unavailable: %w", err)
}

// Migrate применяет миграции goose из встроенной файловой системы.
func Migrate(pool *pgxpool.Pool, migrations fs.FS) error {
	sqlDB := stdlib.OpenDBFromPool(pool)
	defer func(db *sql.DB) { _ = db.Close() }(sqlDB)
	goose.SetBaseFS(migrations)
	goose.SetLogger(goose.NopLogger())
	if err := goose.SetDialect("postgres"); err != nil {
		return err
	}
	return goose.Up(sqlDB, ".")
}
