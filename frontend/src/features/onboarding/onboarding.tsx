"use client";

import { useEffect, useMemo, useState } from "react";
import { useRouter } from "next/navigation";
import { AnimatePresence, motion } from "motion/react";
import { Check, Search } from "lucide-react";
import { toast } from "sonner";
import { Logo } from "@/components/brand";
import { Stepper } from "@/components/stepper";
import { Button } from "@/components/ui/button";
import { Field, Input, Select, Textarea } from "@/components/ui/field";
import { Badge, ErrorState, Skeleton } from "@/components/ui/misc";
import { api, ApiError, errorMessage } from "@/lib/api";
import { formatRub } from "@/lib/format";
import type { CompanyProfile, CompanySearchItem, Me, Role, Topic } from "@/lib/types";
import { useApi } from "@/lib/use-api";
import { cn } from "@/lib/utils";
import { CompanyFacts } from "@/features/company/facts";
import { BusinessFields, cleanBusiness } from "@/features/company/business-fields";
import type { BusinessProfile } from "@/lib/types";

type UserForm = { full_name: string; role_code: string; position_title: string; work_focus: string; topics: string[] };
type CompanyForm = { display_name: string; description: string; products: string; markets: string; suppliers: string; priorities: string; business: BusinessProfile };

export function Onboarding() {
  const router = useRouter();
  const me = useApi<Me>("/me");
  const roles = useApi<{ items: Role[] }>("/roles");
  const topics = useApi<{ items: Topic[] }>("/topics");
  const [step, setStep] = useState(0);
  const [user, setUser] = useState<UserForm | null>(null);
  const [company, setCompany] = useState<CompanyProfile | null>(null);

  useEffect(() => {
    if (!me.data || user) return;
    setUser({
      full_name: me.data.full_name,
      role_code: me.data.role_code,
      position_title: me.data.position_title,
      work_focus: me.data.work_focus,
      topics: me.data.interests.map((i) => i.topic_code),
    });
    const primary = me.data.companies.find((c) => c.is_primary) ?? me.data.companies[0];
    if (primary) api<CompanyProfile>(`/companies/${primary.id}`).then(setCompany).catch(() => undefined);
  }, [me.data, user]);

  const error = me.error || roles.error || topics.error;
  return (
    <div className="relative min-h-dvh">
      <div aria-hidden className="app-backdrop fixed inset-0" />
      <div className="relative mx-auto max-w-[920px] px-4 py-6 sm:px-8">
        <Logo href="/onboarding" />
        <div className="mt-10">
          <h1 className="display text-[34px] sm:text-[44px]">Настройка профиля</h1>
          <p className="mt-2 max-w-[60ch] text-ink-soft">
            От этих данных зависит, какие новости попадут в ленту и как будет построен анализ.
          </p>
          <Stepper current={step} className="mt-8 max-w-[560px]" />
        </div>
        <div className="mt-8 pb-16">
          {error ? (
            <ErrorState error={error} onRetry={() => { me.reload(); roles.reload(); topics.reload(); }} />
          ) : !user || !roles.data || !topics.data ? (
            <div className="glass space-y-4 rounded-[var(--radius-xl)] p-8">
              <Skeleton className="h-6 w-1/3" />
              <Skeleton className="h-11 w-full" />
              <Skeleton className="h-28 w-full" />
            </div>
          ) : (
            <AnimatePresence mode="wait">
              <motion.div
                key={step}
                initial={{ opacity: 0, y: 8 }}
                animate={{ opacity: 1, y: 0 }}
                exit={{ opacity: 0, y: -8 }}
                transition={{ duration: 0.2 }}
              >
                {step === 0 && (
                  <UserStep value={user} roles={roles.data.items} topics={topics.data.items} onDone={(v) => { setUser(v); setStep(1); }} />
                )}
                {step === 1 && (
                  <CompanyStep initial={company} onBack={() => setStep(0)} onDone={(c) => { setCompany(c); setStep(2); }} />
                )}
                {step === 2 && company && (
                  <ReviewStep
                    user={user}
                    roles={roles.data.items}
                    topics={topics.data.items}
                    company={company}
                    onEdit={setStep}
                    onDone={() => router.replace("/home")}
                  />
                )}
              </motion.div>
            </AnimatePresence>
          )}
        </div>
      </div>
    </div>
  );
}

