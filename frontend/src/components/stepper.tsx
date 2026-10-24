import { Check } from "lucide-react";
import { cn } from "@/lib/utils";

export const onboardingSteps = ["Пользователь", "Компания", "Завершение"];

// Индикатор трёх этапов регистрации — настоящая последовательность, поэтому с номерами.
export function Stepper({ current, className }: { current: number; className?: string }) {
  return (
    <ol className={cn("flex items-center gap-2", className)} aria-label="Этапы регистрации">
      {onboardingSteps.map((s, i) => {
        const done = i < current;
        const active = i === current;
        return (
          <li key={s} className="flex flex-1 items-center gap-2" aria-current={active ? "step" : undefined}>
            <span
              className={cn(
                "flex size-6 shrink-0 items-center justify-center rounded-full text-[12px] font-semibold tabular",
                done && "bg-steel text-white",
                active && "bg-white text-steel ring-2 ring-steel",
                !done && !active && "bg-frost text-silver",
              )}
            >
              {done ? <Check className="size-3.5" /> : i + 1}
            </span>
            <span className={cn("text-[13px]", active ? "font-medium text-ink" : "text-silver")}>{s}</span>
            {i < onboardingSteps.length - 1 && <span className="h-px flex-1 bg-line" aria-hidden />}
          </li>
        );
      })}
    </ol>
  );
}
