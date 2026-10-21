import * as React from "react";
import { AlertTriangle, RefreshCw } from "lucide-react";
import { cn } from "@/lib/utils";
import { Button } from "./button";
import { errorMessage } from "@/lib/api";

export function Skeleton({ className }: { className?: string }) {
  return <div className={cn("animate-pulse rounded-[10px] bg-frost/90", className)} aria-hidden />;
}

export function Badge({
  tone = "neutral",
  className,
  children,
  title,
}: {
  tone?: "neutral" | "risk" | "chance" | "mixed" | "bank" | "steel" | "outline";
  className?: string;
  children: React.ReactNode;
  title?: string;
}) {
  const tones = {
    neutral: "bg-frost text-ink-soft",
    risk: "bg-risk-bg text-risk",
    chance: "bg-chance-bg text-chance",
    mixed: "bg-mixed-bg text-mixed",
    bank: "bg-bankview-bg text-bankview",
    steel: "bg-steel text-white",
    outline: "border border-line text-ink-soft bg-white/50",
  } as const;
  return (
    <span
      title={title}
      className={cn("inline-flex items-center gap-1 rounded-full px-2.5 py-0.5 text-[12px] font-medium leading-5", tones[tone], className)}
    >
      {children}
    </span>
  );
}

// Сообщение об ошибке загрузки с кнопкой повтора.
export function ErrorState({ error, onRetry, className }: { error: unknown; onRetry?: () => void; className?: string }) {
  return (
    <div role="alert" className={cn("glass flex flex-col items-start gap-3 rounded-[var(--radius-lg)] p-6", className)}>
      <div className="flex items-center gap-2 text-risk">
        <AlertTriangle className="size-5" aria-hidden />
        <p className="font-medium">Не удалось загрузить данные</p>
      </div>
      <p className="text-sm text-ink-soft">{errorMessage(error)}</p>
      {onRetry && (
        <Button variant="outline" size="sm" onClick={onRetry}>
          <RefreshCw /> Повторить
        </Button>
      )}
    </div>
  );
}

export function EmptyState({
  title,
  children,
  action,
  className,
}: {
  title: string;
  children?: React.ReactNode;
  action?: React.ReactNode;
  className?: string;
}) {
  return (
    <div className={cn("rounded-[var(--radius-lg)] border border-dashed border-line bg-white/40 p-8 text-center", className)}>
      <p className="font-medium text-ink">{title}</p>
      {children && <div className="mx-auto mt-1.5 max-w-md text-sm text-ink-soft">{children}</div>}
      {action && <div className="mt-4 flex justify-center">{action}</div>}
    </div>
  );
}

export function Panel({
  title,
  description,
  action,
  children,
  className,
  as: As = "section",
}: {
  title?: React.ReactNode;
  description?: React.ReactNode;
  action?: React.ReactNode;
  children: React.ReactNode;
  className?: string;
  as?: "section" | "div" | "article";
}) {
  return (
    <As className={cn("glass rounded-[var(--radius-lg)] p-5 sm:p-6", className)}>
      {(title || action) && (
        <header className="mb-4 flex flex-wrap items-start justify-between gap-3">
          <div>
            {title && <h2 className="text-[17px] font-semibold tracking-[-0.01em] text-ink">{title}</h2>}
            {description && <p className="mt-0.5 text-[13px] text-silver">{description}</p>}
          </div>
          {action}
        </header>
      )}
      {children}
    </As>
  );
}
