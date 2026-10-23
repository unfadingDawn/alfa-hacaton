"use client";

import Link from "next/link";
import { createContext, useContext, useEffect } from "react";
import { usePathname, useRouter } from "next/navigation";
import { BarChart3, Bookmark, Home, LifeBuoy, LogOut, Newspaper, UserRound } from "lucide-react";
import { Logo } from "./brand";
import { DISCLAIMER } from "@/lib/format";
import { Avatar } from "./avatar";
import { Skeleton, ErrorState } from "./ui/misc";
import { api } from "@/lib/api";
import type { Me } from "@/lib/types";
import { useApi } from "@/lib/use-api";
import { cn } from "@/lib/utils";

const nav = [
  { href: "/home", label: "Главная", icon: Home },
  { href: "/feed", label: "Лента", icon: Newspaper },
  { href: "/analytics", label: "Аналитика", icon: BarChart3 },
  { href: "/saved", label: "Сохранённые", icon: Bookmark },
  { href: "/support", label: "Поддержка", icon: LifeBuoy },
];
const cabinet = { href: "/cabinet", label: "Личный кабинет", icon: UserRound };

type MeCtx = { me: Me; reload: () => void };
const Ctx = createContext<MeCtx | null>(null);

export function useMe() {
  const v = useContext(Ctx);
  if (!v) throw new Error("useMe вне AppShell");
  return v;
}

export async function logout() {
  try {
    await api("/auth/logout", { method: "POST" });
  } finally {
    window.location.href = "/login";
  }
}

function NavLink({ item, active }: { item: (typeof nav)[number]; active: boolean }) {
  return (
    <Link
      href={item.href}
      aria-current={active ? "page" : undefined}
      className={cn(
        "flex items-center gap-3 rounded-[14px] border border-transparent px-3 py-2.5 text-[14px] transition-[background,box-shadow,color,border-color] duration-200",
        active ? "glass-pill font-medium text-ink" : "text-ink-soft hover:glass-pill-soft hover:text-ink",
      )}
    >
      <item.icon className={cn("size-[18px]", active ? "text-steel" : "text-silver")} aria-hidden />
      {item.label}
    </Link>
  );
}

export function AppShell({ children }: { children: React.ReactNode }) {
  const path = usePathname();
  const router = useRouter();
  const me = useApi<Me>("/me");

  useEffect(() => {
    if (me.data && !me.data.onboarding_completed) router.replace("/onboarding");
  }, [me.data, router]);

  const company = me.data?.companies.find((c) => c.is_primary) ?? me.data?.companies[0];
  const isActive = (href: string) => path === href || path.startsWith(href + "/");

  return (
    <div className="app-backdrop min-h-dvh">
      <aside className="fixed inset-y-0 left-0 z-30 hidden w-[256px] flex-col px-4 py-5 lg:flex">
        <div className="glass flex h-full flex-col rounded-[var(--radius-xl)] p-4">
          <Logo href="/home" className="px-2" />
          <nav className="mt-8 flex flex-col gap-0.5" aria-label="Основная навигация">
            {nav.map((n) => (
              <NavLink key={n.href} item={n} active={isActive(n.href)} />
            ))}
          </nav>
          <div className="mt-auto space-y-1">
            {me.data && (
              <Link
                href={cabinet.href}
                aria-current={isActive(cabinet.href) ? "page" : undefined}
                className={cn(
                  "flex items-center gap-3 rounded-[14px] border border-transparent p-2.5 transition-[background,box-shadow,border-color] duration-200",
                  isActive(cabinet.href) ? "glass-pill" : "hover:glass-pill-soft",
                )}
              >
                <Avatar src={me.data.avatar} name={me.data.full_name} size={36} />
                <span className="min-w-0">
                  <span className="block text-[13px] font-medium text-ink">Личный кабинет</span>
                  <span className="block truncate text-[12px] text-silver">
                    {company ? company.display_name || company.okved_name : me.data.full_name}
                  </span>
                </span>
              </Link>
            )}
            <button onClick={logout} className="flex w-full items-center gap-3 rounded-[14px] border border-transparent px-3 py-2.5 text-[14px] text-ink-soft transition-[background,box-shadow,color] duration-200 hover:glass-pill-soft hover:text-ink">
              <LogOut className="size-[18px] text-silver" aria-hidden /> Выйти
            </button>
          </div>
        </div>
      </aside>

      <header className="glass sticky top-0 z-30 flex items-center justify-between px-4 py-3 lg:hidden">
        <Logo href="/home" />
        <Link href="/cabinet" aria-label="Личный кабинет">
          {me.data && <Avatar src={me.data.avatar} name={me.data.full_name} size={32} />}
        </Link>
      </header>

      <main className="relative px-4 pb-28 pt-6 sm:px-6 lg:ml-[256px] lg:px-10 lg:pb-12 lg:pt-8">
        <div className="mx-auto max-w-[1180px]">
          {me.error ? (
            <ErrorState error={me.error} onRetry={me.reload} />
          ) : !me.data || !me.data.onboarding_completed ? (
            <div className="space-y-4">
              <Skeleton className="h-10 w-1/3" />
              <Skeleton className="h-40" />
              <Skeleton className="h-64" />
            </div>
          ) : (
            <Ctx.Provider value={{ me: me.data, reload: me.reload }}>{children}</Ctx.Provider>
          )}
          <p className="mt-12 border-t border-line/70 pt-4 text-[12px] leading-relaxed text-silver">{DISCLAIMER}</p>
        </div>
      </main>

      <nav className="glass-strong fixed inset-x-3 bottom-3 z-30 grid grid-cols-6 rounded-[20px] px-1 py-1.5 lg:hidden" aria-label="Навигация">
        {[...nav, cabinet].map((n) => {
          const active = isActive(n.href);
          return (
            <Link
              key={n.href}
              href={n.href}
              aria-current={active ? "page" : undefined}
              className={cn("flex flex-col items-center gap-0.5 rounded-[14px] border border-transparent py-1.5 text-[10px] leading-tight", active ? "glass-pill text-steel" : "text-ink-soft")}
            >
              <n.icon className="size-5" aria-hidden />
              <span className="w-full truncate text-center">{SHORT[n.label] ?? n.label}</span>
            </Link>
          );
        })}
      </nav>
    </div>
  );
}

// Короткие подписи для нижней панели на телефоне (шесть пунктов в ~60 px каждый).
const SHORT: Record<string, string> = { "Сохранённые": "Закладки", "Поддержка": "Помощь", "Личный кабинет": "Кабинет" };

export function PageHeader({ title, lead, action }: { title: string; lead?: React.ReactNode; action?: React.ReactNode }) {
  return (
    <div className="mb-7 flex flex-wrap items-end justify-between gap-4">
      <div>
        <h1 className="display text-[30px] sm:text-[36px]">{title}</h1>
        {lead && <p className="mt-2 max-w-[66ch] text-[15px] text-ink-soft">{lead}</p>}
      </div>
      {action}
    </div>
  );
}
