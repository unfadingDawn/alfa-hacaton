// importer загружает данные кейса в PostgreSQL:
//
//	go run ./cmd/importer -data ../data/raw -research ../data/research
//
// Повторный запуск безопасен: компании обновляются (уточнения пользователей
// сохраняются), связи и платежи перезагружаются, материалы обновляются.
package main

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"path/filepath"
	"time"

	"github.com/alfainsight/backend/internal/db/dbq"
	"github.com/alfainsight/backend/internal/importer"
	"github.com/alfainsight/backend/internal/news"
	"github.com/alfainsight/backend/internal/platform/db"
	"github.com/alfainsight/backend/internal/platform/logx"
	"github.com/alfainsight/backend/migrations"
)

func main() {
	dataDir := flag.String("data", "../data/raw", "папка с CSV кейса")
	researchDir := flag.String("research", "../data/research", "папка с текстами аналитических материалов")
	only := flag.String("only", "", "companies|links|payments|research|news — загрузить только один набор")
	withNews := flag.Bool("news", false, "также один раз опросить RSS-ленты (без обращений к модели)")
	flag.Parse()
	log := logx.New(os.Stderr, slog.LevelInfo)

	url := os.Getenv("DATABASE_URL")
	if url == "" {
		fmt.Fprintln(os.Stderr, "DATABASE_URL не задан")
		os.Exit(2)
	}
	ctx := context.Background()
	pool, err := db.Connect(ctx, url)
	if err != nil {
		log.Error("db", "err", err)
		os.Exit(1)
	}
	defer pool.Close()
	if err := db.Migrate(pool, migrations.FS); err != nil {
		log.Error("migrate", "err", err)
		os.Exit(1)
	}
	l := &importer.Loader{Pool: pool, Q: dbq.New(pool), Log: log}
	steps := []struct {
		name string
		run  func() (importer.Report, error)
	}{
		{"companies", func() (importer.Report, error) {
			return l.Companies(ctx, filepath.Join(*dataDir, "df_Uid_data_hack.csv"))
		}},
		{"links", func() (importer.Report, error) {
			return l.PersonLinks(ctx, filepath.Join(*dataDir, "df_Fid_Uid_link_hack.csv"))
		}},
		{"payments", func() (importer.Report, error) {
			return l.Payments(ctx, filepath.Join(*dataDir, "df_Uid_payment_hack.csv"))
		}},
		{"research", func() (importer.Report, error) { return l.Research(ctx, *researchDir) }},
	}
	for _, s := range steps {
		if (*only != "" && *only != s.name) || *only == "news" {
			continue
		}
		rep, err := s.run()
		if err != nil {
			log.Error("import failed", "step", s.name, "err", err)
			os.Exit(1)
		}
		fmt.Printf("%-9s %s\n", s.name, rep)
	}
	if *withNews || *only == "news" {
		in := &news.Ingestor{Q: dbq.New(pool), Log: log, HTTP: &http.Client{Timeout: 30 * time.Second}, InitialDays: 7}
		for _, r := range in.RunOnce(ctx) {
			b, _ := json.Marshal(r)
			fmt.Printf("news      %s\n", b)
		}
	}
}
