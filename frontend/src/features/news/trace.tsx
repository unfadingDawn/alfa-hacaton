"use client";

import { useState } from "react";
import { AlertTriangle, Check, ChevronDown, CircleSlash, Route } from "lucide-react";
import { Badge, Panel } from "@/components/ui/misc";
import type { Evidence, FullAnalysis, Insight, Run, RunStep } from "@/lib/types";
import { cn } from "@/lib/utils";
import { SourceChips } from "./evidence";

// Раскрытая трасса агента (критерий 12): план, инструменты и источники,
// признаки компании, самопроверка и доработка. Скрытые рассуждения модели
// не сохраняются — показываются только структурированные результаты шагов.

type Tab = "plan" | "sources" | "factors" | "check" | "steps";

const tabs: { id: Tab; label: string }[] = [
  { id: "plan", label: "План" },
  { id: "sources", label: "Источники" },
  { id: "factors", label: "Признаки компании" },
  { id: "check", label: "Самопроверка" },
  { id: "steps", label: "Шаги" },
];

const evType: Record<Evidence["type"], string> = {
  article: "Новость",
  related_news: "Связанная новость",
  bank_research: "Аналитика Альфа-Банка",
  company_data: "Данные о компании",
};

export function AgentTrace({ analysis, insight, run, evidence }: { analysis: FullAnalysis; insight: Insight; run?: Run; evidence: Map<string, Evidence> }) {
  const [tab, setTab] = useState<Tab>("plan");
  const [open, setOpen] = useState(false);
  const steps = run?.steps ?? [];
  const tokens = run ? run.prompt_tokens + run.completion_tokens : 0;
  const result = analysis.result;
  const revision = result?.revision;
  const llmSteps = steps.filter((s) => s.model).length;
  return (
    <Panel
      title={
        <span className="inline-flex items-center gap-2">
          <Route className="size-5 text-steel" aria-hidden /> Как получен вывод
        </span>
      }
      description={
        run
          ? `${steps.length} шагов, из них ${llmSteps} запросов к модели · ${tokens.toLocaleString("ru-RU")} токенов${revision?.applied ? " · была доработка" : ""}`
          : "Трасса запуска недоступна"
      }
      action={
        <button
          className="inline-flex items-center gap-1 text-[13px] text-steel hover:underline"
          onClick={() => setOpen(!open)}
          aria-expanded={open}
        >
          {open ? "Свернуть" : "Показать трассу"}
          <ChevronDown className={cn("size-4 transition-transform", open && "rotate-180")} aria-hidden />
        </button>
      }
    >
      {open && (
        <div>
          <div className="mb-4 flex flex-wrap gap-1.5" role="tablist" aria-label="Разделы трассы">
            {tabs.map((t) => (
              <button
                key={t.id}
                role="tab"
                aria-selected={tab === t.id}
                onClick={() => setTab(t.id)}
                className={cn(
                  "rounded-full px-3 py-1 text-[13px] font-medium transition-colors",
                  tab === t.id ? "bg-steel text-white" : "bg-frost text-ink-soft hover:text-ink",
                )}
              >
                {t.label}
              </button>
            ))}
          </div>
          <div role="tabpanel">
            {tab === "plan" && <PlanTab analysis={analysis} />}
            {tab === "sources" && <SourcesTab analysis={analysis} evidence={evidence} />}
            {tab === "factors" && <FactorsTab insight={insight} evidence={evidence} />}
            {tab === "check" && <CheckTab analysis={analysis} insight={insight} />}
            {tab === "steps" && <StepsTab steps={steps} models={run?.models ?? []} />}
          </div>
          <p className="mt-5 border-t border-line pt-3 text-[12px] text-silver">
            Скрытые рассуждения модели не сохраняются и не показываются: трасса содержит только структурированные результаты шагов —
            план, запросы к инструментам, найденные источники, замечания контролёра и изменения после доработки.
          </p>
        </div>
      )}
    </Panel>
  );
}

function Empty({ children }: { children: React.ReactNode }) {
  return <p className="text-sm text-ink-soft">{children}</p>;
}

function List({ title, items, mono }: { title: string; items?: string[]; mono?: boolean }) {
  if (!items?.length) return null;
  return (
    <div>
      <h4 className="text-[13px] font-medium text-silver">{title}</h4>
      <ul className="mt-1.5 space-y-1 text-[14px] leading-relaxed">
        {items.map((x, i) => (
          <li key={i} className={cn("flex gap-2", mono && "tabular")}>
            <span className="text-silver">•</span>
            <span>{x}</span>
          </li>
        ))}
      </ul>
    </div>
  );
}

