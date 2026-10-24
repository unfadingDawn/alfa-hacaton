"use client";

import Link from "next/link";
import { Bookmark, FileText, Newspaper } from "lucide-react";
import { useMe } from "@/components/app-shell";
import { Button } from "@/components/ui/button";
import { Badge, EmptyState, ErrorState, Panel, Skeleton } from "@/components/ui/misc";
import { formatDate, pluralize } from "@/lib/format";
import { cn } from "@/lib/utils";
import type { Dashboard, FeedItem } from "@/lib/types";
import { useApi } from "@/lib/use-api";
import { MethodTag, RelevanceMeter, SourceLine } from "@/features/news/parts";

function greeting() {
  const h = new Date().getHours();
  if (h < 6) return "Доброй ночи";
  if (h < 12) return "Доброе утро";
  if (h < 18) return "Добрый день";
  return "Добрый вечер";
}

export function HomeView() {
  const { me } = useMe();
  const { data, error, loading, reload } = useApi<Dashboard>("/dashboard");
  const firstName = me.full_name.split(" ")[0];

  return (
    <div>
      <div className="mb-6 flex flex-wrap items-center justify-between gap-3">
        <h1 className="display text-[28px] sm:text-[34px]">
          {greeting()}, {firstName}
        </h1>
        <div className="flex flex-wrap gap-2">
          <Button asChild variant="secondary" size="md">
            <Link href="/feed">
              <Newspaper /> Лента
            </Link>
          </Button>
          <Button asChild variant="secondary" size="md">
            <Link href="/feed?source_kind=alfa_research">
              <FileText /> Аналитика банка
            </Link>
          </Button>
          <Button asChild variant="secondary" size="md">
            <Link href="/saved">
              <Bookmark /> Сохранённые
            </Link>
          </Button>
        </div>
      </div>
      {error ? (
        <ErrorState error={error} onRetry={reload} />
      ) : loading || !data ? (
        <div className="space-y-6">
          <Skeleton className="h-[300px] rounded-[var(--radius-xl)]" />
          <div className="grid gap-6 lg:grid-cols-[1.5fr_1fr]">
            <Skeleton className="h-[420px]" />
            <Skeleton className="h-[420px]" />
          </div>
        </div>
      ) : (
        <div className="space-y-6">
          <DayHero data={data} />
          {!data.llm.configured && (
            <div className="rounded-[var(--radius-lg)] border border-mixed/20 bg-mixed-bg px-5 py-4 text-[14px] text-mixed">
              Языковая модель не подключена: в ленте работает локальная предварительная оценка TF-IDF + LSA, персональный анализ недоступен.
            </div>
          )}
          <div className="grid gap-6 lg:grid-cols-[minmax(0,1.5fr)_minmax(0,1fr)]">
            <Panel
              title="Самое важное для вас"
              description="За последние 7 дней, по релевантности для компании и роли"
              action={
                <Button asChild variant="link" size="sm">
                  <Link href="/feed">Вся лента</Link>
                </Button>
              }
            >
              {data.top_important.length === 0 ? (
                <EmptyState title="За неделю новостей нет">Источники опрашиваются каждый час.</EmptyState>
              ) : (
                <ul className="divide-y divide-line">
                  {data.top_important.map((it) => (
                    <TopItem key={it.id} it={it} />
                  ))}
                </ul>
              )}
            </Panel>

            <div className="space-y-6">
              <RiskOpps data={data} />
              <Panel title="Темы дня" description="Разметка тем новостей за сегодня">
                {data.day_summary.topics.length === 0 ? (
                  <p className="text-sm text-ink-soft">Сегодня новостей с размеченными темами пока нет.</p>
                ) : (
                  <ul className="space-y-2">
                    {data.day_summary.topics.slice(0, 6).map((t) => {
                      const max = data.day_summary.topics[0].articles;
                      return (
                        <li key={t.code}>
                          <Link href={`/feed?topic=${t.code}`} className="group block">
                            <div className="flex justify-between text-[14px]">
                              <span className="group-hover:text-steel">{t.name}</span>
                              <span className="text-silver tabular">{t.articles}</span>
                            </div>
                            <div className="mt-1 h-1.5 rounded-full bg-frost">
                              <div className="h-1.5 rounded-full bg-glacier" style={{ width: `${(t.articles / max) * 100}%` }} />
                            </div>
                          </Link>
                        </li>
                      );
                    })}
                  </ul>
                )}
              </Panel>
            </div>
          </div>

          <div className="grid gap-6 lg:grid-cols-2">
            <Panel title="Связано с вашей компанией" description="Темы совпадают с отраслью, продуктами или ВЭД компании">
              <CompactList items={data.company_related} empty="Пока нет новостей с совпадающими темами." />
            </Panel>
            <Panel title="Рынок и регулятор" description="Материалы аналитиков Альфа-Банка и сообщения Банка России">
              <div className="space-y-5">
                <div>
                  <p className="mb-2 text-[13px] font-medium text-bankview">Аналитика Альфа-Банка</p>
                  <CompactList items={data.market.bank_research} empty="Материалы не загружены." />
                </div>
                <div>
                  <p className="mb-2 text-[13px] font-medium text-ink-soft">Банк России</p>
                  <CompactList items={data.market.regulator} empty="Сообщений Банка России за период нет." />
                </div>
              </div>
            </Panel>
          </div>
        </div>
      )}
    </div>
  );
}

