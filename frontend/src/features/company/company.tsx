"use client";

import { useEffect, useState } from "react";
import { toast } from "sonner";
import { useMe } from "@/components/app-shell";
import { Button } from "@/components/ui/button";
import { Field, Input, Textarea } from "@/components/ui/field";
import { Badge, EmptyState, ErrorState, Panel, Skeleton } from "@/components/ui/misc";
import { api, ApiError, errorMessage } from "@/lib/api";
import { formatRub } from "@/lib/format";
import type { BusinessProfile, CompanyProfile, Counterparty, Industry } from "@/lib/types";
import { BusinessFields, cleanBusiness } from "./business-fields";
import { useApi } from "@/lib/use-api";
import { CompanyFacts } from "./facts";

export function CompanyTab() {
  const { me, reload: reloadMe } = useMe();
  const company = me.companies.find((c) => c.is_primary) ?? me.companies[0];
  const { data, error, loading, reload, mutate } = useApi<CompanyProfile>(company ? `/companies/${company.id}` : null);

  if (!company) return <EmptyState title="Компания не выбрана">Выберите компанию в настройке профиля.</EmptyState>;
  if (error) return <ErrorState error={error} onRetry={reload} />;
  if (loading || !data)
    return (
      <div className="space-y-4">
        <Skeleton className="h-12 w-1/2" />
        <Skeleton className="h-72" />
      </div>
    );
  const r = data.relations;
  return (
    <div>
      <div className="glass-night mb-6 rounded-[var(--radius-xl)] p-6 sm:p-7">
        <p className="text-[13px] text-white/60">
          {data.id}, раздел {data.section.letter}, {data.region}
        </p>
        <h2 className="display mt-1 text-[26px] sm:text-[32px]">{data.display_name || data.okved_name}</h2>
        <p className="mt-1 text-[15px] text-white/75">{data.okved_code} — {data.okved_name}</p>
        <div className="mt-5 flex flex-wrap gap-2">
          <span className="glass-night-card rounded-full px-3 py-1 text-[13px]">{data.size_segment}</span>
          {data.revenue != null && <span className="glass-night-card rounded-full px-3 py-1 text-[13px]">Выручка {formatRub(data.revenue)}</span>}
          {data.group_size != null && data.group_size > 1 && (
            <span className="glass-night-card rounded-full px-3 py-1 text-[13px]">
              Группа: {data.group_size} компаний, {formatRub(data.group_revenue)}
            </span>
          )}
        </div>
      </div>
      <div className="grid gap-6 xl:grid-cols-[minmax(0,1.3fr)_minmax(0,1fr)]">
        <div className="space-y-6">
          <Panel title="Чувствительность к внешним событиям" description="Выведено из фактических полей данных банка">
            {data.exposures.length === 0 ? (
              <p className="text-sm text-ink-soft">По данным банка выраженных чувствительностей (кредиты, лизинг, ВЭД и др.) не найдено.</p>
            ) : (
              <ul className="grid gap-4 sm:grid-cols-2">
                {data.exposures.map((e) => (
                  <li key={e.code}>
                    <p className="text-[15px] font-semibold">{e.title}</p>
                    <p className="mt-1 text-[14px] leading-relaxed text-ink-soft">{e.explanation}</p>
                    <p className="mt-1 text-[12px] text-silver">Основание: {e.fact_ids.join(", ")}</p>
                  </li>
                ))}
              </ul>
            )}
            {data.data_gaps.length > 0 && (
              <div className="mt-5 rounded-[12px] bg-mixed-bg px-4 py-3 text-[13px] text-mixed">Не хватает данных: {data.data_gaps.join("; ")}.</div>
            )}
          </Panel>

          {r && (
            <Panel title="Связи компании" description="По агрегированным платежам между компаниями и общим обезличенным физлицам">
              {r.supplier_count + r.customer_count === 0 ? (
                <p className="text-sm text-ink-soft">В файле платежей нет операций этой компании с другими компаниями.</p>
              ) : (
                <>
                  <dl className="grid grid-cols-2 gap-4 sm:grid-cols-4">
                    {[
                      ["Получателей платежей", r.supplier_count],
                      ["Исходящие", formatRub(r.outgoing_amount)],
                      ["Плательщиков", r.customer_count],
                      ["Входящие", formatRub(r.incoming_amount)],
                    ].map(([l, v]) => (
                      <div key={l as string}>
                        <dt className="text-[12px] text-silver">{l}</dt>
                        <dd className="text-[20px] font-semibold tracking-[-0.02em] tabular">{v}</dd>
                      </div>
                    ))}
                  </dl>
                  <div className="mt-6 grid gap-6 sm:grid-cols-2">
                    <IndustryList title="Кому компания платит" items={r.industries.filter((i) => i.direction === "outgoing")} />
                    <IndustryList title="Кто платит компании" items={r.industries.filter((i) => i.direction === "incoming")} />
                  </div>
                  <div className="mt-6 grid gap-6 sm:grid-cols-2">
                    <CpTable title="Крупнейшие получатели" items={r.top_recipients} />
                    <CpTable title="Крупнейшие плательщики" items={r.top_senders} />
                  </div>
                </>
              )}
              <div className="mt-6 grid gap-6 border-t border-line pt-5 sm:grid-cols-2">
                <div>
                  <p className="text-sm font-medium">Группа компаний</p>
                  <p className="mt-1 text-[13px] text-ink-soft">
                    {r.group_size > 1 ? `${r.group_size} компаний в группе ${r.group_id}` : "Других компаний группы в данных нет"}
                  </p>
                  {r.group_members.slice(0, 5).map((m) => (
                    <p key={m.id} className="mt-1 text-[13px] text-silver">
                      {m.id}: {m.okved_name}
                    </p>
                  ))}
                </div>
                <div>
                  <p className="text-sm font-medium">Связи через физлиц</p>
                  <p className="mt-1 text-[13px] text-ink-soft">
                    {r.linked_persons > 0 ? `Связанных обезличенных физлиц: ${r.linked_persons}` : "Связанных физлиц в данных нет"}
                  </p>
                  {r.companies_sharing_persons.slice(0, 5).map((m) => (
                    <p key={m.id} className="mt-1 text-[13px] text-silver">
                      {m.id}
                      {m.okved_name ? `: ${m.okved_name}` : " (нет в справочнике компаний)"}
                    </p>
                  ))}
                </div>
              </div>
            </Panel>
          )}

          <Panel title="Данные банка о компании" description="Исходные значения и поля, из которых они взяты">
            <CompanyFacts facts={data.facts} />
          </Panel>
        </div>
        <EditProfile data={data} onSaved={(p) => { mutate(() => p); reloadMe(); }} />
      </div>
    </div>
  );
}

