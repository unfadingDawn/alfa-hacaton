"use client";

import { useState } from "react";
import { CircleAlert, CornerDownLeft, MessageCircleQuestion, Sparkles } from "lucide-react";
import { toast } from "sonner";
import { Button } from "@/components/ui/button";
import { Textarea } from "@/components/ui/field";
import { Badge, ErrorState, Panel, Skeleton } from "@/components/ui/misc";
import { api, ApiError, errorMessage } from "@/lib/api";
import { formatDate } from "@/lib/format";
import type { AnalysisQuestion, AnswerPoint, QuestionHistory, QuestionLimits } from "@/lib/types";
import { useApi } from "@/lib/use-api";
import { cn } from "@/lib/utils";
import { EvidenceItem, SourceChips } from "./evidence";

const MAX_LEN = 500;

const pointBasis: Record<AnswerPoint["basis"], { label: string; tone: "neutral" | "bank" | "outline" }> = {
  fact: { label: "Факт из новости", tone: "neutral" },
  bank_view: { label: "Позиция Альфа-Банка", tone: "bank" },
  company_data: { label: "Признак компании", tone: "neutral" },
  model_inference: { label: "Вывод модели", tone: "outline" },
};

// «Спросить об анализе»: уточняющие вопросы к готовому анализу (критерий 10).
// Ответ строится только по этому анализу и его источникам.
export function AskAnalysis({ articleId, analysisId }: { articleId: string; analysisId: string }) {
  const path = `/articles/${articleId}/analysis/questions`;
  const { data, error, loading, reload, mutate } = useApi<QuestionHistory>(path);
  const [text, setText] = useState("");
  const [busy, setBusy] = useState<string | null>(null);
  const [err, setErr] = useState<ApiError | null>(null);

  async function ask(body: { question?: string; preset?: string }, key: string) {
    setBusy(key);
    setErr(null);
    try {
      const r = await api<{ item: AnalysisQuestion; cached: boolean; limits: QuestionLimits }>(path, {
        method: "POST",
        body: { ...body, analysis_id: analysisId },
      });
      mutate((d) =>
        d
          ? {
              ...d,
              limits: r.limits,
              // Повтор из кэша поднимает существующий ответ в конец истории, без дубля.
              items: [...d.items.filter((x) => x.id !== r.item.id), r.item],
            }
          : d,
      );
      if (body.question) setText("");
      if (r.cached) toast.success("Ответ взят из сохранённых, без обращения к модели");
      requestAnimationFrame(() => document.getElementById(`qa-${r.item.id}`)?.scrollIntoView({ behavior: "smooth", block: "nearest" }));
    } catch (e) {
      const ae = e instanceof ApiError ? e : new ApiError(0, "unknown", errorMessage(e));
      if (ae.code === "stale_analysis") void reload();
      setErr(ae);
    } finally {
      setBusy(null);
    }
  }

  function submit() {
    const q = text.trim();
    if (q.length < 3) {
      setErr(new ApiError(422, "validation", "Вопрос должен содержать хотя бы 3 символа"));
      return;
    }
    void ask({ question: q }, "free");
  }

  const limits = data?.limits;
  const llmLeft = limits ? remaining(limits) : null;
  const llmBlocked = llmLeft === 0;

  return (
    <Panel
      title={
        <span className="inline-flex items-center gap-2">
          <MessageCircleQuestion className="size-5 text-steel" aria-hidden /> Спросить об анализе
        </span>
      }
      description="Ответ строится только по этому анализу и его источникам: без новых фактов и цифр"
    >
      {error ? (
        <ErrorState error={error} onRetry={reload} />
      ) : loading || !data ? (
        <div className="space-y-3">
          <Skeleton className="h-8 w-3/4" />
          <Skeleton className="h-20" />
        </div>
      ) : (
        <>
          <div className="flex flex-wrap gap-2" role="group" aria-label="Быстрые вопросы">
            {data.quick_questions.map((q) => (
              <Button
                key={q.code}
                variant="outline"
                size="sm"
                loading={busy === q.code}
                disabled={!!busy || (!q.deterministic && llmBlocked)}
                onClick={() => ask({ preset: q.code }, q.code)}
                title={q.deterministic ? "Отвечается по сохранённому анализу, без запроса к модели" : "Запрос к модели"}
              >
                {busy !== q.code && !q.deterministic && <Sparkles className="text-steel" aria-hidden />}
                {q.text}
                {q.deterministic && <span className="font-normal text-silver">· без модели</span>}
              </Button>
            ))}
          </div>

          {data.items.length > 0 && (
            <ol className="mt-5 space-y-5" aria-label="История вопросов">
              {data.items.map((q) => (
                <QuestionItem key={q.id} q={q} />
              ))}
            </ol>
          )}

          <form
            className="mt-5"
            onSubmit={(e) => {
              e.preventDefault();
              submit();
            }}
          >
            <label htmlFor="qa-input" className="sr-only">
              Ваш вопрос об анализе
            </label>
            <Textarea
              id="qa-input"
              value={text}
              maxLength={MAX_LEN}
              onChange={(e) => setText(e.target.value)}
              onKeyDown={(e) => {
                if (e.key === "Enter" && !e.shiftKey && !e.nativeEvent.isComposing) {
                  e.preventDefault();
                  submit();
                }
              }}
              placeholder="Например: как это повлияет на график рефинансирования?"
              className="min-h-16"
              disabled={llmBlocked}
            />
            <div className="mt-2 flex flex-wrap items-center justify-between gap-3">
              <p className="text-[12px] text-silver">
                {limits && <LimitsNote limits={limits} />}
                {text.length > MAX_LEN - 100 && ` · ${text.length}/${MAX_LEN}`}
              </p>
              <Button type="submit" size="sm" loading={busy === "free"} disabled={!!busy || llmBlocked || !text.trim()}>
                {busy !== "free" && <CornerDownLeft aria-hidden />} Спросить
              </Button>
            </div>
          </form>

          {err && (
            <div role="alert" className="mt-3 flex gap-2.5 rounded-[12px] bg-risk-bg px-4 py-3 text-[14px] text-risk">
              <CircleAlert className="mt-0.5 size-4 shrink-0" aria-hidden />
              <span>{err.fields?.question ?? err.message}</span>
            </div>
          )}
        </>
      )}
    </Panel>
  );
}

