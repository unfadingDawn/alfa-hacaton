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
const IMPACT_COLOR: Record<Impact, string> = { risk: "#c2410c", opportunity: "#11804f", mixed: "#8a6a1f", neutral: "#8a93a0" };
const AXIS = { fontSize: 12, fill: "#8a93a0" };
// Порядковая шкала важности: один оттенок, от светлого к тёмному (проверена валидатором, --ordinal).
const IMPORTANCE_STEPS: { key: Importance; label: string; color: string }[] = [
  { key: "critical", label: "Критическая", color: "#8c120b" },
  { key: "high", label: "Высокая", color: "#c8251a" },
  { key: "medium", label: "Средняя", color: "#ea5a47" },
  { key: "low", label: "Низкая", color: "#f4998d" },
];

type Month = {
  month: string;
  total: number;
  relevant: number;
  critical: number;
  high: number;
  medium: number;
  low: number;
  avg_relevance: number;
  llm_rated: number;
  risks: number;
  opportunities: number;
  mixed: number;
  net_impact: number;
  full_analyses: number;
  with_bank_view: number;
};
type KPI = {
  relevant_share: { value: number | null; relevant: number; total: number };
  acuteness: { value: number | null; acute: number; relevant: number };
  balance: { value: number | null; risks: number; opportunities: number; mixed: number; rated: number; net_impact: number };
  bank_backed: { value: number | null; with_bank_view: number; full: number };
  channels: { count: number; top: { code: string; name: string; articles: number; share: number | null } | null };
  peak_month: { month: string; relevant: number } | null;
};

type Analytics = {
  period_days: number;
  articles_total: number;
  topic_distribution: { code: string; name: string; method: "llm" | "keywords" | "metadata"; articles: number }[];
  daily: { day: string; total: number; important: number }[];
  daily_topics: { day: string; topic_code: string; articles: number }[];
  company_topics: { topic: string; hits: number }[];
  impact: { impact_type: Impact; importance_level: Importance; analyses: number }[];
  risks_opportunities: { article_id: string; title: string; kind: "risk" | "opportunity"; item_title: string; relevance_score: number }[];
  monthly: Month[];
  channels: { code: string; name: string; articles: number }[];
  kpi: KPI;
  min_score: number;
  sufficient: { articles: boolean; llm_analyses: boolean };
  notes: string[];
};

