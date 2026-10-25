"use client";

import { useState } from "react";
import { FileText, ShieldCheck } from "lucide-react";
import { Badge, Panel } from "@/components/ui/misc";
import type { ContentLevel, Insight, SourceCompleteness, UnverifiedValue } from "@/lib/types";
import { cn } from "@/lib/utils";

const levelTone: Record<ContentLevel, "chance" | "neutral" | "mixed"> = { full_text: "chance", summary: "neutral", title_only: "mixed" };

// Полнота текста источника: полный текст, заголовок с анонсом или только заголовок.
export function CompletenessBadge({ c }: { c?: SourceCompleteness }) {
  if (!c) return null;
  return (
    <Badge tone={levelTone[c.level]} title={c.note || "Анализ строится по полному тексту публикации"}>
      <FileText className="size-3" aria-hidden /> {c.label}
    </Badge>
  );
}

const kindLabel: Record<UnverifiedValue["kind"], string> = {
  percent: "процент",
  money: "сумма",
  term: "срок",
  date: "дата",
  year: "год",
  entity: "название",
};

const sectionLabel = (path: string) => {
  const name = path.replace(/\[\d+\]$/, "");
  const map: Record<string, string> = {
    "what_happened.facts": "Факт",
    "what_happened.summary": "Что произошло",
    why_it_matters: "Почему важно",
    potential_impact: "Возможное влияние",
    risks: "Риск",
    opportunities: "Возможность",
    recommended_actions: "Действие",
    bank_view: "Позиция банка",
  };
  return map[name] ?? name;
};

function percent(r: number) {
  return `${Math.round(r * 100)}%`;
}

// Строка точности над анализом: покрытие источниками, проверка чисел и полнота источника.
export function AccuracyStrip({ insight }: { insight: Insight }) {
  const cov = insight.source_coverage;
  const fc = insight.fact_check;
  if (!cov && !fc && !insight.source_completeness) return null;
  const unverified = fc?.unverified.length ?? 0;
  return (
    <div className="glass flex flex-wrap items-center gap-x-6 gap-y-3 rounded-[var(--radius-lg)] px-5 py-4" aria-label="Проверка точности анализа">
      <ShieldCheck className="size-5 text-steel" aria-hidden />
      {cov && cov.total > 0 && (
        <div>
          <p className="text-[12px] text-silver">Подтверждено источниками</p>
          <p className="text-[15px] font-semibold tabular">
            {cov.confirmed} из {cov.total} утверждений · {percent(cov.ratio)}
          </p>
        </div>
      )}
      {fc && (
        <div>
          <p className="text-[12px] text-silver">Числа, даты и названия</p>
          <p className={cn("text-[15px] font-semibold tabular", unverified > 0 && "text-risk")}>
            {fc.checked === 0 ? "в тексте нет" : unverified === 0 ? `все ${fc.checked} найдены в источниках` : `не найдено ${unverified} из ${fc.checked}`}
          </p>
        </div>
      )}
      {insight.source_completeness && (
        <div className="flex items-center gap-2">
          <span className="text-[12px] text-silver">Источник:</span>
          <CompletenessBadge c={insight.source_completeness} />
        </div>
      )}
    </div>
  );
}

// Список значений, не найденных в источниках, — в боковой колонке.
export function FactCheckPanel({ insight }: { insight: Insight }) {
  const [open, setOpen] = useState(false);
  const items = insight.fact_check?.unverified ?? [];
  if (!items.length) return null;
  const shown = open ? items : items.slice(0, 3);
  return (
    <Panel title="Проверка фактов" description="Значения из ответа модели, которых нет в источниках">
      <ul className="space-y-3">
        {shown.map((u, i) => (
          <li key={i} className="text-[13px]">
            <div className="flex flex-wrap items-center gap-1.5">
              <Badge tone={u.action === "removed" ? "risk" : "mixed"}>{u.action === "removed" ? "Удалено" : "Помечено"}</Badge>
              <span className="font-medium">«{u.value}»</span>
              <span className="text-silver">
                {kindLabel[u.kind]}, {sectionLabel(u.path).toLowerCase()}
              </span>
            </div>
            <p className="mt-1 text-ink-soft">{u.claim}</p>
          </li>
        ))}
      </ul>
      {items.length > 3 && (
        <button className="mt-3 text-[13px] text-steel hover:underline" onClick={() => setOpen(!open)}>
          {open ? "Свернуть" : `Показать все (${items.length})`}
        </button>
      )}
      <p className="mt-3 text-[12px] text-silver">
        Факты и позиции банка с такими значениями удаляются; в остальных выводах значение нужно проверить по первоисточнику.
      </p>
    </Panel>
  );
}
