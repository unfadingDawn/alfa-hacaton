"use client";

import Link from "next/link";
import { useEffect, useRef, useState } from "react";
import { BookOpen, CircleAlert, CornerDownLeft, ShieldCheck, Sparkles } from "lucide-react";
import { Button } from "@/components/ui/button";
import { Textarea } from "@/components/ui/field";
import { Badge, ErrorState, Skeleton } from "@/components/ui/misc";
import { api, ApiError, errorMessage } from "@/lib/api";
import { useApi } from "@/lib/use-api";
import { cn } from "@/lib/utils";

type SupportAnswer = {
  answer: string;
  points: { text: string; source_ids: string[] }[];
  not_in_kb: boolean;
  out_of_scope: boolean;
  sources: { id: string; title: string; link?: string; link_label?: string }[];
  checks?: { grounding: number; unknown_ui?: string[]; unknown_routes?: string[]; rejected: boolean; notes: string[] };
};

type SupportMessage = {
  id: string;
  question: string;
  preset: string | null;
  method: "kb" | "llm" | "fallback";
  answer: SupportAnswer;
  cached: boolean;
  created_at: string;
};

type History = {
  items: SupportMessage[];
  quick_questions: { code: string; text: string }[];
  left_today: number | null;
  llm_available: boolean;
};

const MAX_LEN = 500;

// Чат с ИИ-помощником: только вопросы о работе сервиса, ответы — по справке
// с проверкой на выдуманные разделы, кнопки и утверждения без опоры.
export function SupportChat({ compact = false, autoFocus = false }: { compact?: boolean; autoFocus?: boolean }) {
  const { data, error, loading, reload, mutate } = useApi<History>("/support/assistant");
  const [text, setText] = useState("");
  const [busy, setBusy] = useState<string | null>(null);
  const [err, setErr] = useState<string | null>(null);
  const listRef = useRef<HTMLDivElement>(null);
  const inputRef = useRef<HTMLTextAreaElement>(null);

  useEffect(() => {
    listRef.current?.scrollTo({ top: listRef.current.scrollHeight, behavior: "smooth" });
  }, [data?.items.length, busy]);

  useEffect(() => {
    if (autoFocus && data) inputRef.current?.focus();
  }, [autoFocus, data]);

  async function ask(body: { question?: string; preset?: string }, key: string) {
    setBusy(key);
    setErr(null);
    try {
      const r = await api<{ item: SupportMessage; left_today: number | null }>("/support/assistant", { method: "POST", body });
      mutate((d) => (d ? { ...d, items: [...d.items, r.item], left_today: r.left_today } : d));
      if (body.question) setText("");
    } catch (e) {
      setErr(e instanceof ApiError ? e.fields?.question ?? e.message : errorMessage(e));
    } finally {
      setBusy(null);
    }
  }

  function submit() {
    const q = text.trim();
    if (q.length < 3) return setErr("Вопрос должен содержать хотя бы 3 символа");
    void ask({ question: q }, "free");
  }

  if (error) return <ErrorState error={error} onRetry={reload} className="m-4" />;
  if (loading || !data)
    return (
      <div className="space-y-3 p-4">
        <Skeleton className="h-16" />
        <Skeleton className="h-8 w-2/3" />
      </div>
    );

  return (
    <div className="flex min-h-0 flex-1 flex-col">
      <div ref={listRef} className={cn("min-h-0 flex-1 space-y-4 overflow-y-auto px-4 py-4", !compact && "sm:px-5")} aria-live="polite">
        <div className="rounded-[14px] bg-frost/70 px-3.5 py-3 text-[14px] leading-relaxed text-ink-soft">
          Здравствуйте! Я подскажу, как пользоваться «Альфа Инсайт»: лента, персональный анализ, проверка точности, настройки профиля.
          Отвечаю только по справке сервиса.
        </div>
        {data.items.map((m) => (
          <Message key={m.id} m={m} />
        ))}
        {busy && (
          <div className="flex items-center gap-2 text-[13px] text-silver">
            <span className="inline-flex gap-1" aria-hidden>
              {[0, 1, 2].map((i) => (
                <span key={i} className="size-1.5 animate-bounce rounded-full bg-steel/70" style={{ animationDelay: `${i * 120}ms` }} />
              ))}
            </span>
            Ищу ответ в справке…
          </div>
        )}
      </div>

      <div className="border-t border-line/80 px-4 pb-4 pt-3">
        <div className="mb-2.5 flex flex-wrap gap-1.5" role="group" aria-label="Частые вопросы">
          {data.quick_questions.map((q) => (
            <button
              key={q.code}
              disabled={!!busy}
              onClick={() => ask({ preset: q.code }, q.code)}
              className="rounded-full border border-line bg-white/70 px-2.5 py-1 text-[12px] text-ink-soft transition-colors hover:border-steel/40 hover:text-ink disabled:opacity-50"
            >
              {q.text}
            </button>
          ))}
        </div>
        <form
          onSubmit={(e) => {
            e.preventDefault();
            submit();
          }}
          className="flex items-end gap-2"
        >
          <label htmlFor={compact ? "support-input-widget" : "support-input"} className="sr-only">
            Вопрос помощнику
          </label>
          <Textarea
            ref={inputRef}
            id={compact ? "support-input-widget" : "support-input"}
            value={text}
            maxLength={MAX_LEN}
            onChange={(e) => setText(e.target.value)}
            onKeyDown={(e) => {
              if (e.key === "Enter" && !e.shiftKey && !e.nativeEvent.isComposing) {
                e.preventDefault();
                submit();
              }
            }}
            placeholder="Как сменить роль? Где трасса агента?"
            className="min-h-11 flex-1 resize-none py-2"
            rows={1}
          />
          <Button type="submit" size="icon" loading={busy === "free"} disabled={!!busy || !text.trim()} aria-label="Отправить вопрос">
            {busy !== "free" && <CornerDownLeft />}
          </Button>
        </form>
        {err && (
          <p role="alert" className="mt-2 flex items-start gap-1.5 text-[12px] text-risk">
            <CircleAlert className="mt-0.5 size-3.5 shrink-0" aria-hidden />
            {err}
          </p>
        )}
        <p className="mt-2 flex items-start gap-1.5 text-[11px] leading-snug text-silver">
          <ShieldCheck className="mt-px size-3.5 shrink-0" aria-hidden />
          <span>
            Ответы ИИ сверяются со справкой: шаги без опоры, выдуманные разделы и кнопки удаляются.
            {data.left_today != null && data.llm_available && ` Вопросов к ИИ сегодня осталось: ${data.left_today}.`}
          </span>
        </p>
      </div>
    </div>
  );
}