function remaining(l: QuestionLimits) {
  const perAnalysis = l.per_analysis > 0 ? l.per_analysis - l.used_for_analysis : Infinity;
  const perDay = l.per_user_daily > 0 ? l.per_user_daily - l.used_today : Infinity;
  return Math.max(0, Math.min(perAnalysis, perDay));
}

function LimitsNote({ limits }: { limits: QuestionLimits }) {
  const left = remaining(limits);
  if (left === 0)
    return <>Лимит вопросов к модели исчерпан. Быстрые вопросы «без модели» и ответы из истории доступны.</>;
  if (!Number.isFinite(left)) return <>Enter — отправить, Shift+Enter — новая строка</>;
  return (
    <>
      Вопросов к модели осталось: {left}
      {limits.per_analysis > 0 && ` (по анализу ${limits.used_for_analysis}/${limits.per_analysis})`}. Повторы и вопросы «без модели»
      лимит не расходуют.
    </>
  );
}

function QuestionItem({ q }: { q: AnalysisQuestion }) {
  const a = q.answer;
  const evidence = new Map(a.sources.map((e) => [e.id, e]));
  const prefix = `qa-${q.id}-`;
  return (
    <li id={`qa-${q.id}`} className="scroll-mt-24">
      <div className="flex justify-end">
        <p className="max-w-[85%] rounded-[14px] rounded-br-[4px] bg-frost px-3.5 py-2 text-[14px] font-medium text-ink">{q.question}</p>
      </div>
      <div className={cn("mt-2 rounded-[14px] border border-line bg-white/60 p-4", q.outdated && "opacity-75")}>
        <div className="mb-2 flex flex-wrap items-center gap-1.5">
          {q.method === "deterministic" ? (
            <Badge tone="outline" title="Ответ собран из сохранённого анализа без запроса к модели">
              Без модели
            </Badge>
          ) : (
            <Badge tone="bank">
              <Sparkles className="size-3" aria-hidden /> Ответ ИИ
            </Badge>
          )}
          {q.cached && <Badge tone="outline">Из кэша</Badge>}
          {a.out_of_scope && <Badge tone="mixed">Вне рамок анализа</Badge>}
          {q.outdated && (
            <Badge tone="mixed" title="Анализ был пересчитан после этого вопроса">
              К предыдущей версии анализа
            </Badge>
          )}
        </div>
        <p className="text-[15px] leading-relaxed">{a.answer}</p>
        {a.points.length > 0 && (
          <ul className="mt-3 space-y-2">
            {a.points.map((p, i) => (
              <li key={i} className="text-[14px] leading-relaxed">
                <span>
                  {p.text}
                  <SourceChips ids={p.source_ids} evidence={evidence} prefix={prefix} />
                </span>
                <Badge tone={pointBasis[p.basis].tone} className="ml-1.5 align-middle text-[11px]">
                  {pointBasis[p.basis].label}
                </Badge>
              </li>
            ))}
          </ul>
        )}
        {a.not_in_sources && (
          <p className="mt-3 rounded-[10px] bg-frost/70 px-3 py-2 text-[13px] text-ink-soft">
            <span className="font-medium">Нет в источниках: </span>
            {a.not_in_sources}
          </p>
        )}
        {a.sources.length > 0 && (
          <div className="mt-4 border-t border-line pt-3">
            <p className="text-[12px] font-medium text-silver">Источники ответа</p>
            <ul className="mt-2 space-y-3">
              {a.sources.map((e) => (
                <EvidenceItem key={e.id} e={e} prefix={prefix} />
              ))}
            </ul>
          </div>
        )}
        <p className="mt-3 text-[12px] text-silver">
          {formatDate(q.created_at)}
          {q.method === "llm" && q.model && ` · ${q.model}`}
          {q.tokens > 0 && ` · ${q.tokens.toLocaleString("ru-RU")} токенов`}
          {q.duration_ms > 0 && q.method === "llm" && !q.cached && ` · ${(q.duration_ms / 1000).toFixed(1)} с`}
          {a.validation_notes && a.validation_notes.length > 0 && ` · исправлений валидатора: ${a.validation_notes.length}`}
        </p>
      </div>
    </li>
  );
}
