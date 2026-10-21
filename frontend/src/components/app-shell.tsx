"use client";

import Link from "next/link";
import { createContext, useContext, useEffect } from "react";
import { usePathname, useRouter } from "next/navigation";
import { BarChart3, Bookmark, Building2, Home, LogOut, Newspaper, Settings } from "lucide-react";
import { Logo } from "./brand";
import { Skeleton, ErrorState } from "./ui/misc";
import { api } from "@/lib/api";
import type { Me } from "@/lib/types";
import { useApi } from "@/lib/use-api";
import { cn } from "@/lib/utils";

const nav = [
  { href: "/home", label: "Главная", icon: Home },
  { href: "/feed", label: "Лента", icon: Newspaper },
  { href: "/analytics", label: "Аналитика", icon: BarChart3 },
  { href: "/company", label: "Компания", icon: Building2 },
  { href: "/saved", label: "Сохранённые", icon: Bookmark },
  { href: "/settings", label: "Настройки", icon: Settings },
];

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

export function AppShell({ children }: { children: React.ReactNode }) {
  const path = usePathname();
  const router = useRouter();
  const me = useApi<Me>("/me");

  useEffect(() => {
    if (me.data && !me.data.onboarding_completed) router.replace("/onboarding");
  }, [me.data, router]);

  const company = me.data?.companies.find((c) => c.is_primary) ?? me.data?.companies[0];

  return (
    <div className="min-h-dvh">
      <div aria-hidden className="pointer-events-none fixed inset-x-0 top-0 h-[260px] bg-cover bg-center opacity-35" style={{ backgroundImage: "url(/waves.jpg)" }} />
      <div aria-hidden className="pointer-events-none fixed inset-x-0 top-0 h-[260px] bg-gradient-to-b from-ice/20 to-ice" />

      <aside className="fixed inset-y-0 left-0 z-30 hidden w-[248px] flex-col px-4 py-6 lg:flex">
        <div className="glass flex h-full flex-col rounded-[var(--radius-xl)] p-4">
          <Logo href="/home" className="px-2" />
          <nav className="mt-8 flex flex-col gap-0.5" aria-label="Основная навигация">
            {nav.map((n) => {
              const active = path === n.href || path.startsWith(n.href + "/");
              return (
                <Link
                  key={n.href}
                  href={n.href}
                  aria-current={active ? "page" : undefined}
                  className={cn(
                    "flex items-center gap-3 rounded-[12px] px-3 py-2.5 text-[14px] transition-colors",
                    active ? "bg-white font-medium text-ink shadow-[var(--shadow-soft)]" : "text-ink-soft hover:bg-white/60 hover:text-ink",
                  )}
                >
                  <n.icon className={cn("size-[18px]", active ? "text-steel" : "text-silver")} aria-hidden />
                  {n.label}
                </Link>
              );
            })}
          </nav>
          <div className="mt-auto space-y-3 px-2">
            {me.data && (
              <div className="text-[13px]">
                <p className="truncate font-medium text-ink">{me.data.full_name}</p>
                <p className="truncate text-silver">{me.data.role?.title ?? "Роль не выбрана"}</p>
                {company && <p className="mt-1 truncate text-silver">{company.display_name || company.okved_name}</p>}
              </div>
            )}
            <button onClick={logout} className="flex items-center gap-2 text-[13px] text-ink-soft hover:text-ink">
              <LogOut className="size-4" aria-hidden /> Выйти
            </button>
          </div>
        </div>
      </aside>

      <header className="glass sticky top-0 z-30 flex items-center justify-between px-4 py-3 lg:hidden">
        <Logo href="/home" />
        <Link href="/settings" className="text-[13px] text-ink-soft">
          {me.data?.full_name}
        </Link>
      </header>

      <main className="relative px-4 pb-28 pt-6 sm:px-6 lg:ml-[248px] lg:px-10 lg:pb-12 lg:pt-10">
        <div className="mx-auto max-w-[1160px]">
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
        </div>
      </main>

      <nav className="glass fixed inset-x-3 bottom-3 z-30 grid grid-cols-6 rounded-[18px] px-1 py-1.5 lg:hidden" aria-label="Навигация">
        {nav.map((n) => {
          const active = path === n.href || path.startsWith(n.href + "/");
          return (
            <Link
              key={n.href}
              href={n.href}
              aria-current={active ? "page" : undefined}
              className={cn("flex flex-col items-center gap-0.5 rounded-[12px] py-1.5 text-[10px]", active ? "bg-white text-steel" : "text-ink-soft")}
            >
              <n.icon className="size-5" aria-hidden />
              {n.label}
            </Link>
          );
        })}
      </nav>
    </div>
  );
}

export function PageHeader({ title, lead, action }: { title: string; lead?: React.ReactNode; action?: React.ReactNode }) {
  return (
    <div className="mb-8 flex flex-wrap items-end justify-between gap-4">
      <div>
        <h1 className="display text-[32px] sm:text-[40px]">{title}</h1>
        {lead && <p className="mt-2 max-w-[64ch] text-[15px] text-ink-soft">{lead}</p>}
      </div>
      {action}
    </div>
  );
}
