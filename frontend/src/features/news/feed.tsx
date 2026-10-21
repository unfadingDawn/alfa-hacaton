"use client";

import { useEffect, useMemo, useState } from "react";
import { useRouter, useSearchParams } from "next/navigation";
import { Plus, Search, Sparkles } from "lucide-react";
import { toast } from "sonner";
import { PageHeader } from "@/components/app-shell";
import { Button } from "@/components/ui/button";
import { Dialog, DialogContent, DialogTrigger } from "@/components/ui/dialog";
import { Field, Input, Select, Textarea } from "@/components/ui/field";
import { EmptyState, ErrorState, Skeleton } from "@/components/ui/misc";
import { api, ApiError, errorMessage } from "@/lib/api";
import { pluralize } from "@/lib/format";
import type { FeedItem, Topic } from "@/lib/types";
import { useApi } from "@/lib/use-api";
import { NewsCard } from "./parts";

type Filters = { q: string; topic: string; min_importance: string; impact: string; period: string; source_kind: string; sort: string };
const initial: Filters = { q: "", topic: "", min_importance: "", impact: "", period: "all", source_kind: "", sort: "relevance" };
const PAGE = 20;

export function Feed({ savedOnly = false }: { savedOnly?: boolean }) {
  const sp = useSearchParams();
  const [filters, setFilters] = useState<Filters>(() => ({
    ...(savedOnly ? { ...initial, period: "all", sort: "date" } : initial),
    topic: sp.get("topic") ?? "",
    source_kind: sp.get("source_kind") ?? "",
  }));
  const [q, setQ] = useState("");
  const [items, setItems] = useState<FeedItem[]>([]);
  const [total, setTotal] = useState(0);
  const [offset, setOffset] = useState(0);
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState<unknown>(null);
  const topics = useApi<{ items: Topic[] }>("/topics");
  const [tick, setTick] = useState(0);

  useEffect(() => {
    const t = setTimeout(() => setFilters((f) => (f.q === q ? f : { ...f, q })), 350);
    return () => clearTimeout(t);
  }, [q]);

  const query = useMemo(() => {
    const p = new URLSearchParams();
    Object.entries(filters).forEach(([k, v]) => v && p.set(k, v));
    if (savedOnly) p.set("saved", "true");
    p.set("limit", String(PAGE));
    return p;
  }, [filters, savedOnly]);

  useEffect(() => {
    const c = new AbortController();
    setLoading(true);
    setError(null);
    api<{ items: FeedItem[]; total: number }>(`/feed?${query}&offset=0`, { signal: c.signal })
      .then((r) => {
        setItems(r.items);
        setTotal(r.total);
        setOffset(r.items.length);
        setLoading(false);
      })
      .catch((e) => {
        if ((e as Error).name !== "AbortError") {
          setError(e);
          setLoading(false);
        }
      });
    return () => c.abort();
  }, [query, tick]);

  async function more() {
    try {
      const r = await api<{ items: FeedItem[]; total: number }>(`/feed?${query}&offset=${offset}`);
      setItems((x) => [...x, ...r.items]);
      setOffset((o) => o + r.items.length);
    } catch (e) {
      toast.error(errorMessage(e));
    }
  }

  const set = (k: keyof Filters) => (e: React.ChangeEvent<HTMLSelectElement>) => setFilters({ ...filters, [k]: e.target.value });
  const filtered = JSON.stringify({ ...filters, q }) !== JSON.stringify(savedOnly ? { ...initial, period: "all", sort: "date", q: "" } : { ...initial, q: "" });

  return (
    <div>
      <PageHeader
        title={savedOnly ? "Сохранённые" : "Лента"}
        lead={
          savedOnly
            ? "Новости, к которым вы хотели вернуться."
            : "Новости из официальных лент и материалы Альфа-Банка, упорядоченные по значимости для вашей компании и роли."
        }
        action={!savedOnly && (
          <div className="flex flex-wrap gap-2">
            <SubmitNewsDialog />
            <RankDialog onDone={() => setTick((t) => t + 1)} />
          </div>
        )}
      />

      <div className="glass mb-6 rounded-[var(--radius-lg)] p-4">
        <div className="relative">
          <Search className="pointer-events-none absolute left-3.5 top-1/2 size-4 -translate-y-1/2 text-silver" aria-hidden />
          <Input aria-label="Поиск по новостям" className="pl-10" value={q} onChange={(e) => setQ(e.target.value)} placeholder="Поиск: ставка, экспорт, сталь…" />
        </div>
        <div className="mt-3 grid grid-cols-2 gap-2 md:grid-cols-3 xl:grid-cols-6">
          <Select aria-label="Тема" value={filters.topic} onChange={set("topic")}>
            <option value="">Все темы</option>
            {topics.data?.items.map((t) => (
              <option key={t.code} value={t.code}>
                {t.name}
              </option>
            ))}
          </Select>
          <Select aria-label="Важность" value={filters.min_importance} onChange={set("min_importance")}>
            <option value="">Любая важность</option>
            <option value="medium">Средняя и выше</option>
            <option value="high">Высокая и выше</option>
            <option value="critical">Только критическая</option>
          </Select>
          <Select aria-label="Тип влияния" value={filters.impact} onChange={set("impact")}>
            <option value="">Любое влияние</option>
            <option value="risk">Риск</option>
            <option value="opportunity">Возможность</option>
            <option value="mixed">Риски и возможности</option>
          </Select>
          <Select aria-label="Период" value={filters.period} onChange={set("period")}>
            <option value="1d">За сутки</option>
            <option value="7d">За 7 дней</option>
            <option value="30d">За 30 дней</option>
            <option value="90d">За 90 дней</option>
            <option value="all">За всё время</option>
          </Select>
          <Select aria-label="Источник" value={filters.source_kind} onChange={set("source_kind")}>
            <option value="">Все источники</option>
            <option value="rss">Новостные ленты</option>
            <option value="alfa_research">Аналитика Альфа-Банка</option>
            <option value="user_submitted">Добавленные мной</option>
          </Select>
          <Select aria-label="Сортировка" value={filters.sort} onChange={set("sort")}>
            <option value="relevance">Сначала релевантные</option>
            <option value="importance">Сначала важные</option>
            <option value="date">Сначала новые</option>
          </Select>
        </div>
        {filters.impact && (
          <p className="mt-2 text-[12px] text-silver">Тип влияния определяет только модель — новости с предварительной оценкой в этот фильтр не попадают.</p>
        )}
      </div>

      <div aria-live="polite">
        {error ? (
          <ErrorState error={error} onRetry={() => setTick((t) => t + 1)} />
        ) : loading ? (
          <div className="space-y-3">
            {Array.from({ length: 4 }).map((_, i) => (
              <Skeleton key={i} className="h-40" />
            ))}
          </div>
        ) : items.length === 0 ? (
          <EmptyState
            title={savedOnly ? "Пока ничего не сохранено" : filtered ? "По этим условиям новостей нет" : "Новостей пока нет"}
            action={
              filtered ? (
                <Button variant="outline" onClick={() => { setQ(""); setFilters(savedOnly ? { ...initial, period: "all", sort: "date" } : initial); }}>
                  Сбросить фильтры
                </Button>
              ) : undefined
            }
          >
            {savedOnly
              ? "Нажмите на значок закладки у новости, чтобы она появилась здесь."
              : filtered
                ? "Измените фильтры или расширьте период."
                : "Источники опрашиваются по расписанию. Если новостей нет долго, проверьте статус источников в настройках."}
          </EmptyState>
        ) : (
          <>
            <p className="mb-3 text-[13px] text-silver tabular">
              {total} {pluralize(total, "новость", "новости", "новостей")}
            </p>
            <div className="space-y-3">
              {items.map((it) => (
                <NewsCard
                  key={it.id}
                  item={it}
                  onSavedChange={savedOnly ? (v) => !v && setItems((x) => x.filter((i) => i.id !== it.id)) : undefined}
                />
              ))}
            </div>
            {offset < total && (
              <div className="mt-6 flex justify-center">
                <Button variant="outline" onClick={more}>
                  Показать ещё
                </Button>
              </div>
            )}
          </>
        )}
      </div>
    </div>
  );
}

