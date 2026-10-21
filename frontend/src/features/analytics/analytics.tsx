"use client";

import Link from "next/link";
import { useMemo, useState } from "react";
import {
  Bar,
  BarChart,
  CartesianGrid,
  Cell,
  Legend,
  Line,
  LineChart,
  ResponsiveContainer,
  Tooltip,
  XAxis,
  YAxis,
} from "recharts";
import { PageHeader } from "@/components/app-shell";
import { Button } from "@/components/ui/button";
import { Select } from "@/components/ui/field";
import { EmptyState, ErrorState, Panel, Skeleton } from "@/components/ui/misc";
import { impactLabel } from "@/lib/format";
import type { Impact, Importance, Topic } from "@/lib/types";
import { useApi } from "@/lib/use-api";
import { cn } from "@/lib/utils";

// Палитра проверена валидатором (светлая тема, поверхность #fbfdff):
// сталь / оранжевый / бирюзовый — порядок фиксирован, цвет следует за сущностью.
const SERIES = ["#2b67a8", "#eb6834", "#1baf7a"];
const IMPACT_COLOR: Record<Impact, string> = { risk: "#a8463d", opportunity: "#1b7a63", mixed: "#8a6a1f", neutral: "#7f90a4" };
const AXIS = { fontSize: 12, fill: "#7f90a4" };

type Analytics = {
  period_days: number;
  articles_total: number;
  topic_distribution: { code: string; name: string; method: "llm" | "keywords" | "metadata"; articles: number }[];
  daily: { day: string; total: number; important: number }[];
  daily_topics: { day: string; topic_code: string; articles: number }[];
  company_topics: { topic: string; hits: number }[];
  impact: { impact_type: Impact; importance_level: Importance; analyses: number }[];
  risks_opportunities: { article_id: string; title: string; kind: "risk" | "opportunity"; item_title: string; relevance_score: number }[];
  sufficient: { articles: boolean; llm_analyses: boolean };
  notes: string[];
};

const dayFmt = new Intl.DateTimeFormat("ru-RU", { day: "numeric", month: "short" });
const fmtDay = (iso: string) => dayFmt.format(new Date(iso));

function TooltipBox({ active, payload, label, labelFormatter }: {
  active?: boolean;
  payload?: { name: string; value: number; color: string }[];
  label?: string;
  labelFormatter?: (l: string) => string;
}) {
  if (!active || !payload?.length) return null;
  return (
    <div className="rounded-[10px] border border-white bg-white/95 px-3 py-2 text-[12px] shadow-[var(--shadow-soft)]">
      {label && <p className="mb-1 font-medium text-ink">{labelFormatter ? labelFormatter(label) : label}</p>}
      {payload.map((p) => (
        <p key={p.name} className="flex items-center gap-2 text-ink-soft">
          <span className="size-2 rounded-full" style={{ background: p.color }} aria-hidden />
          {p.name}: <span className="font-medium text-ink tabular">{p.value}</span>
        </p>
      ))}
    </div>
  );
}

// Панель графика с переключателем «график / таблица» (доступность и точные значения).
function ChartPanel({
  title,
  description,
  table,
  children,
  empty,
}: {
  title: string;
  description?: string;
  table: { head: string[]; rows: (string | number)[][] };
  children: React.ReactNode;
  empty?: string;
}) {
  const [asTable, setAsTable] = useState(false);
  return (
    <Panel
      title={title}
      description={description}
      action={
        !empty && (
          <Button variant="link" size="sm" onClick={() => setAsTable(!asTable)} aria-pressed={asTable}>
            {asTable ? "Показать график" : "Показать таблицей"}
          </Button>
        )
      }
    >
      {empty ? (
        <p className="py-8 text-center text-sm text-ink-soft">{empty}</p>
      ) : asTable ? (
        <div className="max-h-[300px] overflow-auto">
          <table className="w-full text-left text-[13px]">
            <thead className="sticky top-0 bg-white/90 text-silver">
              <tr>
                {table.head.map((h) => (
                  <th key={h} className="py-1.5 pr-4 font-medium">
                    {h}
                  </th>
                ))}
              </tr>
            </thead>
            <tbody className="divide-y divide-line/70">
              {table.rows.map((r, i) => (
                <tr key={i}>
                  {r.map((c, j) => (
                    <td key={j} className={cn("py-1.5 pr-4", typeof c === "number" && "tabular")}>
                      {c}
                    </td>
                  ))}
                </tr>
              ))}
            </tbody>
          </table>
        </div>
      ) : (
        <div className="h-[280px]">{children}</div>
      )}
    </Panel>
  );
}

