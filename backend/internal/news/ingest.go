package news

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	"github.com/alfainsight/backend/internal/db/dbq"
)

type Ingestor struct {
	Q           *dbq.Queries
	Log         *slog.Logger
	HTTP        *http.Client
	InitialDays int
	mu          sync.Mutex
}

type FeedResult struct {
	Source     string `json:"source"`
	Fetched    int    `json:"fetched"`
	Skipped    int    `json:"skipped_by_category"`
	Old        int    `json:"skipped_old"`
	Inserted   int    `json:"inserted"`
	Duplicates int    `json:"duplicates"`
	Error      string `json:"error,omitempty"`
}

// RunOnce опрашивает все включённые RSS-ленты. Модель не вызывается:
// новости только сохраняются; анализ запускается отдельно.
func (in *Ingestor) RunOnce(ctx context.Context) []FeedResult {
	in.mu.Lock()
	defer in.mu.Unlock()
	feeds, err := in.Q.ListEnabledFeeds(ctx)
	if err != nil {
		in.Log.Error("list feeds", "err", err)
		return nil
	}
	var results []FeedResult
	for _, f := range feeds {
		r := in.fetchFeed(ctx, f)
		status := fmt.Sprintf("ok: получено %d, новых %d, дубликатов %d", r.Fetched, r.Inserted, r.Duplicates)
		if r.Error != "" {
			status = "ошибка: " + r.Error
		}
		_ = in.Q.UpdateSourceFetchStatus(ctx, dbq.UpdateSourceFetchStatusParams{ID: f.ID, Status: &status})
		in.Log.Info("feed fetched", "source", f.Code, "fetched", r.Fetched, "inserted", r.Inserted, "error", r.Error)
		results = append(results, r)
	}
	return results
}

func (in *Ingestor) fetchFeed(ctx context.Context, f dbq.Source) FeedResult {
	r := FeedResult{Source: f.Code}
	if f.FeedUrl == nil {
		r.Error = "не задан адрес ленты"
		return r
	}
	cctx, cancel := context.WithTimeout(ctx, 25*time.Second)
	defer cancel()
	items, err := Fetch(cctx, in.HTTP, *f.FeedUrl)
	if err != nil {
		r.Error = err.Error()
		return r
	}
	r.Fetched = len(items)
	cutoff := time.Now().AddDate(0, 0, -in.InitialDays)
	for _, it := range items {
		if !MatchesCategories(it, f.CategoryFilter) {
			r.Skipped++
			continue
		}
		if it.Published.Before(cutoff) {
			r.Old++
			continue
		}
		inserted, dup, err := in.store(ctx, f.ID, it, nil)
		if err != nil {
			in.Log.Warn("store item", "source", f.Code, "err", err)
			continue
		}
		if inserted {
			r.Inserted++
			if dup {
				r.Duplicates++
			}
		}
	}
	return r
}

// store сохраняет запись; дубликат (по хешу или нормализованному заголовку
// за 3 дня) сохраняется со ссылкой duplicate_of и в ленту не попадает.
func (in *Ingestor) store(ctx context.Context, sourceID int32, it Item, submittedBy *uuid.UUID) (inserted, duplicate bool, err error) {
	title := Truncate(it.Title, 500)
	summary := Truncate(it.Description, 2000)
	norm := NormTitle(title)
	hash := ContentHash(title, summary)
	var dupOf *uuid.UUID
	id, err := in.Q.FindDuplicateArticle(ctx, dbq.FindDuplicateArticleParams{
		ContentHash: hash, TitleNorm: norm, Since: it.Published.Add(-72 * time.Hour)})
	if err == nil {
		dupOf = &id
	} else if !errors.Is(err, pgx.ErrNoRows) {
		return false, false, err
	}
	guid := it.GUID
	if guid == "" {
		guid = it.Link
	}
	_, err = in.Q.InsertArticle(ctx, dbq.InsertArticleParams{
		SourceID: sourceID, Url: it.Link, Guid: guid, Title: title, Summary: summary, Body: "",
		PublishedAt: it.Published, TitleNorm: norm, ContentHash: hash, DuplicateOf: dupOf, SubmittedBy: submittedBy,
	})
	if errors.Is(err, pgx.ErrNoRows) {
		return false, false, nil // уже есть (ON CONFLICT по url)
	}
	if err != nil {
		return false, false, err
	}
	return true, dupOf != nil, nil
}

// Submit сохраняет новость, добавленную пользователем для проверки анализа.
// Такая новость помечается источником «Добавлено пользователем».
func (in *Ingestor) Submit(ctx context.Context, userID uuid.UUID, title, text, link string, published time.Time) (uuid.UUID, error) {
	src, err := in.Q.GetSourceByCode(ctx, "user_submitted")
	if err != nil {
		return uuid.Nil, err
	}
	title, text = CleanText(title), strings.TrimSpace(text)
	if link == "" {
		link = "user://" + uuid.NewString()
	}
	norm := NormTitle(title)
	return in.Q.InsertArticle(ctx, dbq.InsertArticleParams{
		SourceID: src.ID, Url: link, Guid: link, Title: Truncate(title, 500), Summary: Truncate(CleanText(text), 600),
		Body: Truncate(text, 20000), PublishedAt: published, TitleNorm: norm, ContentHash: ContentHash(title, text),
		SubmittedBy: &userID,
	})
}

// Loop опрашивает ленты при старте и далее по расписанию.
func (in *Ingestor) Loop(ctx context.Context, every time.Duration) {
	in.RunOnce(ctx)
	t := time.NewTicker(every)
	defer t.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-t.C:
			in.RunOnce(ctx)
		}
	}
}
