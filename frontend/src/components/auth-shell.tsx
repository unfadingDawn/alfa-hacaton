import { DISCLAIMER } from "@/lib/format";
import { Logo } from "./brand";

// Композиция по референсу: стеклянные волны на фоне, крупный заголовок
// и короткое описание слева, форма справа.
export function AuthShell({
  headline,
  lead,
  children,
}: {
  headline: React.ReactNode;
  lead: string;
  children: React.ReactNode;
}) {
  return (
    <div className="relative min-h-dvh overflow-hidden">
      <div aria-hidden className="app-backdrop absolute inset-0" />
      <div className="relative mx-auto flex min-h-dvh max-w-[1240px] flex-col px-4 py-6 sm:px-8 lg:py-10">
        <Logo />
        <div className="grid flex-1 items-center gap-10 py-8 lg:grid-cols-[1.15fr_minmax(380px,440px)] lg:gap-16">
          <div className="max-w-[620px]">
            <h1 className="display text-[40px] text-ink sm:text-[56px] lg:text-[68px]">{headline}</h1>
            <p className="mt-5 max-w-[46ch] text-[17px] leading-relaxed text-ink-soft">{lead}</p>
          </div>
          <div className="glass rounded-[var(--radius-xl)] p-6 sm:p-8">{children}</div>
        </div>
        <p className="text-[12px] text-ink-soft/80">{DISCLAIMER}</p>
      </div>
    </div>
  );
}