// Главный блок, как в кабинете Альфа-Бизнеса: тёмное стекло с красным свечением.
function DayHero({ data }: { data: Dashboard }) {
  const st = data.day_summary.stats;
  const latest = data.market.bank_research[0];
  const rows = [
    { label: "Высокая важность для вас", value: st.high_importance, href: "/feed?min_importance=high", accent: true },
    { label: "Из них оценено моделью", value: st.high_importance_llm, href: "/feed?min_importance=high" },
    { label: "Риски из анализов", value: data.risks.length, href: "/analytics", accent: data.risks.length > 0 },
    { label: "Возможности из анализов", value: data.opportunities.length, href: "/analytics" },
    { label: "Полных ИИ-анализов сегодня", value: st.full_analyses, href: "/feed" },
  ];
  return (
    <section className="glass-night grid gap-4 rounded-[var(--radius-xl)] p-4 sm:p-5 lg:grid-cols-[1.35fr_1fr_1fr]" aria-label="Сводка дня">
      <div className="glass-night-card flex flex-col rounded-[20px] p-5">
        <p className="text-[13px] text-white/60">Сводка дня</p>
        <p className="mt-1 text-[44px] font-semibold leading-none tracking-[-0.03em] tabular sm:text-[56px]">{st.new_articles}</p>
        <p className="mt-2 text-[15px] text-white/75">{pluralize(st.new_articles, "новость", "новости", "новостей")} из подключённых источников</p>
        <p className="mt-auto pt-6 text-[13px] leading-relaxed text-white/60">{data.day_summary.text}</p>
      </div>
      <div className="glass-night-card rounded-[20px] p-5">
        <p className="text-[15px] font-medium">Важное сегодня</p>
        <ul className="mt-3 space-y-2.5">
          {rows.map((r) => (
            <li key={r.label}>
              <Link href={r.href} className="flex items-center justify-between gap-3 text-[14px] text-white/85 hover:text-white">
                {r.label}
                <span
                  className={cn(
                    "min-w-6 rounded-full px-2 py-0.5 text-center text-[12px] font-semibold tabular",
                    r.accent && r.value > 0 ? "bg-steel text-white" : "bg-white/10 text-white/70",
                  )}
                >
                  {r.value}
                </span>
              </Link>
            </li>
          ))}
        </ul>
      </div>
      <div className="glass-night-card flex flex-col rounded-[20px] p-5">
        <p className="text-[15px] font-medium">Позиция Альфа-Банка</p>
        {latest ? (
          <>
            <Link href={`/news/${latest.id}`} className="mt-3 text-[15px] leading-snug text-white hover:underline">
              {latest.title}
            </Link>
            <p className="mt-1 text-[12px] text-white/55">{formatDate(latest.published_at)}</p>
            <p className="mt-3 line-clamp-4 text-[13px] leading-relaxed text-white/65">{latest.summary}</p>
          </>
        ) : (
          <p className="mt-3 text-[13px] text-white/60">Материалы аналитиков не загружены.</p>
        )}
        <Link href="/feed?source_kind=alfa_research" className="mt-auto pt-4 text-[13px] font-medium text-[#ff8a80] hover:underline">
          Вся аналитика банка
        </Link>
      </div>
    </section>
  );
}

