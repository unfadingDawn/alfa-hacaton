package agent

// Уточняющие вопросы к готовому персональному анализу. Ответ строится только
// по сохранённому анализу пользователя и его реестру источников; вопросы к
// чужим анализам невозможны, так как анализ ищется по паре статья+пользователь.

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	"github.com/alfainsight/backend/internal/analysis"
	"github.com/alfainsight/backend/internal/db/dbq"
	"github.com/alfainsight/backend/internal/llm"
)

var (
	ErrNoAnalysis     = errors.New("сначала выполните персональный анализ этой новости")
	ErrStaleAnalysis  = errors.New("анализ был пересчитан, обновите страницу")
	ErrQuestionLimit  = errors.New("лимит уточняющих вопросов исчерпан")
	ErrQuestionLength = errors.New("вопрос должен содержать от 3 до 500 символов")
	ErrUnknownPreset  = errors.New("неизвестный быстрый вопрос")
)

const (
	questionMinRunes = 3
	questionMaxRunes = 500
)

// QuestionLimits — сколько обращений к модели осталось. Ответы из кэша и
// быстрые вопросы без модели лимит не расходуют.
type QuestionLimits struct {
	PerAnalysis     int `json:"per_analysis"`
	UsedForAnalysis int `json:"used_for_analysis"`
	PerUserDaily    int `json:"per_user_daily"`
	UsedToday       int `json:"used_today"`
}

type AskResult struct {
	Question     dbq.AnalysisQuestion
	Cached       bool
	AnalysisHash string
	Limits       QuestionLimits
}

type QuestionHistory struct {
	AnalysisID   uuid.UUID
	AnalysisHash string
	Items        []dbq.AnalysisQuestion
	Limits       QuestionLimits
}

// loadedAnalysis — сохранённый анализ с разобранным результатом.
type loadedAnalysis struct {
	row     dbq.PersonalAnalysis
	hash    string
	insight analysis.Insight
	plan    *analysis.Plan
	cls     *analysis.Classification
	role    string
}

func analysisHash(result []byte) string {
	h := sha256.Sum256(result)
	return hex.EncodeToString(h[:])
}

func (a *Agent) loadAnalysis(ctx context.Context, userID, articleID uuid.UUID) (*loadedAnalysis, error) {
	row, err := a.Q.GetAnalysis(ctx, dbq.GetAnalysisParams{ArticleID: articleID, UserID: userID, Depth: "full"})
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, ErrNoAnalysis
	}
	if err != nil {
		return nil, err
	}
	var res struct {
		Insight        *analysis.Insight        `json:"insight"`
		Plan           *analysis.Plan           `json:"plan"`
		Classification *analysis.Classification `json:"classification"`
		Role           struct {
			Title string `json:"title"`
		} `json:"role"`
	}
	if err := json.Unmarshal(row.Result, &res); err != nil || res.Insight == nil {
		return nil, ErrNoAnalysis
	}
	return &loadedAnalysis{row: row, hash: analysisHash(row.Result), insight: *res.Insight, plan: res.Plan,
		cls: res.Classification, role: res.Role.Title}, nil
}

// qaRegistry — источники, на которые сослался анализ, плюс факты компании,
// для которой он построен (чтобы ответить, какие признаки учитывались).
func (a *Agent) qaRegistry(ctx context.Context, la *loadedAnalysis) (*analysis.Registry, []analysis.ExposureRef) {
	reg := analysis.NewRegistry()
	for _, e := range la.insight.EvidenceSources {
		reg.Add(e)
	}
	var exposures []analysis.ExposureRef
	if prof, err := a.Companies.Load(ctx, la.row.CompanyID, true); err == nil {
		for _, f := range prof.Facts {
			reg.Add(analysis.Evidence{ID: f.ID, Type: "company_data", Title: f.Label, Excerpt: f.Value, Note: f.Source})
		}
		for _, x := range prof.Exposures {
			exposures = append(exposures, analysis.ExposureRef{Title: x.Title, Explanation: x.Explanation, FactIDs: x.FactIDs})
		}
	}
	return reg, exposures
}