function RankDialog({ onDone }: { onDone: () => void }) {
  const [open, setOpen] = useState(false);
  const [busy, setBusy] = useState(false);
  async function run() {
    setBusy(true);
    try {
      const r = await api<{ scored: number; skipped: number }>("/feed/rank", { method: "POST", body: { limit: 8 } });
      toast.success(r.scored ? `Модель оценила ${r.scored} ${pluralize(r.scored, "новость", "новости", "новостей")}` : "Все верхние новости уже оценены моделью");
      setOpen(false);
      onDone();
    } catch (e) {
      toast.error(errorMessage(e));
    } finally {
      setBusy(false);
    }
  }
  return (
    <Dialog open={open} onOpenChange={setOpen}>
      <DialogTrigger asChild>
        <Button>
          <Sparkles /> Оценить ленту моделью
        </Button>
      </DialogTrigger>
      <DialogContent
        title="Оценить ленту моделью"
        description="Модель оценит до 8 новостей с лучшей предварительной оценкой, у которых ещё нет оценки ИИ."
      >
        <p className="text-sm text-ink-soft">
          Будет отправлен один запрос к модели. Расход учитывается в общем дневном лимите ключа команды.
        </p>
        <div className="mt-6 flex justify-end gap-2">
          <Button variant="ghost" onClick={() => setOpen(false)}>
            Отмена
          </Button>
          <Button onClick={run} loading={busy}>
            Оценить
          </Button>
        </div>
      </DialogContent>
    </Dialog>
  );
}

