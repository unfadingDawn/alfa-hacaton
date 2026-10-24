"use client";

import { Sparkles } from "lucide-react";
import { Badge, Skeleton } from "@/components/ui/misc";
import { formatDate, impactLabel, importanceLabel } from "@/lib/format";
import type { Impact, Importance } from "@/lib/types";
import { useApi } from "@/lib/use-api";
import { impactTone } from "@/features/news/parts";

type Lens = {
  role: string;
  industry: string;
  depth: "brief" | "full";
  relevance_score: number;
  importance_level: Importance;
  impact_type: Impact;
  why_it_matters: string;
  bank_view?: string;
  risk?: string;
  opportunity?: string;
};
type Item = { id: string; title: string; url: string; published_at: string; source_name: string; source_kind: string };
type Showcase = {
  example: {
    article: { title: string; summary: string; url: string; published_at: string; source: string };
    lenses: Lens[];
  } | null;
  latest_news: Item[];
  latest_research: Item[];
};

// Реальный пример работы сервиса: последний разбор новости моделью из базы
// (для нескольких ролей, если он есть) и то, что прямо сейчас приходит в ленту.
export function LiveExample() {
  const { data, loading, error } = useApi<Showcase>("/public/showcase");

  if (loading) return <Skeleton className="h-[360px] rounded-[var(--radius-xl)]" />;
  if (error || !data) {
    return (
      <div className="glass rounded-[var(--radius-xl)] p-6 text-[15px] text-ink-soft sm:p-8">
        Пример временно недоступен — сервер не отвечает.
      </div>
    );
  }
  const ex = data.example;
  return (
    <div className="space-y-4">
      {ex ? (
        <figure className="glass rounded-[var(--radius-xl)] p-6 sm:p-8">
          <p className="text-[12px] text-silver">
            {ex.article.source} · {formatDate(ex.article.published_at)}
          </p>
          <p className="mt-1 text-[17px] font-semibold leading-snug">{ex.article.title}</p>
          <div className={ex.lenses.length > 1 ? "mt-5 grid gap-3 md:grid-cols-2" : "mt-5"}>
            {ex.lenses.slice(0, 2).map((l) => (
              <div key={l.role} className="rounded-[16px] border border-white bg-white/70 p-4">
                <div className="flex flex-wrap items-center gap-1.5">
                  <Badge tone="bank">
                    <Sparkles className="size-3" aria-hidden /> {l.role}
                  </Badge>
                  <Badge tone={impactTone[l.impact_type]}>{impactLabel[l.impact_type]}</Badge>
                </div>
                <p className="mt-1.5 text-[12px] text-silver">
                  {l.industry} · важность: {importanceLabel[l.importance_level].toLowerCase()} · релевантность {l.relevance_score}
                </p>
                <p className="mt-2.5 text-[14px] leading-relaxed text-ink">{l.why_it_matters}</p>
                {l.bank_view && (
                  <p className="mt-2.5 border-l-2 border-bankview pl-2.5 text-[13px] leading-relaxed text-ink-soft">
                    <span className="font-medium text-bankview">Позиция Альфа-Банка: </span>
                    {l.bank_view}
                  </p>
                )}
              </div>
            ))}
          </div>
          <figcaption className="mt-4 text-[12px] text-silver">
            Реальный разбор из «Альфа Инсайт»: вывод сформирован моделью по новости, профилю компании и роли пользователя.
            Компания описана отраслью, имена не показываются.
          </figcaption>
        </figure>
      ) : (
        <div className="glass rounded-[var(--radius-xl)] p-6 text-[14px] leading-relaxed text-ink-soft sm:p-8">
          Персональные разборы появятся здесь после первого анализа новости моделью. Ниже — что прямо сейчас приходит в
          ленту.
        </div>
      )}

      <div className="glass rounded-[var(--radius-xl)] p-6 sm:p-8">
        <p className="text-[13px] font-medium text-ink-soft">Сейчас в ленте</p>
        <ul className="mt-3 divide-y divide-line/70">
          {[...data.latest_research, ...data.latest_news].map((n) => (
            <li key={n.id} className="py-2.5 first:pt-0 last:pb-0">
              <p className="text-[14px] font-medium leading-snug">{n.title}</p>
              <p className={n.source_kind === "alfa_research" ? "mt-0.5 text-[12px] text-bankview" : "mt-0.5 text-[12px] text-silver"}>
                {n.source_name} · {formatDate(n.published_at)}
              </p>
            </li>
          ))}
        </ul>
      </div>
    </div>
  );
}
