"use client";

import { useState } from "react";
import { LogOut } from "lucide-react";
import { toast } from "sonner";
import { logout, PageHeader, useMe } from "@/components/app-shell";
import { Button } from "@/components/ui/button";
import { Field, Input, Select, Textarea } from "@/components/ui/field";
import { Badge, ErrorState, Panel, Skeleton } from "@/components/ui/misc";
import { api, ApiError, errorMessage } from "@/lib/api";
import { formatDate } from "@/lib/format";
import type { GuardStatus, Personalization, Role, Topic } from "@/lib/types";
import { useApi } from "@/lib/use-api";
import { cn } from "@/lib/utils";

export function SettingsView() {
  return (
    <div>
      <PageHeader title="Настройки" lead="Профиль, темы и параметры персонализации. Изменения роли и интересов пересчитывают оценки ленты." />
      <div className="grid gap-6 xl:grid-cols-2">
        <ProfileForm />
        <div className="space-y-6">
          <Interests />
          <PersonalizationForm />
        </div>
      </div>
      <SystemStatus />
      <div className="mt-6">
        <Button variant="outline" onClick={logout}>
          <LogOut /> Выйти из аккаунта
        </Button>
      </div>
    </div>
  );
}

function ProfileForm() {
  const { me, reload } = useMe();
  const roles = useApi<{ items: Role[] }>("/roles");
  const [form, setForm] = useState({ full_name: me.full_name, role_code: me.role_code, position_title: me.position_title, work_focus: me.work_focus });
  const [errors, setErrors] = useState<Record<string, string>>({});
  const [busy, setBusy] = useState(false);
  async function save(e: React.FormEvent) {
    e.preventDefault();
    setBusy(true);
    setErrors({});
    try {
      await api("/me/profile", { method: "PUT", body: form });
      toast.success("Профиль сохранён");
      reload();
    } catch (err) {
      if (err instanceof ApiError && err.fields) setErrors(err.fields);
      else toast.error(errorMessage(err));
    } finally {
      setBusy(false);
    }
  }
  return (
    <Panel title="Пользователь">
      <form onSubmit={save} className="space-y-4">
        <Field id="s-name" label="Имя и фамилия" error={errors.full_name}>
          <Input value={form.full_name} onChange={(e) => setForm({ ...form, full_name: e.target.value })} />
        </Field>
        <Field id="s-email" label="Email" hint="Изменение email пока не поддерживается">
          <Input value={me.email} disabled readOnly />
        </Field>
        <Field id="s-role" label="Роль в принятии решений" hint="Определяет линзу анализа новостей" error={errors.role_code}>
          <Select value={form.role_code} onChange={(e) => setForm({ ...form, role_code: e.target.value })}>
            {roles.data?.items.map((r) => (
              <option key={r.code} value={r.code}>
                {r.title}
              </option>
            ))}
          </Select>
        </Field>
        <Field id="s-pos" label="Должность" error={errors.position_title}>
          <Input value={form.position_title} onChange={(e) => setForm({ ...form, position_title: e.target.value })} />
        </Field>
        <Field id="s-focus" label="Рабочие интересы" error={errors.work_focus}>
          <Textarea value={form.work_focus} onChange={(e) => setForm({ ...form, work_focus: e.target.value })} maxLength={1000} />
        </Field>
        <div className="flex justify-end">
          <Button type="submit" loading={busy}>
            Сохранить профиль
          </Button>
        </div>
      </form>
    </Panel>
  );
}

const weights = [
  { v: 0, l: "Не важно" },
  { v: 1, l: "Иногда" },
  { v: 2, l: "Важно" },
  { v: 3, l: "Очень важно" },
];