func (a *Agent) questionLimits(ctx context.Context, userID, analysisID uuid.UUID) (QuestionLimits, error) {
	c, err := a.Q.CountLLMQuestions(ctx, dbq.CountLLMQuestionsParams{AnalysisID: analysisID, UserID: userID,
		Since: time.Now().Add(-24 * time.Hour)})
	if err != nil {
		return QuestionLimits{}, err
	}
	return QuestionLimits{PerAnalysis: a.QuestionsPerAnalysis, UsedForAnalysis: int(c.ForAnalysis),
		PerUserDaily: a.QuestionsPerUserDaily, UsedToday: int(c.ForUserToday)}, nil
}

func (l QuestionLimits) exhausted() bool {
	return (l.PerAnalysis > 0 && l.UsedForAnalysis >= l.PerAnalysis) || (l.PerUserDaily > 0 && l.UsedToday >= l.PerUserDaily)
}

// Questions — история вопросов к текущему анализу пользователя.
func (a *Agent) Questions(ctx context.Context, userID, articleID uuid.UUID) (QuestionHistory, error) {
	la, err := a.loadAnalysis(ctx, userID, articleID)
	if err != nil {
		return QuestionHistory{}, err
	}
	items, err := a.Q.ListAnalysisQuestions(ctx, dbq.ListAnalysisQuestionsParams{AnalysisID: la.row.ID, UserID: userID})
	if err != nil {
		return QuestionHistory{}, err
	}
	lim, err := a.questionLimits(ctx, userID, la.row.ID)
	if err != nil {
		return QuestionHistory{}, err
	}
	return QuestionHistory{AnalysisID: la.row.ID, AnalysisHash: la.hash, Items: items, Limits: lim}, nil
}