function PlanTab({ analysis }: { analysis: FullAnalysis }) {
  const plan = analysis.result?.plan;
  const cls = analysis.result?.classification;
  if (!plan) return <Empty>План не сохранён для этого анализа (выполнен до появления раскрытой трассы).</Empty>;
  return (
    <div className="grid gap-5 sm:grid-cols-2">
      <List title="Вопросы, на которые отвечает анализ" items={plan.focus_questions} />
      <List title="Гипотезы о механизме влияния" items={plan.hypotheses} />
      <List title="Запросы к аналитике Альфа-Банка" items={plan.research_queries.map((q) => `«${q}»`)} />
      <List title="Запросы к связанным новостям" items={plan.news_queries.map((q) => `«${q}»`)} />
      {plan.company_fact_ids.length > 0 && (
        <div>
          <h4 className="text-[13px] font-medium text-silver">Признаки компании для проверки</h4>
          <p className="mt-1.5 text-[14px]">{plan.company_fact_ids.join(", ")}</p>
        </div>
      )}
      {cls?.event_type && (
        <div>
          <h4 className="text-[13px] font-medium text-silver">Тип события по классификации</h4>
          <p className="mt-1.5 text-[14px]">{cls.event_type}</p>
        </div>
      )}
    </div>
  );
}

function SourcesTab({ analysis, evidence }: { analysis: FullAnalysis; evidence: Map<string, Evidence> }) {
  const rep = analysis.result?.sources_report;
  if (!rep) return <Empty>Отчёт об источниках не сохранён для этого анализа.</Empty>;
  const used = rep.sources.filter((s) => s.used);
  const unused = rep.sources.filter((s) => !s.used);
  return (
    <div className="space-y-5">
      <div>
        <h4 className="text-[13px] font-medium text-silver">Использованы в выводах ({used.length})</h4>
        <ul className="mt-2 space-y-2.5">
          {used.map((s) => (
            <li key={s.id} className="text-[14px]">
              <div className="flex flex-wrap items-center gap-1.5">
                <SourceChips ids={[s.id]} evidence={evidence} />
                <span className="font-medium">{s.title}</span>
                <span className="text-[12px] text-silver">
                  {evType[s.type]}
                  {s.page ? `, стр. ${s.page}` : ""}
                </span>
              </div>
              <p className="mt-0.5 text-[13px] text-ink-soft">
                {s.sections.length > 0 ? `Используется: ${s.sections.join(", ")}` : "Основа анализа"}
                {s.query && ` · найден по запросу «${s.query}»`}
              </p>
            </li>
          ))}
        </ul>
      </div>
      {unused.length > 0 && (
        <div>
          <h4 className="text-[13px] font-medium text-silver">Найдены, но не использованы ({unused.length})</h4>
          <ul className="mt-2 space-y-1.5 text-[14px] text-ink-soft">
            {unused.map((s) => (
              <li key={s.id}>
                <span className="tabular text-silver">{s.id}</span> {s.title}
                {s.page ? `, стр. ${s.page}` : ""}
                {s.query && <span className="text-[12px] text-silver"> · запрос «{s.query}»</span>}
              </li>
            ))}
          </ul>
          <p className="mt-1.5 text-[12px] text-silver">Агент получил эти материалы, но не опирался на них: они не относятся к выводам напрямую.</p>
        </div>
      )}
      <div>
        <h4 className="text-[13px] font-medium text-silver">Запросы без результатов ({rep.empty_queries.length})</h4>
        {rep.empty_queries.length === 0 ? (
          <p className="mt-1.5 text-[14px] text-ink-soft">Каждый запрос что-то нашёл.</p>
        ) : (
          <ul className="mt-1.5 space-y-1 text-[14px]">
            {rep.empty_queries.map((q, i) => (
              <li key={i} className="flex items-center gap-2 text-ink-soft">
                <CircleSlash className="size-3.5 text-silver" aria-hidden />«{q.query}»
                <span className="text-[12px] text-silver">
                  {q.tool === "search_news" ? "новости" : "аналитика банка"}
                  {q.stage === "self_check" ? ", дополнительный поиск после самопроверки" : ""}
                </span>
              </li>
            ))}
          </ul>
        )}
      </div>
      <p className="text-[12px] text-silver">Всего признаков компании в профиле: {rep.company_facts_total}; в списке выше — только использованные.</p>
    </div>
  );
}

