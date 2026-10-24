"use client";

import { useRouter, useSearchParams } from "next/navigation";
import { PageHeader } from "@/components/app-shell";
import { CompanyTab } from "@/features/company/company";
import { ProfileTab } from "@/features/settings/settings";
import { cn } from "@/lib/utils";

const tabs = [
  { id: "profile", label: "Профиль" },
  { id: "company", label: "Компания" },
] as const;

// Личный кабинет: профиль пользователя и данные компании в одном разделе.
export function Cabinet() {
  const router = useRouter();
  const sp = useSearchParams();
  const tab = sp.get("tab") === "company" ? "company" : "profile";
  return (
    <div>
      <PageHeader title="Личный кабинет" lead="Ваш профиль, интересы и данные компании. От них зависят отбор новостей и персональный анализ." />
      <div role="tablist" aria-label="Разделы кабинета" className="glass mb-6 inline-flex rounded-[16px] p-1">
        {tabs.map((t) => (
          <button
            key={t.id}
            role="tab"
            aria-selected={tab === t.id}
            onClick={() => router.replace(`/cabinet?tab=${t.id}`, { scroll: false })}
            className={cn(
              "rounded-[12px] px-5 py-2 text-[14px] transition-colors",
              tab === t.id ? "bg-white font-medium text-ink shadow-[var(--shadow-soft)]" : "text-ink-soft hover:text-ink",
            )}
          >
            {t.label}
          </button>
        ))}
      </div>
      <div role="tabpanel">{tab === "profile" ? <ProfileTab /> : <CompanyTab />}</div>
    </div>
  );
}
