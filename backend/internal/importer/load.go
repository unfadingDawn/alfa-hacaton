package importer

import (
	"bufio"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/alfainsight/backend/internal/companies"
	"github.com/alfainsight/backend/internal/db/dbq"
	"github.com/alfainsight/backend/internal/news"
)

type Loader struct {
	Pool *pgxpool.Pool
	Q    *dbq.Queries
	Log  *slog.Logger
}

type Report struct {
	File     string         `json:"file"`
	Rows     int            `json:"rows"`
	Imported int            `json:"imported"`
	Issues   map[string]int `json:"issues"`
	Duration string         `json:"duration"`
}

func (r Report) String() string {
	b, _ := json.Marshal(r)
	return string(b)
}

type issue struct {
	row    int
	kind   string
	detail map[string]any
}

func (l *Loader) writeIssues(ctx context.Context, tx pgx.Tx, file string, issues []issue) error {
	q := l.Q.WithTx(tx)
	if err := q.DeleteImportIssues(ctx, file); err != nil {
		return err
	}
	for _, is := range issues {
		raw, _ := json.Marshal(is.detail)
		row := int32(is.row)
		if err := q.AddImportIssue(ctx, dbq.AddImportIssueParams{FileName: file, SourceRow: &row, Kind: is.kind, Detail: raw}); err != nil {
			return err
		}
	}
	return nil
}

func openCSV(path string, header []string) (*os.File, func() ([]string, error), error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, nil, err
	}
	r := NewReader(bufio.NewReaderSize(f, 1<<20))
	h, err := r.Read()
	if err != nil {
		f.Close()
		return nil, nil, fmt.Errorf("%s: чтение заголовка: %w", filepath.Base(path), err)
	}
	if err := CheckHeader(h, header); err != nil {
		f.Close()
		return nil, nil, fmt.Errorf("%s: %w", filepath.Base(path), err)
	}
	return f, r.Read, nil
}

