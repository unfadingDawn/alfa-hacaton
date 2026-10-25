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
	Updated    int    `json:"updated"`
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
		status := fmt.Sprintf("ok: получено %d, новых %d, дубликатов %d, обновлено %d", r.Fetched, r.Inserted, r.Duplicates, r.Updated)
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
	cctx, cancel := context.WithTimeout(ctx, 40*time.Second)
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
		res, err := in.Store(ctx, f, it)
		if err != nil {
			in.Log.Warn("store item", "source", f.Code, "err", err)
			continue
		}
		switch {
		case res.Inserted:
			r.Inserted++
			if res.DuplicateOf != nil {
				r.Duplicates++
			}
		case res.Updated:
			r.Updated++
		}
	}
	return r
}

// StoreResult — что произошло с записью ленты.
type StoreResult struct {
	ID          uuid.UUID
	Inserted    bool       // новая публикация
	Updated     bool       // та же ссылка, но источник изменил текст
	DuplicateOf *uuid.UUID // то же событие уже есть (тот же или другой источник)
}

// Store сохраняет запись ленты.
//   - Полный текст сохраняется, только если источник сам передаёт его в ленте
//     и для источника разрешено хранение (full_text_policy = 'feed').
//   - Та же ссылка с изменённым текстом — обновление публикации, а не новая новость.
//   - Дубликат (тот же хеш или заголовок за 72 часа либо то же событие в
//     другом издании за ±48 часов) сохраняется со ссылкой duplicate_of и в
//     ленту не попадает.
func (in *Ingestor) Store(ctx context.Context, src dbq.Source, it Item) (StoreResult, error) {
	title := Truncate(it.Title, 500)
	summary := Truncate(it.Description, 2000)
	body := ""
	if src.FullTextPolicy == "feed" && len([]rune(it.FullText)) > len([]rune(summary))+100 {
		body = Truncate(it.FullText, 20000)
	}
	norm := NormTitle(title)
	hash := ContentHash(title, summary+" "+body)

	if id, err := in.Q.UpdateArticleContent(ctx, dbq.UpdateArticleContentParams{Url: it.Link, Title: title, Summary: summary,
		Body: body, TitleNorm: norm, ContentHash: hash}); err == nil {
		return StoreResult{ID: id, Updated: true}, nil
	} else if !errors.Is(err, pgx.ErrNoRows) {
		return StoreResult{}, err
	}

	dupOf, err := in.findDuplicate(ctx, title, norm, hash, it.Published)
	if err != nil {
		return StoreResult{}, err
	}
	guid := it.GUID
	if guid == "" {
		guid = it.Link
	}
	id, err := in.Q.InsertArticle(ctx, dbq.InsertArticleParams{
		SourceID: src.ID, Url: it.Link, Guid: guid, Title: title, Summary: summary, Body: body,
		PublishedAt: it.Published, TitleNorm: norm, ContentHash: hash, DuplicateOf: dupOf,
	})
	if errors.Is(err, pgx.ErrNoRows) {
		return StoreResult{}, nil // уже есть с тем же содержанием (ON CONFLICT по url)
	}
	if err != nil {
		return StoreResult{}, err
	}
	return StoreResult{ID: id, Inserted: true, DuplicateOf: dupOf}, nil
}

func (in *Ingestor) findDuplicate(ctx context.Context, title, norm, hash string, published time.Time) (*uuid.UUID, error) {
	id, err := in.Q.FindDuplicateArticle(ctx, dbq.FindDuplicateArticleParams{
		ContentHash: hash, TitleNorm: norm, Since: published.Add(-72 * time.Hour)})
	if err == nil {
		return &id, nil
	}
	if !errors.Is(err, pgx.ErrNoRows) {
		return nil, err
	}
	cands, err := in.Q.ListDedupCandidates(ctx, dbq.ListDedupCandidatesParams{
		DateFrom: published.Add(-48 * time.Hour), DateTo: published.Add(48 * time.Hour)})
	if err != nil {
		return nil, err
	}
	for _, c := range cands {
		if SameEvent(title, c.Title) {
			id := c.ID
			return &id, nil
		}
	}
	return nil, nil
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
