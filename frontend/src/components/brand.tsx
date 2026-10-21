import Link from "next/link";
import { cn } from "@/lib/utils";

// Знак: призма из двух граней — «линза», через которую новость
// преломляется под конкретную компанию.
export function Logo({ className, href = "/" }: { className?: string; href?: string }) {
  return (
    <Link href={href} className={cn("inline-flex items-center gap-2.5 text-ink", className)} aria-label="AlfaInsight">
      <svg width="28" height="28" viewBox="0 0 28 28" aria-hidden>
        <defs>
          <linearGradient id="lg-a" x1="0" y1="0" x2="1" y2="1">
            <stop offset="0" stopColor="#a9cbea" />
            <stop offset="1" stopColor="#2b67a8" />
          </linearGradient>
        </defs>
        <path d="M14 2 26 24H2Z" fill="url(#lg-a)" opacity=".9" />
        <path d="M14 2 20 24H8Z" fill="#fff" opacity=".55" />
      </svg>
      <span className="text-[17px] font-semibold tracking-[-0.02em]">AlfaInsight</span>
    </Link>
  );
}