function FactorsTab({ insight, evidence }: { insight: Insight; evidence: Map<string, Evidence> }) {
  const factors = insight.used_company_factors ?? [];
  const cited = insight.evidence_sources.filter((e) => e.type === "company_data" && !factors.some((f) => f.fact_id === e.id));
  if (!factors.length && !cited.length) return <Empty>Анализ не опирается на признаки компании.</Empty>;
  return (
    <ul className="space-y-3">
      {factors.map((f) => (
        <li key={f.fact_id} className="text-[14px]">
          <div className="flex flex-wrap items-center gap-1.5">
            <SourceChips ids={[f.fact_id]} evidence={evidence} />
            <span className="font-medium">{evidence.get(f.fact_id)?.title ?? f.fact_id}</span>
            {evidence.get(f.fact_id) && <span className="text-ink-soft">— {evidence.get(f.fact_id)!.excerpt}</span>}
          </div>
          <p className="mt-0.5 text-ink-soft">
            <span className="text-silver">Механизм: </span>
            {f.mechanism}
          </p>
        </li>
      ))}
      {cited.map((e) => (
        <li key={e.id} className="text-[14px]">
          <SourceChips ids={[e.id]} evidence={evidence} /> <span className="font-medium">{e.title}</span>{" "}
          <span className="text-ink-soft">— {e.excerpt}</span>
          <p className="mt-0.5 text-[12px] text-silver">Упоминается в выводах без описанного механизма.</p>
        </li>
      ))}
    </ul>
  );
}

function CheckTab({ analysis, insight }: { analysis: FullAnalysis; insight: Insight }) {
  const rev = analysis.result?.revision;
  const fc = insight.fact_check;
  return (
    <div className="space-y-5">
      {rev ? (
        <div className={cn("flex gap-2.5 rounded-[12px] px-4 py-3 text-[14px]", rev.applied ? "bg-mixed-bg text-mixed" : "bg-chance-bg text-chance")}>
          {rev.applied ? <AlertTriangle className="mt-0.5 size-4 shrink-0" aria-hidden /> : <Check className="mt-0.5 size-4 shrink-0" aria-hidden />}
          <span>{rev.note || (rev.applied ? "Анализ доработан" : "Самопроверка пройдена")}</span>
        </div>
      ) : (
        <Empty>Итог самопроверки не сохранён для этого анализа.</Empty>
      )}
      {rev && rev.issues.length > 0 && (
        <div>
          <h4 className="text-[13px] font-medium text-silver">Замечания контролёра ({rev.issues.length})</h4>
          <ul className="mt-2 space-y-2 text-[14px]">
            {rev.issues.map((x, i) => (
              <li key={i} className="flex gap-2">
                <Badge tone={x.severity === "high" ? "risk" : "neutral"} className="shrink-0 self-start whitespace-nowrap">
                  {x.severity === "high" ? "серьёзное" : "некритичное"}
                </Badge>
                <span>
                  <span className="tabular text-silver">{x.path}</span> {x.problem}
                </span>
              </li>
            ))}
          </ul>
        </div>
      )}
      {rev?.applied && (
        <List title="Что изменилось после доработки" items={rev.changes} />
      )}
      {fc && (
        <p className="text-[13px] text-ink-soft">
          Проверка чисел, дат и названий: проверено {fc.checked}, не найдено в источниках {fc.unverified.length}.
          {insight.source_coverage && ` Покрытие источниками ${Math.round(insight.source_coverage.ratio * 100)}%.`}
        </p>
      )}
    </div>
  );
}

// Детали шагов: показываются только структурированные результаты.
function StepsTab({ steps, models }: { steps: RunStep[]; models: string[] }) {
  if (!steps.length) return <Empty>Шаги не сохранены.</Empty>;
  return (
    <ol className="space-y-2">
      {steps.map((s, i) => (
        <StepItem key={i} n={i + 1} s={s} />
      ))}
      <li className="pt-1 text-[12px] text-silver">Модели: {models.join(", ") || "нет"}</li>
    </ol>
  );
}

function StepItem({ n, s }: { n: number; s: RunStep }) {
  const [open, setOpen] = useState(false);
  const body = stepDetail(s);
  return (
    <li className="rounded-[12px] border border-line bg-white/50">
      <button
        className="flex w-full items-start justify-between gap-3 px-3.5 py-2.5 text-left"
        onClick={() => setOpen(!open)}
        aria-expanded={open}
        disabled={!body}
      >
        <span>
          <span className={cn("text-[14px] font-medium", s.status === "error" && "text-risk")}>
            {n}. {s.title}
          </span>
          <span className="mt-0.5 block text-[13px] text-ink-soft">{s.summary}</span>
          <span className="mt-0.5 block text-[12px] text-silver">
            {s.model ? `${s.model}${s.tokens ? `, ${s.tokens} токенов` : ""}${s.cached ? ", из кэша" : ""}` : "инструмент, без модели"}
            {` · ${(s.duration_ms / 1000).toFixed(1)} с`}
          </span>
        </span>
        {body && <ChevronDown className={cn("mt-1 size-4 shrink-0 text-silver transition-transform", open && "rotate-180")} aria-hidden />}
      </button>
      {open && body && <div className="border-t border-line px-3.5 py-3 text-[13px]">{body}</div>}
    </li>
  );
}

