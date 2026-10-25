"use client";

import Link from "next/link";
import { useEffect, useState } from "react";
import { AnimatePresence, motion } from "motion/react";
import { ArrowLeft, Check, CircleAlert, Loader2, RotateCcw, Sparkles, ThumbsDown, ThumbsUp } from "lucide-react";
import { toast } from "sonner";
import { useMe } from "@/components/app-shell";
import { Button } from "@/components/ui/button";
import { Dialog, DialogContent, DialogTrigger } from "@/components/ui/dialog";
import { Textarea } from "@/components/ui/field";
import { Badge, ErrorState, Panel, Skeleton } from "@/components/ui/misc";
import { api, ApiError, errorMessage } from "@/lib/api";
import { formatDate, horizonLabel, impactLabel, likelihoodLabel } from "@/lib/format";
import type { ArticleDetail, Evidence, FullAnalysis, Insight, RiskOpp, Run } from "@/lib/types";
import { useApi } from "@/lib/use-api";
import { cn } from "@/lib/utils";
import { AccuracyStrip, CompletenessBadge, FactCheckPanel } from "./accuracy";
import { EvidenceItem, SourceChips } from "./evidence";
import { AnalyzingMark } from "./analyzing";
import { AskAnalysis } from "./questions";
import { AgentTrace } from "./trace";
import { ImportanceBadge, MethodTag, RelevanceMeter, SaveButton, ShareButton, SourceLine, WhyItMatters, impactTone } from "./parts";

const basisMeta = {
  fact: { label: "Факт из новости", tone: "neutral" as const },
  bank_view: { label: "Позиция Альфа-Банка", tone: "bank" as const },
  model_inference: { label: "Вывод модели", tone: "outline" as const },
};

const plannedSteps = [
  "Классификация новости",
  "Профиль компании",
  "Планирование анализа",
  "Поиск в аналитике Альфа-Банка",
  "Персональный анализ",
  "Самопроверка по источникам",
];

export function ArticleView({ id }: { id: string }) {
  const { data, error, loading, reload, mutate } = useApi<ArticleDetail>(`/articles/${id}`);
  if (error) {
    if (error instanceof ApiError && error.status === 404)
      return (
        <div className="py-16 text-center">
          <p className="text-lg font-medium">Новость не найдена</p>
          <Button asChild variant="outline" className="mt-4">
            <Link href="/feed">Вернуться в ленту</Link>
          </Button>
        </div>
      );
    return <ErrorState error={error} onRetry={reload} />;
  }
  if (loading || !data)
    return (
      <div className="space-y-4">
        <Skeleton className="h-6 w-40" />
        <Skeleton className="h-24" />
        <Skeleton className="h-72" />
      </div>
    );
  return <Article a={data} onAnalyzed={(analysis, run) => mutate((d) => (d ? { ...d, analysis, run } : d))} />;
}

