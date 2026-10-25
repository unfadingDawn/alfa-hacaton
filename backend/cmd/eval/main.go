package main

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"log/slog"
	"os"
	"strconv"
	"time"

	"github.com/alfainsight/backend/internal/analysis"
	"github.com/alfainsight/backend/internal/evaluation"
	"github.com/alfainsight/backend/internal/llm"
)

func envFloat(key string) float64 {
	v, _ := strconv.ParseFloat(os.Getenv(key), 64)
	return v
}

func main() {
	data := flag.String("data", "../data/evaluation", "директория holdout-набора")
	mode := flag.String("mode", "mock", "mock (без API) или live")
	out := flag.String("out", "", "путь для JSON-отчёта; по умолчанию evaluation-results/<timestamp>.json")
	baseline := flag.String("baseline", "", "предыдущий JSON-отчёт для сравнения")
	k := flag.Int("k", 5, "K для Precision/Recall/NDCG")
	confirm := flag.Bool("confirm-live", false, "обязательное подтверждение расхода LLM-бюджета")
	maxLive := flag.Int("max-live-cases", 5, "максимум live-кейсов; 0 — все")
	model := flag.String("model", os.Getenv("LLM_MODEL_WRITER"), "разрешённая модель для live")
	maxTokens := flag.Int("max-tokens", 2200, "потолок ответа одного live-кейса")
	inputUSD := flag.Float64("input-usd-per-million", envFloat("EVAL_INPUT_USD_PER_MILLION"), "цена 1M input-токенов")
	outputUSD := flag.Float64("output-usd-per-million", envFloat("EVAL_OUTPUT_USD_PER_MILLION"), "цена 1M output-токенов")
	flag.Parse()

	dataset, err := evaluation.Load(*data)
	if err != nil {
		fatal(err)
	}
	var analyzer evaluation.Analyzer = evaluation.MockAnalyzer{}
	if *mode == "live" {
		if !*confirm {
			fatal(fmt.Errorf("live-режим расходует общий ключ: добавьте -confirm-live"))
		}
		baseURL, key := os.Getenv("LLM_BASE_URL"), os.Getenv("LLM_API_KEY")
		if baseURL == "" || key == "" || *model == "" {
			fatal(fmt.Errorf("для live задайте LLM_BASE_URL, LLM_API_KEY и LLM_MODEL_WRITER (или -model)"))
		}
		if _, ok := llm.MatchAllowed(*model); !ok {
			fatal(fmt.Errorf("модель %q не разрешена", *model))
		}
		liveLimit := *maxLive
		if liveLimit <= 0 {
			liveLimit = len(dataset.Judgments)
		}
		guard := llm.NewGuard(llm.GuardConfig{DailyTokenBudget: 100_000, DailyCallLimit: liveLimit,
			PerMinuteLimit: 60, MaxOutputTokens: *maxTokens, MaxInputChars: 24_000, Location: time.Local}, nil)
		client := llm.NewClient(baseURL, key, 120*time.Second, true, guard, *maxTokens, 24_000,
			slog.New(slog.NewTextHandler(io.Discard, nil)))
		analyzer = evaluation.LiveAnalyzer{Client: client, ModelID: *model, MaxTokens: *maxTokens,
			InputUSD: *inputUSD, OutputUSD: *outputUSD}
	} else if *mode != "mock" {
		fatal(fmt.Errorf("неизвестный mode %q", *mode))
	}

	report, err := evaluation.Run(context.Background(), dataset, analyzer,
		evaluation.Options{K: *k, MaxLiveCases: *maxLive, InputUSD: *inputUSD, OutputUSD: *outputUSD})
	if err != nil {
		fatal(err)
	}
	if err := evaluation.Save(report, *out); err != nil {
		fatal(err)
	}
	fmt.Printf("dataset=%s mode=%s model=%s prompt=%s\n", report.DatasetVersion, report.Mode, report.Model, analysis.PromptVersion)
	fmt.Printf("Precision@%d %.3f | Recall@%d %.3f | NDCG@%d %.3f\n", *k, report.Metrics.PrecisionAtK, *k, report.Metrics.RecallAtK, *k, report.Metrics.NDCGAtK)
	fmt.Printf("topics %.3f | score-range %.3f | personalization %.3f | sources %.3f | hallucinations %.3f | JSON %.3f\n",
		report.Metrics.TopicMatch, report.Metrics.ScoreInRangeRate, report.Metrics.PersonalizationRate,
		report.Metrics.ValidSourceRate, report.Metrics.HallucinationRate, report.Metrics.JSONSuccessRate)
	fmt.Printf("avg latency %.0f ms | avg tokens %.0f | avg estimated cost $%.6f\n",
		report.Metrics.AverageLatencyMS, report.Metrics.AverageTokens, report.Metrics.AverageCostUSD)
	if *out == "" {
		fmt.Println("отчёт сохранён в evaluation-results/ (имя содержит UTC timestamp)")
	} else {
		fmt.Println("отчёт:", *out)
	}
	if *baseline != "" {
		delta, err := evaluation.Compare(report, *baseline)
		if err != nil {
			fatal(err)
		}
		b, _ := json.MarshalIndent(delta, "", "  ")
		fmt.Println("изменение относительно baseline:")
		fmt.Println(string(b))
	}
}

func fatal(err error) {
	fmt.Fprintln(os.Stderr, "eval:", err)
	os.Exit(1)
}