function Interests() {
  const { me, reload } = useMe();
  const topics = useApi<{ items: Topic[] }>("/topics");
  const [w, setW] = useState<Record<string, number>>(() => Object.fromEntries(me.interests.map((i) => [i.topic_code, i.weight])));
  const [busy, setBusy] = useState(false);
  async function save() {
    setBusy(true);
    try {
      await api("/me/interests", {
        method: "PUT",
        body: { topics: Object.entries(w).filter(([, v]) => v > 0).map(([code, weight]) => ({ code, weight })) },
      });
      toast.success("Интересы сохранены", { description: "Оценки ленты будут пересчитаны." });
      reload();
    } catch (e) {
      toast.error(errorMessage(e));
    } finally {
      setBusy(false);
    }
  }
  return (
    <Panel title="Темы интересов" description="Вес темы влияет на предварительную оценку и передаётся модели">
      {!topics.data ? (
        <Skeleton className="h-40" />
      ) : (
        <ul className="max-h-[340px] divide-y divide-line/70 overflow-y-auto pr-1">
          {topics.data.items.map((t) => (
            <li key={t.code} className="flex items-center justify-between gap-3 py-2">
              <span className="text-[14px]">{t.name}</span>
              <div className="flex shrink-0 gap-1" role="radiogroup" aria-label={`Важность темы ${t.name}`}>
                {weights.map((x) => (
                  <button
                    key={x.v}
                    role="radio"
                    aria-checked={(w[t.code] ?? 0) === x.v}
                    title={x.l}
                    onClick={() => setW({ ...w, [t.code]: x.v })}
                    className={cn(
                      "size-6 rounded-full border text-[11px] tabular transition-colors",
                      (w[t.code] ?? 0) === x.v ? "border-steel bg-steel text-white" : "border-line bg-white/60 text-silver hover:border-glacier",
                    )}
                  >
                    {x.v}
                  </button>
                ))}
              </div>
            </li>
          ))}
        </ul>
      )}
      <div className="mt-4 flex items-center justify-between">
        <p className="text-[12px] text-silver">0 — не важно, 3 — очень важно</p>
        <Button onClick={save} loading={busy}>
          Сохранить интересы
        </Button>
      </div>
    </Panel>
  );
}

function PersonalizationForm() {
  const { me, reload } = useMe();
  const [p, setP] = useState<Personalization>(me.personalization);
  const [busy, setBusy] = useState(false);
  async function save() {
    setBusy(true);
    try {
      await api("/me/personalization", { method: "PUT", body: p });
      toast.success("Настройки персонализации сохранены");
      reload();
    } catch (e) {
      toast.error(errorMessage(e));
    } finally {
      setBusy(false);
    }
  }
  return (
    <Panel title="Персонализация анализа">
      <div className="grid gap-4 sm:grid-cols-2">
        <Field id="p-h" label="Горизонт планирования">
          <Select value={p.horizon} onChange={(e) => setP({ ...p, horizon: e.target.value as Personalization["horizon"] })}>
            <option value="short">До 3 месяцев</option>
            <option value="medium">3–12 месяцев</option>
            <option value="long">Больше года</option>
          </Select>
        </Field>
        <Field id="p-d" label="Детализация анализа">
          <Select value={p.detail} onChange={(e) => setP({ ...p, detail: e.target.value as Personalization["detail"] })}>
            <option value="detailed">Подробно</option>
            <option value="brief">Кратко</option>
          </Select>
        </Field>
      </div>
      <label className="mt-4 flex items-start gap-3 text-[14px]">
        <input
          type="checkbox"
          className="mt-0.5 size-4 accent-steel"
          checked={p.include_research}
          onChange={(e) => setP({ ...p, include_research: e.target.checked })}
        />
        <span>
          Поднимать в ленте материалы аналитиков Альфа-Банка
          <span className="block text-[12px] text-silver">Если темы материала совпадают с вашим профилем</span>
        </span>
      </label>
      <div className="mt-4 flex justify-end">
        <Button onClick={save} loading={busy}>
          Сохранить
        </Button>
      </div>
    </Panel>
  );
}

type Status = {
  llm: { configured: boolean; host: string; guard: GuardStatus };
  models: { role: string; id: string; allowed: boolean; title: string }[];
  dataset: { companies: number; large_companies: number; person_links: number; payments: number };
  import_issues: { file_name: string; kind: string; issues: number }[];
  sources: { id: number; code: string; name: string; kind: string; enabled: boolean; last_fetch_at: string | null; last_fetch_status: string | null }[];
  notes: string[];
};

const modelRole: Record<string, string> = { fast: "Классификация и оценка", reasoning: "План и самопроверка", writer: "Персональный анализ" };