// Companies загружает df_Uid_data_hack.csv. Повтор ID с одинаковыми данными
// схлопывается; с разными — все строки этого ID уходят в import_issues и
// в справочник не попадают (нельзя выбрать «правильную» версию).
func (l *Loader) Companies(ctx context.Context, path string) (Report, error) {
	start := time.Now()
	file := filepath.Base(path)
	rep := Report{File: file, Issues: map[string]int{}}
	f, read, err := openCSV(path, CompanyHeader)
	if err != nil {
		return rep, err
	}
	defer f.Close()

	byID := map[string][]CompanyRow{}
	var order []string
	var issues []issue
	row := 1
	for {
		rec, err := read()
		if errors.Is(err, io.EOF) {
			break
		}
		row++
		rep.Rows++
		if err != nil {
			issues = append(issues, issue{row, "csv_error", map[string]any{"error": err.Error()}})
			continue
		}
		c, err := ParseCompany(rec, row, companies.Division)
		if err != nil {
			issues = append(issues, issue{row, "invalid_row", map[string]any{"error": err.Error(), "id": rec[0]}})
			continue
		}
		if _, ok := byID[c.ID]; !ok {
			order = append(order, c.ID)
		}
		byID[c.ID] = append(byID[c.ID], c)
	}
	var rows [][]any
	for _, id := range order {
		group := byID[id]
		conflict := false
		for _, g := range group[1:] {
			if !SameContent(group[0].Raw, g.Raw) {
				conflict = true
			}
		}
		if conflict {
			for _, g := range group {
				issues = append(issues, issue{g.Row, "duplicate_id_conflicting", map[string]any{
					"id": id, "okved": g.OkvedCode, "region": g.Region, "revenue": g.Revenue, "copies": len(group)}})
			}
			continue
		}
		if len(group) > 1 {
			issues = append(issues, issue{group[1].Row, "duplicate_id_identical", map[string]any{"id": id, "copies": len(group)}})
		}
		c := group[0]
		rows = append(rows, []any{c.ID, c.OrgType, c.OkfsName, c.Age, c.EcoStage, c.OkvedCode, c.OkvedName, c.OkvedDivision,
			c.Negative, c.WorkersFns, c.WorkersSpark, c.ClientType, c.Rko, c.Gar, c.Loa, c.Lea, c.Dep, c.Zpp, c.Fts,
			c.Countries, c.Factoring, c.Revenue, c.GroupID, c.City, c.Region, int32(c.Row)})
	}
	cols := []string{"id", "org_type", "okfs_name", "company_age", "eco_stage", "okved_code", "okved_name", "okved_division",
		"negative_mark", "workers_fns", "workers_spark", "client_type", "has_rko", "has_guarantee", "has_loan", "has_leasing",
		"has_deposit", "has_payroll", "fts_flag", "fts_countries", "has_factoring", "revenue", "group_id", "city", "region", "source_row"}

	tx, err := l.Pool.Begin(ctx)
	if err != nil {
		return rep, err
	}
	defer func() { _ = tx.Rollback(ctx) }()
	if _, err := tx.Exec(ctx, `CREATE TEMP TABLE companies_stage (LIKE companies INCLUDING DEFAULTS) ON COMMIT DROP`); err != nil {
		return rep, err
	}
	if _, err := tx.CopyFrom(ctx, pgx.Identifier{"companies_stage"}, cols, pgx.CopyFromRows(rows)); err != nil {
		return rep, fmt.Errorf("copy companies: %w", err)
	}
	set := make([]string, 0, len(cols))
	for _, c := range cols[1:] {
		set = append(set, c+" = EXCLUDED."+c)
	}
	// display_name и profile (уточнения пользователей) не перезаписываются.
	sql := fmt.Sprintf(`INSERT INTO companies (%s) SELECT %s FROM companies_stage
		ON CONFLICT (id) DO UPDATE SET %s, imported_at = now()`,
		strings.Join(cols, ","), strings.Join(cols, ","), strings.Join(set, ", "))
	tag, err := tx.Exec(ctx, sql)
	if err != nil {
		return rep, fmt.Errorf("upsert companies: %w", err)
	}
	rep.Imported = int(tag.RowsAffected())
	if err := l.writeIssues(ctx, tx, file, issues); err != nil {
		return rep, err
	}
	if err := tx.Commit(ctx); err != nil {
		return rep, err
	}
	for _, is := range issues {
		rep.Issues[is.kind]++
	}
	rep.Duration = time.Since(start).Round(time.Millisecond).String()
	return rep, nil
}

// PersonLinks загружает df_Fid_Uid_link_hack.csv (полная перезагрузка таблицы).
func (l *Loader) PersonLinks(ctx context.Context, path string) (Report, error) {
	start := time.Now()
	file := filepath.Base(path)
	rep := Report{File: file, Issues: map[string]int{}}
	f, read, err := openCSV(path, LinkHeader)
	if err != nil {
		return rep, err
	}
	defer f.Close()
	seen := map[[2]string]int{}
	var rows [][]any
	var issues []issue
	row := 1
	for {
		rec, err := read()
		if errors.Is(err, io.EOF) {
			break
		}
		row++
		rep.Rows++
		if err != nil || len(rec) != len(LinkHeader) {
			issues = append(issues, issue{row, "invalid_row", map[string]any{"error": fmt.Sprint(err)}})
			continue
		}
		pid, cid := strings.TrimSpace(rec[0]), strings.TrimSpace(rec[3])
		if pid == "" || cid == "" {
			issues = append(issues, issue{row, "empty_id", map[string]any{"person": pid, "company": cid}})
			continue
		}
		key := [2]string{pid, cid}
		if first, ok := seen[key]; ok {
			issues = append(issues, issue{row, "duplicate_link", map[string]any{"person": pid, "company": cid, "first_row": first}})
			continue
		}
		seen[key] = row
		rows = append(rows, []any{pid, cid, Text(rec[1]), Text(rec[2]), int32(row)})
	}
	tx, err := l.Pool.Begin(ctx)
	if err != nil {
		return rep, err
	}
	defer func() { _ = tx.Rollback(ctx) }()
	if _, err := tx.Exec(ctx, `TRUNCATE person_links`); err != nil {
		return rep, err
	}
	n, err := tx.CopyFrom(ctx, pgx.Identifier{"person_links"},
		[]string{"person_id", "company_id", "city_name", "region_name", "source_row"}, pgx.CopyFromRows(rows))
	if err != nil {
		return rep, fmt.Errorf("copy person_links: %w", err)
	}
	var missing int
	if err := tx.QueryRow(ctx, `SELECT count(DISTINCT company_id) FROM person_links pl
		WHERE NOT EXISTS (SELECT 1 FROM companies c WHERE c.id = pl.company_id)`).Scan(&missing); err != nil {
		return rep, err
	}
	if missing > 0 {
		issues = append(issues, issue{0, "unknown_company_reference", map[string]any{
			"distinct_companies": missing, "note": "ссылки сохранены; компаний нет в df_Uid_data_hack.csv"}})
	}
	if err := l.writeIssues(ctx, tx, file, issues); err != nil {
		return rep, err
	}
	if err := tx.Commit(ctx); err != nil {
		return rep, err
	}
	rep.Imported = int(n)
	for _, is := range issues {
		rep.Issues[is.kind]++
	}
	rep.Duration = time.Since(start).Round(time.Millisecond).String()
	return rep, nil
}

