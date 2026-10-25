"use client";

import { useId } from "react";
import { motion, useReducedMotion } from "motion/react";
import { FACETS } from "@/components/brand";

// Анимация во время анализа: знак «Альфа Инсайт» пульсирует, затем импульс
// распрямляется в линию, сворачивается в кольцо и делает оборот, после чего
// возвращается в форму логотипа. Все формы — один путь из одинакового числа
// точек, поэтому motion плавно интерполирует атрибут d.

const SPINE: [number, number][] = [
  [110, 450], [230, 262], [335, 642], [475, 108], [590, 518], [668, 350], [732, 405],
];
const N = 64;
const CX = 421;
const CY = 375;

function resample(points: [number, number][], n: number): [number, number][] {
  const seg: number[] = [];
  let total = 0;
  for (let i = 1; i < points.length; i++) {
    const d = Math.hypot(points[i][0] - points[i - 1][0], points[i][1] - points[i - 1][1]);
    seg.push(d);
    total += d;
  }
  const out: [number, number][] = [];
  for (let k = 0; k < n; k++) {
    let dist = (total * k) / (n - 1);
    let i = 0;
    while (i < seg.length - 1 && dist > seg[i]) {
      dist -= seg[i];
      i++;
    }
    const t = seg[i] === 0 ? 0 : Math.min(1, dist / seg[i]);
    out.push([points[i][0] + (points[i + 1][0] - points[i][0]) * t, points[i][1] + (points[i + 1][1] - points[i][1]) * t]);
  }
  return out;
}

const toPath = (pts: [number, number][]) => "M" + pts.map(([x, y]) => `${x.toFixed(1)} ${y.toFixed(1)}`).join(" L");

const ZIGZAG = toPath(resample(SPINE, N));
const LINE = toPath(Array.from({ length: N }, (_, k) => [130 + (580 * k) / (N - 1), CY] as [number, number]));
const RING = toPath(
  Array.from({ length: N }, (_, k) => {
    // Кольцо начинается слева, как линия, и замыкается на последней точке.
    const a = Math.PI + (2 * Math.PI * k) / (N - 1);
    return [CX + 190 * Math.cos(a), CY + 190 * Math.sin(a)] as [number, number];
  }),
);

const DURATION = 6.4;
const TIMES = [0, 0.16, 0.24, 0.36, 0.48, 0.7, 0.8, 0.9, 0.96, 1];
const loop = { duration: DURATION, times: TIMES, repeat: Infinity, ease: "easeInOut" as const };

export function AnalyzingMark({ size = 168 }: { size?: number }) {
  const reduce = useReducedMotion();
  const id = useId();
  const grad = `${id}-grad`;
  const glow = `${id}-glow`;

  if (reduce) {
    return (
      <motion.svg width={size} height={(size * 550) / 640} viewBox="100 100 640 550" aria-hidden
        animate={{ opacity: [1, 0.55, 1] }} transition={{ duration: 2, repeat: Infinity }}>
        {FACETS.map(([c, d], i) => <path key={i} d={d} fill={c} />)}
      </motion.svg>
    );
  }

  return (
    <motion.svg
      width={size}
      height={(size * 550) / 640}
      viewBox="100 100 640 550"
      aria-hidden
      overflow="visible"
      animate={{ scale: [1, 1.06, 1] }}
      transition={{ duration: 1.6, repeat: Infinity, ease: "easeInOut" }}
    >
      <defs>
        <linearGradient id={grad} x1="0" y1="0" x2="1" y2="1">
          <stop offset="0" stopColor="#ffb7ad" />
          <stop offset="0.45" stopColor="#f0503f" />
          <stop offset="1" stopColor="#c41b12" />
        </linearGradient>
        <filter id={glow} x="-30%" y="-30%" width="160%" height="160%">
          <feGaussianBlur stdDeviation="18" />
        </filter>
      </defs>

      {/* Мягкое свечение, пульсирующее вместе со знаком. */}
      <motion.circle
        cx={CX}
        cy={CY}
        r={230}
        fill="#ef3124"
        filter={`url(#${glow})`}
        animate={{ opacity: [0.08, 0.2, 0.08], r: [200, 240, 200] }}
        transition={{ duration: 1.6, repeat: Infinity, ease: "easeInOut" }}
      />

      {/* Гранёный знак: виден в начале и в конце цикла. */}
      <motion.g animate={{ opacity: [1, 1, 0, 0, 0, 0, 0, 0, 1, 1] }} transition={loop}>
        {FACETS.map(([c, d], i) => (
          <path key={i} d={d} fill={c} />
        ))}
      </motion.g>

      {/* Импульс → линия → кольцо (оборот) → линия → импульс. */}
      <motion.g
        // Вращение вокруг центра собственной рамки: в фазе кольца это центр кольца.
        style={{ originX: 0.5, originY: 0.5 }}
        animate={{ rotate: [0, 0, 0, 0, 0, 360, 360, 360, 360, 360], opacity: [0, 0, 1, 1, 1, 1, 1, 1, 0, 0] }}
        transition={loop}
      >
        <motion.path
          fill="none"
          stroke="#ef3124"
          strokeWidth={44}
          strokeLinecap="round"
          strokeLinejoin="round"
          filter={`url(#${glow})`}
          opacity={0.35}
          animate={{ d: [ZIGZAG, ZIGZAG, ZIGZAG, LINE, RING, RING, LINE, ZIGZAG, ZIGZAG, ZIGZAG] }}
          transition={loop}
        />
        <motion.path
          fill="none"
          stroke={`url(#${grad})`}
          strokeWidth={30}
          strokeLinecap="round"
          strokeLinejoin="round"
          animate={{ d: [ZIGZAG, ZIGZAG, ZIGZAG, LINE, RING, RING, LINE, ZIGZAG, ZIGZAG, ZIGZAG] }}
          transition={loop}
        />
      </motion.g>
    </motion.svg>
  );
}