function MethodBadge({ m }: { m: SupportMessage }) {
  if (m.answer.out_of_scope) return <Badge tone="mixed">Вне темы сервиса</Badge>;
  if (m.method === "kb")
    return (
      <Badge tone="outline">
        <BookOpen className="size-3" aria-hidden /> Из справки
      </Badge>
    );
  if (m.method === "fallback")
    return (
      <Badge tone="mixed" title="Ответ модели не прошёл проверку и заменён статьёй справки">
        <BookOpen className="size-3" aria-hidden /> Справка вместо ответа ИИ
      </Badge>
    );
  return (
    <Badge tone="bank" title="Ответ модели проверен по статьям справки">
      <Sparkles className="size-3" aria-hidden /> Ответ ИИ, проверен по справке
    </Badge>
  );
}

function Message({ m }: { m: SupportMessage }) {
  const a = m.answer;
  const removed = a.checks?.notes?.length ?? 0;
  return (
    <div>
      <div className="flex justify-end">
        <p className="max-w-[85%] rounded-[14px] rounded-br-[4px] bg-steel px-3 py-2 text-[14px] text-white">{m.question}</p>
      </div>
      <div className="mt-2 max-w-[95%] rounded-[14px] rounded-bl-[4px] border border-line bg-white/80 p-3.5">
        <div className="mb-1.5 flex flex-wrap gap-1.5">
          <MethodBadge m={m} />
          {m.cached && <Badge tone="outline">Повтор</Badge>}
        </div>
        <p className="text-[14px] leading-relaxed">{a.answer}</p>
        {a.points.length > 0 && (
          <ol className="mt-2 list-decimal space-y-1 pl-5 text-[14px] leading-relaxed text-ink-soft">
            {a.points.map((p, i) => (
              <li key={i}>{p.text}</li>
            ))}
          </ol>
        )}
        {a.sources.length > 0 && (
          <div className="mt-2.5 flex flex-wrap gap-1.5">
            {a.sources.map((s) =>
              s.link ? (
                <Link
                  key={s.id}
                  href={s.link}
                  className="inline-flex items-center gap-1 rounded-full bg-frost px-2.5 py-0.5 text-[12px] text-ink-soft hover:text-steel"
                  title={s.title}
                >
                  {s.title} →
                </Link>
              ) : (
                <span key={s.id} className="rounded-full bg-frost px-2.5 py-0.5 text-[12px] text-ink-soft">
                  {s.title}
                </span>
              ),
            )}
          </div>
        )}
        {removed > 0 && (
          <details className="mt-2 text-[11px] text-silver">
            <summary className="cursor-pointer">Проверка убрала недостоверных фрагментов: {removed}</summary>
            <ul className="mt-1 list-disc space-y-0.5 pl-4">
              {a.checks!.notes.map((n, i) => (
                <li key={i}>{n}</li>
              ))}
            </ul>
          </details>
        )}
      </div>
    </div>
  );
}