export function AnalyticsView() {
  const [period, setPeriod] = useState("30d");
  const { data, error, loading, reload } = useApi<Analytics>(`/analytics?period=${period}`);
  const topics = useApi<{ items: Topic[] }>("/topics");
  const names = useMemo(() => new Map(topics.data?.items.map((t) => [t.code, t.name]) ?? []), [topics.data]);

  return (
    <div>
      <PageHeader
        title="Аналитика"
        lead="Сводка по новостной повестке и по тому, как она затрагивает вашу компанию. Все показатели считаются по данным в системе."
        action={
          <Select aria-label="Период" value={period} onChange={(e) => setPeriod(e.target.value)} className="w-[180px]">
            <option value="7d">7 дней</option>
            <option value="30d">30 дней</option>
            <option value="90d">90 дней</option>
          </Select>
        }
      />
      {error ? (
        <ErrorState error={error} onRetry={reload} />
      ) : loading || !data ? (
        <div className="grid gap-6 lg:grid-cols-2">
          {Array.from({ length: 4 }).map((_, i) => (
            <Skeleton key={i} className="h-[340px]" />
          ))}
        </div>
      ) : !data.sufficient.articles ? (
        <EmptyState title="За выбранный период новостей нет">Выберите более длинный период.</EmptyState>
      ) : (
        <Charts data={data} names={names} />
      )}
    </div>
  );
}

