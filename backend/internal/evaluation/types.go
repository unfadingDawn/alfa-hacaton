// Package evaluation implements a reproducible holdout evaluation for ranking
// and grounded personalized analysis. Offline runs never call an LLM.
package evaluation

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"time"
)

type Fact struct {
	ID     string   `json:"id"`
	Label  string   `json:"label"`
	Value  string   `json:"value"`
	Topics []string `json:"topics"`
}

type Company struct {
	ID            string   `json:"id"`
	Name          string   `json:"name"`
	Industry      string   `json:"industry"`
	OkvedDivision string   `json:"okved_division"`
	Region        string   `json:"region"`
	Countries     []string `json:"countries"`
	Topics        []string `json:"topics"`
	Facts         []Fact   `json:"facts"`
	DataGaps      []string `json:"data_gaps"`
}

type Article struct {
	ID                    string    `json:"id"`
	Title                 string    `json:"title"`
	Summary               string    `json:"summary"`
	PublishedAt           time.Time `json:"published_at"`
	Source                string    `json:"source"`
	DuplicateOf           string    `json:"duplicate_of,omitempty"`
	BankResearchAvailable bool      `json:"bank_research_available"`
}

type Query struct {
	ID        string         `json:"id"`
	CompanyID string         `json:"company_id"`
	RoleCode  string         `json:"role_code"`
	Interests map[string]int `json:"interests"`
}

type Judgment struct {
	QueryID              string   `json:"query_id"`
	ArticleID            string   `json:"article_id"`
	ExpectedRelevant     bool     `json:"expected_relevant"`
	ExpectedScoreRange   [2]int   `json:"expected_score_range"`
	ExpectedTopics       []string `json:"expected_topics"`
	ExpectedCompanyFacts []string `json:"expected_company_facts"`
	ExpectedImpact       []string `json:"expected_impact"`
	MustNotClaim         []string `json:"must_not_claim"`
}

type companyFile struct {
	Version   string    `json:"version"`
	Companies []Company `json:"companies"`
}

type articleFile struct {
	Version  string    `json:"version"`
	Articles []Article `json:"articles"`
}

type expectedFile struct {
	Version   string     `json:"version"`
	Queries   []Query    `json:"queries"`
	Judgments []Judgment `json:"judgments"`
}

type Dataset struct {
	Version   string
	Companies []Company
	Articles  []Article
	Queries   []Query
	Judgments []Judgment
}

func readJSON(path string, dst any) error {
	b, err := os.ReadFile(path)
	if err != nil {
		return err
	}
	if err := json.Unmarshal(b, dst); err != nil {
		return fmt.Errorf("%s: %w", path, err)
	}
	return nil
}

func Load(dir string) (Dataset, error) {
	var cf companyFile
	var af articleFile
	var ef expectedFile
	if err := readJSON(filepath.Join(dir, "companies.json"), &cf); err != nil {
		return Dataset{}, err
	}
	if err := readJSON(filepath.Join(dir, "articles.json"), &af); err != nil {
		return Dataset{}, err
	}
	if err := readJSON(filepath.Join(dir, "expected.json"), &ef); err != nil {
		return Dataset{}, err
	}
	if cf.Version == "" || cf.Version != af.Version || cf.Version != ef.Version {
		return Dataset{}, fmt.Errorf("версии evaluation-файлов не совпадают: companies=%q articles=%q expected=%q", cf.Version, af.Version, ef.Version)
	}
	d := Dataset{Version: cf.Version, Companies: cf.Companies, Articles: af.Articles, Queries: ef.Queries, Judgments: ef.Judgments}
	return d, d.Validate()
}

func (d Dataset) Validate() error {
	if len(d.Companies) < 5 || len(d.Articles) < 20 || len(d.Queries) < 3 || len(d.Judgments) == 0 {
		return fmt.Errorf("слишком маленький evaluation-набор: companies=%d articles=%d queries=%d judgments=%d",
			len(d.Companies), len(d.Articles), len(d.Queries), len(d.Judgments))
	}
	companies, articles, queries := map[string]bool{}, map[string]bool{}, map[string]bool{}
	for _, c := range d.Companies {
		if c.ID == "" || companies[c.ID] {
			return fmt.Errorf("пустой или повторяющийся company id %q", c.ID)
		}
		companies[c.ID] = true
	}
	for _, a := range d.Articles {
		if a.ID == "" || articles[a.ID] {
			return fmt.Errorf("пустой или повторяющийся article id %q", a.ID)
		}
		articles[a.ID] = true
	}
	for _, a := range d.Articles {
		if a.DuplicateOf != "" && !articles[a.DuplicateOf] {
			return fmt.Errorf("article %s ссылается на неизвестный duplicate_of %s", a.ID, a.DuplicateOf)
		}
	}
	for _, q := range d.Queries {
		if q.ID == "" || queries[q.ID] || !companies[q.CompanyID] {
			return fmt.Errorf("некорректный query %q (company=%q)", q.ID, q.CompanyID)
		}
		queries[q.ID] = true
	}
	positive := map[string]int{}
	for _, j := range d.Judgments {
		if !queries[j.QueryID] || !articles[j.ArticleID] {
			return fmt.Errorf("judgment ссылается на неизвестные query/article: %s/%s", j.QueryID, j.ArticleID)
		}
		if j.ExpectedScoreRange[0] < 0 || j.ExpectedScoreRange[1] > 100 || j.ExpectedScoreRange[0] > j.ExpectedScoreRange[1] {
			return fmt.Errorf("некорректный score range: %s/%s", j.QueryID, j.ArticleID)
		}
		if j.ExpectedRelevant {
			positive[j.QueryID]++
		}
	}
	for _, q := range d.Queries {
		if positive[q.ID] == 0 {
			return fmt.Errorf("query %s не имеет релевантных статей", q.ID)
		}
	}
	return nil
}