function UserStep({ value, roles, topics, onDone }: { value: UserForm; roles: Role[]; topics: Topic[]; onDone: (v: UserForm) => void }) {
  const [form, setForm] = useState(value);
  const [errors, setErrors] = useState<Record<string, string>>({});
  const [saving, setSaving] = useState(false);
  const role = roles.find((r) => r.code === form.role_code);

  async function save() {
    const e: Record<string, string> = {};
    if (form.full_name.trim().length < 2) e.full_name = "Укажите имя";
    if (!form.role_code) e.role_code = "Выберите роль";
    if (form.role_code === "other" && form.position_title.trim().length < 2) e.position_title = "Опишите вашу роль";
    setErrors(e);
    if (Object.keys(e).length) return;
    setSaving(true);
    try {
      await api("/me/profile", {
        method: "PUT",
        body: {
          full_name: form.full_name,
          role_code: form.role_code,
          position_title: form.role_code === "other" ? form.position_title : "",
          work_focus: form.work_focus,
        },
      });
      await api("/me/interests", { method: "PUT", body: { topics: form.topics.map((code) => ({ code, weight: 2 })) } });
      onDone(form);
    } catch (err) {
      if (err instanceof ApiError && err.fields) setErrors(err.fields);
      else toast.error(errorMessage(err));
    } finally {
      setSaving(false);
    }
  }

  const toggle = (code: string) =>
    setForm({ ...form, topics: form.topics.includes(code) ? form.topics.filter((t) => t !== code) : [...form.topics, code] });

  return (
    <div className="glass rounded-[var(--radius-xl)] p-6 sm:p-8">
      <h2 className="text-[22px] font-semibold tracking-[-0.02em]">Кто вы в компании</h2>
      <p className="mt-1 text-sm text-ink-soft">Роль определяет, через какую линзу модель смотрит на каждую новость.</p>
      <Field id="full_name" label="Имя и фамилия" error={errors.full_name} required className="mt-6 max-w-[420px]">
        <Input value={form.full_name} onChange={(e) => setForm({ ...form, full_name: e.target.value })} />
      </Field>
      <fieldset className="mt-6">
        <legend className="text-[13px] font-medium text-ink-soft">
          Роль в принятии решений<span className="ml-0.5 text-steel" aria-hidden>*</span>
        </legend>
        {errors.role_code && <p className="mt-1 text-[13px] text-risk">{errors.role_code}</p>}
        <div className="mt-2 grid gap-2 sm:grid-cols-3">
          {roles.map((r) => (
            <label
              key={r.code}
              className={cn(
                "cursor-pointer rounded-[12px] border px-3.5 py-3 text-sm transition-colors has-[:focus-visible]:ring-2 has-[:focus-visible]:ring-steel",
                form.role_code === r.code ? "border-steel bg-white text-ink" : "border-line bg-white/50 text-ink-soft hover:border-glacier",
              )}
            >
              <input type="radio" name="role" className="sr-only" checked={form.role_code === r.code} onChange={() => setForm({ ...form, role_code: r.code })} />
              <span className="font-medium">{r.title}</span>
            </label>
          ))}
        </div>
        {form.role_code === "other" ? (
          <Field id="position_title" label="Ваша роль в компании" hint="Например: директор по ВЭД, руководитель проектного офиса" error={errors.position_title} required className="mt-4 max-w-[520px]">
            <Input value={form.position_title} onChange={(e) => setForm({ ...form, position_title: e.target.value })} />
          </Field>
        ) : (
          role && <p className="mt-3 text-[13px] text-ink-soft">В анализе для этой роли важны: {role.focus.join(", ")}.</p>
        )}
      </fieldset>
      <Field id="work_focus" label="Рабочие интересы" hint="Например: рефинансирование кредитного портфеля, закупки сырья в Китае" className="mt-6">
        <Textarea value={form.work_focus} onChange={(e) => setForm({ ...form, work_focus: e.target.value })} maxLength={1000} />
      </Field>
      <fieldset className="mt-6">
        <legend className="text-[13px] font-medium text-ink-soft">Темы, которые вы отслеживаете</legend>
        <div className="mt-2 flex flex-wrap gap-2">
          {topics.map((t) => {
            const on = form.topics.includes(t.code);
            return (
              <button
                type="button"
                key={t.code}
                aria-pressed={on}
                onClick={() => toggle(t.code)}
                title={t.description}
                className={cn(
                  "inline-flex items-center gap-1.5 rounded-full border px-3 py-1.5 text-[13px] transition-colors",
                  on ? "border-steel bg-steel text-white" : "border-line bg-white/60 text-ink-soft hover:border-glacier",
                )}
              >
                {on && <Check className="size-3.5" aria-hidden />}
                {t.name}
              </button>
            );
          })}
        </div>
      </fieldset>
      <div className="mt-8 flex justify-end">
        <Button size="lg" onClick={save} loading={saving}>
          Сохранить и продолжить
        </Button>
      </div>
    </div>
  );
}

