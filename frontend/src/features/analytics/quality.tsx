"use client";

import Link from "next/link";
import { useState } from "react";
import { CartesianGrid, Legend, Line, LineChart, ResponsiveContainer, Tooltip, XAxis, YAxis } from "recharts";
import { PageHeader } from "@/components/app-shell";
import { Button } from "@/components/ui/button";
import { Select } from "@/components/ui/field";
import { EmptyState, ErrorState, Panel, Skeleton } from "@/components/ui/misc";
import { useApi } from "@/lib/use-api";

type Quality = {
  period_days: number;
  runs: {
    total: number;
    succeeded: number;
    failed: number;
    success_rate: number | null;
    json_errors: number;
    truncated: number;
    timeouts: number;
    auth_errors: number;
    rate_limited: number;
  };
  performance: {
    avg_latency_ms: number;
    p95_latency_ms: number;
    avg_tokens: number;
    total_tokens: number;
    avg_steps: number;
    llm_steps: number;
    cached_steps: number;
    cache_rate: number | null;
  };
  quality: {
    full_analyses: number;
    with_bank_view: number;
    bank_view_rate: number | null;
    with_company_facts: number;
    company_grounding_rate: number | null;
    avg_sources: number;
    avg_risks: number;
    avg_opportunities: number;
    avg_actions: number;
    avg_validation_notes: number;
    self_checks: number;
    self_check_passed: number;
    self_check_pass_rate: number | null;
    revisions: number;
    revised_analyses: number;
    validator_corrections: number;
  };
  feedback: { total: number; positive: number; negative: number; with_comment: number; helpfulness_rate: number | null };
  daily: { day: string; runs: number; succeeded: number; failed: number; tokens: number; avg_latency_ms: number }[];
  models: { model: string; calls: number; tokens: number; errors: number; cached: number }[];
  prompt_versions: { prompt_version: string; analyses: number; avg_relevance: number; positive: number; negative: number }[];
  sufficient: { runs: boolean; feedback: boolean };
  notes: string[];
};

const dayFmt = new Intl.DateTimeFormat("ru-RU", { day: "numeric", month: "short" });
const percent = (v: number | null) => (v == null ? "—" : `${v}%`);
const seconds = (ms: number) => (ms ? `${(ms / 1000).toFixed(1)} с` : "—");

export function QualityView() {
  const [period, setPeriod] = useState("30d");
  const { data, error, loading, reload } = useApi<Quality>(`/analytics/quality?period=${period}`);
  return (
    <div>
      <PageHeader
        title="Качество AI"
        lead="Стабильность, расход и обоснованность ваших агентных анализов. Страница читает сохранённые трассы и не вызывает модель."
        action={
          <div className="flex items-center gap-2">
            <Button asChild variant="outline" size="sm"><Link href="/analytics">Бизнес-аналитика</Link></Button>
            <Select aria-label="Период качества" value={period} onChange={(e) => setPeriod(e.target.value)} className="w-[140px]">
              <option value="7d">7 дней</option>
              <option value="30d">30 дней</option>
              <option value="90d">90 дней</option>
            </Select>
          </div>
        }
      />
      {error ? <ErrorState error={error} onRetry={reload} /> : loading || !data ? <Loading /> : !data.sufficient.runs ? (
        <EmptyState title="Запусков анализа пока нет">Откройте важную новость и выполните глубокий анализ. Метрики появятся автоматически.</EmptyState>
      ) : <QualityContent data={data} />}
    </div>
  );
}

function Loading() {
  return <div className="grid gap-5 sm:grid-cols-2 lg:grid-cols-4">{Array.from({ length: 8 }).map((_, i) => <Skeleton key={i} className="h-36" />)}</div>;
}