function SystemStatus() {
  const { data, error, loading, reload } = useApi<Status>("/system/status");
  return (
    <Panel title="Интеграции и лимиты" description="Состояние подключения модели, источников и импортированных данных" className="mt-6">
      {error ? (
        <ErrorState error={error} onRetry={reload} />
      ) : loading || !data ? (
        <Skeleton className="h-40" />
      ) : (
        <div className="grid gap-8 lg:grid-cols-3">
          <div>
            <p className="text-sm font-medium">Языковая модель</p>
            <p className={cn("mt-1 text-[13px]", data.llm.configured ? "text-chance" : "text-risk")}>
              {data.llm.configured ? `Подключена: ${data.llm.host}` : "Не подключена — задайте LLM_BASE_URL и LLM_API_KEY"}
            </p>
            <ul className="mt-3 space-y-1.5 text-[13px]">
              {[...data.models].sort((a, b) => a.role.localeCompare(b.role)).map((m) => (
                <li key={m.role}>
                  <span className="text-ink-soft">{modelRole[m.role]}:</span> {m.id || "не задана"}{" "}
                  {m.id && (m.allowed ? <Badge tone="chance">разрешена</Badge> : <Badge tone="risk">не из списка кейса</Badge>)}
                </li>
              ))}
            </ul>
            <p className="mt-3 text-[13px] text-ink-soft tabular">
              Сегодня: {data.llm.guard.calls} из {data.llm.guard.daily_call_limit} запросов,{" "}
              {data.llm.guard.tokens_used.toLocaleString("ru-RU")} из {data.llm.guard.daily_token_budget.toLocaleString("ru-RU")} токенов
            </p>
            {data.llm.guard.paused_until && (
              <p className="mt-1 text-[13px] text-risk">
                Пауза до {formatDate(data.llm.guard.paused_until)}: {data.llm.guard.pause_reason}
              </p>
            )}
          </div>
          <div>
            <p className="text-sm font-medium">Источники новостей</p>
            <ul className="mt-2 space-y-2 text-[13px]">
              {data.sources.map((s) => (
                <li key={s.id}>
                  <span className="text-ink">{s.name}</span>
                  <span className={cn("block", s.last_fetch_status?.startsWith("ошибка") ? "text-risk" : "text-silver")}>
                    {s.kind === "rss"
                      ? s.last_fetch_at
                        ? `${formatDate(s.last_fetch_at)}: ${s.last_fetch_status}`
                        : "ещё не опрашивался"
                      : s.kind === "alfa_research"
                        ? "материалы кейса, загружены импортом"
                        : "новости, добавленные пользователями"}
                  </span>
                </li>
              ))}
            </ul>
          </div>
          <div>
            <p className="text-sm font-medium">Данные кейса</p>
            <dl className="mt-2 grid grid-cols-2 gap-3 text-[13px]">
              <div>
                <dt className="text-silver">Компаний</dt>
                <dd className="text-[18px] font-semibold tabular">{data.dataset.companies.toLocaleString("ru-RU")}</dd>
              </div>
              <div>
                <dt className="text-silver">Из них от 15 млрд ₽</dt>
                <dd className="text-[18px] font-semibold tabular">{data.dataset.large_companies}</dd>
              </div>
              <div>
                <dt className="text-silver">Связей с физлицами</dt>
                <dd className="text-[18px] font-semibold tabular">{data.dataset.person_links.toLocaleString("ru-RU")}</dd>
              </div>
              <div>
                <dt className="text-silver">Платёжных связей</dt>
                <dd className="text-[18px] font-semibold tabular">{data.dataset.payments.toLocaleString("ru-RU")}</dd>
              </div>
            </dl>
            {data.import_issues.length > 0 && (
              <ul className="mt-3 space-y-1 text-[12px] text-silver">
                {data.import_issues.map((i) => (
                  <li key={i.file_name + i.kind}>
                    {i.file_name}: {issueLabel[i.kind] ?? i.kind} ({i.issues})
                  </li>
                ))}
              </ul>
            )}
          </div>
          <ul className="space-y-1 text-[12px] text-silver lg:col-span-3">
            {data.notes.map((n) => (
              <li key={n}>{n}</li>
            ))}
          </ul>
        </div>
      )}
    </Panel>
  );
}

const issueLabel: Record<string, string> = {
  duplicate_id_conflicting: "повтор ID с разными данными, строки исключены",
  duplicate_id_identical: "полные дубликаты схлопнуты",
  unknown_company_reference: "ссылки на компании вне справочника сохранены",
  duplicate_pair: "повтор пары плательщик–получатель",
  self_payments: "платежи компании самой себе, исключены из анализа контрагентов",
  invalid_row: "некорректные строки пропущены",
};