function Article({ a, onAnalyzed }: { a: ArticleDetail; onAnalyzed: (x: FullAnalysis, run?: Run) => void }) {
  const insight = a.analysis?.result?.insight;
  const showBody = !!a.body;
  return (
    <div>
      <Link href="/feed" className="inline-flex items-center gap-1.5 text-[13px] text-ink-soft hover:text-ink">
        <ArrowLeft className="size-4" aria-hidden /> Лента
      </Link>

      <header className="mt-4 max-w-[860px]">
        <SourceLine item={a} />
        <h1 className="display mt-2 text-[28px] sm:text-[36px]">{a.title}</h1>
        {a.summary && !showBody && <p className="mt-3 text-[16px] leading-relaxed text-ink-soft">{a.summary}</p>}
        <div className="mt-4 flex flex-wrap items-center gap-2">
          {a.topics.map((t) => (
            <Badge key={t.code} tone="outline" title={t.method === "keywords" ? "Тема определена по словарю, без ИИ" : t.method === "llm" ? "Тема определена моделью" : "Тема по названию материала"}>
              {t.name}
            </Badge>
          ))}
          <SaveButton id={a.id} saved={a.is_saved} />
          <ShareButton id={a.id} title={a.title} url={a.url} withLabel />
          <CompletenessBadge c={a.source_completeness} />
          {a.updated_at && <Badge tone="outline" title="Источник изменил текст публикации; анализ строится по актуальной версии">Обновлено {formatDate(a.updated_at)}</Badge>}
          {a.source.kind === "user_submitted" && <Badge tone="mixed">Добавлено вручную, достоверность не проверена</Badge>}
        </div>
      </header>

      <Lens a={a} insight={insight} />

      {insight && a.analysis ? (
        <AnalysisBody article={a} analysis={a.analysis} insight={insight} run={a.run} onRerun={onAnalyzed} />
      ) : (
        <RunAnalysis a={a} onDone={onAnalyzed} />
      )}

      {showBody && (
        <Panel title={a.source.kind === "alfa_research" ? "Текст материала" : "Текст новости"} className="mt-6">
          <div className="max-w-[72ch] whitespace-pre-line text-[15px] leading-relaxed text-ink-soft">{a.body}</div>
          {a.source.kind === "alfa_research" && (
            <Link href={a.url} className="mt-4 inline-flex text-sm text-steel hover:underline">
              Открыть материал с разбивкой по страницам
            </Link>
          )}
        </Panel>
      )}
    </div>
  );
}

// «Линза»: для кого и в каком контексте построен вывод — главный визуальный элемент экрана.
function Lens({ a, insight }: { a: ArticleDetail; insight?: Insight }) {
  const { me } = useMe();
  const company = me.companies.find((c) => c.is_primary) ?? me.companies[0];
  const score = insight ? insight.relevance_score : a.brief?.relevance_score;
  const method = insight ? "llm" : a.brief?.method;
  return (
    <section className="relative mt-8 overflow-hidden rounded-[var(--radius-xl)]" aria-label="Контекст анализа">
      <div aria-hidden className="app-backdrop absolute inset-0" />
      <div className="glass relative flex flex-wrap items-center gap-x-8 gap-y-4 rounded-[var(--radius-xl)] bg-white/55 p-5 sm:p-6">
        {score != null && method && <RelevanceMeter score={score} method={method} size={64} />}
        <div className="min-w-[200px] flex-1">
          <p className="text-[13px] text-ink-soft">Анализ для роли</p>
          <p className="text-[19px] font-semibold tracking-[-0.01em]">{me.role?.title ?? "Роль не выбрана"}</p>
          <p className="mt-0.5 text-[13px] text-ink-soft">
            {company ? company.display_name || company.okved_name : "Компания не выбрана"}
            {company?.region ? `, ${company.region}` : ""}
          </p>
        </div>
        <div className="flex flex-wrap gap-1.5">
          {method && <MethodTag method={method} />}
          {insight ? (
            <>
              <ImportanceBadge level={insight.importance_level} />
              <Badge tone={impactTone[insight.impact_type]}>{impactLabel[insight.impact_type]}</Badge>
            </>
          ) : (
            a.brief && <ImportanceBadge level={a.brief.importance_level} />
          )}
        </div>
        <Link href="/settings" className="text-[13px] text-steel hover:underline">
          Изменить роль
        </Link>
      </div>
    </section>
  );
}