type Obj = Record<string, unknown>;
const isObj = (x: unknown): x is Obj => typeof x === "object" && x !== null && !Array.isArray(x);
const strs = (x: unknown): string[] => (Array.isArray(x) ? x.filter((v): v is string => typeof v === "string") : []);

function stepDetail(s: RunStep): React.ReactNode {
  const d = s.detail;
  if (d == null) return null;
  if (s.name.startsWith("search_") && Array.isArray(d)) {
    if (d.length === 0) return <p className="text-ink-soft">Запросов не было.</p>;
    return (
      <ul className="space-y-2">
        {d.map((h, i) => {
          if (typeof h === "string") return <li key={i}>{h}</li>;
          if (!isObj(h)) return null;
          const found = Array.isArray(h.found) ? h.found : [];
          return (
            <li key={i}>
              <p className="font-medium">
                «{String(h.query)}»{h.stage === "self_check" && <span className="font-normal text-silver"> · после самопроверки</span>}
              </p>
              {found.length === 0 ? (
                <p className="text-ink-soft">ничего не найдено</p>
              ) : (
                <ul className="mt-0.5 text-ink-soft">
                  {found.map((f, j) => (
                    <li key={j}>{typeof f === "string" ? f : isObj(f) ? `${f.id} ${f.title}` : null}</li>
                  ))}
                </ul>
              )}
            </li>
          );
        })}
      </ul>
    );
  }
  if (!isObj(d)) return null;
  switch (s.name) {
    case "classify": {
      const facts = Array.isArray(d.key_facts) ? d.key_facts.filter(isObj) : [];
      return (
        <div className="space-y-2">
          {typeof d.event_type === "string" && <p>Тип события: {d.event_type}</p>}
          {facts.length > 0 && (
            <ul className="space-y-0.5">
              {facts.map((f, i) => (
                <li key={i} className="text-ink-soft">
                  {f.confirmed ? "✓ " : "? "}
                  {String(f.text)}
                </li>
              ))}
            </ul>
          )}
        </div>
      );
    }
    case "company_profile": {
      const exps = Array.isArray(d.exposures) ? d.exposures.filter(isObj) : [];
      return (
        <div className="space-y-2">
          {exps.length > 0 && <List title="Чувствительности бизнеса" items={exps.map((e) => String(e.title))} />}
          <List title="Пробелы в данных" items={strs(d.data_gaps)} />
        </div>
      );
    }
    case "plan":
      return (
        <div className="space-y-2">
          <List title="Вопросы" items={strs(d.focus_questions)} />
          <List title="Гипотезы" items={strs(d.hypotheses)} />
          <List title="Запросы к аналитике" items={strs(d.research_queries)} />
          <List title="Запросы к новостям" items={strs(d.news_queries)} />
        </div>
      );
    case "self_check": {
      const issues = Array.isArray(d.issues) ? d.issues.filter(isObj) : [];
      return (
        <div className="space-y-2">
          <p>{d.passed ? "Пройдена" : "Есть серьёзные замечания"}</p>
          {issues.length > 0 && <List title="Замечания" items={issues.map((x) => `[${x.severity}] ${x.path}: ${x.problem}`)} />}
          <List title="Дополнительные запросы" items={strs(d.additional_research_queries)} />
          {typeof d.comment === "string" && d.comment && <p className="text-ink-soft">{d.comment}</p>}
        </div>
      );
    }
    default:
      if (s.name.startsWith("fact_check")) {
        const fc = isObj(d.fact_check) ? d.fact_check : null;
        const un = fc && Array.isArray(fc.unverified) ? fc.unverified.filter(isObj) : [];
        return un.length ? (
          <List title="Не найдено в источниках" items={un.map((u) => `«${u.value}» (${u.path}) — ${u.action === "removed" ? "удалено" : "помечено"}`)} />
        ) : (
          <p className="text-ink-soft">Все значения найдены в источниках.</p>
        );
      }
      if (s.name === "draft" || s.name === "revise") {
        const notes = strs(d.validation_notes);
        return notes.length ? <List title="Исправления валидатора" items={notes} /> : <p className="text-ink-soft">Валидатор не вносил исправлений.</p>;
      }
      return null;
  }
}