// paymentSource потоково отдаёт строки платежей в COPY.
type paymentSource struct {
	read    func() ([]string, error)
	row     int
	cur     []any
	err     error
	rep     *Report
	issues  *[]issue
	seen    map[[2]string]int
	maxKeep int
}

func (p *paymentSource) Next() bool {
	for {
		rec, err := p.read()
		if errors.Is(err, io.EOF) {
			return false
		}
		p.row++
		p.rep.Rows++
		if err != nil || len(rec) != len(PaymentHeader) {
			p.addIssue(issue{p.row, "invalid_row", map[string]any{"error": fmt.Sprint(err)}})
			continue
		}
		ben, payer := strings.TrimSpace(rec[0]), strings.TrimSpace(rec[1])
		amt, err1 := Number(rec[2])
		cnt, err2 := Count(rec[3])
		if ben == "" || payer == "" || err1 != nil || amt == nil || err2 != nil {
			p.addIssue(issue{p.row, "invalid_row", map[string]any{"beneficiary": ben, "payer": payer,
				"amount": rec[2], "count": rec[3]}})
			continue
		}
		key := [2]string{payer, ben}
		if first, ok := p.seen[key]; ok {
			// Обе строки сохраняются как есть; факт повтора фиксируется.
			p.addIssue(issue{p.row, "duplicate_pair", map[string]any{"payer": payer, "beneficiary": ben, "first_row": first}})
		} else {
			p.seen[key] = p.row
		}
		p.cur = []any{payer, ben, *amt, cnt, payer == ben, int32(p.row)}
		return true
	}
}

func (p *paymentSource) addIssue(is issue) {
	p.rep.Issues[is.kind]++
	if len(*p.issues) < p.maxKeep {
		*p.issues = append(*p.issues, is)
	}
}

func (p *paymentSource) Values() ([]any, error) { return p.cur, nil }
func (p *paymentSource) Err() error             { return p.err }

// Payments загружает df_Uid_payment_hack.csv (~1,4 млн строк) через COPY.
func (l *Loader) Payments(ctx context.Context, path string) (Report, error) {
	start := time.Now()
	file := filepath.Base(path)
	rep := Report{File: file, Issues: map[string]int{}}
	f, read, err := openCSV(path, PaymentHeader)
	if err != nil {
		return rep, err
	}
	defer f.Close()
	var issues []issue
	src := &paymentSource{read: read, row: 1, rep: &rep, issues: &issues, seen: map[[2]string]int{}, maxKeep: 5000}
	tx, err := l.Pool.Begin(ctx)
	if err != nil {
		return rep, err
	}
	defer func() { _ = tx.Rollback(ctx) }()
	if _, err := tx.Exec(ctx, `TRUNCATE payments RESTART IDENTITY`); err != nil {
		return rep, err
	}
	n, err := tx.CopyFrom(ctx, pgx.Identifier{"payments"},
		[]string{"payer", "beneficiary", "amount", "pay_count", "is_internal", "source_row"}, src)
	if err != nil {
		return rep, fmt.Errorf("copy payments: %w", err)
	}
	var internal int
	_ = tx.QueryRow(ctx, `SELECT count(*) FROM payments WHERE is_internal`).Scan(&internal)
	if internal > 0 {
		issues = append(issues, issue{0, "self_payments", map[string]any{"rows": internal,
			"note": "PAYER = BENEFICIARY; сохранены с is_internal=true и исключены из анализа контрагентов"}})
		rep.Issues["self_payments"] = internal
	}
	if err := l.writeIssues(ctx, tx, file, issues); err != nil {
		return rep, err
	}
	if _, err := tx.Exec(ctx, `ANALYZE payments`); err != nil {
		return rep, err
	}
	if err := tx.Commit(ctx); err != nil {
		return rep, err
	}
	rep.Imported = int(n)
	rep.Duration = time.Since(start).Round(time.Millisecond).String()
	return rep, nil
}