function IndustryList({ title, items }: { title: string; items: Industry[] }) {
  return (
    <div>
      <p className="text-sm font-medium">{title}</p>
      {items.length === 0 ? (
        <p className="mt-1 text-[13px] text-silver">Контрагенты не найдены в справочнике.</p>
      ) : (
        <ul className="mt-2 space-y-2">
          {items.slice(0, 5).map((i) => (
            <li key={i.division}>
              <div className="flex justify-between gap-3 text-[13px]">
                <span className="line-clamp-1 text-ink-soft" title={i.sample_name}>
                  {i.division}: {i.sample_name}
                </span>
                <span className="shrink-0 text-ink tabular">{Math.round(i.share * 100)}%</span>
              </div>
              <div className="mt-1 h-1.5 rounded-full bg-frost">
                <div className="h-1.5 rounded-full bg-steel" style={{ width: `${Math.max(2, i.share * 100)}%` }} />
              </div>
            </li>
          ))}
        </ul>
      )}
    </div>
  );
}

function CpTable({ title, items }: { title: string; items: Counterparty[] }) {
  return (
    <div>
      <p className="text-sm font-medium">{title}</p>
      <table className="mt-2 w-full text-[13px]">
        <tbody className="divide-y divide-line/70">
          {items.slice(0, 5).map((c) => (
            <tr key={c.id}>
              <td className="py-1.5 pr-2">
                <span className="text-ink">{c.id}</span>
                <span className="line-clamp-1 text-silver">{c.known ? c.okved_name : "нет в справочнике компаний"}</span>
              </td>
              <td className="py-1.5 text-right tabular">{formatRub(c.amount)}</td>
            </tr>
          ))}
        </tbody>
      </table>
    </div>
  );
}