function SubmitNewsDialog() {
  const router = useRouter();
  const [open, setOpen] = useState(false);
  const [form, setForm] = useState({ title: "", text: "", url: "", published_at: "" });
  const [errors, setErrors] = useState<Record<string, string>>({});
  const [busy, setBusy] = useState(false);
  async function submit(e: React.FormEvent) {
    e.preventDefault();
    const fe: Record<string, string> = {};
    if (form.title.trim().length < 5) fe.title = "Не короче 5 символов";
    if (form.text.trim().length < 50) fe.text = "Не короче 50 символов";
    setErrors(fe);
    if (Object.keys(fe).length) return;
    setBusy(true);
    try {
      const r = await api<{ id: string }>("/articles", { method: "POST", body: form });
      toast.success("Новость добавлена");
      setOpen(false);
      router.push(`/news/${r.id}`);
    } catch (err) {
      if (err instanceof ApiError && err.fields) setErrors(err.fields);
      else toast.error(errorMessage(err));
    } finally {
      setBusy(false);
    }
  }
  const set = (k: keyof typeof form) => (e: React.ChangeEvent<HTMLInputElement | HTMLTextAreaElement>) => setForm({ ...form, [k]: e.target.value });
  return (
    <Dialog open={open} onOpenChange={setOpen}>
      <DialogTrigger asChild>
        <Button variant="secondary">
          <Plus /> Добавить новость
        </Button>
      </DialogTrigger>
      <DialogContent
        title="Проверить свою новость"
        description="Вставьте текст новости, которой нет в ленте. Она будет видна только вам и помечена как добавленная вручную."
      >
        <form onSubmit={submit} noValidate className="space-y-4">
          <Field id="n-title" label="Заголовок" error={errors.title}>
            <Input value={form.title} onChange={set("title")} />
          </Field>
          <Field id="n-text" label="Текст новости" error={errors.text}>
            <Textarea value={form.text} onChange={set("text")} className="min-h-40" />
          </Field>
          <div className="grid gap-4 sm:grid-cols-2">
            <Field id="n-url" label="Ссылка на источник" hint="Необязательно" error={errors.url}>
              <Input value={form.url} onChange={set("url")} placeholder="https://" />
            </Field>
            <Field id="n-date" label="Дата публикации" hint="По умолчанию — сегодня" error={errors.published_at}>
              <Input type="date" value={form.published_at} onChange={set("published_at")} />
            </Field>
          </div>
          <div className="flex justify-end gap-2 pt-2">
            <Button type="button" variant="ghost" onClick={() => setOpen(false)}>
              Отмена
            </Button>
            <Button type="submit" loading={busy}>
              Добавить
            </Button>
          </div>
        </form>
      </DialogContent>
    </Dialog>
  );
}