// Research загружает OCR-тексты материалов Альфа-Банка: документы, фрагменты
// для поиска и карточки материалов в ленте.
func (l *Loader) Research(ctx context.Context, dir string) (Report, error) {
	start := time.Now()
	rep := Report{File: dir, Issues: map[string]int{}}
	files, err := filepath.Glob(filepath.Join(dir, "*.txt"))
	if err != nil {
		return rep, err
	}
	sort.Strings(files)
	for _, path := range files {
		raw, err := os.ReadFile(path)
		if err != nil {
			return rep, err
		}
		rep.Rows++
		base := strings.TrimSuffix(filepath.Base(path), ".txt")
		meta, ok := KnownDocs[base]
		if !ok {
			rep.Issues["unknown_document_metadata"]++
			meta = DocMeta{Title: base, Unit: "Альфа-Банк"}
		}
		pages := SplitPages(string(raw))
		chunks := ChunkPages(pages, 900)
		lead := LeadText(pages, 600)
		if err := l.saveDoc(ctx, base, meta, pages, chunks, lead); err != nil {
			return rep, fmt.Errorf("%s: %w", base, err)
		}
		rep.Imported++
	}
	rep.Duration = time.Since(start).Round(time.Millisecond).String()
	return rep, nil
}

func (l *Loader) saveDoc(ctx context.Context, base string, meta DocMeta, pages []Page, chunks []Chunk, lead string) error {
	tx, err := l.Pool.Begin(ctx)
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback(ctx) }()
	q := l.Q.WithTx(tx)
	var pub *time.Time
	if !meta.Published.IsZero() {
		p := meta.Published
		pub = &p
	}
	id, err := q.UpsertResearchDocument(ctx, dbq.UpsertResearchDocumentParams{
		FileName: base + ".pdf", Title: meta.Title, Subtitle: meta.Subtitle, PublishedOn: pub,
		AuthorUnit: meta.Unit, Pages: int32(len(pages)), IsOutdated: meta.Outdated, LeadText: lead})
	if err != nil {
		return err
	}
	if err := q.DeleteResearchChunks(ctx, id); err != nil {
		return err
	}
	var body strings.Builder
	for i, c := range chunks {
		if err := q.InsertResearchChunk(ctx, dbq.InsertResearchChunkParams{DocumentID: id, ChunkNo: int32(i), Page: int32(c.Page), Content: c.Content}); err != nil {
			return err
		}
		if body.Len() < 20000 {
			body.WriteString(c.Content + "\n\n")
		}
	}
	published := meta.Published
	if published.IsZero() {
		published = time.Now()
	}
	articleID, err := q.UpsertResearchArticle(ctx, dbq.UpsertResearchArticleParams{
		Url: fmt.Sprintf("/research/%d", id), Guid: base, Title: meta.Title, Summary: lead, Body: body.String(),
		PublishedAt: published, TitleNorm: news.NormTitle(meta.Title), ContentHash: news.ContentHash(meta.Title, lead)})
	if err != nil {
		return err
	}
	if err := q.DeleteArticleTopics(ctx, articleID); err != nil {
		return err
	}
	for _, t := range meta.Topics {
		if err := q.AddArticleTopic(ctx, dbq.AddArticleTopicParams{ArticleID: articleID, TopicCode: t, Confidence: 1, Method: "metadata"}); err != nil {
			return err
		}
	}
	return tx.Commit(ctx)
}