function RunAnalysis({ a, onDone }: { a: ArticleDetail; onDone: (x: FullAnalysis, run?: Run) => void }) {
  const [busy, setBusy] = useState(false);
  const [err, setErr] = useState<ApiError | null>(null);
  const [stepIdx, setStepIdx] = useState(0);

  useEffect(() => {
    if (!busy) return;
    setStepIdx(0);
    const t = setInterval(() => setStepIdx((i) => Math.min(i + 1, plannedSteps.length - 1)), 6000);
    return () => clearInterval(t);
  }, [busy]);

  async function run() {
    setBusy(true);
    setErr(null);
    try {
      const r = await api<{ analysis: FullAnalysis; run?: Run; cached: boolean }>(`/articles/${a.id}/analyze`, { method: "POST" });
      onDone(r.analysis, r.run);
      toast.success(r.cached ? "Анализ загружен из сохранённых" : "Анализ готов");
    } catch (e) {
      setErr(e instanceof ApiError ? e : new ApiError(0, "unknown", errorMessage(e)));
    } finally {
      setBusy(false);
    }
  }

  return (
    <div className="mt-6 grid gap-6 lg:grid-cols-[minmax(0,1.4fr)_minmax(0,1fr)]">
      <Panel title="Персональный анализ" description="Ещё не выполнялся для вашей роли и компании">
        {a.brief && (
          <div className="mb-5">
            <p className="mb-2 text-[13px] text-silver">Почему новость в вашей ленте</p>
            <WhyItMatters a={a.brief} />
          </div>
        )}
        {busy ? (
          <div className="flex flex-col items-center py-6 text-center" role="status" aria-live="polite">
            <AnalyzingMark size={176} />
            <p className="mt-6 text-[17px] font-semibold tracking-[-0.01em]">Анализ выполняется</p>
            <AnimatePresence mode="wait">
              <motion.p
                key={stepIdx}
                initial={{ opacity: 0, y: 6 }}
                animate={{ opacity: 1, y: 0 }}
                exit={{ opacity: 0, y: -6 }}
                transition={{ duration: 0.25 }}
                className="mt-1 text-[14px] text-ink-soft"
              >
                {plannedSteps[stepIdx]}…
              </motion.p>
            </AnimatePresence>
            <p className="mt-3 text-[12px] text-silver">Обычно до минуты. Можно не закрывать страницу — результат сохранится.</p>
          </div>
        ) : (
          <>
            <p className="max-w-[60ch] text-[15px] leading-relaxed text-ink-soft">
              Агент составит план, найдёт подтверждения в аналитике Альфа-Банка и связанных новостях, сформулирует последствия для
              вашей роли и проверит каждый вывод по источникам. Обычно это 4–5 запросов к модели и до минуты ожидания.
            </p>
            {err && <AnalyzeError err={err} />}
            <Button size="lg" className="mt-5" onClick={run}>
              <Sparkles /> Запустить анализ
            </Button>
          </>
        )}
      </Panel>
      <Panel title="Как строится вывод">
        <ol className="space-y-2.5" aria-live="polite">
          {plannedSteps.map((s, i) => {
            const state = !busy ? "idle" : i < stepIdx ? "done" : i === stepIdx ? "active" : "idle";
            return (
              <li key={s} className="flex items-center gap-3 text-[14px]">
                <span
                  className={cn(
                    "flex size-6 items-center justify-center rounded-full text-[11px] tabular",
                    state === "done" && "bg-steel text-white",
                    state === "active" && "bg-white text-steel ring-2 ring-steel",
                    state === "idle" && "bg-frost text-silver",
                  )}
                >
                  {state === "done" ? <Check className="size-3.5" /> : state === "active" ? <Loader2 className="size-3.5 animate-spin" /> : i + 1}
                </span>
                <span className={state === "idle" ? "text-ink-soft" : "text-ink"}>{s}</span>
              </li>
            );
          })}
        </ol>
        {busy && <p className="mt-4 text-[12px] text-silver">Шаги показаны ориентировочно; точная трасса появится после завершения.</p>}
      </Panel>
    </div>
  );
}

function AnalyzeError({ err }: { err: ApiError }) {
  const hint: Record<string, string> = {
    llm_not_configured: "Администратору нужно указать адрес API и ключ в .env.",
    llm_budget_exceeded: "Дневной лимит общий для команды. Уже выполненные анализы остаются доступны.",
    llm_rate_limited: "Повторите через несколько минут.",
    user_rate_limited: "Лимит защищает общий бюджет команды.",
    llm_unavailable: "Проверьте доступность провайдера модели.",
  };
  return (
    <div role="alert" className="mt-5 flex gap-3 rounded-[12px] bg-risk-bg px-4 py-3 text-[14px] text-risk">
      <CircleAlert className="mt-0.5 size-4 shrink-0" aria-hidden />
      <div>
        <p className="font-medium">{err.message}</p>
        {hint[err.code] && <p className="mt-0.5 text-risk/80">{hint[err.code]}</p>}
      </div>
    </div>
  );
}