function CompanyStep({ initial, onBack, onDone }: { initial: CompanyProfile | null; onBack: () => void; onDone: (c: CompanyProfile) => void }) {
  const [q, setQ] = useState("");
  const [region, setRegion] = useState("");
  const [largeOnly, setLargeOnly] = useState(true);
  const [clientsOnly, setClientsOnly] = useState(true);
  const [selected, setSelected] = useState<string | null>(initial?.id ?? null);
  const [preview, setPreview] = useState<CompanyProfile | null>(initial);
  const [form, setForm] = useState<CompanyForm>(() => toForm(initial));
  const [saving, setSaving] = useState(false);
  const regions = useApi<{ items: { region: string; companies: number }[] }>("/companies/regions");
  const query = useMemo(() => {
    const p = new URLSearchParams({ limit: "12" });
    if (q.trim()) p.set("q", q.trim());
    if (region) p.set("region", region);
    if (largeOnly) p.set("min_revenue", "15000000000");
    if (clientsOnly) p.set("clients_only", "true");
    return `/companies/search?${p}`;
  }, [q, region, largeOnly, clientsOnly]);
  const [debounced, setDebounced] = useState(query);
  useEffect(() => {
    const t = setTimeout(() => setDebounced(query), 300);
    return () => clearTimeout(t);
  }, [query]);
  const results = useApi<{ items: CompanySearchItem[] }>(debounced);

  async function pick(id: string) {
    setSelected(id);
    setPreview(null);
    try {
      const p = await api<CompanyProfile>(`/companies/${id}/preview`);
      setPreview(p);
      setForm(toForm(p));
    } catch (e) {
      toast.error(errorMessage(e));
    }
  }

  const [errors, setErrors] = useState<Record<string, string>>({});
  async function save() {
    if (!selected) return;
    const fe: Record<string, string> = {};
    if (!form.display_name.trim()) fe.display_name = "Обязательное поле";
    if (!form.products.trim()) fe.products = "Обязательное поле";
    if (!form.markets.trim()) fe.markets = "Обязательное поле";
    if (!form.suppliers.trim()) fe.suppliers = "Обязательное поле";
    setErrors(fe);
    if (Object.keys(fe).length) {
      toast.error("Заполните обязательные поля");
      return;
    }
    setSaving(true);
    try {
      const p = await api<CompanyProfile>("/me/companies", {
        method: "POST",
        body: {
          company_id: selected,
          display_name: form.display_name,
          profile: {
            description: form.description,
            products: form.products,
            markets: form.markets,
            suppliers: form.suppliers,
            priorities: form.priorities.split("\n").map((s) => s.trim()).filter(Boolean),
            business: cleanBusiness(form.business),
          },
        },
      });
      onDone(p);
    } catch (e) {
      if (e instanceof ApiError && e.fields) setErrors(e.fields);
      toast.error(errorMessage(e));
    } finally {
      setSaving(false);
    }
  }

  return (
    <div className="grid gap-5 lg:grid-cols-[minmax(0,1fr)_minmax(0,1.1fr)]">
      <div className="glass rounded-[var(--radius-xl)] p-6">
        <h2 className="text-[22px] font-semibold tracking-[-0.02em]">Ваша компания</h2>
        <p className="mt-1 text-sm text-ink-soft">
          Данные банка обезличены, поэтому компанию выбирают по отрасли, региону и масштабу.
        </p>
        <div className="relative mt-5">
          <Search className="pointer-events-none absolute left-3.5 top-1/2 size-4 -translate-y-1/2 text-silver" aria-hidden />
          <Input aria-label="Поиск по отрасли или ОКВЭД" className="pl-10" value={q} onChange={(e) => setQ(e.target.value)} placeholder="Отрасль, код ОКВЭД или ID" />
        </div>
        <div className="mt-3 grid grid-cols-[1fr_auto] items-center gap-3">
          <Select aria-label="Регион" value={region} onChange={(e) => setRegion(e.target.value)}>
            <option value="">Все регионы</option>
            {regions.data?.items.map((r) => (
              <option key={r.region} value={r.region}>
                {r.region}
              </option>
            ))}
          </Select>
          <div className="flex flex-col gap-1.5">
            <label className="flex items-center gap-2 text-[13px] text-ink-soft" title="Сегмент определяется по выручке группы компаний">
              <input type="checkbox" checked={largeOnly} onChange={(e) => setLargeOnly(e.target.checked)} className="size-4 accent-steel" />
              Группа от 15 млрд ₽
            </label>
            <label className="flex items-center gap-2 text-[13px] text-ink-soft">
              <input type="checkbox" checked={clientsOnly} onChange={(e) => setClientsOnly(e.target.checked)} className="size-4 accent-steel" />
              Только клиенты банка
            </label>
          </div>
        </div>
        <div className="mt-4 max-h-[440px] space-y-1.5 overflow-y-auto pr-1" aria-live="polite">
          {results.loading && !results.data ? (
            Array.from({ length: 4 }).map((_, i) => <Skeleton key={i} className="h-16" />)
          ) : results.error ? (
            <ErrorState error={results.error} onRetry={results.reload} />
          ) : results.data?.items.length === 0 ? (
            <p className="py-6 text-center text-sm text-ink-soft">Ничего не найдено. Измените запрос или снимите фильтры. Если список пуст при любых условиях — данные компаний ещё не загружены (команда importer).</p>
          ) : (
            results.data?.items.map((c) => (
              <button
                key={c.id}
                onClick={() => pick(c.id)}
                className={cn(
                  "w-full rounded-[12px] border px-3.5 py-2.5 text-left transition-colors",
                  selected === c.id ? "border-steel bg-white" : "border-transparent bg-white/50 hover:border-line hover:bg-white/80",
                )}
              >
                <span className="line-clamp-1 text-sm font-medium text-ink">{c.display_name || c.okved_name}</span>
                <span className="mt-0.5 block text-[12px] text-silver">
                  {c.id}, ОКВЭД {c.okved_code}, {c.region ?? "регион не указан"}, выручка {formatRub(c.revenue)}
                  {c.group_size && c.group_size > 1 ? `, группа ${formatRub(c.group_revenue)}` : ""}
                </span>
              </button>
            ))
          )}
        </div>
      </div>

      <div className="glass rounded-[var(--radius-xl)] p-6">
        {!selected ? (
          <div className="flex h-full min-h-[300px] items-center justify-center text-center text-sm text-ink-soft">
            Выберите компанию в списке — здесь появятся данные, которые уже известны банку.
          </div>
        ) : !preview ? (
          <div className="space-y-3">
            <Skeleton className="h-6 w-1/2" />
            <Skeleton className="h-40" />
          </div>
        ) : (
          <div>
            <div className="flex flex-wrap items-center gap-2">
              <h3 className="text-[18px] font-semibold">{preview.okved_name}</h3>
              {preview.is_large && <Badge tone="bank">крупный бизнес</Badge>}
            </div>
            <p className="mt-1 text-[13px] text-silver">
              Эти данные уже известны из файлов банка — повторно их вводить не нужно.
            </p>
            <CompanyFacts facts={preview.facts} compact className="mt-4" />
            <div className="mt-6 border-t border-line pt-5">
              <p className="text-sm font-medium">Что знаете только вы</p>
              <p className="text-[13px] text-silver">Так анализ будет точнее. Исходные данные банка не изменятся.</p>
              <div className="mt-4 grid gap-4">
                <Field id="display_name" label="Как называть компанию" error={errors.display_name} required>
                  <Input value={form.display_name} onChange={(e) => setForm({ ...form, display_name: e.target.value })} placeholder="Например: ООО «Северная логистика»" />
                </Field>
                <Field id="products" label="Продукты и услуги" error={errors.products} required>
                  <Input value={form.products} onChange={(e) => setForm({ ...form, products: e.target.value })} />
                </Field>
                <div className="grid items-start gap-4 sm:grid-cols-2">
                  <Field id="markets" label="Рынки и клиенты" error={errors.markets} required>
                    <Input value={form.markets} onChange={(e) => setForm({ ...form, markets: e.target.value })} />
                  </Field>
                  <Field id="suppliers" label="Поставщики и сырьё" error={errors.suppliers} required>
                    <Input value={form.suppliers} onChange={(e) => setForm({ ...form, suppliers: e.target.value })} />
                  </Field>
                </div>
                <Field id="priorities" label="Приоритеты бизнеса" error={errors.priorities}>
                  <Textarea value={form.priorities} onChange={(e) => setForm({ ...form, priorities: e.target.value })} placeholder="Необязательно" />
                </Field>
                <BusinessFields value={form.business} onChange={(b) => setForm({ ...form, business: b })} errors={errors} />
              </div>
            </div>
          </div>
        )}
      </div>

      <div className="flex justify-between lg:col-span-2">
        <Button variant="ghost" onClick={onBack}>
          Назад
        </Button>
        <Button size="lg" onClick={save} loading={saving} disabled={!selected || !preview}>
          Сохранить и продолжить
        </Button>
      </div>
    </div>
  );
}

