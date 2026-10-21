"use client";

import Link from "next/link";
import { useState } from "react";
import { Bookmark, BookmarkCheck, ExternalLink, Sparkles } from "lucide-react";
import { toast } from "sonner";
import { Button } from "@/components/ui/button";
import { Badge } from "@/components/ui/misc";
import { api, errorMessage } from "@/lib/api";
import { formatDate, impactLabel, importanceLabel } from "@/lib/format";
import type { AnalysisRef, FeedItem, Impact, Importance } from "@/lib/types";
import { cn } from "@/lib/utils";

export const impactTone: Record<Impact, "risk" | "chance" | "mixed" | "neutral"> = {
  risk: "risk",
  opportunity: "chance",
  mixed: "mixed",
  neutral: "neutral",
};

// Шкала релевантности: заполненная дуга + число. Оценка модели — сталь, по правилам — светлый глетчер.
export function RelevanceMeter({ score, method, size = 44 }: { score: number; method: "llm" | "rules"; size?: number }) {
  const r = size / 2 - 3;
  const c = 2 * Math.PI * r;
  const filled = (Math.max(0, Math.min(100, score)) / 100) * c;
  return (
    <div
      className="relative shrink-0"
      style={{ width: size, height: size }}
      role="img"
      aria-label={`Релевантность ${score} из 100${method === "rules" ? ", предварительная оценка без ИИ" : ", оценка модели"}`}
    >
      <svg width={size} height={size} className="-rotate-90">
        <circle cx={size / 2} cy={size / 2} r={r} fill="white" stroke="var(--color-frost)" strokeWidth="4" />
        <circle
          cx={size / 2}
          cy={size / 2}
          r={r}
          fill="none"
          stroke={method === "llm" ? "var(--color-steel)" : "var(--color-glacier)"}
          strokeWidth="4"
          strokeLinecap="round"
          strokeDasharray={`${filled} ${c}`}
        />
      </svg>
      <span className="absolute inset-0 flex items-center justify-center text-[13px] font-semibold text-ink tabular">{score}</span>
    </div>
  );
}

export function MethodTag({ method }: { method: "llm" | "rules" }) {
  return method === "llm" ? (
    <Badge tone="bank" title="Оценка выполнена разрешённой языковой моделью">
      <Sparkles className="size-3" aria-hidden /> Оценка ИИ
    </Badge>
  ) : (
    <Badge tone="outline" title="Предварительная оценка по совпадению тем с профилем, без языковой модели">
      Предварительно, без ИИ
    </Badge>
  );
}

export function ImportanceBadge({ level }: { level: Importance }) {
  const tone = level === "critical" || level === "high" ? "steel" : "neutral";
  return <Badge tone={tone}>Важность: {importanceLabel[level].toLowerCase()}</Badge>;
}

export function SourceLine({ item }: { item: Pick<FeedItem, "source" | "published_at" | "url"> }) {
  const external = item.source.kind === "rss" || item.url.startsWith("http");
  return (
    <p className="flex flex-wrap items-center gap-x-2 gap-y-1 text-[12px] text-silver">
      <span className={cn(item.source.kind === "alfa_research" && "font-medium text-bankview")}>{item.source.name}</span>
      <span aria-hidden>/</span>
      <time dateTime={item.published_at}>{formatDate(item.published_at)}</time>
      {external && (
        <a href={item.url} target="_blank" rel="noopener noreferrer" className="inline-flex items-center gap-1 text-steel hover:underline">
          Оригинал <ExternalLink className="size-3" aria-hidden />
        </a>
      )}
    </p>
  );
}

export function SaveButton({ id, saved, onChange }: { id: string; saved: boolean; onChange?: (v: boolean) => void }) {
  const [value, setValue] = useState(saved);
  const [busy, setBusy] = useState(false);
  async function toggle() {
    setBusy(true);
    try {
      await api(`/articles/${id}/save`, { method: value ? "DELETE" : "POST" });
      setValue(!value);
      onChange?.(!value);
      toast.success(value ? "Удалено из сохранённых" : "Сохранено");
    } catch (e) {
      toast.error(errorMessage(e));
    } finally {
      setBusy(false);
    }
  }
  return (
    <Button variant="ghost" size="icon" onClick={toggle} disabled={busy} aria-pressed={value} aria-label={value ? "Удалить из сохранённых" : "Сохранить"}>
      {value ? <BookmarkCheck className="text-steel" /> : <Bookmark />}
    </Button>
  );
}

export function WhyItMatters({ a }: { a: AnalysisRef }) {
  return (
    <div className={cn("rounded-[12px] px-3.5 py-2.5 text-[14px] leading-relaxed", a.method === "llm" ? "bg-bankview-bg/70 text-ink" : "bg-frost/70 text-ink-soft")}>
      {a.why_it_matters}
    </div>
  );
}

export function NewsCard({ item, onSavedChange }: { item: FeedItem; onSavedChange?: (v: boolean) => void }) {
  const a = item.analysis;
  return (
    <article className="glass group rounded-[var(--radius-lg)] p-5">
      <div className="flex gap-4">
        {a && <RelevanceMeter score={a.relevance_score} method={a.method} />}
        <div className="min-w-0 flex-1">
          <SourceLine item={item} />
          <h3 className="mt-1.5 text-[17px] font-semibold leading-snug tracking-[-0.01em]">
            <Link href={`/news/${item.id}`} className="hover:text-steel">
              {item.title}
            </Link>
          </h3>
          {item.summary && <p className="mt-1.5 line-clamp-3 text-[14px] leading-relaxed text-ink-soft">{item.summary}</p>}
          {a && (
            <div className="mt-3 space-y-2.5">
              <div className="flex flex-wrap items-center gap-1.5">
                <MethodTag method={a.method} />
                <ImportanceBadge level={a.importance_level} />
                {a.method === "llm" && a.impact_type !== "neutral" && <Badge tone={impactTone[a.impact_type]}>{impactLabel[a.impact_type]}</Badge>}
              </div>
              <WhyItMatters a={a} />
            </div>
          )}
          <div className="mt-3 flex flex-wrap items-center justify-between gap-2">
            <div className="flex flex-wrap gap-1.5">
              {item.topics.slice(0, 4).map((t) => (
                <span key={t.code} className="text-[12px] text-silver" title={t.method === "keywords" ? "Тема определена по словарю" : undefined}>
                  #{t.name.toLowerCase()}
                </span>
              ))}
            </div>
            <div className="flex items-center gap-1">
              <SaveButton id={item.id} saved={item.is_saved} onChange={onSavedChange} />
              <Button asChild variant="outline" size="sm">
                <Link href={`/news/${item.id}`}>{a?.depth === "full" ? "Открыть анализ" : "Подробнее"}</Link>
              </Button>
            </div>
          </div>
        </div>
      </div>
    </article>
  );
}