function BasisTag({ basis }: { basis: RiskOpp["basis"] }) {
  const m = basisMeta[basis];
  return <Badge tone={m.tone}>{m.label}</Badge>;
}

function AnalysisBody({
  article,
  analysis,
  insight,
  run,
  onRerun,
}: {
  article: ArticleDetail;
  analysis: FullAnalysis;
  insight: Insight;
  run?: Run;
  onRerun: (x: FullAnalysis, run?: Run) => void;
}) {
  const evidence = new Map(insight.evidence_sources.map((e) => [e.id, e]));
  return (
    <div className="mt-6 grid gap-6 xl:grid-cols-[minmax(0,1fr)_360px]">
      <div className="space-y-6">
        <AccuracyStrip insight={insight} />
        <div className="flex flex-wrap items-center gap-3 text-[12px] text-ink-soft">
          <span>Обозначения:</span>
          <Badge tone="neutral">Факт из новости</Badge>
          <Badge tone="bank">Позиция Альфа-Банка</Badge>
          <Badge tone="outline">Вывод модели</Badge>
          <span className="text-silver">Метки в тексте ведут к источникам.</span>
        </div>

        <Panel title="Что произошло" description="Подтверждённые факты из источников">
          <p className="text-[16px] leading-relaxed">{insight.what_happened.summary}</p>
          {insight.what_happened.facts.length > 0 && (
            <ul className="mt-4 space-y-2">
              {insight.what_happened.facts.map((f, i) => (
                <li key={i} className="flex gap-2.5 text-[15px] leading-relaxed">
                  <Check className="mt-1 size-4 shrink-0 text-chance" aria-hidden />
                  <span>
                    {f.text}
                    <SourceChips ids={f.source_ids} evidence={evidence} />
                  </span>
                </li>
              ))}
            </ul>
          )}
        </Panel>

        <Panel title="Почему это важно для вашей компании">
          <p className="text-[16px] leading-relaxed">
            {insight.why_it_matters}
            <SourceChips ids={insight.why_it_matters_source_ids} evidence={evidence} />
          </p>
          {insight.used_company_factors && insight.used_company_factors.length > 0 && (
            <div className="mt-5 border-t border-line pt-4">
              <h3 className="text-[15px] font-semibold">Через какие признаки компании</h3>
              <ul className="mt-2 space-y-2">
                {insight.used_company_factors.map((f) => (
                  <li key={f.fact_id} className="flex gap-2.5 text-[14px] leading-relaxed">
                    <span className="shrink-0 self-start">
                      <SourceChips ids={[f.fact_id]} evidence={evidence} />
                    </span>
                    <span>
                      {evidence.get(f.fact_id) && <span className="font-medium">{evidence.get(f.fact_id)!.title}: </span>}
                      <span className="text-ink-soft">{f.mechanism}</span>
                    </span>
                  </li>
                ))}
              </ul>
            </div>
          )}
          {insight.potential_impact && (
            <div className="mt-5 border-t border-line pt-4">
              <div className="flex items-center gap-2">
                <h3 className="text-[15px] font-semibold">Возможное влияние на бизнес</h3>
                <Badge tone="outline">Вывод модели</Badge>
              </div>
              <p className="mt-2 text-[15px] leading-relaxed text-ink-soft">{insight.potential_impact}</p>
            </div>
          )}
        </Panel>

        <div className="grid gap-6 lg:grid-cols-2">
          <RiskList title="Риски" items={insight.risks} tone="risk" evidence={evidence} empty="Существенных рисков не выявлено." />
          <RiskList title="Возможности" items={insight.opportunities} tone="chance" evidence={evidence} empty="Возможностей не выявлено." />
        </div>

        <Panel title="Рекомендуемые действия" description="Что сделать, с чем, кто отвечает и в какой срок">
          {insight.recommended_actions.length === 0 ? (
            <p className="text-sm text-ink-soft">Модель не предложила действий для этой новости.</p>
          ) : (
            <ul className="divide-y divide-line">
              {insight.recommended_actions.map((x, i) => (
                <li key={i} className="py-3 first:pt-0 last:pb-0">
                  <p className="text-[15px] font-medium">
                    {x.action}
                    <SourceChips ids={x.source_ids} evidence={evidence} />
                  </p>
                  {x.rationale && (
                    <p className="mt-1 text-[14px] text-ink-soft">
                      <span className="text-silver">Почему: </span>
                      {x.rationale}
                    </p>
                  )}
                  <div className="mt-2 flex flex-wrap gap-1.5">
                    {x.object && <Badge tone="neutral" title="Объект действия">{x.object}</Badge>}
                    {x.owner && <Badge tone="outline" title="Ответственный">{x.owner}</Badge>}
                    {x.horizon && <Badge tone="outline" title="Горизонт">{horizonLabel[x.horizon]}</Badge>}
                  </div>
                </li>
              ))}
            </ul>
          )}
        </Panel>

        <Panel title="Как это оценивает Альфа-Банк" description="Только позиции из аналитических материалов банка">
          {insight.bank_view.length === 0 ? (
            <p className="text-sm text-ink-soft">В предоставленных материалах Альфа-Банка нет позиции, напрямую относящейся к этому событию.</p>
          ) : (
            <ul className="space-y-3">
              {insight.bank_view.map((b, i) => (
                <li key={i} className="border-l-2 border-bankview pl-4 text-[15px] leading-relaxed">
                  {b.text}
                  <SourceChips ids={b.source_ids} evidence={evidence} />
                </li>
              ))}
            </ul>
          )}
        </Panel>

        {(insight.assumptions.length > 0 || insight.uncertainty_notes.length > 0) && (
          <Panel title="Допущения и ограничения" description="Что не подтверждено данными и чего не хватает для точного вывода">
            <ul className="space-y-2 text-[14px] leading-relaxed text-ink-soft">
              {insight.assumptions.map((x, i) => (
                <li key={`a${i}`} className="flex gap-2.5">
                  <Badge tone="outline" className="shrink-0 whitespace-nowrap self-start">Допущение</Badge>
                  <span>{x}</span>
                </li>
              ))}
              {insight.uncertainty_notes.map((x, i) => (
                <li key={`u${i}`} className="flex gap-2.5">
                  <Badge tone="neutral" className="shrink-0 whitespace-nowrap self-start">Ограничение</Badge>
                  <span>{x}</span>
                </li>
              ))}
            </ul>
          </Panel>
        )}

        <AskAnalysis key={analysis.id + analysis.created_at} articleId={article.id} analysisId={analysis.id} />
        <AgentTrace analysis={analysis} insight={insight} run={run} evidence={evidence} />
        <Feedback article={article} />
        <Disclaimer />
      </div>

      <aside className="space-y-6">
        <Panel title="Источники" description={`${insight.evidence_sources.length} использовано в выводах`}>
          <ul className="space-y-3">
            {insight.evidence_sources.map((e) => (
              <EvidenceItem key={e.id} e={e} />
            ))}
          </ul>
        </Panel>
        <FactCheckPanel insight={insight} />
        <Rerun articleId={article.id} createdAt={analysis.created_at} onDone={onRerun} />
      </aside>
    </div>
  );
}