function EditProfile({ data, onSaved }: { data: CompanyProfile; onSaved: (p: CompanyProfile) => void }) {
  const init = () => ({
    display_name: data.display_name ?? "",
    description: data.user_profile.description ?? "",
    products: data.user_profile.products ?? "",
    markets: data.user_profile.markets ?? "",
    suppliers: data.user_profile.suppliers ?? "",
    priorities: (data.user_profile.priorities ?? []).join("\n"),
    notes: data.user_profile.notes ?? "",
    business: data.user_profile.business ?? ({} as BusinessProfile),
  });
  const [form, setForm] = useState(init);
  const [busy, setBusy] = useState(false);
  const [errors, setErrors] = useState<Record<string, string>>({});
  useEffect(() => setForm(init()), [data]); // eslint-disable-line react-hooks/exhaustive-deps
  type K = Exclude<keyof ReturnType<typeof init>, "business">;
  const set = (k: K) => (e: React.ChangeEvent<HTMLInputElement | HTMLTextAreaElement>) => setForm({ ...form, [k]: e.target.value });
  async function save(e: React.FormEvent) {
    e.preventDefault();
    const fe: Record<string, string> = {};
    (["display_name", "products", "markets", "suppliers"] as const).forEach((k) => {
      if (!form[k].trim()) fe[k] = "Обязательное поле";
    });
    setErrors(fe);
    if (Object.keys(fe).length) {
      toast.error("Заполните обязательные поля");
      return;
    }
    setBusy(true);
    try {
      const p = await api<CompanyProfile>(`/companies/${data.id}`, {
        method: "PUT",
        body: {
          display_name: form.display_name,
          profile: {
            description: form.description,
            products: form.products,
            markets: form.markets,
            suppliers: form.suppliers,
            notes: form.notes,
            priorities: form.priorities.split("\n").map((s) => s.trim()).filter(Boolean),
            business: cleanBusiness(form.business),
          },
        },
      });
      onSaved(p);
      toast.success("Данные компании сохранены", { description: "Оценки новостей будут пересчитаны с учётом изменений." });
    } catch (err) {
      if (err instanceof ApiError && err.fields) setErrors(err.fields);
      toast.error(errorMessage(err));
    } finally {
      setBusy(false);
    }
  }
  return (
    <Panel
      title="Сведения о компании"
      description="Если направление бизнеса изменится, обновите их здесь — новый кабинет создавать не нужно"
      className="h-fit xl:sticky xl:top-8"
    >
      <form onSubmit={save} className="space-y-4">
        <Field id="c-name" label="Название" error={errors.display_name} required>
          <Input value={form.display_name} onChange={set("display_name")} />
        </Field>
        <Field id="c-products" label="Продукты и услуги" error={errors.products} required>
          <Input value={form.products} onChange={set("products")} />
        </Field>
        <div className="grid items-start gap-4 sm:grid-cols-2">
          <Field id="c-markets" label="Рынки и клиенты" error={errors.markets} required>
            <Input value={form.markets} onChange={set("markets")} />
          </Field>
          <Field id="c-suppliers" label="Поставщики и сырьё" error={errors.suppliers} required>
            <Input value={form.suppliers} onChange={set("suppliers")} />
          </Field>
        </div>
        <Field id="c-prio" label="Приоритеты компании" error={errors.priorities}>
          <Textarea value={form.priorities} onChange={set("priorities")} placeholder="Например: снизить стоимость долга" />
        </Field>
        <Field id="c-desc" label="Чем занимается компания" error={errors.description}>
          <Textarea value={form.description} onChange={set("description")} maxLength={2000} className="min-h-16" />
        </Field>
        <Field id="c-notes" label="Заметки" error={errors.notes}>
          <Textarea value={form.notes} onChange={set("notes")} className="min-h-16" />
        </Field>
        <BusinessFields value={form.business} onChange={(b) => setForm({ ...form, business: b })} errors={errors} />
        <div className="flex items-center justify-between gap-3">
          <Badge tone="outline">Исходные данные банка не меняются</Badge>
          <Button type="submit" loading={busy}>
            Сохранить
          </Button>
        </div>
      </form>
    </Panel>
  );
}