function toForm(p: CompanyProfile | null): CompanyForm {
  const u = p?.user_profile ?? {};
  return {
    display_name: p?.display_name ?? "",
    description: u.description ?? "",
    products: u.products ?? "",
    markets: u.markets ?? "",
    suppliers: u.suppliers ?? "",
    priorities: (u.priorities ?? []).join("\n"),
    business: u.business ?? {},
  };
}

function ReviewStep({
  user,
  roles,
  topics,
  company,
  onEdit,
  onDone,
}: {
  user: UserForm;
  roles: Role[];
  topics: Topic[];
  company: CompanyProfile;
  onEdit: (s: number) => void;
  onDone: () => void;
}) {
  const [saving, setSaving] = useState(false);
  const role = roles.find((r) => r.code === user.role_code);
  async function finish() {
    setSaving(true);
    try {
      await api("/me/onboarding/complete", { method: "POST" });
      toast.success("Профиль настроен");
      onDone();
    } catch (e) {
      toast.error(errorMessage(e));
      setSaving(false);
    }
  }
  return (
    <div className="glass rounded-[var(--radius-xl)] p-6 sm:p-8">
      <h2 className="text-[22px] font-semibold tracking-[-0.02em]">Проверьте данные</h2>
      <p className="mt-1 text-sm text-ink-soft">Всё можно изменить позже в настройках и профиле компании.</p>
      <dl className="mt-6 grid gap-6 sm:grid-cols-2">
        <div>
          <div className="flex items-center justify-between">
            <dt className="text-sm font-medium">Пользователь</dt>
            <Button variant="link" size="sm" onClick={() => onEdit(0)}>
              Изменить
            </Button>
          </div>
          <dd className="mt-2 space-y-1 text-[15px] text-ink-soft">
            <p className="text-ink">{user.full_name}</p>
            <p>
              {role?.title}
              {user.position_title && user.position_title !== role?.title ? `, ${user.position_title}` : ""}
            </p>
            {user.work_focus && <p>{user.work_focus}</p>}
            <div className="flex flex-wrap gap-1.5 pt-1">
              {user.topics.length === 0 ? (
                <span className="text-[13px] text-silver">Темы не выбраны</span>
              ) : (
                user.topics.map((c) => <Badge key={c}>{topics.find((t) => t.code === c)?.name ?? c}</Badge>)
              )}
            </div>
          </dd>
        </div>
        <div>
          <div className="flex items-center justify-between">
            <dt className="text-sm font-medium">Компания</dt>
            <Button variant="link" size="sm" onClick={() => onEdit(1)}>
              Изменить
            </Button>
          </div>
          <dd className="mt-2 space-y-1 text-[15px] text-ink-soft">
            <p className="text-ink">{company.display_name || company.okved_name}</p>
            <p>
              ОКВЭД {company.okved_code}, {company.region}
            </p>
            <p>{company.size_segment}</p>
            {company.exposures.length > 0 && (
              <p className="pt-1 text-[13px]">Чувствительность: {company.exposures.map((e) => e.title.toLowerCase()).join(", ")}</p>
            )}
          </dd>
        </div>
      </dl>
      {company.data_gaps.length > 0 && (
        <div className="mt-6 rounded-[12px] bg-mixed-bg px-4 py-3 text-[13px] text-mixed">
          Не хватает данных: {company.data_gaps.join("; ")}. Анализ отметит связанные выводы как предположения.
        </div>
      )}
      <div className="mt-8 flex justify-between">
        <Button variant="ghost" onClick={() => onEdit(1)}>
          Назад
        </Button>
        <Button size="lg" onClick={finish} loading={saving}>
          Завершить и открыть ленту
        </Button>
      </div>
    </div>
  );
}
