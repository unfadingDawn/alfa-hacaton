// api — HTTP API AlfaInsight.
package main

import (
	"context"
	"errors"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/alfainsight/backend/internal/platform/config"
	"github.com/alfainsight/backend/internal/platform/db"
	"github.com/alfainsight/backend/internal/platform/logx"
	"github.com/alfainsight/backend/internal/server"
	"github.com/alfainsight/backend/migrations"
)

func main() {
	log := logx.New(os.Stdout, slog.LevelInfo)
	cfg, err := config.Load()
	if err != nil {
		log.Error("config", "err", err)
		os.Exit(2)
	}
	log.Info("starting", "config", cfg.String())

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	pool, err := db.Connect(ctx, cfg.DatabaseURL)
	if err != nil {
		log.Error("db connect", "err", err)
		os.Exit(1)
	}
	defer pool.Close()
	if err := db.Migrate(pool, migrations.FS); err != nil {
		log.Error("migrate", "err", err)
		os.Exit(1)
	}
	app := server.New(cfg, pool, log)

	// Фоновые задачи без LLM: опрос RSS, темы по словарю и локальная TF-IDF + LSA модель.
	go func() {
		t := time.NewTicker(cfg.NewsInterval)
		defer t.Stop()
		for {
			if cfg.NewsFetchEnable {
				app.Ingestor.RunOnce(ctx)
			}
			if n, err := app.Agent.TagKeywordTopics(ctx); err != nil {
				log.Warn("keyword topics", "err", err)
			} else if n > 0 {
				log.Info("keyword topics tagged", "articles", n)
			}
			if err := app.Agent.RefreshSemanticModel(ctx, false); err != nil {
				log.Warn("semantic model", "err", err)
			}
			if !cfg.NewsFetchEnable {
				return
			}
			select {
			case <-ctx.Done():
				return
			case <-t.C:
			}
		}
	}()

	srv := &http.Server{Addr: cfg.HTTPAddr, Handler: app.Router, ReadHeaderTimeout: 10 * time.Second,
		ReadTimeout: 30 * time.Second, WriteTimeout: 6 * time.Minute, IdleTimeout: 2 * time.Minute}
	go func() {
		log.Info("listening", "addr", cfg.HTTPAddr)
		if err := srv.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
			log.Error("http", "err", err)
			stop()
		}
	}()
	<-ctx.Done()
	shut, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	_ = srv.Shutdown(shut)
}
