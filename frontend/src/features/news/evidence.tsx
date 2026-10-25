"use client";

import { ExternalLink } from "lucide-react";
import { formatDate } from "@/lib/format";
import type { Evidence } from "@/lib/types";
import { cn } from "@/lib/utils";

// Метки источников в тексте. prefix задаёт якорь списка источников, к которому ведёт метка.
export function SourceChips({ ids, evidence, prefix = "ev-" }: { ids: string[]; evidence: Map<string, Evidence>; prefix?: string }) {
  if (!ids?.length) return null;
  return (
    <span className="ml-1 inline-flex flex-wrap gap-1 align-middle">
      {ids.map((id) => {
        const e = evidence.get(id);
        return (
          <a
            key={id}
            href={`#${prefix}${id}`}
            title={e ? e.title : id}
            className={cn(
              "rounded-[6px] px-1.5 py-px text-[11px] font-medium tabular hover:underline",
              id.startsWith("R") ? "bg-bankview-bg text-bankview" : id.startsWith("C:") ? "bg-frost text-ink-soft" : "bg-white text-ink ring-1 ring-line",
            )}
          >
            {id}
          </a>
        );
      })}
    </span>
  );
}

const evType = { article: "Новость", related_news: "Связанная новость", bank_research: "Аналитика Альфа-Банка", company_data: "Данные о компании" };

export function EvidenceItem({ e, prefix = "ev-" }: { e: Evidence; prefix?: string }) {
  const href =
    e.type === "bank_research" && e.url ? `${e.url}#c${e.id.slice(1)}` : e.url && e.url.startsWith("http") ? e.url : undefined;
  return (
    <li id={`${prefix}${e.id}`} className="scroll-mt-24 rounded-[12px] p-2 -m-2 target:bg-glacier/30">
      <div className="flex items-center gap-2">
        <span className="rounded-[6px] bg-frost px-1.5 py-px text-[11px] font-medium text-ink-soft tabular">{e.id}</span>
        <span className="text-[12px] text-silver">{evType[e.type]}</span>
      </div>
      <p className="mt-1 text-[14px] font-medium leading-snug">
        {href ? (
          <a
            href={href}
            target={href.startsWith("http") ? "_blank" : undefined}
            rel="noopener noreferrer"
            className="hover:text-steel hover:underline"
          >
            {e.title}
            {href.startsWith("http") && <ExternalLink className="ml-1 inline size-3" aria-hidden />}
          </a>
        ) : (
          e.title
        )}
      </p>
      <p className="mt-0.5 line-clamp-3 text-[13px] text-ink-soft">{e.excerpt}</p>
      {(e.date || e.note) && (
        <p className="mt-0.5 text-[12px] text-silver">
          {e.date && formatDate(e.date)}
          {e.date && e.note ? ", " : ""}
          {e.note}
        </p>
      )}
    </li>
  );
}
