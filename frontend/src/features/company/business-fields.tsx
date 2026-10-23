"use client";

import { useState } from "react";
import { ChevronDown, Plus, Trash2 } from "lucide-react";
import { Button } from "@/components/ui/button";
import { Field, Input, Select } from "@/components/ui/field";
import type { Application, BusinessProfile } from "@/lib/types";
import { cn } from "@/lib/utils";

export const currencyOptions = ["USD", "EUR", "CNY", "KZT", "AED", "TRY", "INR", "BYN", "UZS", "KGS", "AMD", "HKD"];
export const productOptions: { v: string; l: string }[] = [
  { v: "leasing", l: "Лизинг" },
  { v: "factoring", l: "Факторинг" },
  { v: "credit", l: "Кредит" },
  { v: "guarantee", l: "Банковская гарантия" },
  { v: "deposit", l: "Депозит" },
  { v: "rko", l: "РКО" },
  { v: "cards", l: "Бизнес-карты" },
  { v: "other", l: "Другое" },
];

const num = (v: string) => (v.trim() === "" ? undefined : Number(v.replace(/\s/g, "").replace(",", ".")));
const tri = (v: boolean | null | undefined) => (v == null ? "" : v ? "yes" : "no");
const fromTri = (v: string) => (v === "" ? null : v === "yes");