// Ask отвечает на уточняющий вопрос к анализу пользователя.
// expectedAnalysis (необязательный) защищает от ответа по пересчитанному анализу.
func (a *Agent) Ask(ctx context.Context, userID, articleID uuid.UUID, question, preset string, expectedAnalysis *uuid.UUID) (AskResult, error) {
	la, err := a.loadAnalysis(ctx, userID, articleID)
	if err != nil {
		return AskResult{}, err
	}
	if expectedAnalysis != nil && *expectedAnalysis != la.row.ID {
		return AskResult{}, ErrStaleAnalysis
	}
	var quick analysis.QuickQuestion
	if preset != "" {
		q, ok := analysis.QuickQuestionByCode(preset)
		if !ok {
			return AskResult{}, ErrUnknownPreset
		}
		quick, question = q, q.Text
	}
	question = strings.Join(strings.Fields(question), " ")
	if n := utf8.RuneCountInString(question); n < questionMinRunes || n > questionMaxRunes {
		return AskResult{}, ErrQuestionLength
	}
	norm := analysis.NormalizeQuestion(question)
	if norm == "" {
		return AskResult{}, ErrQuestionLength
	}
	// Свободный вопрос, совпадающий с быстрым, отвечается так же.
	if preset == "" {
		for _, q := range analysis.QuickQuestions {
			if analysis.NormalizeQuestion(q.Text) == norm {
				quick = q
			}
		}
	}
	lim, err := a.questionLimits(ctx, userID, la.row.ID)
	if err != nil {
		return AskResult{}, err
	}
	res := AskResult{AnalysisHash: la.hash, Limits: lim}
	reg, exposures := a.qaRegistry(ctx, la)
	qc := analysis.QAContext{RoleTitle: orRole(la.role), Insight: &la.insight, Plan: la.plan, Classification: la.cls, Registry: reg}
	var presetPtr *string
	if quick.Code != "" {
		presetPtr = &quick.Code
	}
	base := dbq.InsertAnalysisQuestionParams{AnalysisID: la.row.ID, UserID: userID, AnalysisHash: la.hash, Question: question,
		QuestionNorm: norm, Preset: presetPtr, PromptVersion: analysis.QAPromptVersion, SourceIds: []string{}, Status: "succeeded"}

	// 1. Быстрые вопросы, которые отвечаются без модели.
	if quick.Deterministic {
		start := time.Now()
		ans, _ := analysis.DeterministicAnswer(quick.Code, qc, exposures)
		raw, _ := json.Marshal(ans)
		p := base
		p.Method, p.Answer, p.SourceIds, p.DurationMs = "deterministic", raw, ans.SourceIDs(), int32(time.Since(start).Milliseconds())
		row, err := a.Q.InsertAnalysisQuestion(ctx, p)
		res.Question = row
		return res, err
	}

	// 2. Кэш: тот же вопрос к той же версии анализа и той же версии промпта.
	if ex, err := a.Q.FindCachedAnswer(ctx, dbq.FindCachedAnswerParams{AnalysisHash: la.hash, QuestionNorm: norm,
		PromptVersion: analysis.QAPromptVersion, UserID: userID}); err == nil {
		res.Cached = true
		if ex.UserID == userID && ex.AnalysisID == la.row.ID {
			_ = a.Q.TouchCachedAnswer(ctx, ex.ID)
			res.Question = ex
			return res, nil
		}
		// Ответ получен коллегой для идентичного анализа — копия без расхода токенов.
		p := base
		p.Method, p.Answer, p.SourceIds, p.Model, p.Cached = ex.Method, ex.Answer, ex.SourceIds, ex.Model, true
		_ = a.Q.TouchCachedAnswer(ctx, ex.ID)
		row, err := a.Q.InsertAnalysisQuestion(ctx, p)
		res.Question = row
		return res, err
	} else if !errors.Is(err, pgx.ErrNoRows) {
		return AskResult{}, err
	}

	// 3. Вопрос к модели: лимиты и бюджет.
	if lim.exhausted() {
		return res, ErrQuestionLimit
	}
	if !a.LLM.Configured() {
		return res, llm.ErrNotConfigured
	}
	start := time.Now()
	var ans analysis.Answer
	resp, err := a.chatJSON(ctx, a.Models.Writer, analysis.QuestionMessages(question, qc), 1000, 0.1, &ans)
	if err == nil {
		err = analysis.ValidateAnswer(&ans, reg)
	}
	p := base
	p.Method, p.Model = "llm", resp.Model
	if p.Model == "" {
		p.Model = a.Models.Writer
	}
	p.PromptTokens, p.CompletionTokens = int32(resp.Usage.PromptTokens), int32(resp.Usage.CompletionTokens)
	p.Tokens, p.DurationMs = int32(resp.Usage.TotalTokens), int32(time.Since(start).Milliseconds())
	if resp.Cached {
		// Идентичный запрос уже выполнялся клиентом модели в пределах TTL — токены не расходовались.
		p.Cached, p.Tokens, p.PromptTokens, p.CompletionTokens = true, 0, 0, 0
	}
	if err != nil {
		if !reachedModel(err) {
			return res, err
		}
		msg := err.Error()
		p.Status, p.Error = "failed", &msg
		if _, ierr := a.Q.InsertAnalysisQuestion(context.WithoutCancel(ctx), p); ierr != nil {
			a.Log.Error("save failed question", "err", ierr)
		}
		return res, err
	}
	raw, _ := json.Marshal(ans)
	p.Answer, p.SourceIds = raw, ans.SourceIDs()
	row, err := a.Q.InsertAnalysisQuestion(ctx, p)
	if err != nil {
		return res, err
	}
	if !p.Cached {
		res.Limits.UsedForAnalysis++
		res.Limits.UsedToday++
	}
	res.Question = row
	return res, nil
}

// reachedModel — была ли попытка обращения к модели. Отказы локальных
// ограничителей (бюджет, пауза, частота) не записываются и не расходуют лимит.
func reachedModel(err error) bool {
	for _, e := range []error{llm.ErrBudgetExceeded, llm.ErrPaused, llm.ErrRateLimited, llm.ErrNotConfigured, llm.ErrModelNotAllowed} {
		if errors.Is(err, e) {
			return false
		}
	}
	return true
}

func orRole(s string) string {
	if strings.TrimSpace(s) == "" {
		return "сотрудник компании"
	}
	return s
}