function QualityContent({ data }: { data: Quality }) {
  const cards = [
    { label: "Успешные запуски", value: percent(data.runs.success_rate), note: `${data.runs.succeeded} из ${data.runs.total}` },
    { label: "Полезность", value: percent(data.feedback.helpfulness_rate), note: data.feedback.total ? `${data.feedback.positive} положительных из ${data.feedback.total}` : "Отзывов пока нет" },
    { label: "Опора на компанию", value: percent(data.quality.company_grounding_rate), note: `${data.quality.with_company_facts} из ${data.quality.full_analyses} анализов` },
    { label: "Позиция банка", value: percent(data.quality.bank_view_rate), note: `${data.quality.with_bank_view} из ${data.quality.full_analyses} анализов` },
  ];
  return (
    <div className="space-y-6">
      <ul className="grid gap-4 sm:grid-cols-2 lg:grid-cols-4">
        {cards.map((x) => <li key={x.label} className="glass rounded-[var(--radius-lg)] p-5"><p className="text-[13px] text-ink-soft">{x.label}</p><p className="mt-2 text-[32px] font-semibold tabular">{x.value}</p><p className="mt-2 text-[12px] text-silver">{x.note}</p></li>)}
      </ul>

      <div className="grid gap-6 lg:grid-cols-2">
        <Panel title="Стабильность" description="Ошибки полного агентного конвейера">
          <MetricGrid items={[
            ["Ошибок", data.runs.failed], ["Неверный JSON", data.runs.json_errors], ["Обрезано", data.runs.truncated],
            ["Таймауты", data.runs.timeouts], ["Авторизация", data.runs.auth_errors], ["Лимиты", data.runs.rate_limited],
          ]} />
        </Panel>
        <Panel title="Производительность" description="По сохранённым трассам запусков">
          <MetricGrid items={[
            ["Среднее время", seconds(data.performance.avg_latency_ms)], ["P95", seconds(data.performance.p95_latency_ms)],
            ["Токенов на анализ", data.performance.avg_tokens.toLocaleString("ru-RU")], ["Всего токенов", data.performance.total_tokens.toLocaleString("ru-RU")],
            ["Шагов на анализ", data.performance.avg_steps], ["Кэш шагов", percent(data.performance.cache_rate)],
          ]} />
        </Panel>
        <Panel title="Самопроверка и grounding" description="Как агент проверяет и дорабатывает результат">
          <MetricGrid items={[
            ["Проверок", data.quality.self_checks], ["Пройдено", percent(data.quality.self_check_pass_rate)],
            ["Доработок", data.quality.revisions], ["Исправлений валидатора", data.quality.validator_corrections],
            ["Источников в среднем", data.quality.avg_sources], ["Действий в среднем", data.quality.avg_actions],
          ]} />
        </Panel>
        <Panel title="Обратная связь" description="Оценки текущего пользователя">
          <MetricGrid items={[
            ["Всего", data.feedback.total], ["Полезно", data.feedback.positive], ["Не полезно", data.feedback.negative],
            ["С комментарием", data.feedback.with_comment], ["Рисков в среднем", data.quality.avg_risks],
            ["Возможностей", data.quality.avg_opportunities],
          ]} />
        </Panel>
      </div>

      <Panel title="Динамика запусков" description="Успешные и неуспешные анализы по дням">
        <div className="h-[280px]">
          <ResponsiveContainer>
            <LineChart data={data.daily} margin={{ left: -18, right: 8, top: 8 }}>
              <CartesianGrid vertical={false} stroke="#e2e5ea" />
              <XAxis dataKey="day" tickFormatter={(x) => dayFmt.format(new Date(x))} fontSize={12} tickLine={false} axisLine={false} />
              <YAxis allowDecimals={false} fontSize={12} tickLine={false} axisLine={false} />
              <Tooltip labelFormatter={(x) => dayFmt.format(new Date(String(x)))} />
              <Legend />
              <Line type="monotone" dataKey="succeeded" name="Успешно" stroke="#11804f" strokeWidth={2} />
              <Line type="monotone" dataKey="failed" name="Ошибки" stroke="#c2410c" strokeWidth={2} />
            </LineChart>
          </ResponsiveContainer>
        </div>
      </Panel>

      <div className="grid gap-6 lg:grid-cols-2">
        <TablePanel title="Модели" head={["Модель", "Вызовы", "Токены", "Ошибки", "Кэш"]}
          rows={data.models.map((x) => [x.model, x.calls, x.tokens, x.errors, x.cached])} />
        <TablePanel title="Версии промпта" head={["Версия", "Анализы", "Ср. score", "+", "−"]}
          rows={data.prompt_versions.map((x) => [x.prompt_version, x.analyses, x.avg_relevance, x.positive, x.negative])} />
      </div>
      <p className="text-[12px] text-silver">{data.notes.join(" ")}</p>
    </div>
  );
}

function MetricGrid({ items }: { items: [string, string | number][] }) {
  return <dl className="grid grid-cols-2 gap-x-6 gap-y-4 sm:grid-cols-3">{items.map(([label, value]) => <div key={label}><dt className="text-[12px] text-silver">{label}</dt><dd className="mt-1 text-[20px] font-semibold tabular">{value}</dd></div>)}</dl>;
}

function TablePanel({ title, head, rows }: { title: string; head: string[]; rows: (string | number)[][] }) {
  return <Panel title={title}>{rows.length ? <div className="overflow-auto"><table className="w-full text-left text-[13px]"><thead className="text-silver"><tr>{head.map((x) => <th key={x} className="pb-2 pr-3 font-medium">{x}</th>)}</tr></thead><tbody className="divide-y divide-line">{rows.map((r, i) => <tr key={i}>{r.map((x, j) => <td key={j} className="py-2 pr-3 tabular">{x}</td>)}</tr>)}</tbody></table></div> : <p className="text-sm text-ink-soft">Данных пока нет.</p>}</Panel>;
}