// Обязательный дисклеймер (ответ организаторов на вебинаре, п. 49).
export function Disclaimer() {
  return (
    <p className="px-1 text-[12px] leading-relaxed text-silver">
      Материал подготовлен автоматически на основе открытых источников и аналитики Альфа-Банка и не является
      индивидуальной инвестиционной рекомендацией. Выводы, помеченные как «вывод модели», сформированы ИИ и не
      отражают официальную позицию банка.
    </p>
  );
}

function RiskList({
  title,
  items,
  tone,
  evidence,
  empty,
}: {
  title: string;
  items: RiskOpp[];
  tone: "risk" | "chance";
  evidence: Map<string, Evidence>;
  empty: string;
}) {
  return (
    <Panel title={title}>
      {items.length === 0 ? (
        <p className="text-sm text-ink-soft">{empty}</p>
      ) : (
        <ul className="space-y-4">
          {items.map((r, i) => (
            <li key={i} className={cn("border-l-2 pl-4", tone === "risk" ? "border-risk" : "border-chance")}>
              <p className="text-[15px] font-semibold">{r.title}</p>
              <p className="mt-1 text-[14px] leading-relaxed text-ink-soft">
                {r.description}
                <SourceChips ids={r.source_ids} evidence={evidence} />
              </p>
              <div className="mt-2 flex flex-wrap gap-1.5">
                <BasisTag basis={r.basis} />
                <Badge tone="outline">{likelihoodLabel[r.likelihood]}</Badge>
                <Badge tone="outline">{horizonLabel[r.horizon]}</Badge>
              </div>
            </li>
          ))}
        </ul>
      )}
    </Panel>
  );
}

