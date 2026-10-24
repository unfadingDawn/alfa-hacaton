import type { Impact, Importance } from "./types";

const dateFmt = new Intl.DateTimeFormat("ru-RU", { day: "numeric", month: "long" });
const dateYearFmt = new Intl.DateTimeFormat("ru-RU", { day: "numeric", month: "long", year: "numeric" });
const timeFmt = new Intl.DateTimeFormat("ru-RU", { hour: "2-digit", minute: "2-digit" });

export function formatDate(iso: string) {
  const d = new Date(iso);
  const now = new Date();
  const sameDay = d.toDateString() === now.toDateString();
  if (sameDay) return `сегодня, ${timeFmt.format(d)}`;
  const y = new Date(now);
  y.setDate(now.getDate() - 1);
  if (d.toDateString() === y.toDateString()) return `вчера, ${timeFmt.format(d)}`;
  return d.getFullYear() === now.getFullYear() ? dateFmt.format(d) : dateYearFmt.format(d);
}

export function formatRub(v: number | null | undefined) {
  if (v == null) return "нет данных";
  if (v >= 1e9) return `${(v / 1e9).toLocaleString("ru-RU", { maximumFractionDigits: 1 })} млрд ₽`;
  if (v >= 1e6) return `${(v / 1e6).toLocaleString("ru-RU", { maximumFractionDigits: 1 })} млн ₽`;
  return `${Math.round(v).toLocaleString("ru-RU")} ₽`;
}

export const importanceLabel: Record<Importance, string> = {
  low: "Низкая",
  medium: "Средняя",
  high: "Высокая",
  critical: "Критическая",
};

export const impactLabel: Record<Impact, string> = {
  risk: "Риск",
  opportunity: "Возможность",
  mixed: "Риски и возможности",
  neutral: "Нейтрально",
};

export const horizonLabel = { short: "до 3 месяцев", medium: "3–12 месяцев", long: "больше года" } as const;
export const likelihoodLabel = { low: "низкая вероятность", medium: "средняя вероятность", high: "высокая вероятность" } as const;

export function pluralize(n: number, one: string, few: string, many: string) {
  const m10 = n % 10;
  const m100 = n % 100;
  if (m10 === 1 && m100 !== 11) return one;
  if (m10 >= 2 && m10 <= 4 && (m100 < 12 || m100 > 14)) return few;
  return many;
}

// Единый дисклеймер для всех страниц сервиса.
export const DISCLAIMER =
  "Информация в сервисе несёт исключительно информационный характер и не является инвестиционной рекомендацией.";