// Сведения с главного экрана кабинета «Альфа-Бизнес». Все поля необязательные:
// при входе из Альфа-Бизнеса они будут заполняться автоматически.
export function BusinessFields({
  value,
  onChange,
  errors = {},
  defaultOpen = false,
}: {
  value: BusinessProfile;
  onChange: (v: BusinessProfile) => void;
  errors?: Record<string, string>;
  defaultOpen?: boolean;
}) {
  const [open, setOpen] = useState(defaultOpen);
  const set = <K extends keyof BusinessProfile>(k: K, v: BusinessProfile[K]) => onChange({ ...value, [k]: v });
  const apps = value.applications ?? [];
  const setApp = (i: number, a: Partial<Application>) => set("applications", apps.map((x, j) => (j === i ? { ...x, ...a } : x)));
  const cur = value.currencies ?? [];
  const err = (k: string) => errors[`business.${k}`];
  const filled = Object.entries(value).filter(([k, v]) => !["source", "updated_at"].includes(k) && v != null && v !== "" && !(Array.isArray(v) && v.length === 0)).length;

  return (
    <section className="rounded-[16px] border border-line bg-white/60">
      <button
        type="button"
        onClick={() => setOpen(!open)}
        aria-expanded={open}
        className="flex w-full items-center justify-between gap-3 px-4 py-3.5 text-left"
      >
        <span>
          <span className="block text-[15px] font-medium">Данные из личного кабинета Альфа-Бизнеса</span>
          <span className="block text-[13px] text-silver">
            {value.source === "alfa_business"
              ? "Получены из интернет-банка"
              : filled
                ? `Заполнено полей: ${filled}. Необязательно — при входе из Альфа-Бизнеса подтянутся автоматически`
                : "Необязательно — при входе из Альфа-Бизнеса подтянутся автоматически"}
          </span>
        </span>
        <ChevronDown className={cn("size-5 shrink-0 text-silver transition-transform", open && "rotate-180")} aria-hidden />
      </button>
      {open && (
        <div className="space-y-5 border-t border-line px-4 pb-5 pt-4">
          <div className="grid gap-4 sm:grid-cols-2">
            <Field id="ab-companies" label="Компаний в кабинете" error={err("companies_in_account")}>
              <Input inputMode="numeric" value={value.companies_in_account ?? ""} onChange={(e) => set("companies_in_account", num(e.target.value))} placeholder="Например, 7" />
            </Field>
            <Field id="ab-rub" label="Счетов в рублях" error={err("accounts_rub")}>
              <Input inputMode="numeric" value={value.accounts_rub ?? ""} onChange={(e) => set("accounts_rub", num(e.target.value))} />
            </Field>
            <Field id="ab-fx" label="Счетов в валюте" error={err("accounts_fx")}>
              <Input inputMode="numeric" value={value.accounts_fx ?? ""} onChange={(e) => set("accounts_fx", num(e.target.value))} />
            </Field>
          </div>

          <fieldset>
            <legend className="text-[13px] font-medium text-ink-soft">Валюты счетов</legend>
            {err("currencies") && <p className="mt-1 text-[13px] text-risk">{err("currencies")}</p>}
            <div className="mt-2 flex flex-wrap gap-1.5">
              {currencyOptions.map((c) => {
                const on = cur.includes(c);
                return (
                  <button
                    key={c}
                    type="button"
                    aria-pressed={on}
                    onClick={() => set("currencies", on ? cur.filter((x) => x !== c) : [...cur, c])}
                    className={cn(
                      "rounded-full border px-3 py-1 text-[13px] tabular transition-colors",
                      on ? "border-steel bg-steel text-white" : "border-line bg-white/70 text-ink-soft hover:border-glacier",
                    )}
                  >
                    {c}
                  </button>
                );
              })}
            </div>
          </fieldset>

          <div className="grid gap-4 sm:grid-cols-2">
            <Field id="ab-balance" label="Остаток на счетах, ₽" error={err("balance_rub")}>
              <Input inputMode="decimal" value={value.balance_rub ?? ""} onChange={(e) => set("balance_rub", num(e.target.value) ?? null)} />
            </Field>
            <Field id="ab-ovd" label="Овердрафт">
              <Select value={tri(value.overdraft_available)} onChange={(e) => set("overdraft_available", fromTri(e.target.value))}>
                <option value="">Не указано</option>
                <option value="yes">Есть</option>
                <option value="no">Нет</option>
              </Select>
            </Field>
            <Field id="ab-ovd-sum" label="Остаток с овердрафтом, ₽" error={err("balance_with_overdraft_rub")}>
              <Input inputMode="decimal" value={value.balance_with_overdraft_rub ?? ""} onChange={(e) => set("balance_with_overdraft_rub", num(e.target.value) ?? null)} disabled={value.overdraft_available === false} />
            </Field>
          </div>

          <div className="grid gap-4 sm:grid-cols-2">
            <Field id="ab-cards" label="Бизнес-карты">
              <Select value={tri(value.business_cards)} onChange={(e) => set("business_cards", fromTri(e.target.value))}>
                <option value="">Не указано</option>
                <option value="yes">Есть</option>
                <option value="no">Нет</option>
              </Select>
            </Field>
            <Field id="ab-gov" label="Госзакупки">
              <Select value={tri(value.gov_procurement)} onChange={(e) => set("gov_procurement", fromTri(e.target.value))}>
                <option value="">Не указано</option>
                <option value="yes">Участвуем</option>
                <option value="no">Не участвуем</option>
              </Select>
            </Field>
            <Field id="ab-risk" label="Индикатор риска" error={err("risk_indicator")}>
              <Select value={value.risk_indicator ?? ""} onChange={(e) => set("risk_indicator", e.target.value as BusinessProfile["risk_indicator"])}>
                <option value="">Не указано</option>
                <option value="low">Низкий риск</option>
                <option value="medium">Средний риск</option>
                <option value="high">Высокий риск</option>
              </Select>
            </Field>
            <Field id="ab-tariff" label="Тариф" error={err("tariff")}>
              <Input value={value.tariff ?? ""} onChange={(e) => set("tariff", e.target.value)} />
            </Field>
          </div>

          <fieldset>
            <legend className="text-[13px] font-medium text-ink-soft">Активные заявки</legend>
            {err("applications") && <p className="mt-1 text-[13px] text-risk">{err("applications")}</p>}
            <div className="mt-2 space-y-2">
              {apps.map((a, i) => (
                <div key={i} className="grid grid-cols-2 items-center gap-2 2xl:grid-cols-[1fr_1fr_1fr_auto]">
                  <Select aria-label="Продукт" value={a.product} onChange={(e) => setApp(i, { product: e.target.value })}>
                    {productOptions.map((p) => (
                      <option key={p.v} value={p.v}>
                        {p.l}
                      </option>
                    ))}
                  </Select>
                  <Input aria-label="Сумма, ₽" inputMode="decimal" placeholder="Сумма, ₽" value={a.amount ?? ""} onChange={(e) => setApp(i, { amount: num(e.target.value) ?? null })} />
                  <Input aria-label="Статус" placeholder="Статус, например «на подпись»" value={a.status ?? ""} onChange={(e) => setApp(i, { status: e.target.value })} />
                  <Button type="button" variant="ghost" size="icon" aria-label="Удалить заявку" onClick={() => set("applications", apps.filter((_, j) => j !== i))}>
                    <Trash2 />
                  </Button>
                </div>
              ))}
              <Button type="button" variant="outline" size="sm" onClick={() => set("applications", [...apps, { product: "leasing" }])}>
                <Plus /> Добавить заявку
              </Button>
            </div>
          </fieldset>
        </div>
      )}
    </section>
  );
}

// Убирает пустые значения перед отправкой.
export function cleanBusiness(b: BusinessProfile): BusinessProfile | undefined {
  const out: Record<string, unknown> = {};
  Object.entries(b).forEach(([k, v]) => {
    if (k === "source" || k === "updated_at") return;
    if (v == null || v === "" || (typeof v === "number" && Number.isNaN(v)) || (Array.isArray(v) && v.length === 0)) return;
    out[k] = v;
  });
  return Object.keys(out).length ? (out as BusinessProfile) : undefined;
}
