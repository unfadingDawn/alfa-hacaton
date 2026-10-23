import Link from "next/link";
import { useId } from "react";
import { cn } from "@/lib/utils";

export const BRAND_NAME = "Альфа Инсайт";

// Знак «Альфа Инсайт»: гранёный красный импульс (векторная перерисовка утверждённой иконки).
// Грани — [цвет, путь] в системе координат 0..800 × 0..688.
const FACETS: [string, string][] = [
  ["#ff9e91", "M110 450 L230 262 L175 452Z"],
  ["#ea4a3b", "M230 262 L272 392 L175 452Z"],
  ["#f46a59", "M230 262 L335 642 L272 392Z"],
  ["#c41b12", "M335 642 L338 478 L272 392Z"],
  ["#ff8a7b", "M335 642 L475 108 L338 478Z"],
  ["#de2a1e", "M475 108 L468 322 L338 478Z"],
  ["#ffb7ad", "M475 108 L590 518 L468 322Z"],
  ["#e8402f", "M590 518 L566 462 L468 322Z"],
  ["#f0503f", "M590 518 L668 350 L566 462Z"],
  ["#ff9d90", "M668 350 L640 408 L566 462Z"],
  ["#fb7b6d", "M668 350 L732 405 L640 408Z"],
  ["#d52619", "M732 405 L700 412 L640 408Z"],
];

// BrandMark рисует знак. shimmer — блик, который переливается по граням
// (CSS-анимация .brand-sheen; отключается при «уменьшении движения» в системе).
export function BrandMark({ size = 28, shimmer = false, className }: { size?: number; shimmer?: boolean; className?: string }) {
  const id = useId();
  const clip = `${id}-clip`;
  const sheen = `${id}-sheen`;
  return (
    <svg
      width={size}
      height={(size * 550) / 640}
      viewBox="100 100 640 550"
      className={className}
      aria-hidden
    >
      <defs>
        <clipPath id={clip}>
          {FACETS.map(([, d], i) => (
            <path key={i} d={d} />
          ))}
        </clipPath>
        <linearGradient id={sheen} x1="0" y1="0" x2="1" y2="0">
          <stop offset="0" stopColor="#fff" stopOpacity="0" />
          <stop offset="0.5" stopColor="#fff" stopOpacity="0.95" />
          <stop offset="1" stopColor="#fff" stopOpacity="0" />
        </linearGradient>
      </defs>
      {FACETS.map(([c, d], i) => (
        <path key={i} d={d} fill={c} />
      ))}
      {shimmer && (
        <g clipPath={`url(#${clip})`}>
          <rect className="brand-sheen" x="-200" y="40" width="260" height="680" fill={`url(#${sheen})`} />
        </g>
      )}
    </svg>
  );
}

export function Logo({ className, href = "/", shimmer = false }: { className?: string; href?: string; shimmer?: boolean }) {
  return (
    <Link href={href} className={cn("inline-flex items-center gap-2.5 text-ink", className)} aria-label={BRAND_NAME}>
      <BrandMark size={30} shimmer={shimmer} />
      <span className="text-[17px] font-semibold tracking-[-0.02em]">{BRAND_NAME}</span>
    </Link>
  );
}