function Charts({ data, names }: { data: Analytics; names: Map<string, string> }) {
  const topicTotals = useMemo(() => {
    const m = new Map<string, { name: string; total: number; llm: number; keywords: number; metadata: number }>();
    data.topic_distribution.forEach((r) => {
      const x = m.get(r.code) ?? { name: r.name, total: 0, llm: 0, keywords: 0, metadata: 0 };
      x.total += r.articles;
      x[r.method] += r.articles;
      m.set(r.code, x);
    });
    return [...m.values()].sort((a, b) => b.total - a.total).slice(0, 10);
  }, [data.topic_distribution]);

  const daily = data.daily.map((d) => ({ day: d.day, Всего: d.total, Важные: d.important }));
  const importantTotal = data.daily.reduce((s, d) => s + d.important, 0);

  const top3 = useMemo(() => {
    const sums = new Map<string, number>();
    data.daily_topics.forEach((d) => sums.set(d.topic_code, (sums.get(d.topic_code) ?? 0) + d.articles));
    return [...sums.entries()].sort((a, b) => b[1] - a[1]).slice(0, 3).map(([c]) => c);
  }, [data.daily_topics]);
  const agenda = useMemo(() => {
    const byDay = new Map<string, Record<string, number | string>>();
    data.daily.forEach((d) => byDay.set(d.day, { day: d.day, ...Object.fromEntries(top3.map((t) => [t, 0])) }));
    data.daily_topics.forEach((d) => {
      if (!top3.includes(d.topic_code)) return;
      const row = byDay.get(d.day) ?? { day: d.day };
      row[d.topic_code] = d.articles;
      byDay.set(d.day, row);
    });
    return [...byDay.values()].sort((a, b) => String(a.day).localeCompare(String(b.day)));
  }, [data.daily, data.daily_topics, top3]);

  const impact = useMemo(() => {
    const m = new Map<Impact, number>();
    data.impact.forEach((r) => m.set(r.impact_type, (m.get(r.impact_type) ?? 0) + r.analyses));
    return (["risk", "opportunity", "mixed", "neutral"] as Impact[]).map((k) => ({ key: k, name: impactLabel[k], value: m.get(k) ?? 0 }));
  }, [data.impact]);
  const llmCount = impact.reduce((s, x) => s + x.value, 0);
  const companyTopics = data.company_topics.map((t) => ({ name: names.get(t.topic) ?? t.topic, hits: t.hits }));

  return (
    <div className="space-y-6">
      <dl className="grid gap-4 sm:grid-cols-3">
        {[
          { label: "Новостей за период", value: data.articles_total },
          { label: "Важных для вас", value: importantTotal, hint: "высокая и критическая важность" },
          { label: "Оценок моделью", value: llmCount, hint: "релевантность и полный анализ" },
        ].map((s) => (
          <div key={s.label} className="glass rounded-[var(--radius-lg)] px-5 py-4">
            <dt className="text-[13px] text-ink-soft">{s.label}</dt>
            <dd className="mt-1 text-[34px] font-semibold tracking-[-0.03em] tabular">{s.value}</dd>
            {s.hint && <dd className="text-[12px] text-silver">{s.hint}</dd>}
          </div>
        ))}
      </dl>

      <div className="grid gap-6 lg:grid-cols-2">
        <ChartPanel
          title="Количество новостей по дням"
          description="Всего и важных для вас (по лучшей доступной оценке)"
          table={{ head: ["День", "Всего", "Важные"], rows: data.daily.map((d) => [fmtDay(d.day), d.total, d.important]) }}
          empty={daily.length === 0 ? "Нет данных за период." : undefined}
        >
          <ResponsiveContainer>
            <BarChart data={daily} barGap={2} margin={{ left: -18, right: 8, top: 8 }}>
              <CartesianGrid vertical={false} stroke="#e2ecf6" />
              <XAxis dataKey="day" tickFormatter={fmtDay} tick={AXIS} axisLine={false} tickLine={false} minTickGap={16} />
              <YAxis allowDecimals={false} tick={AXIS} axisLine={false} tickLine={false} />
              <Tooltip content={<TooltipBox labelFormatter={fmtDay} />} cursor={{ fill: "#e2ecf6", opacity: 0.5 }} />
              <Legend iconType="circle" iconSize={8} wrapperStyle={{ fontSize: 12, color: "#3a4d63" }} />
              <Bar dataKey="Всего" fill={SERIES[0]} radius={[4, 4, 0, 0]} maxBarSize={18} />
              <Bar dataKey="Важные" fill={SERIES[1]} radius={[4, 4, 0, 0]} maxBarSize={18} />
            </BarChart>
          </ResponsiveContainer>
        </ChartPanel>

        <ChartPanel
          title="Распределение новостей по темам"
          description="10 самых частых тем; у новостей без анализа темы размечены по словарю"
          table={{
            head: ["Тема", "Всего", "Моделью", "По словарю", "По материалу"],
            rows: topicTotals.map((t) => [t.name, t.total, t.llm, t.keywords, t.metadata]),
          }}
          empty={topicTotals.length === 0 ? "Темы ещё не размечены." : undefined}
        >
          <ResponsiveContainer>
            <BarChart data={topicTotals} layout="vertical" margin={{ left: 8, right: 24 }}>
              <XAxis type="number" hide />
              <YAxis type="category" dataKey="name" width={170} tick={{ ...AXIS, fill: "#3a4d63" }} axisLine={false} tickLine={false} />
              <Tooltip content={<TooltipBox />} cursor={{ fill: "#e2ecf6", opacity: 0.5 }} />
              <Bar dataKey="total" name="Новостей" fill={SERIES[0]} radius={[0, 4, 4, 0]} maxBarSize={14} label={{ position: "right", fontSize: 11, fill: "#3a4d63" }} />
            </BarChart>
          </ResponsiveContainer>
        </ChartPanel>

        <ChartPanel
          title="Динамика повестки"
          description="Три самые частые темы периода по дням"
          table={{
            head: ["День", ...top3.map((t) => names.get(t) ?? t)],
            rows: agenda.map((r) => [fmtDay(String(r.day)), ...top3.map((t) => Number(r[t] ?? 0))]),
          }}
          empty={top3.length === 0 ? "Недостаточно размеченных новостей." : undefined}
        >
          <ResponsiveContainer>
            <LineChart data={agenda} margin={{ left: -18, right: 8, top: 8 }}>
              <CartesianGrid vertical={false} stroke="#e2ecf6" />
              <XAxis dataKey="day" tickFormatter={(d) => fmtDay(String(d))} tick={AXIS} axisLine={false} tickLine={false} minTickGap={16} />
              <YAxis allowDecimals={false} tick={AXIS} axisLine={false} tickLine={false} />
              <Tooltip content={<TooltipBox labelFormatter={fmtDay} />} />
              <Legend iconType="circle" iconSize={8} wrapperStyle={{ fontSize: 12, color: "#3a4d63" }} />
              {top3.map((t, i) => (
                <Line key={t} type="monotone" dataKey={t} name={names.get(t) ?? t} stroke={SERIES[i]} strokeWidth={2} dot={{ r: 3 }} activeDot={{ r: 5 }} />
              ))}
            </LineChart>
          </ResponsiveContainer>
        </ChartPanel>

        <ChartPanel
          title="Темы, которые чаще всего затрагивают компанию"
          description="По оценкам модели с релевантностью от 40"
          table={{ head: ["Тема", "Совпадений"], rows: companyTopics.map((t) => [t.name, t.hits]) }}
          empty={companyTopics.length === 0 ? "Появится после оценки новостей моделью: откройте ленту и нажмите «Оценить ленту моделью»." : undefined}
        >
          <ResponsiveContainer>
            <BarChart data={companyTopics} layout="vertical" margin={{ left: 8, right: 24 }}>
              <XAxis type="number" hide />
              <YAxis type="category" dataKey="name" width={170} tick={{ ...AXIS, fill: "#3a4d63" }} axisLine={false} tickLine={false} />
              <Tooltip content={<TooltipBox />} cursor={{ fill: "#e2ecf6", opacity: 0.5 }} />
              <Bar dataKey="hits" name="Совпадений" fill={SERIES[0]} radius={[0, 4, 4, 0]} maxBarSize={14} label={{ position: "right", fontSize: 11, fill: "#3a4d63" }} />
            </BarChart>
          </ResponsiveContainer>
        </ChartPanel>
      </div>

      <div className="grid gap-6 lg:grid-cols-[minmax(0,1fr)_minmax(0,1.3fr)]">
        <ChartPanel
          title="Тип влияния"
          description="Среди новостей, оценённых моделью"
          table={{ head: ["Тип", "Оценок"], rows: impact.map((x) => [x.name, x.value]) }}
          empty={llmCount === 0 ? "Нет оценок моделью за период." : undefined}
        >
          <ResponsiveContainer>
            <BarChart data={impact} layout="vertical" margin={{ left: 8, right: 24 }}>
              <XAxis type="number" hide />
              <YAxis type="category" dataKey="name" width={150} tick={{ ...AXIS, fill: "#3a4d63" }} axisLine={false} tickLine={false} />
              <Tooltip content={<TooltipBox />} cursor={{ fill: "#e2ecf6", opacity: 0.5 }} />
              <Bar dataKey="value" name="Оценок" radius={[0, 4, 4, 0]} maxBarSize={16} label={{ position: "right", fontSize: 11, fill: "#3a4d63" }}>
                {impact.map((x) => (
                  <Cell key={x.key} fill={IMPACT_COLOR[x.key]} />
                ))}
              </Bar>
            </BarChart>
          </ResponsiveContainer>
        </ChartPanel>

        <Panel title="Основные риски и возможности" description="Из полных ИИ-анализов за период, по релевантности">
          {data.risks_opportunities.length === 0 ? (
            <p className="py-8 text-center text-sm text-ink-soft">Появятся после первого полного анализа новости.</p>
          ) : (
            <ul className="space-y-3">
              {data.risks_opportunities.slice(0, 8).map((r, i) => (
                <li key={i} className={cn("border-l-2 pl-3", r.kind === "risk" ? "border-risk" : "border-chance")}>
                  <p className="text-[14px] font-medium">
                    <span className="sr-only">{r.kind === "risk" ? "Риск: " : "Возможность: "}</span>
                    {r.item_title}
                  </p>
                  <Link href={`/news/${r.article_id}`} className="text-[12px] text-silver hover:text-steel">
                    {r.title}
                  </Link>
                </li>
              ))}
            </ul>
          )}
        </Panel>
      </div>

      <ul className="space-y-1 text-[12px] text-silver">
        {data.notes.map((n) => (
          <li key={n}>{n}</li>
        ))}
      </ul>
    </div>
  );
}