function TopItem({ it }: { it: FeedItem }) {
  const a = it.analysis;
  return (
    <li className="flex gap-4 py-4 first:pt-0 last:pb-0">
      {a && <RelevanceMeter score={a.relevance_score} method={a.method} />}
      <div className="min-w-0">
        <SourceLine item={it} />
        <Link href={`/news/${it.id}`} className="mt-1 block text-[16px] font-semibold leading-snug hover:text-steel">
          {it.title}
        </Link>
        {a && (
          <>
            <p className="mt-1 line-clamp-2 text-[14px] text-ink-soft">{a.why_it_matters}</p>
            <div className="mt-2">
              <MethodTag method={a.method} />
            </div>
          </>
        )}
      </div>
    </li>
  );
}

function CompactList({ items, empty }: { items: FeedItem[]; empty: string }) {
  if (items.length === 0) return <p className="text-sm text-ink-soft">{empty}</p>;
  return (
    <ul className="space-y-3">
      {items.map((it) => (
        <li key={it.id}>
          <Link href={`/news/${it.id}`} className="text-[15px] font-medium leading-snug hover:text-steel">
            {it.title}
          </Link>
          <p className="mt-0.5 text-[12px] text-silver">
            {it.source.name}, {formatDate(it.published_at)}
          </p>
        </li>
      ))}
    </ul>
  );
}

function RiskOpps({ data }: { data: Dashboard }) {
  const has = data.risks.length + data.opportunities.length > 0;
  return (
    <Panel title="Риски и возможности" description="Из выполненных ИИ-анализов за 30 дней">
      {!has ? (
        <p className="text-sm text-ink-soft">{data.risks_note || "Нет данных."} Откройте важную новость и запустите анализ.</p>
      ) : (
        <ul className="space-y-3">
          {data.risks.map((r, i) => (
            <li key={`r${i}`} className="border-l-2 border-risk pl-3">
              <Link href={`/news/${r.article_id}`} className="text-[14px] font-medium hover:text-steel">
                {r.title}
              </Link>
              <p className="text-[12px] text-silver">{r.article_title}</p>
            </li>
          ))}
          {data.opportunities.map((r, i) => (
            <li key={`o${i}`} className="border-l-2 border-chance pl-3">
              <Link href={`/news/${r.article_id}`} className="text-[14px] font-medium hover:text-steel">
                {r.title}
              </Link>
              <p className="text-[12px] text-silver">{r.article_title}</p>
            </li>
          ))}
        </ul>
      )}
      <div className="mt-4 flex gap-3 text-[12px] text-silver">
        <span className="flex items-center gap-1.5">
          <span className="h-3 w-0.5 bg-risk" aria-hidden /> риск
        </span>
        <span className="flex items-center gap-1.5">
          <span className="h-3 w-0.5 bg-chance" aria-hidden /> возможность
        </span>
        <Badge tone="outline" className="ml-auto">
          {data.llm.guard.calls} запросов к модели сегодня
        </Badge>
      </div>
    </Panel>
  );
}
