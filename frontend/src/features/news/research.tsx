"use client";

import Link from "next/link";
import { useEffect } from "react";
import { ArrowLeft } from "lucide-react";
import { Badge, ErrorState, Panel, Skeleton } from "@/components/ui/misc";
import { Button } from "@/components/ui/button";
import { formatDate } from "@/lib/format";
import { useApi } from "@/lib/use-api";

type Doc = {
  document: { id: number; title: string; subtitle: string; published_on: string | null; author_unit: string; pages: number; is_outdated: boolean; file_name: string };
  chunks: { id: number; chunk_no: number; page: number; content: string }[];
  article_id: string | null;
};

// Материал Альфа-Банка по страницам: сюда ведут ссылки [R…] из анализа.
export function ResearchView({ id }: { id: string }) {
  const { data, error, loading, reload } = useApi<Doc>(`/research/${id}`);
  useEffect(() => {
    if (data && window.location.hash) document.querySelector(window.location.hash)?.scrollIntoView({ block: "center" });
  }, [data]);
  if (error) return <ErrorState error={error} onRetry={reload} />;
  if (loading || !data) return <Skeleton className="h-96" />;
  const d = data.document;
  const pages = new Map<number, Doc["chunks"]>();
  data.chunks.forEach((c) => pages.set(c.page, [...(pages.get(c.page) ?? []), c]));
  return (
    <div className="max-w-[860px]">
      <Link href="/feed?source_kind=alfa_research" className="inline-flex items-center gap-1.5 text-[13px] text-ink-soft hover:text-ink">
        <ArrowLeft className="size-4" aria-hidden /> Аналитика Альфа-Банка
      </Link>
      <p className="mt-4 text-[13px] font-medium text-bankview">{d.author_unit}</p>
      <h1 className="display mt-1 text-[30px] sm:text-[38px]">{d.title}</h1>
      <div className="mt-3 flex flex-wrap items-center gap-2 text-[13px] text-ink-soft">
        {d.published_on && <span>{formatDate(d.published_on)}</span>}
        <span>{d.pages} стр.</span>
        {d.is_outdated && <Badge tone="mixed">Материал 2021 года — может быть неактуален</Badge>}
        {data.article_id && (
          <Button asChild variant="outline" size="sm">
            <Link href={`/news/${data.article_id}`}>Анализ для моей компании</Link>
          </Button>
        )}
      </div>
      <p className="mt-3 text-[12px] text-silver">
        Текст получен распознаванием PDF из материалов кейса; таблицы и графики в него не входят.
      </p>
      <div className="mt-6 space-y-6">
        {[...pages.entries()].map(([page, chunks]) => (
          <Panel key={page} title={`Страница ${page}`}>
            <div className="space-y-3">
              {chunks.map((c) => (
                <p key={c.id} id={`c${c.id}`} className="scroll-mt-24 rounded-[8px] text-[15px] leading-relaxed text-ink-soft target:bg-bankview-bg target:text-ink">
                  <span className="mr-2 text-[11px] text-silver tabular">R{c.id}</span>
                  {c.content}
                </p>
              ))}
            </div>
          </Panel>
        ))}
      </div>
    </div>
  );
}
