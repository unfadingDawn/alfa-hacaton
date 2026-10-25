// migrate — управление схемой БД при выкатке и откате версии.
// API применяет миграции вперёд автоматически при старте; откатывать схему
// нужно явно этой утилитой, до запуска предыдущей версии API.
//
//	migrate status          — применённые и ожидающие миграции
//	migrate version         — текущая версия схемы
//	migrate up              — применить все миграции
//	migrate down            — откатить одну последнюю миграцию
//	migrate down-to 7       — откатить до версии 7 включительно
package main

import (
	"context"
	"database/sql"
	"fmt"
	"os"
	"strconv"
	"time"

	"github.com/jackc/pgx/v5/stdlib"
	"github.com/pressly/goose/v3"

	"github.com/alfainsight/backend/internal/platform/db"
	"github.com/alfainsight/backend/migrations"
)

func main() {
	if len(os.Args) < 2 {
		usage()
	}
	url := os.Getenv("DATABASE_URL")
	if url == "" {
		fail(fmt.Errorf("DATABASE_URL не задан"))
	}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
	defer cancel()
	pool, err := db.Connect(ctx, url)
	if err != nil {
		fail(err)
	}
	defer pool.Close()
	sqlDB := stdlib.OpenDBFromPool(pool)
	defer func(d *sql.DB) { _ = d.Close() }(sqlDB)
	goose.SetBaseFS(migrations.FS)
	if err := goose.SetDialect("postgres"); err != nil {
		fail(err)
	}

	switch os.Args[1] {
	case "status":
		err = goose.StatusContext(ctx, sqlDB, ".")
	case "version":
		err = goose.VersionContext(ctx, sqlDB, ".")
	case "up":
		err = goose.UpContext(ctx, sqlDB, ".")
	case "down":
		err = goose.DownContext(ctx, sqlDB, ".")
	case "down-to":
		if len(os.Args) < 3 {
			usage()
		}
		v, perr := strconv.ParseInt(os.Args[2], 10, 64)
		if perr != nil {
			fail(fmt.Errorf("версия должна быть числом: %w", perr))
		}
		err = goose.DownToContext(ctx, sqlDB, ".", v)
	default:
		usage()
	}
	if err != nil {
		fail(err)
	}
}

func usage() {
	fmt.Fprintln(os.Stderr, "использование: migrate status|version|up|down|down-to <версия>")
	os.Exit(2)
}

func fail(err error) {
	fmt.Fprintln(os.Stderr, "migrate:", err)
	os.Exit(1)
}