function Rerun({ articleId, createdAt, onDone }: { articleId: string; createdAt: string; onDone: (x: FullAnalysis, run?: Run) => void }) {
  const [open, setOpen] = useState(false);
  const [busy, setBusy] = useState(false);
  async function go() {
    setBusy(true);
    try {
      const r = await api<{ analysis: FullAnalysis; run?: Run }>(`/articles/${articleId}/analyze?force=true`, { method: "POST" });
      onDone(r.analysis, r.run);
      toast.success("Анализ пересчитан");
      setOpen(false);
    } catch (e) {
      toast.error(errorMessage(e));
    } finally {
      setBusy(false);
    }
  }
  return (
    <div className="px-1 text-[12px] text-silver">
      Анализ от {formatDate(createdAt)}.{" "}
      <Dialog open={open} onOpenChange={setOpen}>
        <DialogTrigger asChild>
          <button className="inline-flex items-center gap-1 text-steel hover:underline">
            <RotateCcw className="size-3" aria-hidden /> Пересчитать
          </button>
        </DialogTrigger>
        <DialogContent title="Пересчитать анализ?" description="Это 4–5 новых запросов к модели из общего дневного лимита команды.">
          <p className="text-sm text-ink-soft">
            Если вы изменили роль, интересы или данные компании, анализ пересчитается автоматически при следующем открытии.
          </p>
          <div className="mt-6 flex justify-end gap-2">
            <Button variant="ghost" onClick={() => setOpen(false)}>
              Отмена
            </Button>
            <Button onClick={go} loading={busy}>
              Пересчитать
            </Button>
          </div>
        </DialogContent>
      </Dialog>
    </div>
  );
}

function Feedback({ article }: { article: ArticleDetail }) {
  const [rating, setRating] = useState<number | null>(article.feedback?.rating ?? null);
  const [comment, setComment] = useState(article.feedback?.comment ?? "");
  const [busy, setBusy] = useState(false);
  async function send(r: number) {
    setBusy(true);
    try {
      await api(`/articles/${article.id}/feedback`, { method: "POST", body: { rating: r, comment } });
      setRating(r);
      toast.success("Спасибо, отзыв сохранён");
    } catch (e) {
      toast.error(errorMessage(e));
    } finally {
      setBusy(false);
    }
  }
  return (
    <Panel title="Анализ был полезен?" description="Отзывы используются для оценки качества персонализации">
      <Textarea
        aria-label="Комментарий к анализу"
        value={comment}
        onChange={(e) => setComment(e.target.value)}
        placeholder="Что было неточно или чего не хватило (необязательно)"
        className="min-h-16"
        maxLength={1000}
      />
      <div className="mt-3 flex gap-2">
        <Button variant={rating === 1 ? "primary" : "outline"} size="sm" disabled={busy} onClick={() => send(1)} aria-pressed={rating === 1}>
          <ThumbsUp /> Полезно
        </Button>
        <Button variant={rating === -1 ? "primary" : "outline"} size="sm" disabled={busy} onClick={() => send(-1)} aria-pressed={rating === -1}>
          <ThumbsDown /> Не полезно
        </Button>
      </div>
    </Panel>
  );
}