const dayFmt = new Intl.DateTimeFormat("ru-RU", { day: "numeric", month: "short" });
const fmtDay = (iso: string) => dayFmt.format(new Date(iso));
const monthFmt = new Intl.DateTimeFormat("ru-RU", { month: "short", year: "numeric" });
const fmtMonth = (ym: string) => monthFmt.format(new Date(`${ym}-01T12:00:00`)).replace(" г.", "");

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
  className,
  title,
  description,
  table,
  children,
  empty,
}: {
  className?: string;
  title: string;
  description?: string;
  table: { head: string[]; rows: (string | number)[][] };
  children: React.ReactNode;
  empty?: string;
}) {
  const [asTable, setAsTable] = useState(false);
  return (
    <Panel
      className={className}
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
  const [period, setPeriod] = useState("365d");
  const { data, error, loading, reload } = useApi<Analytics>(`/analytics?period=${period}`);
  const topics = useApi<{ items: Topic[] }>("/topics");
  const names = useMemo(() => new Map(topics.data?.items.map((t) => [t.code, t.name]) ?? []), [topics.data]);

  return (
    <div>
      <PageHeader
        title="Аналитика"
        lead="Как новости отражаются на вашей компании: по месяцам, по важности и по типу влияния. Все показатели считаются по новостям и оценкам в системе."
        action={
          <div className="flex flex-wrap items-center gap-2">
            <Button asChild variant="outline" size="sm">
              <Link href="/analytics/quality">Качество AI</Link>
            </Button>
            <Select aria-label="Период" value={period} onChange={(e) => setPeriod(e.target.value)} className="w-[180px]">
              <option value="30d">30 дней</option>
              <option value="90d">3 месяца</option>
              <option value="180d">6 месяцев</option>
              <option value="365d">12 месяцев</option>
            </Select>
          </div>
        }
      />
      {error ? (
        <ErrorState error={error} onRetry={reload} />
      ) : loading || !data ? (
        <div className="grid gap-6 lg:grid-cols-2">
          {Array.from({ length: 6 }).map((_, i) => (
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

  const monthly = data.monthly;
  const relevantTotal = monthly.reduce((s, m) => s + m.relevant, 0);

  return (
    <div className="space-y-6">
      <KpiGrid kpi={data.kpi} />
      <div className="grid gap-6 lg:grid-cols-2">
      <ChartPanel
        className="lg:col-span-2"
        title="Как новости отражаются на бизнесе"
        description={`Новости, которые касаются вашей компании (оценка от ${data.min_score}), по месяцам и важности для вас`}
        table={{
          head: ["Месяц", "Всего новостей", "Касаются компании", ...IMPORTANCE_STEPS.map((x) => x.label), "Средняя оценка"],
          rows: monthly.map((m) => [fmtMonth(m.month), m.total, m.relevant, m.critical, m.high, m.medium, m.low, m.avg_relevance]),
        }}
        empty={relevantTotal === 0 ? "За период нет новостей, которые касаются компании. Дополните профиль компании и интересы." : undefined}
      >
        <ResponsiveContainer>
          <BarChart data={monthly} margin={{ left: -18, right: 8, top: 16 }} barCategoryGap="28%">
            <CartesianGrid vertical={false} stroke="#e2e5ea" />
            <XAxis dataKey="month" tickFormatter={fmtMonth} tick={AXIS} axisLine={false} tickLine={false} />
            <YAxis allowDecimals={false} tick={AXIS} axisLine={false} tickLine={false} />
            <Tooltip content={<MonthTooltip />} cursor={{ fill: "#eceef1", opacity: 0.6 }} />
            <Legend content={() => <ImportanceLegend />} />
            {IMPORTANCE_STEPS.map((x, i) => (
              <Bar
                key={x.key}
                dataKey={x.key}
                name={x.label}
                stackId="imp"
                fill={x.color}
                stroke="#fbfcfd"
                strokeWidth={1.5}
                maxBarSize={48}
                radius={i === IMPORTANCE_STEPS.length - 1 ? [4, 4, 0, 0] : 0}
              />
            ))}
          </BarChart>
        </ResponsiveContainer>
      </ChartPanel>

      <ChartPanel
        title="Тип влияния"
        description="Среди новостей, оценённых моделью"
        table={{ head: ["Тип", "Оценок"], rows: impact.map((x) => [x.name, x.value]) }}
        empty={llmCount === 0 ? "Нет оценок моделью за период. Откройте ленту и нажмите «Оценить ленту моделью»." : undefined}
      >
        <ResponsiveContainer>
          <BarChart data={impact} margin={{ left: -18, right: 8, top: 16 }}>
            <CartesianGrid vertical={false} stroke="#e2e5ea" />
            <XAxis dataKey="name" tick={AXIS} axisLine={false} tickLine={false} interval={0} />
            <YAxis allowDecimals={false} tick={AXIS} axisLine={false} tickLine={false} />
            <Tooltip content={<TooltipBox />} cursor={{ fill: "#eceef1", opacity: 0.6 }} />
            <Bar dataKey="value" name="Оценок" radius={[4, 4, 0, 0]} maxBarSize={44} label={{ position: "top", fontSize: 11, fill: "#46515f" }}>
              {impact.map((x) => (
                <Cell key={x.key} fill={IMPACT_COLOR[x.key]} />
              ))}
            </Bar>
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
            <CartesianGrid vertical={false} stroke="#e2e5ea" />
            <XAxis dataKey="day" tickFormatter={(d) => fmtDay(String(d))} tick={AXIS} axisLine={false} tickLine={false} minTickGap={16} />
            <YAxis allowDecimals={false} tick={AXIS} axisLine={false} tickLine={false} />
            <Tooltip content={<TooltipBox labelFormatter={fmtDay} />} />
            <Legend iconType="circle" iconSize={8} wrapperStyle={{ fontSize: 12, color: "#46515f" }} />
            {top3.map((t, i) => (
              <Line key={t} type="monotone" dataKey={t} name={names.get(t) ?? t} stroke={SERIES[i]} strokeWidth={2} dot={{ r: 3 }} activeDot={{ r: 5 }} />
            ))}
          </LineChart>
        </ResponsiveContainer>
      </ChartPanel>
      </div>
    </div>
  );
}

function KpiGrid({ kpi }: { kpi: KPI }) {
  const tiles: { label: string; value: string; sub: string; small?: boolean }[] = [
    {
      label: "Острота повестки",
      value: kpi.acuteness.value == null ? "—" : `${kpi.acuteness.value}%`,
      sub: kpi.acuteness.relevant
        ? `значимых для компании новостей — высокой или критической важности (${kpi.acuteness.acute} из ${kpi.acuteness.relevant})`
        : "за период нет новостей, которые касаются компании",
    },
    {
      label: "Главный канал влияния",
      value: kpi.channels.top?.name ?? "—",
      sub: kpi.channels.top ? `${kpi.channels.top.share ?? 0}% значимых новостей · всего каналов: ${kpi.channels.count}` : "совпадений тем нет",
      small: true,
    },
    {
      label: "Самый напряжённый месяц",
      value: kpi.peak_month ? fmtMonth(kpi.peak_month.month) : "—",
      sub: kpi.peak_month ? `значимых для компании новостей: ${kpi.peak_month.relevant}` : "нет данных",
      small: true,
    },
  ];
  return (
    <ul className="grid gap-4 sm:grid-cols-3">
      {tiles.map((t) => (
        <li key={t.label} className="glass rounded-[var(--radius-lg)] p-5">
          <p className="text-[13px] text-ink-soft">{t.label}</p>
          <p className={cn("mt-1.5 font-semibold tracking-[-0.02em] tabular", t.small ? "text-[22px] leading-tight" : "text-[34px] leading-none", "text-ink")}>
            {t.value}
          </p>
          <p className="mt-2 text-[12px] leading-snug text-silver">{t.sub}</p>
        </li>
      ))}
    </ul>
  );
}

// Легенда в порядке шкалы (recharts иначе сортирует подписи по алфавиту).
function ImportanceLegend() {
  return (
    <ul className="flex flex-wrap justify-center gap-x-4 gap-y-1 pt-2 text-[12px] text-ink-soft">
      {IMPORTANCE_STEPS.map((x) => (
        <li key={x.key} className="flex items-center gap-1.5">
          <span className="size-2 rounded-full" style={{ background: x.color }} aria-hidden />
          {x.label}
        </li>
      ))}
    </ul>
  );
}

function MonthTooltip({ active, payload, label }: { active?: boolean; payload?: { payload: Month }[]; label?: string }) {
  if (!active || !payload?.length || !label) return null;
  const m = payload[0].payload;
  return (
    <div className="rounded-[10px] border border-white bg-white/95 px-3 py-2 text-[12px] shadow-[var(--shadow-soft)]">
      <p className="mb-1 font-medium text-ink">{fmtMonth(label)}</p>
      <p className="text-ink-soft">
        Касаются компании: <span className="font-medium text-ink tabular">{m.relevant}</span> из {m.total}
      </p>
      {IMPORTANCE_STEPS.filter((x) => m[x.key] > 0).map((x) => (
        <p key={x.key} className="flex items-center gap-2 text-ink-soft">
          <span className="size-2 rounded-full" style={{ background: x.color }} aria-hidden />
          {x.label}: <span className="font-medium text-ink tabular">{m[x.key]}</span>
        </p>
      ))}
      {m.relevant > 0 && <p className="mt-1 text-silver">Средняя оценка: {m.avg_relevance}</p>}
    </div>
  );
}
