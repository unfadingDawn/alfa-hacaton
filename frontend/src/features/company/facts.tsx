import type { Fact } from "@/lib/types";
import { cn } from "@/lib/utils";

const compactIds = ["C:okved", "C:revenue", "C:group_revenue", "C:segment", "C:region", "C:workers", "C:age", "C:eco_stage", "C:client_type", "C:has_loan",
  "C:has_leasing", "C:fts", "C:fts_countries"];

// Таблица признаков компании с указанием поля-источника в данных банка.
export function CompanyFacts({ facts, compact, className }: { facts: Fact[]; compact?: boolean; className?: string }) {
  const list = compact ? facts.filter((f) => compactIds.includes(f.id)) : facts;
  return (
    <dl className={cn("divide-y divide-line/70 text-[14px]", className)}>
      {list.map((f) => (
        <div key={f.id} className="grid gap-x-4 gap-y-0.5 py-2 sm:grid-cols-[minmax(0,0.9fr)_minmax(0,1.1fr)]">
          <dt className="text-ink-soft">{f.label}</dt>
          <dd className="text-ink">
            {f.value}
            {!compact && <span className="mt-0.5 block text-[12px] text-silver">{f.source}</span>}
          </dd>
        </div>
      ))}
    </dl>
  );
}
